package planexec_test

import (
	"context"
	"encoding/json"
	"sort"
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
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
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
	// systemPrompts records the system prompt each call was made under, aligned by
	// index with inputs. A child's identity lives here rather than in its input —
	// the input is just the task text — so this is where a test checks WHICH prompt
	// a child was spawned as.
	systemPrompts []string
	n             atomic.Int32
}

func (p *scriptedPlanner) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			// The system prompt is a session-level setting, so it is read here and
			// attributed to the calls this session makes.
			cfg := gollem.NewSessionConfig(opts...)
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
					p.systemPrompts = append(p.systemPrompts, cfg.SystemPrompt())
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

// seenSystemPrompts returns the system prompt of each call, in call order.
func (p *scriptedPlanner) seenSystemPrompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.systemPrompts))
	copy(out, p.systemPrompts)
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

// testSpend is what these runs are judged against in money. A limiter with no
// resolver stops every run, so one is required even where the test is about a
// step ceiling; this prices a run far below its allowance so the money arm stays
// out of the way. A test about the money ceiling passes its own through
// newRuntimeWithSpend.
func testSpend() budget.LimitResolver {
	return func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{
			Budget: pricing.FromUSD(1000),
			Rate:   pricing.Rate{Input: 1, Output: 1},
		}
	}
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
	return newRuntimeWithSpend(t, llm, cfg, testSpend(), progress, factory, pcfg)
}

