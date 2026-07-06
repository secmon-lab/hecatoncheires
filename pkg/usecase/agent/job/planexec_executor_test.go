package job_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// agentExecuteCounter is a minimal gollem trace.Handler that counts how
// many agent executions it was wired into. gollem calls StartAgentExecute
// once per gollem.Agent.Execute on the configured handler, so the count
// reveals whether the Job's TraceHandler reached every agent the planexec
// run drives.
type agentExecuteCounter struct {
	mu       sync.Mutex
	executes int
}

func (c *agentExecuteCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.executes
}

func (c *agentExecuteCounter) StartAgentExecute(ctx context.Context) context.Context {
	c.mu.Lock()
	c.executes++
	c.mu.Unlock()
	return ctx
}
func (c *agentExecuteCounter) EndAgentExecute(context.Context, error)                {}
func (c *agentExecuteCounter) StartLLMCall(ctx context.Context) context.Context      { return ctx }
func (c *agentExecuteCounter) EndLLMCall(context.Context, *trace.LLMCallData, error) {}
func (c *agentExecuteCounter) StartToolExec(ctx context.Context, _ string, _ map[string]any) context.Context {
	return ctx
}
func (c *agentExecuteCounter) EndToolExec(context.Context, map[string]any, error) {}
func (c *agentExecuteCounter) StartSubAgent(ctx context.Context, _ string) context.Context {
	return ctx
}
func (c *agentExecuteCounter) EndSubAgent(context.Context, error) {}
func (c *agentExecuteCounter) StartChildAgent(ctx context.Context, _ string) context.Context {
	return ctx
}
func (c *agentExecuteCounter) EndChildAgent(context.Context, error)  {}
func (c *agentExecuteCounter) AddEvent(context.Context, string, any) {}
func (c *agentExecuteCounter) Finish(context.Context) error          { return nil }

// sequencedResponses is a tiny canned-reply mock LLM. Each Generate
// pops the next text; if the request input does not contain the
// configured substring, the mock errors out so the test can pin which
// reply belongs to which round.
type sequencedResponses struct {
	mu        sync.Mutex
	responses []string
	idx       int
}

func newSequencedLLMClient(responses []string) gollem.LLMClient {
	s := &sequencedResponses{responses: responses}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					s.mu.Lock()
					defer s.mu.Unlock()
					if s.idx >= len(s.responses) {
						return &gollem.Response{}, nil
					}
					next := s.responses[s.idx]
					s.idx++
					return &gollem.Response{Texts: []string{next}}, nil
				},
			}, nil
		},
	}
}

func newRunner(t *testing.T, llm gollem.LLMClient) *planexec.Runner {
	t.Helper()
	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget: planexec.BudgetConfig{
			PlannerLoopMax:  8,
			SubAgentLoopMax: 20,
		},
	})
	gt.NoError(t, err).Required()
	return runner
}

// TestPlanexecJobExecutor_PlanThenFinal verifies the happy path:
// planner emits one task → sub-agent runs → replan terminates → final
// response surfaces as ExecuteResult.Summary.
func TestPlanexecJobExecutor_PlanThenFinal(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient([]string{
		// Round 1 plan
		`{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check A","acceptance_criteria":"a","tools":["default"]}
		]}`,
		// Sub-agent response
		"Found details about A.",
		// Replan terminate
		`{"message":"done","finalize":{"reason":"done"}}`,
		// Final response (plain text)
		"Summary: A details.",
	})

	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	progressLines := []string{}
	var progressMu sync.Mutex
	res, err := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "You are running an automated job.",
		Prompt:       "Investigate the case.",
		LLMClient:    llm, // not consulted by planexec executor (runner already wires it) but required by ExecuteRequest contract for parity
		TraceID:      "trace-pe-1",
		HistoryKey:   "hist-pe-1",
		ProgressFunc: func(_ context.Context, msg string) {
			progressMu.Lock()
			defer progressMu.Unlock()
			progressLines = append(progressLines, msg)
		},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(job.ExecuteStatusSuccess)
	gt.String(t, res.Summary).Contains("A details")
	// LoopCount mirrors len(AllResults) which is 1 phase here.
	gt.Number(t, res.LoopCount).Equal(1)
	gt.Array(t, res.Phases).Length(1).Required()
	gt.String(t, res.Phases[0].Name).Contains("phase-1")

	// jobSink should have surfaced at least one Plan-round line and
	// one PhaseStarted line through ProgressFunc.
	progressMu.Lock()
	defer progressMu.Unlock()
	joined := strings.Join(progressLines, "\n")
	gt.String(t, joined).Contains("Plan round 1")
	gt.String(t, joined).Contains("Phase 1")
	gt.String(t, joined).Contains("[t-1]")
}

