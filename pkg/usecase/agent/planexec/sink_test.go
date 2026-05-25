package planexec_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// SinkFuncs is the struct-of-funcs adapter to planexec.Sink. An all-zero
// SinkFuncs MUST be safe to use: every method becomes a no-op. This is
// what tests and minimal hosts rely on instead of writing a mock.
func TestSinkFuncs_AllNilNoOp(t *testing.T) {
	var s planexec.Sink = planexec.SinkFuncs{}

	// None of these calls may panic.
	ctx := context.Background()
	s.Notify(ctx, "hello")
	s.PlanProposed(ctx, planexec.PlanInfo{Round: 1, Reasoning: "x"})
	s.PhaseStarted(ctx, 1, []planexec.TaskInfo{{ID: "t1", Title: "test"}})
	s.TaskProgress(ctx, "t1", "running")
	s.TaskFinished(ctx, planexec.TaskResult{
		TaskID: "t1",
		Title:  "test",
		Status: planexec.TaskStatusCompleted,
	})
}

func TestSinkFuncs_DispatchesProvidedCallbacks(t *testing.T) {
	notifyHits := 0
	planHits := 0
	startedHits := 0
	progressHits := 0
	finishedHits := 0

	s := planexec.SinkFuncs{
		NotifyFn:       func(_ context.Context, _ string) { notifyHits++ },
		PlanProposedFn: func(_ context.Context, _ planexec.PlanInfo) { planHits++ },
		PhaseStartedFn: func(_ context.Context, _ int, _ []planexec.TaskInfo) { startedHits++ },
		TaskProgressFn: func(_ context.Context, _, _ string) { progressHits++ },
		TaskFinishedFn: func(_ context.Context, _ planexec.TaskResult) { finishedHits++ },
	}

	ctx := context.Background()
	s.Notify(ctx, "n")
	s.PlanProposed(ctx, planexec.PlanInfo{})
	s.PhaseStarted(ctx, 1, nil)
	s.TaskProgress(ctx, "t", "x")
	s.TaskFinished(ctx, planexec.TaskResult{TaskID: "t"})

	gt.Number(t, notifyHits).Equal(1)
	gt.Number(t, planHits).Equal(1)
	gt.Number(t, startedHits).Equal(1)
	gt.Number(t, progressHits).Equal(1)
	gt.Number(t, finishedHits).Equal(1)
}
