package planexec_test

import (
	"context"
	"encoding/json"
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
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// caseDraft is a structured terminal output, standing in for a host that ends its
// turn with an object rather than prose.
type caseDraft struct {
	Title string `json:"title"`
}

// Validate is where a host's own invariants live: the schema check verifies the
// JSON shape only.
func (d caseDraft) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return goerr.New("title is required")
	}
	return nil
}

// scriptedPlanner answers each Generate with the next scripted reply. An extra
// call fails rather than silently repeating the last one, so a strategy that
// looped would be caught instead of hanging.
type scriptedPlanner struct {
	mu      sync.Mutex
	replies []string
	// inputs records the user text each call received, so a test can assert what
	// the planner was actually told.
	inputs []string
	n      atomic.Int32
}

func (p *scriptedPlanner) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					var b strings.Builder
					for _, in := range input {
						if txt, ok := in.(gollem.Text); ok {
							b.WriteString(string(txt))
						}
					}
					i := int(p.n.Add(1)) - 1
					p.mu.Lock()
					p.inputs = append(p.inputs, b.String())
					p.mu.Unlock()
					if i >= len(p.replies) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					return &gollem.Response{Texts: []string{p.replies[i]}, InputToken: 5, OutputToken: 3}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

func (p *scriptedPlanner) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.inputs))
	copy(out, p.inputs)
	return out
}

// recordingProgress captures every milestone render, so a test can assert what
// the user was shown and that the message id is reused.
type recordingProgress struct {
	mu     sync.Mutex
	lines  [][]string
	lastTS string
}

func (p *recordingProgress) Render(_ context.Context, _ planexec.ProgressTarget, messageTS string, lines []string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]string, len(lines))
	copy(cp, lines)
	p.lines = append(p.lines, cp)
	p.lastTS = messageTS
	return "1700000000.000001", nil
}

func (p *recordingProgress) renders() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]string, len(p.lines))
	copy(out, p.lines)
	return out
}

// childMetadataRecorder captures the metadata each child Process was spawned
// with. It is a ToolFactory because that is the one place the kernel hands the
// strategy's child scope back to the test.
type childMetadataRecorder struct {
	mu   sync.Mutex
	seen []map[string]string
}

func (r *childMetadataRecorder) factory() agentkit.ToolFactory {
	return func(_ context.Context, proc *agentkit.Process) ([]gollem.Tool, error) {
		if proc.ParentID != nil {
			r.mu.Lock()
			cp := make(map[string]string, len(proc.Metadata))
			for k, v := range proc.Metadata {
				cp[k] = v
			}
			r.seen = append(r.seen, cp)
			r.mu.Unlock()
		}
		return nil, nil
	}
}

func (r *childMetadataRecorder) children() []map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]string, len(r.seen))
	copy(out, r.seen)
	return out
}

type planexecRuntime struct {
	kernel *agentkit.Kernel
	agent  agentkit.Agent[planexec.Input]
}

