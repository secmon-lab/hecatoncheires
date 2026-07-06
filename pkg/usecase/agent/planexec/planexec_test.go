package planexec_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/llm/claude"
	"github.com/gollem-dev/gollem/llm/gemini"
	"github.com/gollem-dev/gollem/llm/openai"
	"github.com/gollem-dev/gollem/mock"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// --- Sequential LLM mock --------------------------------------------

// sequencedLLM returns canned responses in order across all sessions /
// Generate calls. Each NewSession returns a SessionMock whose Generate
// pops the next item from the shared slice. If the slice is exhausted
// or a request's input text does not match the expected condition, the
// mock returns an error.
type sequencedLLM struct {
	mu        sync.Mutex
	responses []sequencedResponse
	idx       int
	inputs    []string
}

type sequencedResponse struct {
	text string
	// If matchSubstr is non-empty, the Generate input MUST contain it
	// or the mock returns an error. Lets the test pin "which round
	// gets which canned reply".
	matchSubstr string
}

func newSequencedLLM(responses []sequencedResponse) *sequencedLLM {
	return &sequencedLLM{responses: responses}
}

func (l *sequencedLLM) Client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					l.mu.Lock()
					defer l.mu.Unlock()
					if l.idx >= len(l.responses) {
						return nil, errExhausted
					}
					var sb strings.Builder
					for _, in := range input {
						if txt, ok := in.(gollem.Text); ok {
							sb.WriteString(string(txt))
							sb.WriteString("\n")
						}
					}
					inputText := sb.String()
					l.inputs = append(l.inputs, inputText)
					next := l.responses[l.idx]
					l.idx++
					if next.matchSubstr != "" && !strings.Contains(inputText, next.matchSubstr) {
						return nil, errInputMismatch{want: next.matchSubstr, got: inputText}
					}
					return &gollem.Response{Texts: []string{next.text}}, nil
				},
			}, nil
		},
	}
}

func (l *sequencedLLM) calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.idx
}

// inputAt returns the concatenated user-text input the mock received on its
// i-th Generate call (0-based), or "" if that call did not happen. Lets a
// test assert *what* each LLM call was actually asked, not merely how many
// calls fired.
func (l *sequencedLLM) inputAt(i int) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i < 0 || i >= len(l.inputs) {
		return ""
	}
	return l.inputs[i]
}

type exhaustedError struct{}

func (exhaustedError) Error() string { return "sequencedLLM exhausted" }

var errExhausted = exhaustedError{}

type errInputMismatch struct{ want, got string }

func (e errInputMismatch) Error() string {
	return "sequencedLLM input does not contain expected substring: want=" + e.want
}

// --- Common test fixtures -------------------------------------------

var knownToolIDs = []string{"core_ro", "slack_ro", "notion", "github"}

type stubResolver struct{}

func (stubResolver) Resolve(_ []string) []gollem.Tool { return nil }

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

func baseRequest() planexec.RunRequest {
	ts := time.Now().Format("150405.000000000")
	return planexec.RunRequest{
		HistoryKey:   "ssn-" + ts,
		TraceID:      "trace-" + ts,
		UserInput:    "Please investigate the case.",
		SystemPrompt: "You are a test planner.",
		ToolResolver: stubResolver{},
		KnownToolIDs: knownToolIDs,
		Sink:         planexec.SinkFuncs{},
		// agentarchive.MemoryTraceRepository requires a session_id label
		// to persist the trace. Real hosts set this through the trace
		// metadata they build per-turn.
		TraceMetadata: trace.TraceMetadata{
			Labels: map[string]string{
				"session_id": "ssn-" + ts,
			},
		},
	}
}

