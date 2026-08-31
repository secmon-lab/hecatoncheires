package wsagent_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// testSpend is what these runs are judged against in money. A limiter with no
// resolver stops every run, so one is required even though these tests are about
// what the workspace agent replies rather than what it costs; the allowance is
// far above anything they spend.
func testSpend() budget.LimitResolver {
	return func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{
			Budget: pricing.FromUSD(1000),
			Rate:   pricing.Rate{Input: 1, Output: 1},
		}
	}
}

const (
	durableChannelID = "C-WS"
	durableThreadTS  = "1700000000.000100"
	durableSessionID = "s-ws-1"
	durableActorID   = "U-HUMAN"
)

// hostCall records one Slack-facing call a finished turn made.
type hostCall struct {
	Kind      string // "reply" or "failure"
	ChannelID string
	ThreadTS  string
	Text      string
}

// recordingHost captures every Host call so a test asserts what the user saw,
// not merely that the run ended without an error.
type recordingHost struct {
	mu    sync.Mutex
	calls []hostCall
}

func (h *recordingHost) Reply(_ context.Context, channelID, threadTS, text string) error {
	h.record(hostCall{Kind: "reply", ChannelID: channelID, ThreadTS: threadTS, Text: text})
	return nil
}

func (h *recordingHost) ReportFailure(_ context.Context, channelID, threadTS, reason string) error {
	h.record(hostCall{Kind: "failure", ChannelID: channelID, ThreadTS: threadTS, Text: reason})
	return nil
}

func (h *recordingHost) record(c hostCall) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, c)
}

func (h *recordingHost) Calls() []hostCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]hostCall, len(h.calls))
	copy(out, h.calls)
	return out
}

// recordingProgress captures each milestone render and the message id it was
// asked to update.
type recordingProgress struct {
	mu      sync.Mutex
	renders [][]string
	seenTS  []string
}

func (p *recordingProgress) Render(_ context.Context, target planexec.ProgressTarget,
	messageTS string, lines []string,
) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]string, len(lines))
	copy(cp, lines)
	p.renders = append(p.renders, cp)
	p.seenTS = append(p.seenTS, messageTS)
	return "1700000000.000900", nil
}

func (p *recordingProgress) targets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.seenTS))
	copy(out, p.seenTS)
	return out
}

// lastRender returns the lines of the most recent render, which is what the user
// is left looking at once the turn ends.
func (p *recordingProgress) lastRender() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.renders) == 0 {
		return nil
	}
	last := p.renders[len(p.renders)-1]
	out := make([]string, len(last))
	copy(out, last)
	return out
}

// durableLLM answers with replies[i] on the i-th Generate. An extra call fails
// rather than silently repeating the last answer, so a strategy that looped is
// caught instead of hanging.
func durableLLM(replies ...string) gollem.LLMClient {
	client, _ := recordingDurableLLM(replies...)
	return client
}

// recordingDurableLLM is durableLLM plus the system prompt each call was made
// under, in call order. The prompt is where the host's per-run decisions actually
// land — the language directive above all — so it is the observable a test asserting
// on them has to read.
func recordingDurableLLM(replies ...string) (gollem.LLMClient, func() []string) {
	var n atomic.Int32
	var mu sync.Mutex
	var prompts []string
	client := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			// The system prompt is a session-level setting, so it is read here and
			// attributed to the calls this session makes.
			cfg := gollem.NewSessionConfig(opts...)
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					mu.Lock()
					prompts = append(prompts, cfg.SystemPrompt())
					mu.Unlock()
					i := int(n.Add(1)) - 1
					if i >= len(replies) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					return &gollem.Response{Texts: []string{replies[i]}, InputToken: 5, OutputToken: 3}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
	return client, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(prompts))
		copy(out, prompts)
		return out
	}
}

// failingLLM refuses every Generate, which is how a run reaches ProcessFailed.
func failingLLM() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return nil, goerr.New("the model is unreachable")
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