func generousBudget() budget.Config {
	return budget.Config{MaxSteps: 64, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
}

// newTextRuntime registers a text-only plan-execute agent plus the task agent its
// children run as, on a real Kernel.
func newTextRuntime(t *testing.T, llm gollem.LLMClient, cfg budget.Config,
	progress planexec.Progress, factory agentkit.ToolFactory,
) *planexecRuntime {
	t.Helper()
	return newRuntime[planexec.TextResult](t, llm, cfg, progress, factory,
		planexec.Config[planexec.TextResult]{TextOnly: true})
}

func newRuntime[T planexec.Validatable](t *testing.T, llm gollem.LLMClient, cfg budget.Config,
	progress planexec.Progress, factory agentkit.ToolFactory, pcfg planexec.Config[T],
) *planexecRuntime {
	t.Helper()

	store := agentarchive.NewMemoryHistoryStore()
	reg := agentkit.NewRegistry()
	taskAgent, err := react.Register(reg, agentkernel.AgentTask, 1, cfg.Limiter(),
		agentkit.WithHistoryStore[react.Output](store))
	gt.NoError(t, err).Required()

	handle, err := planexec.Register(reg, agentkernel.AgentProposal, 1, taskAgent, progress,
		cfg.Limiter(), pcfg, agentkit.WithHistoryStore[planexec.Output[T]](store))
	gt.NoError(t, err).Required()

	opts := []agentkit.KernelOption{}
	if factory != nil {
		opts = append(opts, agentkit.WithToolFactory(factory))
	}
	k, err := agentkit.New(agentprocmemory.New(), llm, reg, opts...)
	gt.NoError(t, err).Required()
	return &planexecRuntime{kernel: k, agent: handle}
}

// run drives the process to a terminal state and returns it.
func (rt *planexecRuntime) run(t *testing.T, in planexec.Input, meta map[string]string) *agentkit.Process {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(ctx, agentkit.WithPollInterval(5*time.Millisecond)) }()

	opts := []agentkit.SpawnOption{}
	if meta != nil {
		opts = append(opts, agentkit.WithMetadata(meta))
	}
	pid, err := rt.agent.Spawn(ctx, rt.kernel, in, opts...)
	gt.NoError(t, err).Required()

	for {
		proc, gerr := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, gerr).Required()
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

func textInput() planexec.Input {
	return planexec.Input{
		SystemPrompt: "you coordinate investigations",
		UserInput:    "what happened here?",
		KnownToolIDs: []string{"slack_ro", "notion"},
	}
}

// decodeText reads a finished text-only run's output.
func decodeText(t *testing.T, raw []byte) planexec.Output[planexec.TextResult] {
	t.Helper()
	out, err := planexec.DecodeOutput[planexec.TextResult](raw)
	gt.NoError(t, err).Required()
	return out
}

// plan → collect → replan → final is the loop's spine. Each of the four is its
// own checkpointed transition, and the child task runs as a real child Process
// rather than a goroutine.
func TestPlanCollectReplanFinal(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		// plan: one task
		`{"tasks":[{"id":"t1","title":"Read the thread","description":"read it","acceptance_criteria":"the thread is summarised","tools":["slack_ro"]}]}`,
		// the child task's own answer
		`the thread says the deploy failed`,
		// replan: finalize
		`{"finalize":{"reason":"enough is known"}}`,
		// final: prose
		`The deploy failed.`,
	}}
	progress := &recordingProgress{}
	rt := newTextRuntime(t, planner.client(), generousBudget(), progress, nil)

	in := textInput()
	in.Progress = planexec.ProgressTarget{ChannelID: "C1", ThreadTS: "1700000000.000001"}
	proc := rt.run(t, in, nil)

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("The deploy failed.")

	// The round's observation trail reached the terminal output.
	gt.Array(t, out.Observations).Length(1).Required()
	gt.Array(t, out.Observations[0].Results).Length(1).Required()
	gt.Value(t, out.Observations[0].Results[0].Status).Equal(planexec.TaskStatusCompleted)
	gt.String(t, out.Observations[0].Results[0].Summary).Contains("deploy failed")

	// The replan round was told what the child found, which is what makes the
	// observations more than bookkeeping.
	seen := planner.seen()
	gt.Number(t, len(seen)).GreaterOrEqual(3)
	gt.String(t, seen[2]).Contains("Observations from prior investigations")
	gt.String(t, seen[2]).Contains("deploy failed")

	// Milestones accumulate into ONE message, reused by id.
	renders := progress.renders()
	gt.Number(t, len(renders)).GreaterOrEqual(3)
	gt.String(t, renders[0][0]).Contains("Planning")
	gt.String(t, progress.lastTS).Equal("1700000000.000001")
	last := renders[len(renders)-1]
	gt.Number(t, len(last)).GreaterOrEqual(len(renders[0]))
}

