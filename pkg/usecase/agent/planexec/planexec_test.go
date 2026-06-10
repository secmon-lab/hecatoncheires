package planexec_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gollem/trace"
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
		// Round 2: replan, terminate (no tasks, no question).
		{text: `{"message":"done","tasks":[]}`},
		// Final-response LLM call (plain text).
		{text: "Here's what I found: A details."},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.FinalText).Contains("A details")
	gt.Bool(t, res.EndedWithQuestion).False()
	gt.Array(t, res.AllResults).Length(1).Required()
	gt.Number(t, len(res.AllResults[0].Tasks)).Equal(1)

	async.Wait()
	// 4 LLM calls in total: 1 planner + 1 sub-agent + 1 replan + 1 final.
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

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.EndedWithQuestion).True()
	gt.String(t, res.FinalText).Equal("")
	gt.Bool(t, questionFired.Load()).True()

	async.Wait()
	// 3 calls: planner + sub-agent + replan. No final-response phase
	// because the question terminated the loop.
	gt.Number(t, llm.calls()).Equal(3)
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

	res, err := runner.Run(ctx, baseRequest())
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
		// Round 2: terminate
		{text: `{"message":"done","tasks":[]}`},
		// Final JSON
		{text: `{"workspace_id":"ws-1","title":"Found case","description":"long desc"}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.FinalOutputSchema = &gollem.Parameter{
		Type: gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"workspace_id": {Type: gollem.TypeString},
			"title":        {Type: gollem.TypeString},
			"description":  {Type: gollem.TypeString},
		},
	}

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.FinalText).Equal("")
	gt.String(t, string(res.FinalRaw)).Contains(`"workspace_id":"ws-1"`)
	gt.String(t, string(res.FinalRaw)).Contains(`"title":"Found case"`)
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
		// Round 2: terminate.
		{text: `{"tasks":[]}`},
		// Final
		{text: "done."},
	})

	runner := newRunner(t, llm.Client())
	res, err := runner.Run(ctx, baseRequest())
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.FinalText).Equal("done.")
}

// --- Integration: OnFinalize rejects then accepts -------------------

// When OnFinalize returns an error (validation OR commit failure), the
// runner must fold the error back as another planner round and try the
// final phase again, succeeding once OnFinalize accepts.
func TestRunner_Run_OnFinalizeRejectThenAccept(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		// Round 1: one task.
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "investigation result"},
		// Round 2: terminate → final attempt #1 (rejected by OnFinalize).
		{text: `{"message":"done","tasks":[]}`},
		{text: `{"title":""}`}, // empty title → OnFinalize rejects
		// Round 3: terminate again → final attempt #2 (accepted).
		{text: `{"message":"retry","tasks":[]}`},
		{text: `{"title":"Found case"}`},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.FinalOutputSchema = &gollem.Parameter{
		Type: gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"title": {Type: gollem.TypeString},
		},
	}
	var finalizeCalls atomic.Int32
	var committed atomic.Value
	req.OnFinalize = func(_ context.Context, raw json.RawMessage) error {
		finalizeCalls.Add(1)
		var out struct {
			Title string `json:"title"`
		}
		gt.NoError(t, json.Unmarshal(raw, &out)).Required()
		if out.Title == "" {
			return goerr.New("title is required")
		}
		committed.Store(out.Title)
		return nil
	}

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	// OnFinalize was called twice: first rejected, second accepted.
	gt.Number(t, int(finalizeCalls.Load())).Equal(2)
	gt.Value(t, committed.Load()).Equal("Found case")
	gt.String(t, string(res.FinalRaw)).Contains("Found case")
}

// OnFinalize that keeps rejecting must eventually exhaust the round
// budget and fall back without completing.
func TestRunner_Run_OnFinalizeAlwaysRejects_Fallback(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"start","tasks":[
			{"id":"t-1","title":"A","description":"x","acceptance_criteria":"a","tools":["slack_ro"]}
		]}`},
		{text: "result"},
		// Round 2: terminate → final (rejected).
		{text: `{"message":"done","tasks":[]}`},
		{text: `{"title":""}`},
		// Round 3: terminate → final (rejected). Budget (3) now exhausted.
		{text: `{"message":"again","tasks":[]}`},
		{text: `{"title":""}`},
	})

	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm.Client(),
		HistoryRepo: agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget:      planexec.BudgetConfig{PlannerLoopMax: 3, SubAgentLoopMax: 20},
	})
	gt.NoError(t, err).Required()

	req := baseRequest()
	req.FinalOutputSchema = &gollem.Parameter{
		Type:       gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{"title": {Type: gollem.TypeString}},
	}
	req.OnFinalize = func(_ context.Context, _ json.RawMessage) error {
		return goerr.New("always reject")
	}

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusFallbackBudget)
}