// newRuntimeWithSpend is newRuntime with the money resolver named, for a test
// about the money ceiling rather than the step one.
func newRuntimeWithSpend[T planexec.Validatable](t *testing.T, llm gollem.LLMClient,
	cfg budget.Config, spend budget.LimitResolver,
	progress planexec.Progress, factory agentkit.ToolFactory, pcfg planexec.Config[T],
) *planexecRuntime {
	t.Helper()

	store := agentarchive.NewMemoryHistoryStore()
	reg := agentkit.NewRegistry()
	taskAgent, err := react.Register(reg, agentkernel.AgentTask, 1, cfg.Limiter(spend),
		agentkit.WithHistoryStore[react.Output](store))
	gt.NoError(t, err).Required()

	handle, err := planexec.Register(reg, agentkernel.AgentProposal, 1, taskAgent, progress,
		cfg.Limiter(spend), pcfg, agentkit.WithHistoryStore[planexec.Output[T]](store))
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
		`{"tasks":[{"id":"t1","title":"Read the thread","description":"read it","acceptance_criteria":"the thread is summarised","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["notion"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

// A direct reply reaches the user in full. Every other child's text is a summary
// fed back into the planner's context and is bounded at subAgentSummaryMaxBytes
// (8 KiB) with an "…[truncated]" marker appended; this one is the reply itself, so
// applying that bound would cut a user-facing message off mid-sentence and publish
// the marker.
func TestALongDirectReplyIsNotTruncated(t *testing.T) {
	// Comfortably past the sub-agent summary bound, and distinctive at the end so a
	// truncation is visible rather than merely a length mismatch.
	const tail = "END-OF-REPLY"
	long := strings.Repeat("a very long but entirely legitimate answer. ", 400) + tail
	gt.Number(t, len(long)).GreaterOrEqual(8 * 1024).Required()

	planner := &scriptedPlanner{replies: []string{`{"direct":{"tools":[]}}`, long}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	in := textInput()
	in.AllowDirect = true
	proc := rt.run(t, in, nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputDirect)
	gt.String(t, out.Text).NotContains("[truncated]")
	gt.String(t, out.Text).HasSuffix(tail)
	gt.Number(t, len(out.Text)).Equal(len(long))
}

// The direct child writes the message the user reads, so it is prompted as its
// author — not as an investigation sub-agent reporting to the planner. When it was
// prompted as the latter (launchDirect reused buildSubAgentSystemPrompt between
// #261 and this test), that prompt's output rules were obeyed and the resulting
// report — a "Conclusion" line, a "Supporting Evidence" section, and a closing
// remark about what "the parent planner" should do — was posted into the Slack
// thread verbatim as the answer.
func TestTheDirectChildIsPromptedToWriteTheUsersReply(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"direct":{"tools":[]}}`,
		`Understood, thanks for the update.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil, nil)

	in := textInput()
	in.AllowDirect = true
	in.LanguageLabel = "Japanese"
	in.TaskContext = "channel_id: C123\nthread_ts: 1700000000.000100"
	proc := rt.run(t, in, nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	prompts := planner.seenSystemPrompts()
	gt.Array(t, prompts).Length(2).Required()
	child := prompts[1]

	// It is the direct template, and specifically NOT the sub-agent one.
	gt.String(t, child).Contains("Direct reply (planexec runtime)")
	gt.String(t, child).NotContains("investigation sub-agent dispatched by a parent planner")
	gt.String(t, child).NotContains("the parent planner can fold into its next decision")
	// The two things the sub-agent template asked for and this one must forbid.
	gt.String(t, child).Contains("posted to the user verbatim")
	gt.String(t, child).Contains(`No "Conclusion" heading, no "Supporting Evidence" section`)
	// The host's persona prompt and the run's tool-pinning context are carried, so
	// the reply knows who it is and its tools have the identifiers they need.
	gt.String(t, child).Contains(in.SystemPrompt)
	gt.String(t, child).Contains("thread_ts: 1700000000.000100")
	// The language directive reaches the one call whose text the user actually sees.
	gt.String(t, child).Contains("MUST be written in **Japanese**")
}

// A failed child is an observation, not a failed run: the planner decides on the
// next round whether the partial picture is enough.
func TestAFailedChildIsReportedToThePlanner(t *testing.T) {
	const (
		planJSON     = `{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	// The opening plan call is nowhere near the ceiling and must not be nagged;
	// the terminal call is past the threshold and must be told what the reserve is
	// for. It reaches the call through the SYSTEM prompt, which is the only route
	// that also works when the terminal call sends no user turn.
	prompts := planner.seenSystemPrompts()
	gt.Array(t, prompts).Length(3).Required()
	gt.Bool(t, strings.Contains(prompts[0], "reserve")).False()
	gt.String(t, prompts[2]).Contains("THIS turn is your final tool call")
}

// TestASpentBudgetWrapsTheRunUpInsteadOfFailingIt is the regression test for the
// Job run that ended as FAILED with "cost budget exhausted ($2.31/$2.00)".
//
// Its four sub-agents had already created and updated the knowledge entries the
// Job existed to write; what the run lost was the transition in which it would
// have said so. That happened because the money ceiling answered LimitStop, which
// agentkit reads at the transition boundary and acts on WITHOUT calling Step — so
// the planner never reached stepReplan, never saw a notice, and never produced a
// terminal output.
//
// Here the same thing happens to the money — the budget is spent before the first
// round even finishes — and the run must still reach its terminal output and
// succeed. The steps ceiling is far out of reach so that only the money arm can
// produce the verdict under test.
func TestASpentBudgetWrapsTheRunUpInsteadOfFailingIt(t *testing.T) {
	// The write the Job existed for. In production this was knowledge__create_knowledge,
	// which had already succeeded when the run was killed.
	writer := &recordingTool{name: "knowledge__create_knowledge"}
	planner := &toolCallingPlanner{replies: []any{
		// 1: the parent plans one task.
		`{"tasks":[{"id":"t1","title":"Record","description":"record it","acceptance_criteria":"the entry exists","tools":["slack_ro"],"budget_usd":0.01}]}`,
		// 2-3: the child performs the write and reports it.
		&gollem.FunctionCall{ID: "w1", Name: "knowledge__create_knowledge", Arguments: map[string]any{"title": "e"}},
		`recorded the entry`,
		// No replan reply is scripted: reaching one would fail the test, which is
		// how the run is pinned to going straight to the terminal call.
		// 4: the terminal output.
		`what the sub-agent recorded`,
	}}
	// The arithmetic is what makes this the production shape rather than an
	// ordinary notice. Every call reports 5 input and 3 output tokens, so each
	// costs 8 NanoUSD at this rate against a 20-NanoUSD allowance whose notice
	// threshold is 16:
	//
	//   - the parent's own plan call leaves it at 8 — under the threshold, so it
	//     is never told anything and plans a full round;
	//   - the child spends 16 of its own, also under the threshold, so it too runs
	//     to completion and its write lands;
	//   - the child's 16 is then folded into the parent in ONE write, taking it to
	//     24 — past the whole budget, having never seen "nearly".
	//
	// That jump is what used to answer LimitStop and kill the run with the write
	// already done and nothing said about it.
	crossedByTheFold := func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{Budget: 20, Rate: pricing.Rate{Input: 1, Output: 1}}
	}
	cfg := budget.Config{MaxSteps: 64, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	rt := newRuntimeWithSpend(t, planner.client(), cfg, crossedByTheFold, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{writer}, nil
		},
		planexec.Config[planexec.TextResult]{TextOnly: true})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	gt.Value(t, proc.Failure).Nil()

	// The write happened once, and the run got to report it. Losing the second
	// half while keeping the first is the whole of the production failure.
	gt.Array(t, writer.calls()).Length(1)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("what the sub-agent recorded")

	// Four calls: plan, the child's two, the terminal output. No replan — the
	// reserve diverts stepReplan to the terminal call without generating, so no
	// further round is started on a budget that is already gone.
	prompts := planner.systemSeen()
	gt.Array(t, prompts).Length(4).Required()
	// Neither the parent's plan call nor either of the child's crossed anything on
	// its own: only their sum did. This is what distinguishes the fold from an
	// ordinary notice, and it is observable — a reserve instruction on any of the
	// first three would mean the run was warned before the fold.
	gt.Bool(t, strings.Contains(prompts[0], "reserve")).False()
	gt.Bool(t, strings.Contains(prompts[1], "reserve")).False()
	gt.Bool(t, strings.Contains(prompts[2], "reserve")).False()
	gt.String(t, prompts[3]).Contains("THIS turn is your final tool call")
}

