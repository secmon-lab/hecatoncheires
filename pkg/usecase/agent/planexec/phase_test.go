package planexec_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// ----- Sub-agent prompt template (prompts/subagent.md) --------------

func TestSubAgentPrompt_RendersTaskFields(t *testing.T) {
	task := planexec.TaskPlan{
		ID:                 "t-1",
		Title:              "Recent thread",
		Description:        "Read the parent thread.",
		AcceptanceCriteria: "Top ten messages summarised.",
		Tools:              []string{"slack_ro"},
	}
	got, err := planexec.RenderSubAgentPromptForTest(task, false)
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("- ID: t-1")
	gt.String(t, got).Contains("- Title: Recent thread")
	gt.String(t, got).Contains("Read the parent thread.")
	gt.String(t, got).Contains("Top ten messages summarised.")
	gt.String(t, got).Contains("investigation sub-agent")
}

func TestSubAgentPrompt_EmptyFieldsStillWellFormed(t *testing.T) {
	// All required fields are usually enforced by TaskPlan.Validate
	// before this template is reached, but the template itself must
	// not panic on zero values.
	got, err := planexec.RenderSubAgentPromptForTest(planexec.TaskPlan{}, false)
	gt.NoError(t, err).Required()
	gt.String(t, got).Contains("## Your Task")
	gt.String(t, got).Contains("Output rules")
}

func TestSubAgentPrompt_ObservationOnlyByDefault(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Investigate", Description: "d", AcceptanceCriteria: "a"}
	got, err := planexec.RenderSubAgentPromptForTest(task, false)
	gt.NoError(t, err).Required()
	// allowWrites=false keeps the observation-only prohibition.
	gt.String(t, got).Contains("observation-only")
	gt.String(t, got).Contains("Do NOT post messages or mutate")
}

func TestSubAgentPrompt_WriteModeGrantsWrites(t *testing.T) {
	task := planexec.TaskPlan{ID: "t-1", Title: "Post", Description: "post the summary", AcceptanceCriteria: "posted"}
	got, err := planexec.RenderSubAgentPromptForTest(task, true)
	gt.NoError(t, err).Required()
	// allowWrites=true drops the observation-only prohibition entirely and
	// grants the write permission (guarded by "after ... enough supporting
	// information").
	gt.Bool(t, contains(got, "observation-only")).False()
	gt.Bool(t, contains(got, "Do NOT post messages or mutate")).False()
	gt.String(t, got).Contains("you MAY use it")
	gt.String(t, got).Contains("enough supporting information")
}

// ----- formatObservationsAsUserTurn ---------------------------------

func TestFormatObservations_RendersStatusAndCriteria(t *testing.T) {
	tasks := []planexec.TaskPlan{
		{ID: "t-1", Title: "A", AcceptanceCriteria: "X identified", Tools: []string{"slack_ro"}},
	}
	results := []planexec.TaskResult{
		{
			TaskID: "t-1", Title: "A", AcceptanceCriteria: "X identified",
			Status: planexec.TaskStatusCompleted, Summary: "We found the cause.",
		},
	}
	got := planexec.FormatObservationsForTest(tasks, results)
	gt.String(t, got).Contains("# Observations from prior investigations")
	gt.String(t, got).Contains("## t-1: A")
	gt.String(t, got).Contains("**Status**: completed")
	gt.String(t, got).Contains("**Acceptance criteria**: X identified")
	gt.String(t, got).Contains("We found the cause.")
}

func TestFormatObservations_FailedHasErrorBlock(t *testing.T) {
	tasks := []planexec.TaskPlan{
		{ID: "t-2", Title: "B", AcceptanceCriteria: "Y resolved", Tools: []string{"github"}},
	}
	results := []planexec.TaskResult{
		{
			TaskID: "t-2", Title: "B", AcceptanceCriteria: "Y resolved",
			Status: planexec.TaskStatusFailed, Error: "rate limited",
		},
	}
	got := planexec.FormatObservationsForTest(tasks, results)
	gt.String(t, got).Contains("**Status**: failed")
	gt.String(t, got).Contains("**Error**: rate limited")
}