// finalPayload is a Validatable structured final-output type for Run[T] tests.
type finalPayload struct {
	WorkspaceID string `json:"workspace_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (p finalPayload) Validate() error {
	if p.Title == "" {
		return goerr.New("title is required")
	}
	return nil
}

// --- Integration: plan → phase → replan(empty) → final --------------

func TestRunner_Run_PlanThenTerminate(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1: planner emits one task.
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check thread A","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		// Sub-agent t-1 response (plain text).
		{text: "Found A details."},
		// Round 2: replan, terminate via explicit finalize.
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final-response LLM call (plain text).
		{text: "Here's what I found: A details."},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.Text).Contains("A details")
	gt.Bool(t, res.EndedWithQuestion).False()
	gt.Array(t, res.AllResults).Length(1).Required()
	gt.Number(t, len(res.AllResults[0].Tasks)).Equal(1)

	async.Wait()
	// 4 LLM calls in total: 1 planner + 1 sub-agent + 1 replan + 1 final.
	gt.Number(t, llm.calls()).Equal(4)
}

// --- Integration: host TraceHandler receives events from every agent -

// TestRunner_Run_ForwardsTraceHandlerToAllAgents pins the fix for the
// planexec Job empty-timeline bug: a host-supplied TraceHandler must be
// wired into the planner, every sub-agent, AND the final-response agent,
// not only the run's internal archive recorder. The flow drives four
// gollem agent executions (planner round 1, sub-agent t-1, replan round
// 2, final). gollem invokes StartAgentExecute / Finish on the configured
// handler once per execution, so the host handler MUST observe four of
// each. Before the fix the sub-agent execution was wired only to its
// private loop counter, so the handler would observe three — the missing
// fourth is exactly the investigation work that vanished from the Job
// timeline. (Per-call LLM hooks are asserted in the trace_tee unit test
// via direct invocation; a mock LLM client never fires StartLLMCall, so
// agent-execution count is the observable wiring signal here.)
func TestRunner_Run_ForwardsTraceHandlerToAllAgents(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check thread A","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "Found A details."},
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		{text: "Here's what I found: A details."},
	})

	rec := &recordingTraceHandler{name: "host"}
	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.TraceHandler = rec

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)

	async.Wait()

	// Four agent executions fired; the host handler must have seen every
	// one — including the sub-agent's, which is the path the bug dropped.
	gt.Number(t, llm.calls()).Equal(4)
	gt.Number(t, rec.snapshotAgentExecutes()).Equal(4)
	gt.Number(t, rec.snapshotFinishes()).Equal(4)
}

// TestRunner_Run_NilTraceHandlerKeepsArchiveOnly proves the proposal
// host path is unchanged: with no host handler, the run still completes
// and the (separate) archive recorder remains the only trace sink. The
// run must not panic or change behaviour when TraceHandler is nil.
func TestRunner_Run_NilTraceHandlerKeepsArchiveOnly(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check thread A","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "Found A details."},
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		{text: "Here's what I found: A details."},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.TraceHandler = nil

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.Text).Contains("A details")

	async.Wait()
	gt.Number(t, llm.calls()).Equal(4)
}

// --- Integration: plan → phase → replan(question, terminate) --------

func TestRunner_Run_QuestionTerminates(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1: one task.
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"check A","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "found something ambiguous"},
		// Round 2: replan asks the user.
		{text: `{"message":"need more","question":{
			"reason":"ambiguity","items":[{"id":"q1","text":"Which?","type":"select","options":["A","B"]}]
		}}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.AllowQuestion = true
	questionFired := atomic.Bool{}
	req.OnQuestion = func(_ context.Context, q planexec.Question) (planexec.QuestionResult, error) {
		questionFired.Store(true)
		gt.String(t, q.Reason).Equal("ambiguity")
		gt.Array(t, q.Items).Length(1)
		return planexec.QuestionResult{Terminate: true}, nil
	}

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.EndedWithQuestion).True()
	gt.String(t, res.Text).Equal("")
	gt.Bool(t, questionFired.Load()).True()

	async.Wait()
	// 3 calls: planner + sub-agent + replan. No final-response phase
	// because the question terminated the loop.
	gt.Number(t, llm.calls()).Equal(3)
}

// --- Integration: Resume re-enters at replan with answers folded in -----