// TestTheReserveAllowsATerminalToolCall pins the two moves the reserve is for on
// the plan-execute path: the call the turn still needs, and then the terminal
// output. The side effects a turn was asked for are tool calls, so a reserve that
// only bought a terminal output would let the run describe work it never did.
func TestTheReserveAllowsATerminalToolCall(t *testing.T) {
	writer := &recordingTool{name: "case__update_case"}
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		// No replan reply is scripted: reaching one would fail the test, which is
		// how the run is pinned to going straight to the terminal call.
		&gollem.FunctionCall{ID: "w1", Name: "case__update_case", Arguments: map[string]any{"id": "case-1"}},
		`the final answer`,
	}}
	// Notice at 15% of 20 steps = 3 committed transitions, which the first round
	// and its child cross together.
	cfg := budget.Config{MaxSteps: 20, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.15}
	rt := newTextRuntime(t, planner.client(), cfg, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{writer}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("final answer")

	// The call actually ran: the reserve's instruction is not a dry run.
	gt.Array(t, writer.calls()).Length(1)

	prompts := planner.systemSeen()
	gt.Array(t, prompts).Length(4).Required()
	// The reserve's first move asks for the call and forbids writing the output.
	gt.String(t, prompts[2]).Contains("THIS turn is your final tool call")
	gt.Bool(t, strings.Contains(prompts[2], "Do not call any tool again")).False()
	// The second move carries that call's result and the opposite instruction.
	gt.String(t, prompts[3]).Contains("The budget reserve is spent")
	gt.String(t, prompts[3]).Contains("Do not call any tool again")
	// It carries the result and nothing else: a call answering a tool round may
	// not send a user turn as well.
	gt.String(t, planner.seen()[3]).Equal("")
	gt.Value(t, planner.answeredWith()[3]).Equal([]string{"w1"})
}

// TestAnEmptyFirstReserveMoveStillGetsTheOutputAsked pins the failure mode the
// reserve's first instruction creates: it tells the terminal call not to write
// the output yet, and stepFinal otherwise reads an empty reply as "the final
// response was empty" and ends the turn on a fallback with no retry. A model that
// simply obeyed would then lose the turn, and the reserve would be spent on
// nothing — the exact outcome this path exists to avoid.
func TestAnEmptyFirstReserveMoveStillGetsTheOutputAsked(t *testing.T) {
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		// The reserve's first move: obeyed "do not write the terminal output", made
		// no tool call, and said nothing.
		``,
		`the final answer`,
	}}
	cfg := budget.Config{MaxSteps: 20, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.15}
	rt := newTextRuntime(t, planner.client(), cfg, nil, nil)

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	// The turn produced an answer rather than falling back on the empty reply.
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("final answer")

	prompts := planner.systemSeen()
	gt.Array(t, prompts).Length(4).Required()
	gt.String(t, prompts[2]).Contains("THIS turn is your final tool call")
	// The round is spent by the empty reply, so the call that follows asks for the
	// output instead of repeating the request for a tool call.
	gt.String(t, prompts[3]).Contains("The budget reserve is spent")
	gt.Bool(t, strings.Contains(prompts[3], "THIS turn is your final tool call")).False()
}

// TestATerminalOutputRetryInTheReserveIsAskedForTheOutput pins that a rejected
// terminal output stops the run being told not to write one. The retry's user
// turn says "re-emit the output correctly"; leaving reserveInstruction in the
// system prompt would say "do not write the terminal output on this turn"
// alongside it, and a model cannot satisfy both.
func TestATerminalOutputRetryInTheReserveIsAskedForTheOutput(t *testing.T) {
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		// The reserve's first move answers with a terminal output that Validate
		// rejects, which sends stepFinal into its retry path.
		`{"title":"","description":"no title"}`,
		`{"title":"Drafted","description":"under the reserve"}`,
	}}
	cfg := budget.Config{MaxSteps: 20, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.15}
	rt := newRuntime[caseDraft](t, planner.client(), cfg, nil, nil, planexec.Config[caseDraft]{})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	out, err := planexec.DecodeOutput[caseDraft](proc.Output)
	gt.NoError(t, err).Required()
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.Value(t, out.Data).NotNil().Required()
	gt.String(t, out.Data.Title).Equal("Drafted")

	prompts := planner.systemSeen()
	gt.Array(t, prompts).Length(4).Required()
	gt.String(t, prompts[2]).Contains("THIS turn is your final tool call")
	gt.String(t, prompts[3]).Contains("The budget reserve is spent")
	gt.Bool(t, strings.Contains(prompts[3], "Do not write the terminal output")).False()
}

// TestTheReserveDoesNotStarveTheTerminalPrompt pins that the reserve instruction
// is ADDED to the terminal prompt rather than replacing it. The call that follows
// the tool round sends no user turn, so its whole brief — the observation trail
// and the output format — rides in the system prompt; a reserve instruction that
// displaced it would leave the model told to stop calling tools and never told
// what to write.
func TestTheReserveDoesNotStarveTheTerminalPrompt(t *testing.T) {
	writer := &recordingTool{name: "case__update_case"}
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		&gollem.FunctionCall{ID: "w1", Name: "case__update_case", Arguments: map[string]any{"id": "case-1"}},
		`the final answer`,
	}}
	cfg := budget.Config{MaxSteps: 20, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.15}
	rt := newTextRuntime(t, planner.client(), cfg, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{writer}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	prompts := planner.systemSeen()
	gt.Array(t, prompts).Length(4).Required()
	gt.String(t, prompts[3]).Contains("The investigation loop has finished")
	gt.String(t, prompts[3]).Contains("Emit plain natural-language text")
	gt.String(t, prompts[3]).Contains("The budget reserve is spent")
}

// recordingTool answers a planner lookup and records that it was called.
type recordingTool struct {
	name string
	// params replaces the default parameter set when non-nil, for a test about
	// the arguments themselves.
	params map[string]*gollem.Parameter
	mu     sync.Mutex
	args   []map[string]any
}