// ----- executePhase end-to-end via mock LLM -------------------------

// stubResolverNoTools satisfies ToolResolver with an empty tool slice so
// the mock LLM never has to handle a tool call.
type stubResolverNoTools struct{}

func (stubResolverNoTools) Resolve(_ []string) []gollem.Tool { return nil }

// fakeSessionConfig drives the canned per-task response.
type fakeSessionConfig struct {
	text string
	err  error
}

// newFakeLLM produces a gollem mock whose NewSession returns a session
// that picks its canned response by inspecting the input text against
// byDescription. This lets parallel sub-agents each receive their per-
// task config regardless of dispatch order.
func newFakeLLM(byDescription map[string]fakeSessionConfig) *mock.LLMClientMock {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			calls := atomic.Int32{}
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					n := calls.Add(1)
					if n > 1 {
						return nil, errors.New("unexpected second Generate call")
					}
					if len(input) == 0 {
						return nil, errors.New("no input passed to Generate")
					}
					txt, ok := input[0].(gollem.Text)
					if !ok {
						return nil, errors.New("expected gollem.Text input")
					}
					cfg, ok := byDescription[string(txt)]
					if !ok {
						return nil, errors.New("no fakeSessionConfig for description: " + string(txt))
					}
					if cfg.err != nil {
						return nil, cfg.err
					}
					return &gollem.Response{Texts: []string{cfg.text}}, nil
				},
			}, nil
		},
	}
}

// recordingSink captures Sink events for assertion. Concurrent-safe so
// the parallel sub-agents can update it without contention.
type recordingSink struct {
	mu                sync.Mutex
	phaseStarted      []phaseStartedEvent
	taskProgress      map[string][]string
	taskFinished      map[string]planexec.TaskResult
	taskProgressOrder []string
}

type phaseStartedEvent struct {
	phase int
	tasks []planexec.TaskInfo
}

func newRecordingSink() *recordingSink {
	return &recordingSink{
		taskProgress: make(map[string][]string),
		taskFinished: make(map[string]planexec.TaskResult),
	}
}

func (s *recordingSink) Notify(_ context.Context, _ string) {}

func (s *recordingSink) PlanProposed(_ context.Context, _ planexec.PlanInfo) {}

func (s *recordingSink) PhaseStarted(_ context.Context, phase int, tasks []planexec.TaskInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phaseStarted = append(s.phaseStarted, phaseStartedEvent{phase: phase, tasks: tasks})
}

func (s *recordingSink) TaskProgress(_ context.Context, id, line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskProgress[id] = append(s.taskProgress[id], line)
	s.taskProgressOrder = append(s.taskProgressOrder, id+": "+line)
}

func (s *recordingSink) TaskFinished(_ context.Context, r planexec.TaskResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskFinished[r.TaskID] = r
}

func (s *recordingSink) latestProgress(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := s.taskProgress[id]
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func TestExecutePhase_MixedSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"check thread A": {text: "summary one"},
		"check thread B": {err: errors.New("denied")},
	})
	sink := newRecordingSink()

	tasks := []planexec.TaskPlan{
		{ID: "t-1", Title: "A", Description: "check thread A", AcceptanceCriteria: "a", Tools: []string{"slack_ro"}},
		{ID: "t-2", Title: "B", Description: "check thread B", AcceptanceCriteria: "b", Tools: []string{"slack_ro"}},
	}
	results := planexec.ExecutePhaseForTest(ctx, tasks, sink, stubResolverNoTools{}, llm, 20, nil, false)
	async.Wait()

	gt.Array(t, results).Length(2).Required()

	byID := map[string]planexec.TaskResult{}
	for _, r := range results {
		byID[r.TaskID] = r
	}
	gt.Value(t, byID["t-1"].Status).Equal(planexec.TaskStatusCompleted)
	gt.String(t, byID["t-1"].Summary).Equal("summary one")
	gt.Value(t, byID["t-2"].Status).Equal(planexec.TaskStatusFailed)
	gt.String(t, byID["t-2"].Error).Equal("denied")

	// Per-task progress terminates in "done" / "failed".
	gt.String(t, sink.latestProgress("t-1")).Contains("done")
	gt.String(t, sink.latestProgress("t-2")).Contains("failed")

	// TaskFinished fired exactly once per task.
	gt.Map(t, sink.taskFinished).HasKey("t-1")
	gt.Map(t, sink.taskFinished).HasKey("t-2")
	gt.Value(t, sink.taskFinished["t-1"].Status).Equal(planexec.TaskStatusCompleted)
	gt.Value(t, sink.taskFinished["t-2"].Status).Equal(planexec.TaskStatusFailed)
}