func TestRunner_Resume_EntersReplanWithAnswers(t *testing.T) {
	ctx := context.Background()
	// The FIRST response after Resume must be parsed as a REPLAN (empty
	// tasks → terminate). A first-round plan parse would reject empty tasks
	// (minTasksPerPhase=1), so accepting this proves Resume entered at the
	// replan branch rather than re-planning from round 0.
	llm := newSequencedLLM([]sequencedResponse{
		// Replan round: terminate via explicit finalize (the answer was enough).
		{text: `{"message":"done","finalize":{"reason":"done"}}`, matchSubstr: "User answers"},
		// Final-response LLM call.
		{text: "Resolved using the user's answer: A."},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	// A resumed turn may itself ask another question, so OnQuestion stays
	// wired even though this scenario terminates without re-asking.
	req.AllowQuestion = true
	req.OnQuestion = func(_ context.Context, _ planexec.Question) (planexec.QuestionResult, error) {
		return planexec.QuestionResult{Terminate: true}, nil
	}

	resumeReq := planexec.ResumeRequest{
		RunRequest: req,
		Question: planexec.Question{
			Reason: "ambiguity",
			Items: []planexec.QuestionItem{
				{ID: "q1", Text: "Which environment?", Type: "select", Options: []string{"A", "B"}},
			},
		},
		Answers: []planexec.QuestionAnswer{{ID: "q1", Choice: "A"}},
	}

	res, err := planexec.ResumeText(ctx, runner, resumeReq)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.Text).Contains("Resolved using the user's answer")
	gt.Bool(t, res.EndedWithQuestion).False()

	async.Wait()
	// 2 calls: the resumed replan + the final-response phase. No fresh
	// round-0 plan and no sub-agent re-execution.
	gt.Number(t, llm.calls()).Equal(2)

	// The resumed planner input must carry the user's answer verbatim and
	// be labelled as a question-answer turn, proving the answer was folded
	// into the conversation rather than discarded.
	firstInput := llm.inputAt(0)
	gt.String(t, firstInput).Contains("User answers")
	gt.String(t, firstInput).Contains("q1")
	gt.String(t, firstInput).Contains("Which environment?")
	gt.String(t, firstInput).Contains("Answer (select): A")
}

func TestRunner_Resume_RejectsEmptyAnswers(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM(nil)
	runner := newRunner(t, llm.Client())
	req := baseRequest()
	resumeReq := planexec.ResumeRequest{
		RunRequest: req,
		Question: planexec.Question{
			Reason: "x",
			Items:  []planexec.QuestionItem{{ID: "q1", Text: "?", Type: "free_text"}},
		},
		Answers: nil,
	}
	_, err := planexec.ResumeText(ctx, runner, resumeReq)
	gt.Error(t, err)
	// No LLM call should have fired on a validation failure.
	gt.Number(t, llm.calls()).Equal(0)
}

// --- Integration: budget exhausted → Fallback -----------------------

func TestRunner_Run_PlannerBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	// Every planner round fails validation, so retries burn the budget.
	llm := newSequencedLLM([]sequencedResponse{
		{text: `not json at all`},
		{text: `still not json`},
	})

	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm.Client(),
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget: planexec.BudgetConfig{
			PlannerLoopMax:  2,
			SubAgentLoopMax: 20,
		},
	})
	gt.NoError(t, err).Required()

	res, err := planexec.RunText(ctx, runner, baseRequest())
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusFallbackBudget)
	gt.String(t, res.FallbackReason).Contains("planner budget exhausted")
	// Loop never produced any phase results.
	gt.Array(t, res.AllResults).Length(0)
}

// --- Integration: schema-bound final response (structured JSON) -----

func TestRunner_Run_StructuredFinalOutput(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"go","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "investigation result"},
		// Round 2: terminate via explicit finalize
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final JSON
		{text: `{"workspace_id":"ws-1","title":"Found case","description":"long desc"}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()

	res, err := planexec.Run[finalPayload](ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.Text).Equal("")
	gt.Value(t, res.Data).NotNil().Required()
	gt.String(t, res.Data.WorkspaceID).Equal("ws-1")
	gt.String(t, res.Data.Title).Equal("Found case")
}

// --- Integration: validation retry recovery -------------------------

func TestRunner_Run_RetriesOnValidationFailure(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1 attempt 1: bad JSON.
		{text: `{"tasks":[]}`}, // empty tasks → validation fails
		// Round 1 attempt 2: valid.
		{text: `{"message":"recover","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "ok"},
		// Round 2: terminate via explicit finalize.
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final
		{text: "done."},
	})

	runner := newRunner(t, llm.Client())
	res, err := planexec.RunText(ctx, runner, baseRequest())
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.Text).Equal("done.")
}

