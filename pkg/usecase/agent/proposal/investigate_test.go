package proposal_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// recordingHandler captures Trace lines (phase-level) and TraceTask
// lines (per-task) so the test can assert on the progress narrative.
// Question / Materialize / PostBusy are unused here (investigate flow
// doesn't reach terminal actions) so they fail the test if invoked.
type recordingHandler struct {
	mu             sync.Mutex
	lines          []string
	registered     []proposal.TaskInfo
	taskLines      map[string][]string
	taskLineLatest map[string]string
}

func (h *recordingHandler) Question(_ context.Context, _ *model.Session, _ proposal.QuestionPayload) error {
	return errors.New("unexpected Question in investigate test")
}
func (h *recordingHandler) Materialize(_ context.Context, _ *model.Session, _ proposal.MaterializePayload) error {
	return errors.New("unexpected Materialize in investigate test")
}
func (h *recordingHandler) Trace(_ context.Context, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, line)
}
func (h *recordingHandler) TraceRound(_ context.Context, _, _ string) {}
func (h *recordingHandler) RegisterTasks(_ context.Context, tasks []proposal.TaskInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.registered = append(h.registered, tasks...)
}
func (h *recordingHandler) TraceTask(_ context.Context, taskID, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.taskLines == nil {
		h.taskLines = make(map[string][]string)
		h.taskLineLatest = make(map[string]string)
	}
	h.taskLines[taskID] = append(h.taskLines[taskID], line)
	h.taskLineLatest[taskID] = line
}
func (h *recordingHandler) PostBusy(_ context.Context, _ *model.Session, _ agent.BusyInfo) error {
	return errors.New("unexpected PostBusy in investigate test")
}

func (h *recordingHandler) Lines() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.lines))
	copy(out, h.lines)
	return out
}

func (h *recordingHandler) TaskLatest(taskID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.taskLineLatest[taskID]
}

func (h *recordingHandler) TaskHistory(taskID string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.taskLines[taskID]))
	copy(out, h.taskLines[taskID])
	return out
}

func (h *recordingHandler) RegisteredTasks() []proposal.TaskInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]proposal.TaskInfo, len(h.registered))
	copy(out, h.registered)
	return out
}

// fakeSessionConfig is the canned outcome the mock produces when a
// sub-agent's task description matches the configured key. The mock looks
// up by exact match on the user input passed to Session.Generate (which
// the runtime sets to task.Description).
type fakeSessionConfig struct {
	text string
	err  error
}

// newFakeLLM produces a gollem mock whose NewSession returns a session
// that picks its canned response by inspecting the input text against
// `byDescription`. This lets parallel sub-agents each receive the
// per-task config the test specified, regardless of dispatch order.
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

func mustUseCase(t *testing.T, llm gollem.LLMClient) *proposal.UseCase {
	t.Helper()
	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := proposal.New(deps, 8, 20)
	gt.NoError(t, err).Required()
	return uc
}

func TestRunOneInvestigation_Completed(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"Investigate the recent error spike.": {text: "Found the cause: stale config."},
	})
	uc := mustUseCase(t, llm)
	resolver := agent.NewToolSetResolver(agent.ToolSetDeps{})

	h := &recordingHandler{}
	task := proposal.PlanInvestigateTaskForTest{
		ID: "inv-1", Title: "Lookup cause",
		Description:        "Investigate the recent error spike.",
		AcceptanceCriteria: "Root cause identified.",
		Tools:              []string{agent.ToolSetCoreRO},
	}
	res := proposal.RunOneInvestigationForTest(uc, ctx, task, h, resolver)
	gt.Value(t, res.Status).Equal(proposal.InvestigationCompletedForTest)
	gt.String(t, res.Summary).Contains("Found the cause")
	gt.Value(t, res.TaskID).Equal("inv-1")
	gt.Value(t, res.Title).Equal("Lookup cause")
	gt.Value(t, res.AcceptanceCriteria).Equal("Root cause identified.")
	gt.Value(t, res.InnerLoopsMax).Equal(20)

	// Per-task line transitions are emitted via TraceTask, not Trace —
	// runOneInvestigation pushes "running…" first, then any progress
	// line(s) from the LLM hook, then a "done" line as the terminal
	// state. The phase-level Trace channel is unused here because there
	// is no plan.Message wrapping this single task.
	history := h.TaskHistory("inv-1")
	gt.Bool(t, len(history) >= 2).True()
	gt.String(t, history[0]).Contains("running")
	gt.String(t, h.TaskLatest("inv-1")).Contains("done")
	gt.Array(t, h.Lines()).Length(0)
}