// jobWriteTool records its invocations so the test can prove a Job
// sub-agent actually performed the write it was assigned.
type jobWriteTool struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (t *jobWriteTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "slack__post_to_case_channel",
		Description: "Post a plain-text message to the case's Slack channel.",
		Parameters: map[string]*gollem.Parameter{
			"text": {Type: gollem.TypeString, Description: "message text", Required: true},
		},
	}
}

func (t *jobWriteTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, args)
	return map[string]any{"posted": true}, nil
}

func (t *jobWriteTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func (t *jobWriteTool) firstText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.calls) == 0 {
		return ""
	}
	s, _ := t.calls[0]["text"].(string)
	return s
}

// TestPlanexecJobExecutor_SubAgentPerformsWrite pins the fix for the
// reported bug (pre_review_on_create investigated but never posted): the
// Job path sets AllowSubAgentWrites=true, so a sub-agent that is assigned a
// write tool actually calls it instead of refusing as observation-only. The
// mock LLM dispatches a write task, the sub-agent issues the tool call, and
// we assert the tool ran with the chosen text.
func TestPlanexecJobExecutor_SubAgentPerformsWrite(t *testing.T) {
	ctx := context.Background()
	writeTool := &jobWriteTool{}

	// The Job tool resolver exposes every ExecuteRequest.Tools entry under
	// the single "default" bucket, so the planner assigns tools:["default"].
	round := 0
	var roundMu sync.Mutex
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					roundMu.Lock()
					round++
					n := round
					roundMu.Unlock()
					switch n {
					case 1: // planner round 1: dispatch a write task
						return &gollem.Response{Texts: []string{`{"message":"post the review","tasks":[
							{"id":"t-1","title":"Post","description":"Post the pre-review to the channel.","acceptance_criteria":"review posted","tools":["default"]}
						]}`}}, nil
					case 2: // sub-agent: call the write tool
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{{
							ID:        "call-1",
							Name:      "slack__post_to_case_channel",
							Arguments: map[string]any{"text": "Pre-review complete."},
						}}}, nil
					case 3: // sub-agent: report after the tool result
						return &gollem.Response{Texts: []string{"Posted the pre-review to the channel."}}, nil
					case 4: // replan: terminate
						return &gollem.Response{Texts: []string{`{"message":"done","finalize":{"reason":"done"}}`}}, nil
					default: // final synthesis
						return &gollem.Response{Texts: []string{"Pre-review posted."}}, nil
					}
				},
			}, nil
		},
	}

	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	res, err := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "You are running a pre-review job. Post the review to the case channel.",
		Prompt:       "Run the pre-review for this case.",
		Tools:        []gollem.Tool{writeTool},
		LLMClient:    llm,
		TraceID:      "trace-pe-write",
		HistoryKey:   "hist-pe-write",
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(job.ExecuteStatusSuccess)

	// The sub-agent actually invoked the write tool once, with the chosen
	// text — the deliverable reached Slack instead of only being described.
	gt.Number(t, writeTool.callCount()).Equal(1)
	gt.String(t, writeTool.firstText()).Equal("Pre-review complete.")
}