// --- Integration: structured final output validation retry ----------

// When the structured final output fails T.Validate(), Run[T] must
// regenerate within the same final phase (not by re-entering the planner
// loop), succeeding once a later attempt validates.
func TestRunner_Run_StructuredFinalValidationRetry(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1: one task.
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "investigation result"},
		// Round 2: terminate via explicit finalize.
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final output attempt 1: empty title → Validate() fails.
		{text: `{"title":""}`},
		// Final output attempt 2: valid.
		{text: `{"workspace_id":"ws-1","title":"Found case","description":"d"}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()

	res, err := planexec.Run[finalPayload](ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Value(t, res.Data).NotNil().Required()
	gt.String(t, res.Data.Title).Equal("Found case")
}

// The structured final output that keeps failing T.Validate() must
// eventually exhaust the final-phase retries and fall back without
// completing (it does not re-enter the planner loop).
func TestRunner_Run_StructuredFinalValidationExhausted(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "result"},
		// Round 2: terminate via explicit finalize.
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final output attempts 1-3 (finalOutputMaxRetry(2)+1): always invalid.
		{text: `{"title":""}`},
		{text: `{"title":""}`},
		{text: `{"title":""}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()

	res, err := planexec.Run[finalPayload](ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusFallbackError)
	gt.Value(t, res.Data).Nil()
}

// --- Integration: direct mode (round-1 fast path) -------------------

// recordingResolver records the tool ids it was asked to resolve so a
// direct-mode test can assert the planner's chosen tools reached the
// resolver. It returns no concrete tools (the mock LLM needs none).
type recordingResolver struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *recordingResolver) Resolve(ids []string) []gollem.Tool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, ids)
	return nil
}

func (r *recordingResolver) lastCall() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return nil
	}
	return r.calls[len(r.calls)-1]
}

func TestRunner_Run_DirectMode(t *testing.T) {
	ctx := context.Background()
	const userMsg = "Thanks, that is all I needed for now."
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1: the planner gate. It must receive the user's request and
		// chooses the direct path (no tasks).
		{text: `{"message":"answering directly","direct":{}}`, matchSubstr: userMsg},
		// The direct ReAct reply. matchSubstr pins this second call to the
		// direct user prompt (prompts/direct.md), proving it is the direct
		// loop and not, say, a final-synthesis call.
		{text: "Here is the direct answer.", matchSubstr: "Answer the request directly"},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.UserInput = userMsg
	req.AllowDirect = true

	var phaseStarted atomic.Bool
	req.Sink = planexec.SinkFuncs{
		PhaseStartedFn: func(_ context.Context, _ int, _ []planexec.TaskInfo) {
			phaseStarted.Store(true)
		},
	}

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	gt.String(t, res.Text).Equal("Here is the direct answer.")
	// No investigation phase ran.
	gt.Array(t, res.AllResults).Length(0)
	gt.Bool(t, phaseStarted.Load()).False()

	async.Wait()
	// Exactly 2 LLM calls: planner gate + direct reply. No sub-agent, no
	// replan, no separate final-synthesis call.
	gt.Number(t, llm.calls()).Equal(2)
	// Verify the *content* of each call, not just the count:
	// - call 0 is the planner gate and carries the user's request,
	gt.String(t, llm.inputAt(0)).Contains(userMsg)
	// - call 1 is the direct ReAct loop: it runs the direct user prompt and
	//   restates the user's request (prompts/direct.md interpolates UserInput).
	gt.String(t, llm.inputAt(1)).Contains("Answer the request directly")
	gt.String(t, llm.inputAt(1)).Contains(userMsg)
}