// A child's toolsets must be the ones its task was planned with, while the rest
// of the parent's scope survives. SpawnChild REPLACES the metadata map, so a
// child built from scratch would lose the workspace and case and have no tools at
// all.
func TestChildInheritsTheParentScopeWithItsOwnToolsets(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["notion"]}]}`,
		`read`,
		`{"finalize":{"reason":"done"}}`,
		`ok`,
	}}
	recorder := &childMetadataRecorder{}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, recorder.factory())

	parent := agentkernel.Scope{
		WorkspaceID: "ws-1", CaseID: 42, ActorUserID: "U1",
		ToolSets: []string{agentkernel.ToolSetsAll},
	}
	proc := rt.run(t, textInput(), parent.Metadata())
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	children := recorder.children()
	gt.Array(t, children).Length(1).Required()
	child := agentkernel.ScopeFrom(children[0])
	// Narrowed to the task's toolsets…
	gt.Array(t, child.ToolSets).Equal([]string{"notion"})
	// …while everything the child needs to build any tool at all survived.
	gt.String(t, child.WorkspaceID).Equal("ws-1")
	gt.Number(t, child.CaseID).Equal(42)
	gt.String(t, child.ActorUserID).Equal("U1")
}

// A planner reply that does not parse must be corrected on the NEXT transition,
// not retried inside this one: one transition is one LLM call, so an in-place
// retry would spend the budget without checkpointing between attempts.
func TestRejectedPlanIsCorrectedOnTheNextTransition(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`not json at all`,
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		`answer`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	seen := planner.seen()
	gt.Number(t, len(seen)).GreaterOrEqual(2)
	// The second planner call carries the rejection, so the model can fix it.
	gt.String(t, seen[1]).Contains("Your previous response was rejected")
	// And each attempt was its own committed transition.
	gt.Number(t, proc.Metrics.Steps).GreaterOrEqual(2)
}

// A question ends the turn rather than waiting on an await: holding the run open
// while a person takes minutes or hours would pin its subject and block every
// later turn on the thread.
func TestQuestionEndsTheTurn(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"question":{"reason":"the environment is ambiguous","items":[{"id":"env","text":"Which environment?","type":"free_text"}]}}`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	in := textInput()
	in.AllowQuestion = true
	proc := rt.run(t, in, nil)

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputQuestion)
	gt.Value(t, out.Question).NotNil().Required()
	gt.Array(t, out.Question.Items).Length(1).Required()
	gt.String(t, out.Question.Items[0].Text).Contains("Which environment?")
	// The observations gathered before the question are carried, so the host can
	// show what was learnt while asking.
	gt.Array(t, out.Observations).Length(1)
}

// The direct path answers a trivial request without an investigation round, and
// its single child's reply IS the turn's answer.
func TestDirectPathAnswersWithoutInvestigating(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"direct":{"reason":"trivial","tools":[]}}`,
		`Yes, it is deployed.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	in := textInput()
	in.AllowDirect = true
	proc := rt.run(t, in, nil)

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputDirect)
	gt.String(t, out.Text).Contains("Yes, it is deployed.")
	// No replan and no final call happened: the script would have failed on an
	// extra Generate.
	gt.Number(t, len(planner.seen())).Equal(2)
}

// A failed child is an observation, not a failed run: the planner decides on the
// next round whether the partial picture is enough.
func TestAFailedChildIsReportedToThePlanner(t *testing.T) {
	const (
		planJSON     = `{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`
		finalizeJSON = `{"finalize":{"reason":"enough"}}`
	)
	// Call 1 is the parent's plan. Calls 2 and 3 are the child's two attempts —
	// WithMaxStepAttempts(1) permits one retry after the first, so BOTH must fail
	// or the retry would succeed and the child would not be a failed task at all.
	// Calls 4 and 5 are the parent's replan and final.
	var calls atomic.Int32
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					switch calls.Add(1) {
					case 1:
						return &gollem.Response{Texts: []string{planJSON}}, nil
					case 2, 3:
						return nil, goerr.New("the child's model is unreachable")
					case 4:
						return &gollem.Response{Texts: []string{finalizeJSON}}, nil
					default:
						return &gollem.Response{Texts: []string{"partial answer"}}, nil
					}
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
	rt := newTextRuntime(t, llm, generousBudget(), nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	served := make(chan error, 1)
	// One attempt per transition, so the child fails instead of being retried
	// through agentkit's backoff for the length of the test.
	go func() {
		served <- rt.kernel.Serve(ctx,
			agentkit.WithPollInterval(5*time.Millisecond), agentkit.WithMaxStepAttempts(1))
	}()
	pid, err := rt.agent.Spawn(ctx, rt.kernel, textInput())
	gt.NoError(t, err).Required()
	var proc *agentkit.Process
	for {
		got, gerr := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, gerr).Required()
		if got.Status.Terminal() {
			proc = got
			break
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return
		case <-time.After(3 * time.Millisecond):
		}
	}
	cancel()
	<-served

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Array(t, out.Observations).Length(1).Required()
	gt.Array(t, out.Observations[0].Results).Length(1).Required()
	gt.Value(t, out.Observations[0].Results[0].Status).Equal(planexec.TaskStatusFailed)
	gt.String(t, out.Observations[0].Results[0].Error).NotEqual("")
}

