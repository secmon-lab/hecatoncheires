package job_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// setupCase creates a workspace-scoped in-memory repo with a single OPEN
// Case ready for Job dispatch tests.
func setupCase(t *testing.T, wsID string) (interfaces.Repository, *model.Case) {
	t.Helper()
	repo := memory.New()
	created, err := repo.Case().Create(context.Background(), wsID, &model.Case{
		Title:      "T",
		Status:     types.CaseStatusOpen,
		ReporterID: "U-REP",
	})
	gt.NoError(t, err).Required()
	return repo, created
}

// setupCaseWithSlack creates an OPEN Case bound to the given Slack channel
// (and optional thread). channelID empty → no Slack binding; threadTS set →
// thread-mode Case.
func setupCaseWithSlack(t *testing.T, wsID, channelID, threadTS string) (interfaces.Repository, *model.Case) {
	t.Helper()
	repo := memory.New()
	created, err := repo.Case().Create(context.Background(), wsID, &model.Case{
		Title:          "T",
		Status:         types.CaseStatusOpen,
		ReporterID:     "U-REP",
		SlackChannelID: channelID,
		SlackThreadTS:  threadTS,
	})
	gt.NoError(t, err).Required()
	return repo, created
}

// inertLLM satisfies gollem.LLMClient but panics if ever called. The
// recordingExecutor short-circuits before the LLM is touched.
func inertLLM() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			panic("LLM should not be called when executor is mocked")
		},
	}
}

func TestJobActorContext(t *testing.T) {
	ctx := context.Background()
	gt.Bool(t, job.IsJobActorContext(ctx)).False()

	ctx = job.WithJobActor(ctx, job.JobActorMarker{JobID: "j-1"})
	gt.Bool(t, job.IsJobActorContext(ctx)).True()

	gt.Bool(t, job.IsJobActorContext(context.TODO())).False()
}

func TestQuietContext(t *testing.T) {
	// Absent marker → not quiet.
	gt.Bool(t, job.IsQuietForTest(context.Background())).False()

	// Explicit true / false round-trip.
	gt.Bool(t, job.IsQuietForTest(job.WithQuietForTest(context.Background(), true))).True()
	gt.Bool(t, job.IsQuietForTest(job.WithQuietForTest(context.Background(), false))).False()

	// nil context is treated as not quiet (no panic).
	gt.Bool(t, job.IsQuietForTest(nil)).False()
}

// recordingExecutor counts how many times it was called and what prompts
// it received. It returns success without exercising the LLM.
type recordingExecutor struct {
	calls atomic.Int32
}

func (r *recordingExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	r.calls.Add(1)
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

func TestPublish_DispatchesMatchingJobs(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws", Name: "WS"},
		Jobs:      []*model.Job{j},
	})

	repo, c := setupCase(t, "ws")

	exec := &recordingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:        repo,
		Registry:    registry,
		LLMClient:   inertLLM(),
		Executors:   map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		ToolBuilder: nil,
	})
	uc := job.NewUseCase(registry, runner)

	uc.Publish(context.Background(), job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	async.Wait()
	gt.Number(t, exec.calls.Load()).Equal(int32(1))
}

func TestPublish_SuppressesOnlyOriginatingJob(t *testing.T) {
	// The originating Job (whose agent performed the write) must not re-fire
	// itself, but a DIFFERENT Job listening on the same lifecycle event still
	// fires — so an on-created Job that closes the case can trigger the
	// on-closed Job.
	newFixture := func(t *testing.T) (*job.UseCase, *recordingExecutor, *model.Case) {
		t.Helper()
		registry := model.NewWorkspaceRegistry()
		summarize := &model.Job{
			ID:     "summarize",
			Prompt: "x",
			Events: model.JobEvents{
				Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleClosed}},
			},
		}
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws"},
			Jobs:      []*model.Job{summarize},
		})

		repo, c := setupCase(t, "ws")
		exec := &recordingExecutor{}
		runner := job.NewJobRunner(job.RunnerDeps{
			Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		})
		return job.NewUseCase(registry, runner), exec, c
	}

	closedEvent := func(c *model.Case) job.Event {
		return job.Event{
			Domain:        model.JobEventDomainCase,
			WorkspaceID:   "ws",
			CaseID:        c.ID,
			CaseLifecycle: model.CaseLifecycleClosed,
		}
	}

	t.Run("same job id is suppressed", func(t *testing.T) {
		uc, exec, c := newFixture(t)
		ctx := job.WithJobActor(context.Background(), job.JobActorMarker{JobID: "summarize"})
		uc.Publish(ctx, closedEvent(c))
		async.Wait()
		gt.Number(t, exec.calls.Load()).Equal(int32(0))
	})

	t.Run("different job id still fires", func(t *testing.T) {
		uc, exec, c := newFixture(t)
		ctx := job.WithJobActor(context.Background(), job.JobActorMarker{JobID: "other"})
		uc.Publish(ctx, closedEvent(c))
		async.Wait()
		gt.Number(t, exec.calls.Load()).Equal(int32(1))
	})

	t.Run("no actor marker fires", func(t *testing.T) {
		uc, exec, c := newFixture(t)
		uc.Publish(context.Background(), closedEvent(c))
		async.Wait()
		gt.Number(t, exec.calls.Load()).Equal(int32(1))
	})
}

func TestPublish_IgnoresNonMatchingJobs(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs: []*model.Job{
			{
				ID:     "scheduled-only",
				Prompt: "x",
				Events: model.JobEvents{
					Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
				},
			},
		},
	})

	repo, c := setupCase(t, "ws")

	exec := &recordingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
	})
	uc := job.NewUseCase(registry, runner)

	uc.Publish(context.Background(), job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	async.Wait()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
}

func TestPublish_IgnoresUnknownWorkspace(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	repo, _ := setupCase(t, "ws-a")

	exec := &recordingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
	})
	uc := job.NewUseCase(registry, runner)
	uc.Publish(context.Background(), job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws-b", // not registered
		CaseID:        1,
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	async.Wait()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
}