func TestRunner_Run_DirectMode_ResolvesChosenTools(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"ok","direct":{"tools":["slack_ro","github"]}}`},
		{text: "answer", matchSubstr: "Answer the request directly"},
	})

	resolver := &recordingResolver{}
	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.AllowDirect = true
	req.ToolResolver = resolver

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	// The planner's chosen tool ids reached the resolver verbatim.
	got := resolver.lastCall()
	gt.Array(t, got).Length(2).Required()
	gt.String(t, got[0]).Equal("slack_ro")
	gt.String(t, got[1]).Equal("github")
}

// The direct path must NOT consult the structured final-output generator
// even when the host is driving a Run[T] turn — that machinery is reserved
// for the structured investigate terminal. A direct reply always produces
// plain text in Text with Data left nil.
func TestRunner_Run_DirectMode_StructuredSkipsFinalGen(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"ok","direct":{}}`},
		{text: "plain direct reply", matchSubstr: "Answer the request directly"},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.AllowDirect = true

	res, err := planexec.Run[finalPayload](ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	gt.String(t, res.Text).Equal("plain direct reply")
	gt.Value(t, res.Data).Nil()
}

// --- Integration: real-LLM plan/direct routing ---------------------

// newRoutingTestLLMClient builds a real provider client from the same
// TEST_LLM_* env vars the eval harness uses. It is gated solely on
// TEST_LLM_PROVIDER (like pkg/usecase/eval), so `zenv go test ./...` runs it
// whenever a provider is configured — there is no extra opt-in gate. The
// provider switch mirrors realLLMForThreadCreate so the conventions stay in
// lockstep.
func newRoutingTestLLMClient(t *testing.T) gollem.LLMClient {
	t.Helper()
	ctx := context.Background()
	model := os.Getenv("TEST_LLM_MODEL")
	switch os.Getenv("TEST_LLM_PROVIDER") {
	case "openai":
		key := os.Getenv("TEST_LLM_OPENAI_API_KEY")
		gt.Value(t, key).NotEqual("")
		var opts []openai.Option
		if model != "" {
			opts = append(opts, openai.WithModel(model))
		}
		c, err := openai.New(ctx, key, opts...)
		gt.NoError(t, err).Required()
		return c
	case "claude":
		key := os.Getenv("TEST_LLM_CLAUDE_API_KEY")
		project := os.Getenv("TEST_LLM_GEMINI_PROJECT_ID")
		switch {
		case key != "":
			var opts []claude.Option
			if model != "" {
				opts = append(opts, claude.WithModel(model))
			}
			c, err := claude.New(ctx, key, opts...)
			gt.NoError(t, err).Required()
			return c
		case project != "":
			location := os.Getenv("TEST_LLM_GEMINI_LOCATION")
			gt.Value(t, location).NotEqual("")
			var opts []claude.VertexOption
			if model != "" {
				opts = append(opts, claude.WithVertexModel(model))
			}
			c, err := claude.NewWithVertex(ctx, location, project, opts...)
			gt.NoError(t, err).Required()
			return c
		default:
			t.Skip("claude provider needs TEST_LLM_CLAUDE_API_KEY or TEST_LLM_GEMINI_PROJECT_ID")
			return nil
		}
	case "gemini":
		project := os.Getenv("TEST_LLM_GEMINI_PROJECT_ID")
		location := os.Getenv("TEST_LLM_GEMINI_LOCATION")
		gt.Value(t, project).NotEqual("")
		gt.Value(t, location).NotEqual("")
		var opts []gemini.Option
		if model != "" {
			opts = append(opts, gemini.WithModel(model))
		}
		c, err := gemini.New(ctx, project, location, opts...)
		gt.NoError(t, err).Required()
		return c
	default:
		t.Skip("TEST_LLM_PROVIDER must be openai | claude | gemini")
		return nil
	}
}