func (t *recordingTool) Spec() gollem.ToolSpec {
	params := t.params
	if params == nil {
		params = map[string]*gollem.Parameter{
			"id": {Type: gollem.TypeString, Description: "what to look up"},
		}
	}
	return gollem.ToolSpec{
		Name:        t.name,
		Description: "look something up",
		Parameters:  params,
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
	mu sync.Mutex
	// replies[i] answers the i-th Generate: a string (JSON or prose), a
	// *gollem.FunctionCall, a []*gollem.FunctionCall, or an error the call fails
	// with (which the runtime retries from the checkpoint).
	replies []any
	n       atomic.Int32
	inputs  []string
	// systemPrompts[i] is the system prompt the i-th Generate ran under, which is
	// where an instruction reaches a call that sends no user turn.
	systemPrompts []string
	// respIDs[i] is the tool-call ids the i-th Generate was answered with, in the
	// order they arrived. Recorded per call because a turn's responses must all
	// reach ONE call.
	respIDs [][]string
}

// client answers the i-th Generate with replies[i] and grows the conversation by
// one message per call, so the history a later call is seeded with is the one the
// run actually built.
//
// The answers are read off that history rather than off the inputs: the results
// reach the model through the conversation, which is where the session's CallTool
// appends them.
func (p *toolCallingPlanner) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			cfg := gollem.NewSessionConfig(opts...)
			var seeded []gollem.Message
			if h := cfg.History(); h != nil {
				seeded = h.Messages
			}
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
					p.systemPrompts = append(p.systemPrompts, cfg.SystemPrompt())
					p.respIDs = append(p.respIDs, trailingToolResponseIDs(seeded))
					p.mu.Unlock()
					if i >= len(p.replies) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					switch reply := p.replies[i].(type) {
					case error:
						return nil, reply
					case *gollem.FunctionCall:
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{reply},
							InputToken: 5, OutputToken: 3}, nil
					case []*gollem.FunctionCall:
						return &gollem.Response{FunctionCalls: reply,
							InputToken: 5, OutputToken: 3}, nil
					default:
						return &gollem.Response{Texts: []string{reply.(string)},
							InputToken: 5, OutputToken: 3}, nil
					}
				},
				HistoryFunc: func() (*gollem.History, error) {
					grown := make([]gollem.Message, len(seeded), len(seeded)+1)
					copy(grown, seeded)
					grown = append(grown, gollem.Message{Role: gollem.RoleAssistant})
					return &gollem.History{LLType: gollem.LLMTypeOpenAI,
						Version: gollem.HistoryVersion, Messages: grown}, nil
				},
			}, nil
		},
	}
}

// trailingToolResponseIDs reports the tool-call ids the conversation's LAST
// message answers, in order — empty unless that message is a tool message.
//
// Only the trailing message counts: a provider reads the results as the answer to
// the model turn immediately before them, so results that ended up anywhere else
// did not answer the call they belong to.
func trailingToolResponseIDs(messages []gollem.Message) []string {
	if len(messages) == 0 {
		return nil
	}
	last := messages[len(messages)-1]
	if last.Role != gollem.RoleTool {
		return nil
	}
	var ids []string
	for _, c := range last.Contents {
		resp, err := c.GetToolResponseContent()
		if err != nil {
			continue
		}
		ids = append(ids, resp.ToolCallID)
	}
	return ids
}

func (p *toolCallingPlanner) seen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.inputs))
	copy(out, p.inputs)
	return out
}

func (p *toolCallingPlanner) systemSeen() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.systemPrompts))
	copy(out, p.systemPrompts)
	return out
}

func (p *toolCallingPlanner) answeredWith() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]string, len(p.respIDs))
	copy(out, p.respIDs)
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	// The planning call that followed the lookup restated nothing: its user turn
	// carries the tool result and no text, because the request is already in the
	// conversation and re-sending it would ask again as though nothing had been
	// learnt.
	seen := planner.seen()
	gt.Number(t, len(seen)).GreaterOrEqual(2).Required()
	gt.String(t, seen[0]).Contains("what happened here?")
	gt.String(t, seen[1]).Equal("")
	gt.Value(t, planner.answeredWith()[1]).Equal([]string{"c1"})
}