// A structured host's terminal output is decoded, Validate()d, and put through
// its finalizers. A rejection is fed back and regenerated — bounded — because the
// model can fix its own JSON but cannot fix an infrastructure error.
func TestStructuredFinalIsValidatedAndRegenerated(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		// First terminal output fails Validate (empty title).
		`{"title":"  "}`,
		// Second fails the host finalizer.
		`{"title":"forbidden"}`,
		// Third is accepted.
		`{"title":"Deploy failure"}`,
	}}
	var finalizerCalls atomic.Int32
	var seenWorkspace []string
	cfg := planexec.Config[caseDraft]{
		Finalizers: []planexec.Finalizer[caseDraft]{
			func(_ context.Context, meta map[string]string, d *caseDraft) error {
				finalizerCalls.Add(1)
				seenWorkspace = append(seenWorkspace, meta["workspace_id"])
				if d.Title == "forbidden" {
					return goerr.New("that title is not allowed in this workspace")
				}
				return nil
			},
		},
	}
	rt := newRuntime(t, planner.client(), generousBudget(), nil, nil, cfg)

	proc := rt.run(t, textInput(), map[string]string{"workspace_id": "ws-1"})
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	out, err := planexec.DecodeOutput[caseDraft](proc.Output)
	gt.NoError(t, err).Required()
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.Value(t, out.Data).NotNil().Required()
	gt.String(t, out.Data.Title).Equal("Deploy failure")

	// The finalizer ran on every shape-valid attempt, and the rejection reached
	// the model.
	gt.Number(t, finalizerCalls.Load()).Equal(2)
	// Each call saw the run's own scope, which is what lets one registered
	// finalizer validate against the workspace the run belongs to.
	gt.Array(t, seenWorkspace).Equal([]string{"ws-1", "ws-1"})
	seen := planner.seen()
	gt.String(t, seen[len(seen)-1]).Contains("rejected")
}

// Past the retry bound the run reports a fallback rather than failing outright:
// the host still has the observation trail to tell the user what was learnt.
func TestStructuredFinalFallsBackAfterTooManyRejections(t *testing.T) {
	replies := []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
	}
	// Every terminal attempt is invalid.
	for range 6 {
		replies = append(replies, `{"title":""}`)
	}
	planner := &scriptedPlanner{replies: replies}
	rt := newRuntime(t, planner.client(), generousBudget(), nil, nil, planexec.Config[caseDraft]{})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	out, err := planexec.DecodeOutput[caseDraft](proc.Output)
	gt.NoError(t, err).Required()
	gt.Value(t, out.Kind).Equal(planexec.OutputFallback)
	gt.String(t, out.FallbackReason).Contains("title is required")
	gt.Value(t, out.Data).Nil()
	// The trail survives a fallback, which is what lets the host say what it did
	// learn instead of only that it failed.
	gt.Array(t, out.Observations).Length(1)
}

// Crossing the notice ratio stops the run opening new rounds and sends it to
// produce an answer from what it has. Enforcement alone would cut it off
// mid-investigation with nothing to show.
func TestBudgetNoticeWrapsTheRunUp(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		// No replan reply is scripted: reaching one would fail the test.
		`the partial answer`,
	}}
	// Notice at 40% of 6 steps, so it fires during the first round and the replan
	// is skipped.
	cfg := budget.Config{MaxSteps: 6, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.4}
	rt := newTextRuntime(t, planner.client(), cfg, nil, nil)

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("partial answer")
}