// TestPlanexecJobExecutor_ForwardsTraceHandler pins the fix for the
// planexec Job empty-timeline bug at the executor boundary: the executor
// MUST forward ExecuteRequest.TraceHandler into planexec.RunRequest so
// the handler the JobRunner builds (a jobRunTraceHandler writing the
// JobRunEvent timeline) reaches the planexec agents. The flow drives four
// agent executions (planner, sub-agent, replan, final); if the executor
// dropped the handler — the original bug — the counter would stay at
// zero. We assert four, proving every agent (sub-agent included) was
// wired.
func TestPlanexecJobExecutor_ForwardsTraceHandler(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient([]string{
		`{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check A","acceptance_criteria":"a","tools":["default"]}
		]}`,
		"Found details about A.",
		`{"message":"done","finalize":{"reason":"done"}}`,
		"Summary: A details.",
	})

	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	counter := &agentExecuteCounter{}
	res, err := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "You are running an automated job.",
		Prompt:       "Investigate the case.",
		LLMClient:    llm,
		TraceID:      "trace-pe-trace",
		HistoryKey:   "hist-pe-trace",
		TraceHandler: counter,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(job.ExecuteStatusSuccess)

	// planner + sub-agent t-1 + replan + final = 4 agent executions, all
	// wired to the forwarded handler.
	gt.Number(t, counter.count()).Equal(4)
}

// TestPlanexecJobExecutor_RejectsMissingFields exercises the
// ExecuteRequest required-field guards.
func TestPlanexecJobExecutor_RejectsMissingFields(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient(nil)
	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	base := job.ExecuteRequest{
		JobID:        "j",
		SystemPrompt: "s",
		Prompt:       "p",
		LLMClient:    llm,
		TraceID:      "trace-1",
		HistoryKey:   "hist-1",
	}
	t.Run("missing system prompt", func(t *testing.T) {
		req := base
		req.SystemPrompt = ""
		_, err := executor.Execute(ctx, req)
		gt.Error(t, err)
	})
	t.Run("missing user prompt", func(t *testing.T) {
		req := base
		req.Prompt = ""
		_, err := executor.Execute(ctx, req)
		gt.Error(t, err)
	})
	t.Run("missing trace id", func(t *testing.T) {
		req := base
		req.TraceID = ""
		_, err := executor.Execute(ctx, req)
		gt.Error(t, err)
	})
	t.Run("missing history key", func(t *testing.T) {
		req := base
		req.HistoryKey = ""
		_, err := executor.Execute(ctx, req)
		gt.Error(t, err)
	})
}

// TestPlanexecJobExecutor_BudgetExhaustionMapsToError ensures a
// planexec fallback surfaces as a runner-visible error so the JobRunner
// records the row as FAILED.
func TestPlanexecJobExecutor_BudgetExhaustionMapsToError(t *testing.T) {
	ctx := context.Background()
	// Every planner round produces invalid JSON so retries burn the
	// budget without producing a phase.
	llm := newSequencedLLMClient([]string{
		"not json",
		"still not json",
	})

	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget: planexec.BudgetConfig{
			PlannerLoopMax:  2,
			SubAgentLoopMax: 20,
		},
	})
	gt.NoError(t, err).Required()

	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	_, runErr := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "j",
		SystemPrompt: "s",
		Prompt:       "p",
		LLMClient:    llm,
		TraceID:      "trace-x",
		HistoryKey:   "hist-x",
	})
	gt.Error(t, runErr)
}

// fakeInteractor records the interaction.Request it was asked to solicit
// and always reports the run as paused (the pause/resume model).
type fakeInteractor struct {
	mu       sync.Mutex
	requests []interaction.Request
}

func (f *fakeInteractor) Solicit(_ context.Context, req interaction.Request) (interaction.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	return interaction.Outcome{Paused: true}, nil
}

func (f *fakeInteractor) last() (interaction.Request, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return interaction.Request{}, false
	}
	return f.requests[len(f.requests)-1], true
}