// A planner that only ever calls tools must be stopped: its lookups are free of
// the round budget, so nothing else would bound them.
// EVERY tool call the planner makes must be answered, including the ones past its
// allowance, and the allowance is enforced by telling the planner to stop.
//
// Leaving a call unanswered is not a lesser outcome, it is a broken conversation:
// the model's function-call turn is already in the history, and a provider rejects
// the next request unless the number of function responses equals the number of
// calls in it. Dropping the over-budget calls did exactly that against a live
// model — "Please ensure that the number of function response parts is equal to the
// number of function call parts" — retried until the run failed, so a create turn
// that asked for one lookup too many produced no case at all.
func TestPlannerToolCallsAreAlwaysAnswered(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	call := func() any {
		return &gollem.FunctionCall{ID: "c", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}}
	}
	planner := &toolCallingPlanner{replies: []any{
		// Four rounds spend the allowance; the fifth is past it and must STILL be
		// answered rather than dropped.
		call(), call(), call(), call(), call(),
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	// All five ran: none was dropped, so no call was left without a response.
	gt.Array(t, lookup.calls()).Length(5)

	// Each of the five answering turns reported its result and nothing else, which
	// is the only shape a provider accepts for them. (Call 0 is the opening
	// request; the calls after the fifth answer are the plan and what follows it.)
	answered := planner.answeredWith()
	seen := planner.seen()
	for i := 1; i <= 5; i++ {
		gt.Array(t, answered[i]).Length(1)
		gt.String(t, seen[i]).Equal("")
	}
}

// The allowance is enforced by telling the planner to stop, which is the only
// lever there is: agentkit's WithTools appends to the session's tools and nothing
// removes them, so the planner cannot be denied the tools it already has.
//
// The instruction rides in the SYSTEM prompt. It cannot ride in the user turn: the
// turn that would carry it is the one reporting the tool results, and a turn
// holding function responses may hold nothing else.
func TestTheToolAllowanceIsToldInTheSystemPrompt(t *testing.T) {
	within, err := planexec.PlannerSystemPromptForTest(planexec.PlannerToolRoundsMaxForTest - 1)
	gt.NoError(t, err).Required()
	gt.Bool(t, contains(within, "Do not call any more tools")).False()

	spent, err := planexec.PlannerSystemPromptForTest(planexec.PlannerToolRoundsMaxForTest)
	gt.NoError(t, err).Required()
	gt.String(t, spent).Contains("Do not call any more tools")
	// The host's own prompt survives alongside it.
	gt.String(t, spent).Contains("host prompt")
}

// TestTheAllowanceIsToldInTheSystemPrompt pins where the planner learns what it
// may divide, and that the guidance asking it to divide arrives with the figure.
//
// It is the SYSTEM prompt for the reason the tool-allowance notice is: a planning
// call following a tool round sends no user turn, so a figure the planner has to
// have cannot ride on one.
func TestTheAllowanceIsToldInTheSystemPrompt(t *testing.T) {
	line := planexec.BudgetPrefixForTest(pricing.FromUSD(0.217), pricing.FromUSD(2))
	gt.String(t, line).Equal("[budget] remaining $0.22 of $2.00")

	with, err := planexec.PlannerSystemPromptWithBudgetForTest(0, line)
	gt.NoError(t, err).Required()
	gt.String(t, with).Contains(line)
	// The instruction that makes the figure actionable is there too. A figure with
	// no instruction leaves the planner nothing to do with it; an instruction with
	// no figure asks it to divide an amount it was never told.
	gt.String(t, with).Contains("Every task you emit carries a `budget_usd`")
	gt.String(t, with).Contains("host prompt")

	// A host that wired no remaining figure gets neither.
	without, err := planexec.PlannerSystemPromptWithBudgetForTest(0, "")
	gt.NoError(t, err).Required()
	gt.Bool(t, contains(without, "[budget]")).False()
	gt.Bool(t, contains(without, "budget_usd")).False()
}

// runBudgetMeta is the run-level metadata these tests spawn with: a $2.00 budget,
// so the root's own figure is distinguishable from any share it hands a child.
var runBudgetMeta = map[string]string{"budget_nano_usd": "2000000000"}

// budgetRecorder reads the spend ceiling off every Process the runtime builds
// tools for — the only place a test can see what a spawn actually wrote to a
// child's metadata.
//
// It keys on the Process id because agentkit calls the ToolFactory once per
// transition, not once per Process: a plain slice would report the same run
// several times and say nothing about how many Processes there were.
type budgetRecorder struct {
	mu   sync.Mutex
	seen map[string]string
}

func newBudgetRecorder() *budgetRecorder {
	return &budgetRecorder{seen: map[string]string{}}
}

func (r *budgetRecorder) factory() agentkit.ToolFactory {
	return func(_ context.Context, proc *agentkit.Process) ([]gollem.Tool, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.seen[string(proc.ID)] = agentkernel.ScopeFrom(proc.Metadata).Budget.USD()
		return nil, nil
	}
}

// amounts returns one allowance per Process, sorted, so a test names the figures
// rather than their order.
func (r *budgetRecorder) amounts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.seen))
	for _, usd := range r.seen {
		out = append(out, usd)
	}
	sort.Strings(out)
	return out
}

// TestEachChildIsSpawnedWithItsOwnAllowance is what the whole per-task budget
// exists for: the amount the planner wrote for a task reaches that task's Process,
// so the limiter judges the child against its share rather than against the run's
// whole budget.
func TestEachChildIsSpawnedWithItsOwnAllowance(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[` +
			`{"id":"t1","title":"Heavy","description":"search everything","acceptance_criteria":"a","tools":["slack_ro"],"budget_usd":0.30},` +
			`{"id":"t2","title":"Light","description":"read one thread","acceptance_criteria":"b","tools":["slack_ro"],"budget_usd":0.05}]}`,
		`the heavy one`,
		`the light one`,
		`{"finalize":{"reason":"done"}}`,
		`the answer`,
	}}

	rec := newBudgetRecorder()
	remaining := func(map[string]string, agentkit.Metrics) (pricing.NanoUSD, pricing.NanoUSD) {
		return pricing.FromUSD(1), pricing.FromUSD(2)
	}
	rt := newRuntimeWithSpend(t, planner.client(), generousBudget(), testSpend(), nil, rec.factory(),
		planexec.Config[planexec.TextResult]{TextOnly: true, Remaining: remaining})

	proc := rt.run(t, textInput(), runBudgetMeta)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// Three Processes: the root on the run's own $2.00, and its two children on the
	// unequal shares the planner wrote — not on the $1.00 that was left, and not on
	// half of it each.
	gt.Array(t, rec.amounts()).Equal([]string{"$0.05", "$0.30", "$2.00"})
}

// TestAChildInheritsTheRunsBudgetWithoutARemainingFigure pins the other side: a
// host that wired no Config.Remaining leaves the child's metadata as it was, so
// the child is judged against the run's own figure on its own metrics. That is
// weaker than a share of what is left, and it is what every host had before the
// planner was asked to allocate.
func TestAChildInheritsTheRunsBudgetWithoutARemainingFigure(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"a","tools":["slack_ro"]}]}`,
		`read`,
		`{"finalize":{"reason":"done"}}`,
		`the answer`,
	}}

	rec := newBudgetRecorder()
	rt := newRuntimeWithSpend(t, planner.client(), generousBudget(), testSpend(), nil, rec.factory(),
		planexec.Config[planexec.TextResult]{TextOnly: true})

	proc := rt.run(t, textInput(), runBudgetMeta)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// Both the root and its child report the run's own $2.00.
	gt.Array(t, rec.amounts()).Equal([]string{"$2.00", "$2.00"})
}

