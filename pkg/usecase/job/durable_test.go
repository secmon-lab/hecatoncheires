package job_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// A one-shot sweep must execute the runs it dispatched before it exits.
//
// `Run` returns as soon as a run is recorded, so a batch command that only
// dispatched would leave the work for a worker that may not be running — a
// scheduled sweep would silently stop executing anything. Drain is what makes the
// sweep own its runs: it drives the worker in the foreground until every run it
// started is terminal.
func TestDurableRuntime_DrainExecutesTheRunsTheSweepDispatched(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()

	jobs := make([]*model.Job, 0, 2)
	for i := range 2 {
		jobs = append(jobs, &model.Job{
			ID:     fmt.Sprintf("swept-%d", i),
			Prompt: "summarise the case",
			Events: model.JobEvents{
				Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
			},
		})
	}
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: jobs})

	llm := singleReplyLLM("the case looks fine", 120, 34)
	durable := &job.DurableRuntime{History: agentarchive.NewMemoryHistoryStore()}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: llm, Durable: durable,
	})
	// No worker: Drain is the only thing that can move these runs, which is exactly
	// the situation `hecatoncheires tick` is in.
	bindDurableJobRuntime(t, runner, durable, repo, registry, llm)
	durable.TrackSpawns()

	for _, j := range jobs {
		gt.NoError(t, runner.Run(ctx, j, job.Event{
			Domain: model.JobEventDomainScheduled, WorkspaceID: "ws", CaseID: c.ID,
			Timestamp: time.Now().UTC(),
		})).Required()
	}

	// Nothing has executed yet: the runs are recorded and waiting.
	for _, j := range jobs {
		key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
		logs, err := repo.JobRunLog().List(ctx, key, 10)
		gt.NoError(t, err).Required()
		gt.Array(t, logs).Length(1).Required()
		gt.Value(t, logs[0].Stage).Equal(model.JobRunStageRunning)
	}

	gt.NoError(t, durable.Drain(ctx)).Required()

	// Drain returned only once every run reached its outcome, so the results are
	// readable without waiting for anything.
	for _, j := range jobs {
		key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
		run, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, run).NotNil().Required()
		gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)

		logs, err := repo.JobRunLog().List(ctx, key, 10)
		gt.NoError(t, err).Required()
		gt.Array(t, logs).Length(1).Required()
		gt.Value(t, logs[0].Stage).Equal(model.JobRunStageSuccess)
	}
}

// Drain waits for the runs THIS process started, not for whatever else is in the
// store. A sweep that waited on another instance's runs would hang for as long as
// they take, and a sweep that tracked nothing would exit having executed nothing.
func TestDurableRuntime_DrainWithoutTrackingWaitsForNothing(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "untracked",
		Prompt: "summarise the case",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	llm := singleReplyLLM("the case looks fine", 120, 34)
	durable := &job.DurableRuntime{History: agentarchive.NewMemoryHistoryStore()}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: llm, Durable: durable,
	})
	bindDurableJobRuntime(t, runner, durable, repo, registry, llm)
	// TrackSpawns deliberately NOT called: this stands in for `serve`, which never
	// drains and must not accumulate a list of every run it has ever started.

	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain: model.JobEventDomainScheduled, WorkspaceID: "ws", CaseID: c.ID,
		Timestamp: time.Now().UTC(),
	})).Required()

	// Returns immediately rather than driving the run, and does not block.
	gt.NoError(t, durable.Drain(ctx)).Required()

	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	logs, err := repo.JobRunLog().List(ctx, key, 10)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(1).Required()
	gt.Value(t, logs[0].Stage).Equal(model.JobRunStageRunning)
}