// routingSystemPrompt is a compact host base prompt that gives the planner
// enough role context to make a realistic direct-vs-investigate choice.
const routingSystemPrompt = "You are an assistant embedded in a case-management Slack thread. " +
	"You help triage and answer questions about an ongoing case. " +
	"Read-only investigation tools (Slack history, Notion, GitHub, past cases) are available to your sub-agents."

// TestRunner_Run_RealLLM_PlanVsDirectRouting verifies, against a real LLM,
// that the round-1 gate actually routes a trivial request through the direct
// fast path while a request that genuinely needs investigation is planned as
// tasks. This is the behavioural contract the mock tests cannot prove: the
// model itself must make the call.
func TestRunner_Run_RealLLM_PlanVsDirectRouting(t *testing.T) {
	ctx := context.Background()
	llm := newRoutingTestLLMClient(t)

	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		// Bound the investigate path so the complex case cannot run away.
		Budget: planexec.BudgetConfig{PlannerLoopMax: 4, SubAgentLoopMax: 6},
	})
	gt.NoError(t, err).Required()

	t.Run("trivial request goes direct", func(t *testing.T) {
		req := baseRequest()
		req.AllowDirect = true
		req.SystemPrompt = routingSystemPrompt
		req.UserInput = "A teammate just wrote: \"Thanks, that's all I needed for now!\" " +
			"Acknowledge it briefly and politely. No investigation, lookup, or tools are required."

		res, err := planexec.RunText(ctx, runner, req)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Direct).True()
		gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
		gt.String(t, res.Text).NotEqual("")
		// The direct path never runs an investigation phase.
		gt.Array(t, res.AllResults).Length(0)
		async.Wait()
	})

	t.Run("complex request is investigated", func(t *testing.T) {
		req := baseRequest()
		req.AllowDirect = true
		req.SystemPrompt = routingSystemPrompt
		req.UserInput = "Investigate the root cause of the recurring production login outage on this case: " +
			"correlate the discussion in this Slack thread, find related past cases, and check the linked " +
			"Notion runbook and the relevant GitHub commits, then summarise what you found."

		res, err := planexec.RunText(ctx, runner, req)
		gt.NoError(t, err).Required()
		// The model must NOT short-circuit a multi-source investigation.
		gt.Bool(t, res.Direct).False()
		// At least one investigation phase ran (true regardless of whether the
		// loop completed or hit the bounded budget).
		gt.Number(t, len(res.AllResults)).GreaterOrEqual(1)
		async.Wait()
	})
}