// TestTheDirectChildGetsWhatIsLeft pins the one child nobody divides for: the
// direct path spawns a single agent whose text IS the reply, so there is nothing
// to split and no planner decision to respect.
func TestTheDirectChildGetsWhatIsLeft(t *testing.T) {
	planner := &scriptedPlanner{replies: []string{
		`{"direct":{"tools":["slack_ro"]}}`,
		`answered directly`,
	}}

	rec := newBudgetRecorder()
	remaining := func(map[string]string, agentkit.Metrics) (pricing.NanoUSD, pricing.NanoUSD) {
		return pricing.FromUSD(0.75), pricing.FromUSD(2)
	}
	in := textInput()
	in.AllowDirect = true
	rt := newRuntimeWithSpend(t, planner.client(), generousBudget(), testSpend(), nil, rec.factory(),
		planexec.Config[planexec.TextResult]{TextOnly: true, Remaining: remaining})

	proc := rt.run(t, in, runBudgetMeta)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputDirect)

	// The root's own $2.00, and the direct child's whole remaining $0.75.
	gt.Array(t, rec.amounts()).Equal([]string{"$0.75", "$2.00"})
}

// A planner turn asking for several tools at once is answered by ONE call
// carrying every result. Reporting them one at a time is what a provider rejects
// ("the number of function response parts is equal to the number of function call
// parts of the function call turn"), and once the conversation holds that split it
// stays broken for the rest of the run.
func TestParallelPlannerToolCallsAreAnsweredInOneTurn(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		[]*gollem.FunctionCall{
			{ID: "c1", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}},
			{ID: "c2", Name: "get_workspace", Arguments: map[string]any{"id": "ws-2"}},
			{ID: "c3", Name: "get_workspace", Arguments: map[string]any{"id": "ws-3"}},
		},
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	// Each call ran once, as its own transition.
	gt.Array(t, lookup.calls()).Length(3)

	// The planning call that follows carries all three, in the order asked, and
	// no other call carries any.
	answered := planner.answeredWith()
	gt.Array(t, answered).Length(5).Required()
	gt.Array(t, answered[0]).Length(0)
	gt.Value(t, answered[1]).Equal([]string{"c1", "c2", "c3"})
	for i := 2; i < len(answered); i++ {
		gt.Array(t, answered[i]).Length(0)
	}
}

// A planning call that FAILS after a tool round is retried from the checkpoint,
// and the retry must send the same shape: no user turn, with the results still
// answering the model's calls exactly once.
//
// This pins where ToolsAnswered is cleared. Clearing it before the call, or on
// the error path, would make the retry send a text turn while the conversation
// already answers the model — and the results are in the conversation whatever
// the retry does, so sending them "again" is not an option either.
func TestAnsweredToolCallsSurviveAFailedPlanningCall(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		[]*gollem.FunctionCall{
			{ID: "c1", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}},
			{ID: "c2", Name: "get_workspace", Arguments: map[string]any{"id": "ws-2"}},
		},
		// The planning call that reports them fails; agentkit retries the whole
		// transition from the last checkpoint, which is the one the tool phase left.
		goerr.New("the provider is briefly unavailable"),
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	// The tools ran once, in the transitions that committed before the failure.
	gt.Array(t, lookup.calls()).Length(2)

	answered := planner.answeredWith()
	seen := planner.seen()
	gt.Number(t, len(answered)).GreaterOrEqual(3).Required()
	// The failed call and the retry both continue from the same answered round.
	gt.Value(t, answered[1]).Equal([]string{"c1", "c2"})
	gt.Value(t, answered[2]).Equal([]string{"c1", "c2"})
	gt.String(t, seen[1]).Equal("")
	gt.String(t, seen[2]).Equal("")
	// Once that call succeeds the flag is gone: the next planning call sends a turn
	// again rather than an empty request.
	gt.Array(t, answered[3]).Length(0)
	gt.String(t, seen[3]).NotEqual("")
}