func TestRunOneInvestigation_FailedSurfaceErrorInTrace(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"x": {err: errors.New("upstream LLM 5xx")},
	})
	uc := mustUseCase(t, llm)
	resolver := agent.NewToolSetResolver(agent.ToolSetDeps{})

	h := &recordingHandler{}
	task := proposal.PlanInvestigateTaskForTest{
		ID: "inv-2", Title: "Lookup cause",
		Description: "x", AcceptanceCriteria: "y",
		Tools: []string{agent.ToolSetSlackRO},
	}
	res := proposal.RunOneInvestigationForTest(uc, ctx, task, h, resolver)
	gt.Value(t, res.Status).Equal(proposal.InvestigationFailedForTest)
	gt.String(t, res.Error).Contains("upstream LLM 5xx")
	gt.String(t, h.TaskLatest("inv-2")).Contains("failed")
}

func TestRunInvestigationsParallel_MixedSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	llm := newFakeLLM(map[string]fakeSessionConfig{
		"check thread A": {text: "summary one"},
		"check thread B": {err: errors.New("denied")},
	})
	uc := mustUseCase(t, llm)
	resolver := agent.NewToolSetResolver(agent.ToolSetDeps{})

	h := &recordingHandler{}
	plan := &proposal.PlanInvestigateForTest{
		Message: "Looking at A and B",
		Tasks: []proposal.PlanInvestigateTaskForTest{
			{ID: "inv-1", Title: "A", Description: "check thread A", AcceptanceCriteria: "a", Tools: []string{agent.ToolSetSlackRO}},
			{ID: "inv-2", Title: "B", Description: "check thread B", AcceptanceCriteria: "b", Tools: []string{agent.ToolSetSlackRO}},
		},
	}
	results := proposal.RunInvestigationsParallelForTest(uc, ctx, plan, h, resolver)
	async.Wait()

	gt.Array(t, results).Length(2).Required()

	// Map results by TaskID to make assertions order-independent.
	byID := map[string]proposal.InvestigationResultForTest{}
	for _, r := range results {
		byID[r.TaskID] = r
	}
	gt.Value(t, byID["inv-1"].Status).Equal(proposal.InvestigationCompletedForTest)
	gt.String(t, byID["inv-1"].Summary).Equal("summary one")
	gt.Value(t, byID["inv-2"].Status).Equal(proposal.InvestigationFailedForTest)
	gt.String(t, byID["inv-2"].Error).Equal("denied")

	// Phase prelude is on the phase-level trace channel.
	found := false
	for _, line := range h.Lines() {
		if strings.Contains(line, "Looking at A and B") {
			found = true
			break
		}
	}
	gt.Bool(t, found).True()

	// Both tasks were registered in the order they appeared in the
	// plan, before any sub-agent goroutine started. This is the
	// "blocks first, sub-agents second" contract from Handler docs.
	registered := h.RegisteredTasks()
	gt.Array(t, registered).Length(2).Required()
	gt.Value(t, registered[0].ID).Equal("inv-1")
	gt.Value(t, registered[0].Title).Equal("A")
	gt.Value(t, registered[1].ID).Equal("inv-2")
	gt.Value(t, registered[1].Title).Equal("B")

	// And per-task lines transitioned independently to their terminal
	// state (done vs failed).
	gt.String(t, h.TaskLatest("inv-1")).Contains("done")
	gt.String(t, h.TaskLatest("inv-2")).Contains("failed")
}