type durableHarness struct {
	agent    *wsagent.Durable
	host     *recordingHost
	progress *recordingProgress
	kernel   *agentkit.Kernel
	locator  agentkernel.Locator
}

// newDurableHarness wires the durable workspace agent onto a real Kernel with an
// in-process Process store. The tool factory returns nothing: the tools an agent
// gets are pkg/agent/kernel's contract, and this test is about the host's own —
// spawn, turn lock, redelivery, completion.
func newDurableHarness(t *testing.T, llm gollem.LLMClient) *durableHarness {
	t.Helper()

	procRepo := agentprocmemory.New()
	locator, err := agentkernel.NewLocator(procRepo)
	gt.NoError(t, err).Required()

	host := &recordingHost{}
	progress := &recordingProgress{}
	wa, err := wsagent.NewDurable(host, locator)
	gt.NoError(t, err).Required()

	store := agentarchive.NewMemoryHistoryStore()
	cfg := budget.Config{MaxSteps: 64, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	reg := agentkit.NewRegistry()
	taskAgent, err := react.Register(reg, agentkernel.AgentTask, 1, cfg.Limiter(testSpend()),
		agentkit.WithHistoryStore[react.Output](store))
	gt.NoError(t, err).Required()
	gt.NoError(t, wa.Register(reg, taskAgent, progress, cfg.Limiter(testSpend()), store)).Required()

	k, err := agentkit.New(procRepo, llm, reg,
		agentkit.WithToolFactory(func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return nil, nil
		}))
	gt.NoError(t, err).Required()
	wa.Bind(k, nil)

	return &durableHarness{agent: wa, host: host, progress: progress, kernel: k, locator: locator}
}

func durableRequest(triggerTS string) wsagent.TurnRequest {
	return wsagent.TurnRequest{
		Session: &model.Session{
			ID:          durableSessionID,
			ChannelID:   durableChannelID,
			ThreadTS:    durableThreadTS,
			WorkspaceID: "ws-1",
			Kind:        model.SessionKindWorkspaceAgent,
		},
		Workspace:   &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-1", Name: "WS"}},
		ActorID:     durableActorID,
		MentionText: "<@bot> which cases are still open?",
		TriggerTS:   triggerTS,
	}
}

// awaitTerminal drives the worker until the newest run reaches a terminal state.
func (h *durableHarness) awaitTerminal(t *testing.T, pid agentkit.ProcessID) *agentkit.Process {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() {
		served <- agentkernel.Serve(ctx, h.kernel, agentkit.WithPollInterval(5*time.Millisecond))
	}()

	for {
		proc, err := h.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			cancel()
			<-served
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return proc
		case <-time.After(3 * time.Millisecond):
		}
	}
}

// spawned resolves the run a trigger key started.
func (h *durableHarness) spawned(t *testing.T, key string) agentkit.ProcessID {
	t.Helper()
	pid, err := h.locator.ByTrigger(context.Background(), key)
	gt.NoError(t, err).Required()
	gt.Value(t, pid).NotEqual(agentkit.ProcessID(""))
	return pid
}