// A single value the planner sent for an array-typed argument reaches the tool as
// the batch of one it meant. gollem refuses the whole call otherwise, before the
// tool runs, and a model answering that refusal re-emits the same call.
func TestASingleValueForAnArrayArgumentStillReachesTheTool(t *testing.T) {
	lookup := &recordingTool{
		name: "get_workspace",
		params: map[string]*gollem.Parameter{
			"ids": {Type: gollem.TypeArray, Items: &gollem.Parameter{Type: gollem.TypeString}},
		},
	}
	planner := &toolCallingPlanner{replies: []any{
		[]*gollem.FunctionCall{{ID: "c1", Name: "get_workspace", Arguments: map[string]any{"ids": "ws-1"}}},
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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

	got := lookup.calls()
	gt.Array(t, got).Length(1).Required()
	gt.Value(t, got[0]["ids"]).Equal([]any{"ws-1"})
}

// A planning phase that asked for tools can reach the terminal output without
// another planning call: the budget notice routes plan / replan straight to final.
// The terminal call therefore has to carry those results too — leaving the model's
// function calls unanswered is what a provider rejects, and it would reject this
// request and every one after it.
func TestPlannerToolResultsReachTheTerminalCall(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		// Round 1: look something up instead of planning.
		[]*gollem.FunctionCall{
			{ID: "c1", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}},
			{ID: "c2", Name: "get_workspace", Arguments: map[string]any{"id": "ws-2"}},
		},
		// The planning call that follows never happens: the notice fires first and
		// the run goes to produce its answer. This reply is the terminal one.
		`the partial answer`,
	}}
	// The notice fires almost immediately, so the run wraps up right after the
	// tool phase drains.
	cfg := budget.Config{MaxSteps: 8, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.1}
	rt := newTextRuntime(t, planner.client(), cfg, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{lookup}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.String(t, out.Text).Contains("partial answer")

	gt.Array(t, lookup.calls()).Length(2)

	// Both results reached the terminal call, in the order asked.
	answered := planner.answeredWith()
	gt.Array(t, answered).Length(2).Required()
	gt.Array(t, answered[0]).Length(0)
	gt.Value(t, answered[1]).Equal([]string{"c1", "c2"})

	// This is the ONLY terminal call the run makes, so the final prompt has to
	// reach it — in the SYSTEM prompt, since the call sends no user turn. Without
	// it the model is never told to write the user-visible answer, nor given the
	// observation trail to write it from, and the run answers whatever the planner
	// conversation happened to leave it on.
	gt.String(t, planner.seen()[1]).Equal("")
	gt.String(t, planner.systemSeen()[1]).Contains("Produce the final response for the user")
	gt.String(t, planner.systemSeen()[1]).Contains("no investigations were run before the loop exited")
}

