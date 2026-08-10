package kernel_test

import (
	"context"
	"errors"
	"fmt"
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
//
// Every attempt is also announced on `attempts`, which is how the tests wait for
// the worker to reach the gate instead of sleeping until it probably has.
type countingGate struct {
	limit int

	mu       sync.Mutex
	held     int
	peak     int
	refusals int
	asked    []kernel.SlotRef
	err      error

	// attempts receives one value per Acquire. Buffered generously and dropped
	// when full, so the gate never blocks the worker it is measuring.
	attempts chan struct{}
}

func newCountingGate(limit int) *countingGate {
	return &countingGate{limit: limit, attempts: make(chan struct{}, 256)}
}

func (g *countingGate) Acquire(_ context.Context, ref kernel.SlotRef) (kernel.SlotHold, error) {
	g.mu.Lock()
	g.asked = append(g.asked, ref)
	err := g.err
	full := g.held >= g.limit
	if err == nil && !full {
		g.held++
		if g.held > g.peak {
			g.peak = g.held
		}
	}
	if err != nil || full {
		g.refusals++
	}
	g.mu.Unlock()

	select {
	case g.attempts <- struct{}{}:
	default:
	}

	switch {
	case err != nil:
		return nil, err
	case full:
		// An untyped nil: a typed nil in a non-nil interface would read as
		// "admitted" to the caller.
		return nil, nil
	}
	return &countingHold{gate: g}, nil
}

// waitForAttempts blocks until the gate has been asked n times.
func (g *countingGate) waitForAttempts(t *testing.T, n int) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for range n {
		select {
		case <-g.attempts:
		case <-deadline:
			asked, _, _ := g.stats()
			gt.Number(t, len(asked)).GreaterOrEqual(n).Required()
			return
		}
	}
}

// setLimit changes the capacity mid-test, so a delayed run can be observed
// completing once room appears.
func (g *countingGate) setLimit(n int) {
	g.mu.Lock()
	g.limit = n
	g.mu.Unlock()
}

func (g *countingGate) stats() (asked []kernel.SlotRef, refusals, held int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]kernel.SlotRef, len(g.asked))
	copy(out, g.asked)
	return out, g.refusals, g.held
}

// maxHeld is the highest number of slots held at the same moment. It is the
// figure the whole gate exists to bound.
func (g *countingGate) maxHeld() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.peak
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
					// Long enough that two admitted runs would demonstrably overlap,
					// so a gate that failed to bound them is caught rather than
					// merely unlikely to be caught.
					time.Sleep(20 * time.Millisecond)
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

// serve starts the worker and returns the function that stops it and waits for it
// to let go. Registered with t.Cleanup as well, so a failing assertion cannot
// leave the goroutine running into the next test.
func (rt *slotRuntime) serve(t *testing.T) func() {
	t.Helper()
	return rt.serveWith(t)
}

// serveWith is serve with extra worker options, for a test that needs the worker
// to be capable of running several Processes at once.
func (rt *slotRuntime) serveWith(t *testing.T, opts ...agentkit.ServeOption) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	all := append([]agentkit.ServeOption{agentkit.WithPollInterval(5 * time.Millisecond)}, opts...)
	go func() { served <- rt.kernel.Serve(ctx, all...) }()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			err := <-served
			// Serve returns the context error it stopped on; anything else is a
			// worker failure the test should not hide.
			if err != nil && !errors.Is(err, context.Canceled) {
				gt.NoError(t, err)
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// awaitTerminal waits for pid to reach want.
func (rt *slotRuntime) awaitTerminal(t *testing.T, pid agentkit.ProcessID, want agentkit.ProcessStatus) *agentkit.Process {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		proc, err := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			gt.Value(t, proc.Status).Equal(want)
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return proc
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// runUntilTerminal starts the worker, waits for pid to finish, and stops it.
func (rt *slotRuntime) runUntilTerminal(t *testing.T, pid agentkit.ProcessID) *agentkit.Process {
	t.Helper()
	stop := rt.serve(t)
	proc := rt.awaitTerminal(t, pid, agentkit.ProcessSucceeded)
	stop()
	return proc
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
	gate := newCountingGate(1)
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
	gate := newCountingGate(0) // would refuse everything it was asked about
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
	gate := newCountingGate(0)
	rt := newSlotRuntime(t, gate)

	pid := rt.spawn(t, ctx, gatedScope())
	stop := rt.serve(t)

	// Two refusals is the evidence the run is being RETRIED rather than dropped.
	// Waiting on the gate's own signal rather than sleeping keeps the test's
	// outcome independent of machine load.
	gate.waitForAttempts(t, 2)

	proc, err := rt.kernel.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	gt.Bool(t, proc.Status.Terminal()).False()
	// Refusing must not spend the retry budget — no Step ran.
	gt.Number(t, proc.StepAttempts).Equal(0)
	_, refusals, _ := gate.stats()
	gt.Number(t, refusals).GreaterOrEqual(2)

	// Free capacity; the same run now completes without being re-spawned.
	gate.setLimit(1)
	rt.awaitTerminal(t, pid, agentkit.ProcessSucceeded)
	stop()
}

// TestSlotGateBoundsConcurrentExecution is the gate's whole reason for existing:
// with capacity for one, two runs must not execute at the same time.
//
// The tests above check one run being admitted or refused, which a gate that
// simply said yes to everything would also pass. This one measures the peak
// number of slots held simultaneously, so a broken bound is a failure rather than
// a coincidence — and every run still finishes, because a refusal delays work
// instead of dropping it.
func TestSlotGateBoundsConcurrentExecution(t *testing.T) {
	ctx := context.Background()
	gate := newCountingGate(1)
	rt := newSlotRuntime(t, gate)

	const runs = 4
	pids := make([]agentkit.ProcessID, 0, runs)
	for i := range runs {
		sc := gatedScope()
		// Distinct jobs, so nothing but the gate can serialise them.
		sc.JobID = fmt.Sprintf("job-%d", i)
		pids = append(pids, rt.spawn(t, ctx, sc))
	}

	// Several poll loops and plenty of concurrency, so the worker would happily
	// run all four at once if the gate did not stop it.
	stop := rt.serveWith(t, agentkit.WithMaxConcurrent(runs), agentkit.WithPollConcurrency(2))
	for _, pid := range pids {
		rt.awaitTerminal(t, pid, agentkit.ProcessSucceeded)
	}
	stop()

	gt.Number(t, gate.maxHeld()).Equal(1)
	// And nothing was leaked: every hold was released.
	_, _, held := gate.stats()
	gt.Number(t, held).Equal(0)
}

// A gate that cannot report its own state must fail CLOSED. Proceeding would
// admit an unbounded number of runs — the exact blowout the gate exists to
// prevent — so the run waits instead.
func TestSlotGateFailsClosedWhenItCannotBeRead(t *testing.T) {
	ctx := context.Background()
	gate := newCountingGate(10)
	gate.err = goerr.New("slot backend unavailable")
	rt := newSlotRuntime(t, gate)

	pid := rt.spawn(t, ctx, gatedScope())
	stop := rt.serve(t)
	gate.waitForAttempts(t, 2)
	stop()

	proc, err := rt.kernel.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	gt.Bool(t, proc.Status.Terminal()).False()
	gt.Number(t, proc.StepAttempts).Equal(0)
}