// A turn whose planner finalizes must post its answer to the thread the mention
// came from — from the completion handler, since StartTurn returned long before
// the model answered — and draw its milestones into one message that is updated
// rather than reposted.
func TestDurableStartTurnPostsTheAnswer(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		// plan: one task
		`{"tasks":[{"id":"t1","title":"List open cases","description":"list them","acceptance_criteria":"the open cases are listed","tools":["case_multi"]}]}`,
		// the child task's answer
		`case 3 and case 7 are open`,
		// replan: finalize
		`{"finalize":{"reason":"the cases are known"}}`,
		// final: prose
		`Cases 3 and 7 are still open.`,
	))

	res, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000200"))
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusStarted)

	key := agentkernel.TriggerKey(durableChannelID, durableThreadTS, "1700000000.000200")
	proc := h.awaitTerminal(t, h.spawned(t, key))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("reply")
	gt.Value(t, calls[0].ChannelID).Equal(durableChannelID)
	gt.Value(t, calls[0].ThreadTS).Equal(durableThreadTS)
	gt.String(t, calls[0].Text).Contains("Cases 3 and 7 are still open.")

	// The first render posts a new message (empty id); every later one updates the
	// id the first returned, so a run drawing from another instance keeps writing
	// into the same Slack message instead of starting a second one.
	targets := h.progress.targets()
	gt.Number(t, len(targets)).GreaterOrEqual(2).Required()
	gt.Value(t, targets[0]).Equal("")
	for _, ts := range targets[1:] {
		gt.Value(t, ts).Equal("1700000000.000900")
	}

	// The lines accumulate across the turn's transitions, so what the user is left
	// looking at is the whole trail rather than only the last step.
	trail := strings.Join(h.progress.lastRender(), "\n")
	gt.String(t, trail).Contains("Planning")
	gt.String(t, trail).Contains("Investigating")
	gt.String(t, trail).Contains("Re-planning")
	gt.String(t, trail).Contains("Writing the answer")
}

// Every call in the run must be told which language to answer in. The host is the
// only thing that knows — planexec renders the directive from Input.LanguageLabel
// and omits it entirely when that is empty — and a run with no directive answers a
// Japanese thread in English, since every prompt around it is written in English.
// There is no other symptom: the run succeeds and nothing logs.
func TestDurableStartTurnTellsTheRunWhichLanguageToAnswerIn(t *testing.T) {
	ctx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
	llm, prompts := recordingDurableLLM(
		`{"direct":{"tools":[]}}`,
		// The reply's own wording is not the contract here — the directive handed to
		// the model is — so the fixture stays English.
		`Still looking into it.`,
	)
	h := newDurableHarness(t, llm)

	_, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000210"))
	gt.NoError(t, err).Required()

	key := agentkernel.TriggerKey(durableChannelID, durableThreadTS, "1700000000.000210")
	proc := h.awaitTerminal(t, h.spawned(t, key))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// The planner call and the direct-reply child: both carry the directive, and the
	// second is the one whose text the user actually reads.
	seen := prompts()
	gt.Array(t, seen).Length(2).Required()
	gt.String(t, seen[0]).Contains("**Japanese**")
	gt.String(t, seen[1]).Contains("**Japanese**")
}

// A run the model never answers must tell the user the turn ended, rather than
// leaving the thread silent.
func TestDurableStartTurnReportsAFailure(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, failingLLM())

	res, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000201"))
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusStarted)

	key := agentkernel.TriggerKey(durableChannelID, durableThreadTS, "1700000000.000201")
	proc := h.awaitTerminal(t, h.spawned(t, key))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("failure")
	gt.Value(t, calls[0].ChannelID).Equal(durableChannelID)
	gt.Value(t, calls[0].ThreadTS).Equal(durableThreadTS)
	gt.String(t, calls[0].Text).NotEqual("")
}

// A second mention arriving while a turn is live must be refused as busy and
// name the run holding the thread, not queue a second run onto it.
func TestDurableStartTurnRefusesASecondTurnOnTheSameThread(t *testing.T) {
	ctx := context.Background()
	// One scripted reply: the first run parks mid-plan (its child never gets an
	// answer) so the thread stays held while the second turn is attempted.
	h := newDurableHarness(t, durableLLM(
		`{"tasks":[{"id":"t1","title":"Look","description":"look","acceptance_criteria":"looked","tools":["case_multi"]}]}`,
	))

	first, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000202"))
	gt.NoError(t, err).Required()
	gt.Value(t, first.Status).Equal(wsagent.StatusStarted)

	second, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000203"))
	gt.NoError(t, err).Required()
	gt.Value(t, second.Status).Equal(wsagent.StatusBusy)
	// The busy notice names the run holding the thread — the one just spawned.
	gt.Value(t, second.BusyOwner).Equal(string(h.spawned(t,
		agentkernel.TriggerKey(durableChannelID, durableThreadTS, "1700000000.000202"))))

	// Nothing was posted: a refused turn must leave the thread untouched.
	gt.Array(t, h.host.Calls()).Length(0)
}