// The terminal call may itself ask for a tool: agentkit declares the claim's tools
// on every call, so a model that spent the run calling tools can answer this one
// with another call. The run must service it and come back for the answer — reading
// a function-call reply as "no answer" ended the turn in a fallback that had
// nothing to say, with the investigation already paid for.
func TestTerminalCallMayAskForATool(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		// The terminal call asks for a lookup instead of writing the answer.
		&gollem.FunctionCall{ID: "f1", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}},
		// With the result in hand it writes it.
		`The workspace has a severity field.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{lookup}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("severity field")

	gt.Array(t, lookup.calls()).Length(1)
	// The result reached the call that then produced the answer.
	answered := planner.answeredWith()
	gt.Array(t, answered).Length(5).Required()
	gt.Value(t, answered[4]).Equal([]string{"f1"})
}

// A terminal call that keeps asking for tools has to be told to stop, or the run
// spends its whole step budget looking things up and ends with a fallback instead
// of the answer it had already paid for. The instruction is the only lever:
// agentkit's WithTools appends and nothing removes them.
func TestTerminalCallIsToldWhenItsToolAllowanceIsSpent(t *testing.T) {
	lookup := &recordingTool{name: "get_workspace"}
	call := func() any {
		return &gollem.FunctionCall{ID: "f", Name: "get_workspace", Arguments: map[string]any{"id": "ws-1"}}
	}
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		`read it`,
		`{"finalize":{"reason":"done"}}`,
		// Four terminal calls that each ask for a lookup instead of answering.
		call(), call(), call(), call(),
		`Answered at last.`,
	}}
	rt := newTextRuntime(t, planner.client(), generousBudget(), nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{lookup}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out := decodeText(t, proc.Output)
	gt.Value(t, out.Kind).Equal(planexec.OutputFinal)
	gt.String(t, out.Text).Contains("Answered at last")

	// Every call was answered, and each answering call sent no user turn of its own.
	gt.Array(t, lookup.calls()).Length(4)
	answered := planner.answeredWith()
	gt.Array(t, answered).Length(8).Required()
	for i := 4; i < 8; i++ {
		gt.Array(t, answered[i]).Length(1)
		gt.String(t, planner.seen()[i]).Equal("")
	}

	// The instruction therefore has to arrive in the system prompt, and it arrives
	// exactly once — the same notice in the user turn as well would say it twice on
	// every call that does send one.
	spent := planner.systemSeen()[7]
	gt.String(t, spent).Contains("Do not call any more tools")
	gt.Number(t, strings.Count(spent, "Do not call any more tools")).Equal(1)
	// The first terminal call is still within the allowance, so it is not nagged.
	gt.Bool(t, contains(planner.systemSeen()[3], "Do not call any more tools")).False()
}

// A SUB-AGENT'S SPEND IS CHARGED TO ITS PARENT, and lands in one jump at the
// child's terminal commit.
//
// agentkit folds a finished child's whole Metrics into its parent
// (worker.go reportToParent: `pClone.Metrics = pClone.Metrics.add(child.Metrics)`),
// so a planexec run's MaxSteps bounds the WHOLE SUBTREE, not the planner's own
// transitions — and the parent's counter can cross the ceiling by a child's worth
// of steps in a single fold, which the per-transition Limit check cannot catch
// beforehand.
//
// This test pins that, because the two numbers a host configures (root vs task)
// only make sense once it is known: a root ceiling must cover every child it can
// spawn, or the run dies of its children's spend with the planner barely started.
func TestAChildsStepsAreChargedToItsParent(t *testing.T) {
	tool := &recordingTool{name: "get_workspace"}
	// The child is asked to call the tool four times before answering, so its own
	// spend is unmistakably larger than the planner's handful of transitions.
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		&gollem.FunctionCall{ID: "a1", Name: "get_workspace", Arguments: map[string]any{"id": "1"}},
		&gollem.FunctionCall{ID: "a2", Name: "get_workspace", Arguments: map[string]any{"id": "2"}},
		&gollem.FunctionCall{ID: "a3", Name: "get_workspace", Arguments: map[string]any{"id": "3"}},
		&gollem.FunctionCall{ID: "a4", Name: "get_workspace", Arguments: map[string]any{"id": "4"}},
		`the child is done`,
		`{"finalize":{"reason":"done"}}`,
		`the answer`,
	}}
	cfg := budget.Config{MaxSteps: 1000, MaxInputTokens: 10_000_000, MaxOutputTokens: 10_000_000, NoticeRatio: 0.99}
	rt := newTextRuntime(t, planner.client(), cfg, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{tool}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	gt.Array(t, tool.calls()).Length(4)

	// The planner itself ran: plan, collect, replan, final — a single-digit number
	// of transitions. Its recorded Steps is far larger, because the child's are in
	// it: 4 tool calls plus the LLM calls around them.
	gt.Number(t, proc.Metrics.Steps).GreaterOrEqual(13)
	// The same fold applies to LLM calls: the planner made 3 (plan, replan, final)
	// and the child 5.
	gt.Number(t, proc.Metrics.LLMCalls).Equal(8)
	gt.Number(t, proc.Metrics.ToolCalls).Equal(4)
}

// A parent can therefore be killed by its children's spend while it has barely
// run: the fold lands after the child finishes, so the ceiling is crossed between
// two of the parent's own transitions and the next Limit check stops the run.
func TestAParentIsStoppedByItsChildrensSpend(t *testing.T) {
	tool := &recordingTool{name: "get_workspace"}
	planner := &toolCallingPlanner{replies: []any{
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
		&gollem.FunctionCall{ID: "a1", Name: "get_workspace", Arguments: map[string]any{"id": "1"}},
		&gollem.FunctionCall{ID: "a2", Name: "get_workspace", Arguments: map[string]any{"id": "2"}},
		&gollem.FunctionCall{ID: "a3", Name: "get_workspace", Arguments: map[string]any{"id": "3"}},
		&gollem.FunctionCall{ID: "a4", Name: "get_workspace", Arguments: map[string]any{"id": "4"}},
		`the child is done`,
		// Never reached: the fold stops the parent before it can replan.
		`{"finalize":{"reason":"done"}}`,
		`the answer`,
	}}
	// Ten steps is more than the planner needs to plan and collect, and less than
	// the child spends.
	cfg := budget.Config{MaxSteps: 10, MaxInputTokens: 10_000_000, MaxOutputTokens: 10_000_000, NoticeRatio: 0.99}
	rt := newTextRuntime(t, planner.client(), cfg, nil,
		func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{tool}, nil
		})

	proc := rt.run(t, textInput(), nil)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
	gt.Value(t, proc.Failure).NotNil().Required()
	gt.String(t, proc.Failure.Message).Contains("step budget exhausted")
	// Past the ceiling, not at it: the child's spend arrived in one fold.
	gt.Number(t, proc.Metrics.Steps).GreaterOrEqual(int64(cfg.MaxSteps))
}

// A planning call sends exactly one user turn, except the one following the tool
// phase, which sends none.
//
// Both halves were learnt from live rejections, and they pull in opposite
// directions. An empty input adds no user content, so a call that sends nothing
// with no answered calls behind it ENDS on the previous model turn. A call that
// does have them is the answer to the model's tool call, and text sent with it
// stops the provider recognising it as that answer. Both surface as "Requests
// ending with a model turn are not supported".
func TestAPlanningCallSendsOneWellFormedTurn(t *testing.T) {
	// Nothing to report and nothing to say: still one turn.
	gt.Number(t, planexec.PlannerInputsForTest("", false)).Equal(1)
	// The answered calls are the turn; this call adds nothing to them.
	gt.Number(t, planexec.PlannerInputsForTest("", true)).Equal(0)
	// Text is the turn when no answered calls are waiting.
	gt.Number(t, planexec.PlannerInputsForTest("here are the observations", false)).Equal(1)
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
		`{"tasks":[{"id":"t1","title":"Read","description":"read it","acceptance_criteria":"done","tools":["slack_ro"],"budget_usd":0.01}]}`,
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
	taskAgent, err := react.Register(reg, agentkernel.AgentTask, 1, cfg.Limiter(testSpend()))
	gt.NoError(t, err).Required()

	_, err = planexec.Register(reg, agentkernel.AgentProposal, 1, taskAgent, nil, nil,
		planexec.Config[planexec.TextResult]{TextOnly: true})
	gt.Error(t, err).Is(agentkit.ErrInvalidAgentDef)

	_, err = planexec.Register(reg, agentkernel.AgentWorkspace, 1,
		agentkit.Agent[react.Input]{}, nil, cfg.Limiter(testSpend()),
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