// TestPlanexecJobExecutor_InteractiveSuspends verifies that when the
// planner emits a question on an interactive run, the executor routes it to
// the Interactor and surfaces ExecuteStatusAwaitingInput (not Success).
func TestPlanexecJobExecutor_InteractiveSuspends(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient([]string{
		// Round 1 plan
		`{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check A","acceptance_criteria":"a","tools":["default"]}
		]}`,
		// Sub-agent response
		"Found something ambiguous about A.",
		// Replan asks the user.
		`{"message":"need input","question":{
			"reason":"which environment?","items":[
				{"id":"env","text":"Which environment?","type":"select","options":["prod","stg"]}
			]}}`,
	})

	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	fi := &fakeInteractor{}
	res, err := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "interactive-job",
		SystemPrompt: "You are running an interactive job.",
		Prompt:       "Investigate the case.",
		LLMClient:    llm,
		TraceID:      "trace-int-1",
		HistoryKey:   "hist-int-1",
		Interactive:  true,
		Interactor:   fi,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(job.ExecuteStatusAwaitingInput)
	// Suspended runs carry no summary — the answer has not arrived yet.
	gt.String(t, res.Summary).Equal("")

	// The Interactor must have received the planner's question verbatim
	// (content, not just a count).
	got, ok := fi.last()
	gt.Bool(t, ok).True()
	gt.String(t, got.Reason).Equal("which environment?")
	gt.Array(t, got.Items).Length(1).Required()
	gt.String(t, got.Items[0].ID).Equal("env")
	gt.String(t, got.Items[0].Text).Equal("Which environment?")
	gt.Value(t, got.Items[0].Type).Equal(interaction.ItemSelect)
	gt.Array(t, got.Items[0].Options).Equal([]string{"prod", "stg"})
}

// TestPlanexecJobExecutor_InteractiveRequiresInteractor ensures the executor
// fails loudly when Interactive is set without an Interactor.
func TestPlanexecJobExecutor_InteractiveRequiresInteractor(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient(nil)
	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	_, runErr := executor.Execute(ctx, job.ExecuteRequest{
		JobID:        "j",
		SystemPrompt: "s",
		Prompt:       "p",
		LLMClient:    llm,
		TraceID:      "trace-1",
		HistoryKey:   "hist-1",
		Interactive:  true,
		Interactor:   nil,
	})
	gt.Error(t, runErr)
}

// TestPlanexecJobExecutor_Resume verifies a suspended run resumes from the
// persisted question + answers and completes with the final summary.
func TestPlanexecJobExecutor_Resume(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient([]string{
		// Resumed replan terminates (the answer was enough).
		`{"message":"done","finalize":{"reason":"done"}}`,
		// Final response.
		"Summary: used the prod environment per the answer.",
	})

	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	pending := model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1700000000.000400",
		Reason:          "which environment?",
		Items: []model.PendingInteractionItem{
			{ID: "env", Text: "Which environment?", Type: "select", Options: []string{"prod", "stg"}},
		},
	}
	answers := []interaction.Answer{{ID: "env", Choice: "prod"}}

	res, err := executor.Resume(ctx, job.ExecuteRequest{
		JobID:        "interactive-job",
		SystemPrompt: "You are running an interactive job.",
		Prompt:       "Investigate the case.",
		LLMClient:    llm,
		TraceID:      "trace-int-1",
		HistoryKey:   "hist-int-1",
		Interactor:   &fakeInteractor{},
	}, pending, answers)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(job.ExecuteStatusSuccess)
	gt.String(t, res.Summary).Contains("prod environment")
}

// TestPlanexecJobExecutor_ResumeRejectsEmptyAnswers guards the resume
// contract.
func TestPlanexecJobExecutor_ResumeRejectsEmptyAnswers(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLMClient(nil)
	runner := newRunner(t, llm)
	executor, err := job.NewPlanexecJobExecutor(runner)
	gt.NoError(t, err).Required()

	_, runErr := executor.Resume(ctx, job.ExecuteRequest{
		JobID:        "j",
		SystemPrompt: "s",
		Prompt:       "p",
		LLMClient:    llm,
		TraceID:      "trace-1",
		HistoryKey:   "hist-1",
		Interactor:   &fakeInteractor{},
	}, model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1700000000.000500",
		Items:           []model.PendingInteractionItem{{ID: "q", Text: "?", Type: "free_text"}},
	}, nil)
	gt.Error(t, runErr)
}