func TestExecutePhase_TruncatesLongSummary(t *testing.T) {
	ctx := context.Background()
	// Build a 10KB summary; expected truncation marker appended.
	big := strings.Repeat("a", 10*1024)
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"long task": {text: big},
	})
	sink := newRecordingSink()

	tasks := []planexec.TaskPlan{
		{ID: "t-1", Title: "long", Description: "long task", AcceptanceCriteria: "x", Tools: []string{"slack_ro"}},
	}
	results := planexec.ExecutePhaseForTest(ctx, tasks, sink, stubResolverNoTools{}, llm, 20, nil, false)
	async.Wait()

	gt.Array(t, results).Length(1).Required()
	r := results[0]
	gt.Value(t, r.Status).Equal(planexec.TaskStatusCompleted)
	// Truncated to the cap plus the marker tail; the original was
	// longer than the cap so we MUST see the marker.
	gt.String(t, r.Summary).Contains("[truncated]")
	gt.Bool(t, len(r.Summary) < len(big)).True()
}

func TestExecutePhase_EmptyTasksReturnsNil(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(nil)
	sink := newRecordingSink()
	results := planexec.ExecutePhaseForTest(ctx, nil, sink, stubResolverNoTools{}, llm, 20, nil, false)
	async.Wait()
	gt.Array(t, results).Length(0)
}

// panicResolver panics when a task requests the "panic_tool" id, and returns no
// tools otherwise. Resolve is called inside runOneTask (before gollem.Execute,
// so gollem cannot swallow the panic), giving a deterministic way to drive a
// sub-agent panic through executePhase's recovery path.
type panicResolver struct{}

func (panicResolver) Resolve(ids []string) []gollem.Tool {
	for _, id := range ids {
		if id == "panic_tool" {
			panic("boom in sub-agent")
		}
	}
	return nil
}

// TestExecutePhase_RecoversSubAgentPanic pins the failure-propagation fix: when
// a sub-agent panics, executePhase must (1) fill that task's result with a
// TaskStatusFailed observation carrying the panic reason — so the planner learns
// the task failed instead of seeing a contentless zero-value entry — and (2) let
// the other tasks in the phase complete normally. Without the recover, results[i]
// would stay a zero-value TaskResult (empty Status).
func TestExecutePhase_RecoversSubAgentPanic(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"check thread A": {text: "summary one"},
	})
	sink := newRecordingSink()

	tasks := []planexec.TaskPlan{
		{ID: "t-1", Title: "A", Description: "check thread A", AcceptanceCriteria: "a", Tools: []string{"slack_ro"}},
		{ID: "t-2", Title: "Boom", Description: "will panic", AcceptanceCriteria: "b", Tools: []string{"panic_tool"}},
	}
	results := planexec.ExecutePhaseForTest(ctx, tasks, sink, panicResolver{}, llm, 20, nil, false)
	async.Wait()

	gt.Array(t, results).Length(2).Required()
	byID := map[string]planexec.TaskResult{}
	for _, r := range results {
		byID[r.TaskID] = r
	}
	// The panicking task surfaces as a failed observation, not a zero value.
	gt.Value(t, byID["t-2"].Status).Equal(planexec.TaskStatusFailed)
	gt.String(t, byID["t-2"].Error).Contains("sub-agent panicked")
	gt.String(t, byID["t-2"].Title).Equal("Boom")
	// The sibling task still completed — one panic does not sink the phase.
	gt.Value(t, byID["t-1"].Status).Equal(planexec.TaskStatusCompleted)
	gt.String(t, byID["t-1"].Summary).Equal("summary one")
}