// recordingTool answers a planner lookup and records that it was called.
type recordingTool struct {
	name string
	mu   sync.Mutex
	args []map[string]any
}

func (t *recordingTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        t.name,
		Description: "look something up",
		Parameters: map[string]*gollem.Parameter{
			"id": {Type: gollem.TypeString, Description: "what to look up"},
		},
	}
}

func (t *recordingTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.args = append(t.args, args)
	return map[string]any{"result": "workspace ws-1 has a severity field"}, nil
}

func (t *recordingTool) calls() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]map[string]any, len(t.args))
	copy(out, t.args)
	return out
}

// toolCallingPlanner answers the i-th Generate with replies[i]: either a JSON plan
// or, when the entry is a tool name, a request to call it.
type toolCallingPlanner struct {
	mu      sync.Mutex
	replies []any // string (JSON) or *gollem.FunctionCall
	n       atomic.Int32
	inputs  []string
}

func (p *toolCallingPlanner) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					var b strings.Builder
					for _, in := range input {
						if txt, ok := in.(gollem.Text); ok {
							b.WriteString(string(txt))
						}
					}
					i := int(p.n.Add(1)) - 1
					p.mu.Lock()
					p.inputs = append(p.inputs, b.String())
					p.mu.Unlock()
					if i >= len(p.replies) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					switch reply := p.replies[i].(type) {
					case *gollem.FunctionCall:
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{reply},
							InputToken: 5, OutputToken: 3}, nil
					default:
						return &gollem.Response{Texts: []string{reply.(string)},
							InputToken: 5, OutputToken: 3}, nil
					}
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

func (p *toolCallingPlanner) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.inputs))
	copy(out, p.inputs)
	return out
}

// The planner may look something up before it decides. The lookup runs as its own
// transition, and the decision that follows continues from the conversation rather
// than re-asking the original request.
func TestPlannerToolCallRunsBeforeThePlan(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		// round 1: look the workspace up instead of deciding
		&gollem.FunctionCall{ID: "c1", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}},
		// with the answer in hand, plan
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		`The severity field is set.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{lookup}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)

	// The tool actually ran, with the planner's arguments.
	calls := lookup.calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0]["id"]).Equal("ws-1")

	// The planning call that followed the lookup sent no user turn: the request is
	// already in the conversation, and re-sending it would ask again as though
	// nothing had been learnt.
	seen := planner.seen()
	gt.Number(t, len(seen)).GreaterOrEqual(2).Required()
	gt.String(t, seen[0]).Contains("what happened here?")
	gt.String(t, seen[1]).Equal("")
}

// A planner that only ever calls tools must be stopped: its lookups are free of
// the round budget, so nothing else would bound them.
func TestPlannerToolCallsAreBounded(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	call := func() any {
		return &gollem.FunctionCall{ID: "c", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}}
	}
	planner := &toolCallingPlanner{replies: []any{
		call(), call(), call(), call(),
		// The fifth is past the bound: the calls are dropped and this text is parsed
		// as a plan instead, which fails validation and comes back as a correction.
		call(),
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		`Answered.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{lookup}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// Four lookups ran; the fifth request was refused rather than executed.
	gt.Array(t, lookup.calls()).Length(4)
	// And the planner was told to decide instead.
	seen := planner.seen()
	gt.String(t, seen[5]).Contains("rejected")
}

// countingProgress counts how many NEW messages were posted, which is the thing
// a duplicate would show up as.
type countingProgress struct {
	mu     sync.Mutex
	posts  int
	update int
}

func (p *countingProgress) Render(_ context.Context, _ planexec.ProgressTarget,
	messageTS string, _ []string,
) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if messageTS == "" {
		p.posts++
	} else {
		p.update++
	}
	return "1700000000.000001", nil
}

func (p *countingProgress) counts() (posts, updates int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.posts, p.update
}