// When AllowDirect is false (the default), a planner that emits a direct
// payload is rejected and retried; the investigate path still drives the
// turn to completion.
func TestRunner_Run_DirectRejectedWhenNotAllowed(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1 attempt 1: direct, but AllowDirect is false → rejected.
		{text: `{"message":"try direct","direct":{}}`},
		// Round 1 attempt 2: fall back to a valid task plan.
		{text: `{"message":"plan","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "investigation result"},
		// Round 2: terminate via explicit finalize.
		{text: `{"message":"done","finalize":{"reason":"done"}}`},
		// Final synthesis.
		{text: "final plain answer"},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	// req.AllowDirect defaults to false.

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).False()
	gt.String(t, res.Text).Equal("final plain answer")
	gt.Array(t, res.AllResults).Length(1)
}

// --- Sub-agent writes (AllowSubAgentWrites) -------------------------

// recordingWriteTool is a gollem.Tool that records its invocations so a
// test can prove a sub-agent actually performed the write it was assigned,
// rather than merely describing it in text.
type recordingWriteTool struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (t *recordingWriteTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "slack_write",
		Description: "Post a plain-text message to the case's Slack channel.",
		Parameters: map[string]*gollem.Parameter{
			"text": {Type: gollem.TypeString, Description: "message text", Required: true},
		},
	}
}

func (t *recordingWriteTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, args)
	return map[string]any{"posted": true}, nil
}

func (t *recordingWriteTool) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func (t *recordingWriteTool) firstText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.calls) == 0 {
		return ""
	}
	s, _ := t.calls[0]["text"].(string)
	return s
}

// writeToolResolver hands the recording write tool to any task whose tool
// list names "slack_write", and nothing otherwise.
type writeToolResolver struct{ tool gollem.Tool }

func (r writeToolResolver) Resolve(ids []string) []gollem.Tool {
	if slices.Contains(ids, "slack_write") {
		return []gollem.Tool{r.tool}
	}
	return nil
}

// TestRunner_Run_SubAgentPerformsWrite drives the full loop with a mock LLM
// that dispatches a write task and issues a tool call from the sub-agent. It
// proves that, with AllowSubAgentWrites=true, a sub-agent actually invokes an
// assigned write tool end-to-end (planner → write task → sub-agent tool call
// → replan terminate → final), which is exactly what the observation-only
// restriction previously prevented.
func TestRunner_Run_SubAgentPerformsWrite(t *testing.T) {
	ctx := context.Background()
	writeTool := &recordingWriteTool{}
	round := atomic.Int32{}
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					switch round.Add(1) {
					case 1: // planner round 1: dispatch a single write task
						return &gollem.Response{Texts: []string{`{"message":"post the summary","tasks":[
							{"id":"t-1","title":"Post","description":"Post the one-line summary to the channel.","acceptance_criteria":"summary posted","tools":["slack_write"]}
						]}`}}, nil
					case 2: // sub-agent: call the write tool
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{{
							ID:        "call-1",
							Name:      "slack_write",
							Arguments: map[string]any{"text": "One-line summary."},
						}}}, nil
					case 3: // sub-agent: report after the tool result comes back
						return &gollem.Response{Texts: []string{"Posted the one-line summary to the channel."}}, nil
					case 4: // replan: terminate via explicit finalize
						return &gollem.Response{Texts: []string{`{"message":"done","finalize":{"reason":"done"}}`}}, nil
					default: // final synthesis
						return &gollem.Response{Texts: []string{"Summary posted."}}, nil
					}
				},
			}, nil
		},
	}

	runner := newRunner(t, llm)
	req := baseRequest()
	req.AllowSubAgentWrites = true
	req.KnownToolIDs = []string{"slack_write"}
	req.ToolResolver = writeToolResolver{tool: writeTool}

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	async.Wait()

	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	// The sub-agent actually invoked the write tool exactly once, with the
	// text the model chose — not merely wrote it into the final summary.
	gt.Number(t, writeTool.callCount()).Equal(1)
	gt.String(t, writeTool.firstText()).Equal("One-line summary.")
}

// TestRunner_Run_RealLLM_SubAgentPerformsWrite is the live-LLM counterpart:
// against a real provider, the planner must dispatch a write task and the
// sub-agent must actually call the posting tool to satisfy the goal. Gated on
// TEST_LLM_PROVIDER via newRoutingTestLLMClient, like the routing test.
func TestRunner_Run_RealLLM_SubAgentPerformsWrite(t *testing.T) {
	ctx := context.Background()
	llm := newRoutingTestLLMClient(t)

	writeTool := &recordingWriteTool{}
	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget:      planexec.BudgetConfig{PlannerLoopMax: 5, SubAgentLoopMax: 6},
	})
	gt.NoError(t, err).Required()

	req := baseRequest()
	// AllowDirect stays false so a side-effecting goal cannot be short-
	// circuited through the direct path — it must go through a task.
	req.AllowSubAgentWrites = true
	req.KnownToolIDs = []string{"slack_write"}
	req.ToolResolver = writeToolResolver{tool: writeTool}
	req.SystemPrompt = "You are an agent handling a support case. Your deliverable is to POST a short " +
		"acknowledgement to the case's Slack channel using the `slack_write` tool. Writing the message " +
		"as text without actually calling the tool does NOT count as done."
	req.UserInput = "A new case was just created: \"Login page returns HTTP 500 for some users.\" " +
		"Post a one-line acknowledgement to the channel confirming the case is being looked into."

	res, err := planexec.RunText(ctx, runner, req)
	gt.NoError(t, err).Required()
	async.Wait()

	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	// The live model must have driven a sub-agent to actually call the write
	// tool at least once.
	gt.Number(t, writeTool.callCount()).GreaterOrEqual(1)
}
