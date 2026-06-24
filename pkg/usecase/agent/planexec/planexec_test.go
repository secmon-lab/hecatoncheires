package planexec_test

import (
	"context"
	"encoding/json"
	"os"
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
		{text: `{"message":"done","tasks":[]}`},
		{text: "Here's what I found: A details."},
	})

	rec := &recordingTraceHandler{name: "host"}
	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.TraceHandler = rec

	res, err := runner.Run(ctx, req)
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
		{text: `{"message":"done","tasks":[]}`},
		{text: "Here's what I found: A details."},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.TraceHandler = nil

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.String(t, res.FinalText).Contains("A details")

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

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	gt.String(t, res.FinalText).Equal("Here is the direct answer.")
	gt.Value(t, res.FinalRaw).Nil()
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

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	// The planner's chosen tool ids reached the resolver verbatim.
	got := resolver.lastCall()
	gt.Array(t, got).Length(2).Required()
	gt.String(t, got[0]).Equal("slack_ro")
	gt.String(t, got[1]).Equal("github")
}

// The direct path must NOT consult OnFinalize / FinalOutputSchema even when
// the host wired them — those are reserved for the structured investigate
// terminal.
func TestRunner_Run_DirectMode_SkipsOnFinalize(t *testing.T) {
	ctx := context.Background()
	llm := newSequencedLLM([]sequencedResponse{
		{text: `{"message":"ok","direct":{}}`},
		{text: "plain direct reply", matchSubstr: "Answer the request directly"},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	req.AllowDirect = true
	req.FinalOutputSchema = &gollem.Parameter{
		Type:       gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{"title": {Type: gollem.TypeString}},
	}
	req.OnFinalize = func(_ context.Context, _ json.RawMessage) error {
		t.Fatal("OnFinalize must not be called on the direct path")
		return nil
	}

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).True()
	gt.String(t, res.FinalText).Equal("plain direct reply")
	gt.Value(t, res.FinalRaw).Nil()
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

		res, err := runner.Run(ctx, req)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Direct).True()
		gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
		gt.String(t, res.FinalText).NotEqual("")
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

		res, err := runner.Run(ctx, req)
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
		// Round 2: terminate.
		{text: `{"message":"done","tasks":[]}`},
		// Final synthesis.
		{text: "final plain answer"},
	})

	runner := newRunner(t, llm.Client())
	req := baseRequest()
	// req.AllowDirect defaults to false.

	res, err := runner.Run(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(planexec.StatusCompleted)
	gt.Bool(t, res.Direct).False()
	gt.String(t, res.FinalText).Equal("final plain answer")
	gt.Array(t, res.AllResults).Length(1)
}
