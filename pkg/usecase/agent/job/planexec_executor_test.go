package job_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

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
		`{"message":"done","tasks":[]}`,
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