// A run's milestones go into ONE message that is updated in place, across every
// transition of the run — which is what makes the message id checkpointed state
// rather than an in-process variable. Without that, a run picked up by another
// instance would start a second message.
func TestProgressDrawsOneMessageAndUpdatesIt(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		`Done.`,
	}}
	progress := &countingProgress{}
	rt := newTextRuntime(t, planner.client(), generousBudget(), progress, nil)

	in := textInput()
	in.Progress = planexec.ProgressTarget{ChannelID: "C1", ThreadTS: "1700000000.000001"}
	proc := rt.run(t, in, nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// One post, then updates for every later milestone.
	posts, updates := progress.counts()
	gt.Number(t, posts).Equal(1)
	gt.Number(t, updates).GreaterOrEqual(3)
}

// recordingAsker captures the question and the await key it must be answered on.
type recordingAsker struct {
	mu       sync.Mutex
	asked    []planexec.Question
	pid      agentkit.ProcessID
	key      agentkit.AwaitKey
	workspce string
}

func (a *recordingAsker) Ask(_ context.Context, pid agentkit.ProcessID, meta map[string]string,
	key agentkit.AwaitKey, q planexec.Question,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked = append(a.asked, q)
	a.pid, a.key, a.workspce = pid, key, meta["workspace_id"]
	return nil
}

func (a *recordingAsker) target() (agentkit.ProcessID, agentkit.AwaitKey) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pid, a.key
}

func (a *recordingAsker) questions() []planexec.Question {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]planexec.Question, len(a.asked))
	copy(out, a.asked)
	return out
}

// A host that waits in-band keeps ONE Process across the human's reply: the run
// parks on a question await, the answer is delivered with Respond, and the same
// run finishes with its budget and history intact.
func TestQuestionSuspendsAndResumesTheSameRun(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`the thread does not say which environment`,
		`{"question":{"reason":"which environment?","items":[{"id":"env","text":"Which environment?","type":"select","options":["staging","production"]}]}}`,
		// After the answer the run replans and finalises.
		`{"finalize":{"reason":"the environment is known"}}`,
		`Production broke.`,
	}}
	asker := &recordingAsker{}
	rt := newRuntime(t, planner.client(), generousBudget(), nil, nil,
		planexec.Config[planexec.TextResult]{TextOnly: true, Asker: asker})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(ctx, agentkit.WithPollInterval(5*time.Millisecond)) }()
	defer func() { cancel(); <-served }()

	in := textInput()
	in.AllowQuestion = true
	in.SuspendOnQuestion = true
	pid, err := rt.agent.Spawn(ctx, rt.kernel, in,
		agentkit.WithMetadata(map[string]string{"workspace_id": "ws-1"}))
	gt.NoError(t, err).Required()

	// The run parks on its question, and the asker was told where to send the
	// answer.
	var askPID agentkit.ProcessID
	var askKey agentkit.AwaitKey
	for {
		askPID, askKey = asker.target()
		if askKey != "" {
			break
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return
		case <-time.After(3 * time.Millisecond):
		}
	}
	gt.Value(t, askPID).Equal(pid)
	gt.Value(t, asker.workspce).Equal("ws-1")
	questions := asker.questions()
	gt.Array(t, questions).Length(1).Required()
	gt.String(t, questions[0].Reason).Equal("which environment?")

	// Answering continues the same Process.
	answer := planexec.RenderAnswers(questions[0], []planexec.QuestionAnswer{
		{ID: "env", Choice: "production"},
	})
	gt.NoError(t, rt.kernel.Respond(ctx, pid, askKey, []byte(answer))).Required()

	for {
		proc, gerr := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, gerr).Required()
		if proc.Status.Terminal() {
			gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
			out := decodeText(t, proc.Output)
			gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
			gt.String(t, out.Text).Contains("Production broke.")
			break
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return
		case <-time.After(3 * time.Millisecond):
		}
	}

	// The replan round that followed was told what the user answered.
	seen := planner.seen()
	gt.String(t, seen[len(seen)-2]).Contains("production")
}

