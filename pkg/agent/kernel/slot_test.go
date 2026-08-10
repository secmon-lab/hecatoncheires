package kernel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// countingGate hands out at most `limit` slots at a time and records what it was
// asked for, so a test can assert who was gated rather than only how often.
type countingGate struct {
	limit int

	mu       sync.Mutex
	held     int
	refusals int
	asked    []kernel.SlotRef
	err      error
}

func (g *countingGate) Acquire(_ context.Context, ref kernel.SlotRef) (kernel.SlotHold, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asked = append(g.asked, ref)
	if g.err != nil {
		return nil, g.err
	}
	if g.held >= g.limit {
		g.refusals++
		return nil, nil
	}
	g.held++
	return &countingHold{gate: g}, nil
}

func (g *countingGate) stats() (asked []kernel.SlotRef, refusals, held int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]kernel.SlotRef, len(g.asked))
	copy(out, g.asked)
	return out, g.refusals, g.held
}

type countingHold struct {
	gate *countingGate
	once sync.Once
}

func (h *countingHold) Release(context.Context) {
	h.once.Do(func() {
		h.gate.mu.Lock()
		h.gate.held--
		h.gate.mu.Unlock()
	})
}

// slotRuntime is a Kernel wired through kernel.Build with a gate, so the tests
// exercise the real middleware ordering rather than the guard in isolation.
type slotRuntime struct {
	kernel *agentkit.Kernel
	agent  agentkit.Agent[react.Input]
}

func newSlotRuntime(t *testing.T, gate kernel.SlotGate) *slotRuntime {
	t.Helper()

	// The case has to exist: the tool factory loads it on every claim, and a
	// missing one fails the claim before the gate's behaviour could be observed.
	repo := memory.New()
	seedCase(t, context.Background(), repo, &model.Case{Title: "gated run target"})

	cfg := budget.Config{MaxSteps: 16, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	reg := agentkit.NewRegistry()
	handle, err := react.Register(reg, kernel.AgentCaseChannel, 1, cfg.Limiter(),
		agentkit.WithHistoryStore[react.Output](agentarchive.NewMemoryHistoryStore()))
	gt.NoError(t, err).Required()

	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{Texts: []string{"done"}}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}

	k, err := kernel.Build(kernel.Deps{
		Repo:    agentprocmemory.New(),
		History: agentarchive.NewMemoryHistoryStore(),
		LLM:     llm,
		Trace:   agentarchive.NewMemoryTraceRepository(),
		Budgets: kernel.Budgets{Root: cfg, Task: cfg},
		Agents:  reg,
		Slots:   gate,
		Tools:   kernel.ToolDeps{Repo: repo, Registry: testRegistry(channelWorkspace())},
	})
	gt.NoError(t, err).Required()
	return &slotRuntime{kernel: k, agent: handle}
}

func (rt *slotRuntime) spawn(t *testing.T, ctx context.Context, sc kernel.Scope) agentkit.ProcessID {
	t.Helper()
	pid, err := rt.agent.Spawn(ctx, rt.kernel,
		react.Input{SystemPrompt: "be helpful", Prompt: "go"},
		agentkit.WithMetadata(sc.Metadata()))
	gt.NoError(t, err).Required()
	return pid
}

