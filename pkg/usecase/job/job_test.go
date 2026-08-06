package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// A sweep can only summarise the runs it dispatched if Publish registers them,
// so the wait must not return while one is still in flight.
func TestPublish_RegistersScheduledRunsWithTheSweep(t *testing.T) {
	uc, exec, caseID := scheduledJobsFixture(t, "a")
	stats := job.NewTickStatsForTest(time.Now().UTC())
	ctx := job.WithTickStatsForTest(context.Background(), stats)

	uc.Publish(ctx, job.Event{
		Domain:      model.JobEventDomainScheduled,
		WorkspaceID: "ws",
		CaseID:      caseID,
		Timestamp:   time.Now().UTC(),
		ActorUserID: model.SystemActorID,
		JobID:       "a",
	})

	// The wait covers the dispatched run, so by the time it returns the run has
	// executed.
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, 5*time.Second)).True()
	gt.Value(t, exec.firedJobIDs()).Equal([]string{"a"})
	async.Wait()

	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, time.Now().UTC(), true))
	gt.Value(t, attrs["completed"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["started"].Int64()).Equal(int64(1))
}

// A lifecycle event dispatched while a sweep's context is in scope belongs to
// neither the sweep's wait nor its counts: it was never one of the due pairs.
func TestPublish_LifecycleRunsAreNotPartOfTheSweep(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "on_created",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})
	repo, c := setupCase(t, "ws")
	exec := &recordingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
	})
	uc := job.NewUseCase(registry, runner)

	stats := job.NewTickStatsForTest(time.Now().UTC())
	ctx := job.WithTickStatsForTest(context.Background(), stats)
	uc.Publish(ctx, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})

	// Nothing was registered, so the sweep's wait completes immediately even
	// though a run is (or was) in flight.
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, time.Second)).True()
	async.Wait()
	gt.Value(t, exec.firedJobIDs()).Equal([]string{"on_created"})

	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, time.Now().UTC(), true))
	gt.Value(t, attrs["due_total"].Int64()).Equal(int64(0))
	gt.Value(t, attrs["completed"].Int64()).Equal(int64(0))
	gt.Value(t, attrs["started"].Int64()).Equal(int64(0))
}

// Outside a sweep the collector is absent, and dispatch must behave exactly as
// before.
func TestPublish_WithoutSweepContext(t *testing.T) {
	uc, exec, caseID := scheduledJobsFixture(t, "a")

	uc.Publish(context.Background(), job.Event{
		Domain:      model.JobEventDomainScheduled,
		WorkspaceID: "ws",
		CaseID:      caseID,
		Timestamp:   time.Now().UTC(),
		ActorUserID: model.SystemActorID,
		JobID:       "a",
	})
	async.Wait()
	gt.Value(t, exec.firedJobIDs()).Equal([]string{"a"})
}