// A run told to wait in-band with no asker must fail loudly: it would otherwise
// park on an await nobody can see, and wait forever.
func TestSuspendOnQuestionRequiresAnAsker(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"]}]}`,
		`nothing conclusive`,
		`{"question":{"reason":"which environment?","items":[{"id":"env","text":"Which?","type":"select","options":["a","b"]}]}}`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	in := textInput()
	in.AllowQuestion = true
	in.SuspendOnQuestion = true
	proc := rt.run(t, in, nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
}

func TestRenderAnswersLabelsEachAnswer(t *testing.T) {
	q := planexec.Question{
		Reason: "which environment?",
		Items: []planexec.QuestionItem{
			{ID: "env", Text: "Which environment?", Type: planexec.QuestionItemSelect},
			{ID: "note", Text: "Anything else?", Type: planexec.QuestionItemFreeText},
		},
	}
	got := planexec.RenderAnswers(q, []planexec.QuestionAnswer{
		{ID: "env", Choice: "production"},
		{ID: "note", FreeText: "started after the 14:00 deploy"},
		{ID: "gone", Choice: "x"},
	})
	gt.String(t, got).Contains("## env — Which environment?")
	gt.String(t, got).Contains("Answer (select): production")
	gt.String(t, got).Contains("## note — Anything else?")
	gt.String(t, got).Contains("Answer (free_text): started after the 14:00 deploy")
	// An answer whose question is gone is kept rather than dropped.
	gt.String(t, got).Contains("unknown question id")
}

func TestRegisterRequiresALimiterAndATaskAgent(t *testing.T) {
	reg := agentkit.NewRegistry()
	cfg := generousBudget()
	taskAgent, err := react.Register(reg, agentkernel.AgentTask, 1, cfg.Limiter())
	gt.NoError(t, err).Required()

	_, err = planexec.Register(reg, agentkernel.AgentProposal, 1, taskAgent, nil, nil,
		planexec.Config[planexec.TextResult]{TextOnly: true})
	gt.Error(t, err).Is(agentkit.ErrInvalidAgentDef)

	_, err = planexec.Register(reg, agentkernel.AgentWorkspace, 1,
		agentkit.Agent[react.Input]{}, nil, cfg.Limiter(),
		planexec.Config[planexec.TextResult]{TextOnly: true})
	gt.Error(t, err).Is(agentkit.ErrInvalidAgentDef)
}

func TestInputValidate(t *testing.T) {
	testCases := map[string]struct {
		mutate  func(planexec.Input) planexec.Input
		wantErr bool
	}{
		"valid":             {mutate: func(in planexec.Input) planexec.Input { return in }},
		"no system prompt":  {mutate: func(in planexec.Input) planexec.Input { in.SystemPrompt = ""; return in }, wantErr: true},
		"no user input":     {mutate: func(in planexec.Input) planexec.Input { in.UserInput = ""; return in }, wantErr: true},
		"no known tool ids": {mutate: func(in planexec.Input) planexec.Input { in.KnownToolIDs = nil; return in }, wantErr: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.mutate(textInput()).Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestTextResultAlwaysValidates(t *testing.T) {
	gt.NoError(t, planexec.TextResult{}.Validate())
}

func TestDecodeOutputRejectsGarbage(t *testing.T) {
	_, err := planexec.DecodeOutput[planexec.TextResult]([]byte("{"))
	gt.Error(t, err)
}

// The output round-trips through the bytes a Process stores, which is how a
// parent reads a child's result and how a host reads a finished run's.
func TestOutputRoundTrip(t *testing.T) {
	original := planexec.Output[planexec.TextResult]{
		Kind: planexec.OutputFinal,
		Text: "answer",
		Observations: []planexec.PhaseSummary{{
			Phase:   1,
			Tasks:   []planexec.TaskPlan{{ID: "t1", Title: "Read"}},
			Results: []planexec.TaskResult{{TaskID: "t1", Status: planexec.TaskStatusCompleted, Summary: "ok"}},
		}},
	}
	raw, err := json.Marshal(original)
	gt.NoError(t, err).Required()
	got, err := planexec.DecodeOutput[planexec.TextResult](raw)
	gt.NoError(t, err).Required()
	gt.Value(t, got.Kind).Equal(planexec.OutputFinal)
	gt.String(t, got.Text).Equal("answer")
	gt.Array(t, got.Observations).Length(1).Required()
	gt.String(t, got.Observations[0].Results[0].Summary).Equal("ok")
}