// runUntilTerminal drives the worker until pid finishes, or fails the test.
func (rt *slotRuntime) runUntilTerminal(t *testing.T, pid agentkit.ProcessID) *agentkit.Process {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(ctx, agentkit.WithPollInterval(5*time.Millisecond)) }()
	for {
		proc, err := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			cancel()
			<-served
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// slotCaseID is the id the seeded case gets from the per-workspace counter.
const slotCaseID = 1

func gatedScope() kernel.Scope {
	return kernel.Scope{
		WorkspaceID: "ws-1", CaseID: slotCaseID, JobID: "job-nightly",
		ActorUserID: "U1", ToolSets: []string{kernel.ToolSetsAll},
		SlotGated: true,
	}
}

// A gated run must ask the gate and then run. The reference handed over names
// the run, which is what lets an operator see which job holds capacity.
func TestSlotGateAdmitsAGatedRun(t *testing.T) {
	ctx := context.Background()
	gate := &countingGate{limit: 1}
	rt := newSlotRuntime(t, gate)

	pid := rt.spawn(t, ctx, gatedScope())
	proc := rt.runUntilTerminal(t, pid)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	asked, refusals, held := gate.stats()
	gt.Number(t, len(asked)).GreaterOrEqual(1)
	gt.Value(t, asked[0]).Equal(kernel.SlotRef{WorkspaceID: "ws-1", CaseID: slotCaseID, JobID: "job-nightly"})
	gt.Number(t, refusals).Equal(0)
	// Every hold taken was released, so the gate is not leaking capacity.
	gt.Number(t, held).Equal(0)
}

// A run that did not opt in must never be asked. Making an interactive turn
// queue behind a batch would make a person wait for a scheduled job.
func TestSlotGateIgnoresAnUngatedRun(t *testing.T) {
	ctx := context.Background()
	gate := &countingGate{limit: 0} // would refuse everything it was asked about
	rt := newSlotRuntime(t, gate)

	sc := gatedScope()
	sc.SlotGated = false
	pid := rt.spawn(t, ctx, sc)
	proc := rt.runUntilTerminal(t, pid)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	asked, _, _ := gate.stats()
	gt.Array(t, asked).Length(0)
}

// A full gate must DELAY the run, not fail it: the claim is refused, the Process
// goes back to pending, and it runs once capacity frees. A gate that failed runs
// would turn a busy deployment into a stream of failed jobs.
func TestSlotGateDelaysARunItCannotAdmit(t *testing.T) {
	ctx := context.Background()
	gate := &countingGate{limit: 0}
	rt := newSlotRuntime(t, gate)

	pid := rt.spawn(t, ctx, gatedScope())

	serveCtx, cancel := context.WithCancel(ctx)
	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(serveCtx, agentkit.WithPollInterval(5*time.Millisecond)) }()

	// Wait until the gate has refused at least twice, which shows the run is
	// being retried rather than dropped.
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, refusals, _ := gate.stats()
		if refusals >= 2 {
			break
		}
		gt.Bool(t, time.Now().Before(deadline)).True().Required()
		time.Sleep(5 * time.Millisecond)
	}

	proc, err := rt.kernel.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	gt.Bool(t, proc.Status.Terminal()).False()
	// Refusing must not spend the retry budget — no Step ran.
	gt.Number(t, proc.StepAttempts).Equal(0)

	// Free capacity; the same run now completes without being re-spawned.
	gate.mu.Lock()
	gate.limit = 1
	gate.mu.Unlock()

	for {
		proc, err := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
			break
		}
		gt.Bool(t, time.Now().Before(deadline)).True().Required()
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-served
}

// A gate that cannot report its own state must fail CLOSED. Proceeding would
// admit an unbounded number of runs — the exact blowout the gate exists to
// prevent — so the run waits instead.
func TestSlotGateFailsClosedWhenItCannotBeRead(t *testing.T) {
	ctx := context.Background()
	gate := &countingGate{limit: 10, err: goerr.New("slot backend unavailable")}
	rt := newSlotRuntime(t, gate)

	pid := rt.spawn(t, ctx, gatedScope())

	serveCtx, cancel := context.WithCancel(ctx)
	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(serveCtx, agentkit.WithPollInterval(5*time.Millisecond)) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		asked, _, _ := gate.stats()
		if len(asked) >= 2 {
			break
		}
		gt.Bool(t, time.Now().Before(deadline)).True().Required()
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-served

	proc, err := rt.kernel.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	gt.Bool(t, proc.Status.Terminal()).False()
	gt.Number(t, proc.StepAttempts).Equal(0)
}