// A re-delivered Slack event must be dropped silently, with no second run and no
// "busy" message — the same precedence the pre-agentkit turn lock applied.
func TestDurableStartTurnDropsARedeliveredTrigger(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		`{"tasks":[{"id":"t1","title":"Look","description":"look","acceptance_criteria":"looked","tools":["case_multi"]}]}`,
	))

	req := durableRequest("1700000000.000204")
	first, err := h.agent.StartTurn(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, first.Status).Equal(wsagent.StatusStarted)

	again, err := h.agent.StartTurn(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, again.Status).Equal(wsagent.StatusIdempotent)
	gt.Array(t, h.host.Calls()).Length(0)
}

// The run must carry the mentioning user as its access actor. Without one the
// usecase layer reads the run as a system context and bypasses private-case
// access control entirely, so this is a security invariant rather than a detail.
func TestDurableStartTurnRecordsTheActorAndScope(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		`{"tasks":[{"id":"t1","title":"Look","description":"look","acceptance_criteria":"looked","tools":["case_multi"]}]}`,
	))

	_, err := h.agent.StartTurn(ctx, durableRequest("1700000000.000205"))
	gt.NoError(t, err).Required()

	pid := h.spawned(t, agentkernel.TriggerKey(durableChannelID, durableThreadTS, "1700000000.000205"))
	proc, err := h.kernel.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()

	sc := agentkernel.ScopeFrom(proc.Metadata)
	gt.Value(t, sc.ActorUserID).Equal(durableActorID)
	gt.Value(t, sc.WorkspaceID).Equal("ws-1")
	gt.Value(t, sc.SessionID).Equal(durableSessionID)
	gt.Value(t, sc.ChannelID).Equal(durableChannelID)
	gt.Value(t, sc.ThreadTS).Equal(durableThreadTS)
	// Workspace-scoped, not pinned to a case: the cross-case tools take a case id
	// at call time.
	gt.Value(t, sc.CaseID).Equal(int64(0))
	// A slot-gated run is a scheduled batch; an interactive turn must never queue
	// behind one.
	gt.Value(t, sc.SlotGated).Equal(false)
	gt.Value(t, proc.Agent).Equal(agentkernel.AgentWorkspace)
}

// A turn with no actor must be refused at Spawn. Once the Process exists a claim
// that refuses to run it is requeued forever and holds the thread's subject with
// it, so no later turn on that thread could start.
func TestDurableStartTurnRefusesATurnWithNoActor(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM())

	req := durableRequest("1700000000.000206")
	req.ActorID = ""
	_, err := h.agent.StartTurn(ctx, req)
	gt.Error(t, err).Required()
	gt.String(t, strings.ToLower(err.Error())).Contains("actorid")
}

// A turn with no mention text must be refused: it is the planner's first user
// message, and planexec rejects an empty one at Init — which would fail the run
// after it was already recorded instead of telling the caller now.
func TestDurableStartTurnRequiresMentionText(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM())

	req := durableRequest("1700000000.000207")
	req.MentionText = ""
	_, err := h.agent.StartTurn(ctx, req)
	gt.Error(t, err).Required()
	gt.String(t, strings.ToLower(err.Error())).Contains("mentiontext")
}

// An unbound host must say so rather than panicking on a nil Kernel.
func TestDurableStartTurnRefusesWhenUnbound(t *testing.T) {
	wa, err := wsagent.NewDurable(&recordingHost{}, nil)
	gt.NoError(t, err).Required()

	_, err = wa.StartTurn(context.Background(), durableRequest("1700000000.000208"))
	gt.Error(t, err).Required()
}

func TestNewDurableRequiresAHost(t *testing.T) {
	_, err := wsagent.NewDurable(nil, nil)
	gt.Error(t, err).Required()
}
