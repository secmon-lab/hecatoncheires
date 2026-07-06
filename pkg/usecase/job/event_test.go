package job_test

import (
	"context"
	"sync"
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

func TestQuietContext(t *testing.T) {
	// Absent marker → not quiet.
	gt.Bool(t, job.IsQuietForTest(context.Background())).False()

	// Explicit true / false round-trip.
	gt.Bool(t, job.IsQuietForTest(job.WithQuietForTest(context.Background(), true))).True()
	gt.Bool(t, job.IsQuietForTest(job.WithQuietForTest(context.Background(), false))).False()

	// nil context is treated as not quiet (no panic).
	gt.Bool(t, job.IsQuietForTest(nil)).False()
}

// recordingExecutor counts how many times it was called and records the
// JobID of every Job it ran, so tests can assert not just how many Jobs
// fired but which ones. It returns success without exercising the LLM.
type recordingExecutor struct {
	calls  atomic.Int32
	mu     sync.Mutex
	jobIDs []string
}

func (r *recordingExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	r.calls.Add(1)
	r.mu.Lock()
	r.jobIDs = append(r.jobIDs, req.JobID)
	r.mu.Unlock()
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// firedJobIDs returns a copy of the recorded JobIDs, safe to read after
// async.Wait().
func (r *recordingExecutor) firedJobIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.jobIDs))
	copy(out, r.jobIDs)
	return out
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
		gt.Array(t, exec.firedJobIDs()).Equal([]string{"summarize"})
	})

	t.Run("no actor marker fires", func(t *testing.T) {
		uc, exec, c := newFixture(t)
		uc.Publish(context.Background(), closedEvent(c))
		async.Wait()
		gt.Array(t, exec.firedJobIDs()).Equal([]string{"summarize"})
	})

	// The core of this fix: when the originating Job and a sibling BOTH match
	// the same lifecycle in one Publish, only the originator is suppressed and
	// the sibling still fires. This is the on-created-closes-case →
	// on-closed-sibling scenario the blanket guard used to break.
	t.Run("originator suppressed while matching sibling fires", func(t *testing.T) {
		registry := model.NewWorkspaceRegistry()
		originator := &model.Job{
			ID:     "closer",
			Prompt: "x",
			Events: model.JobEvents{
				Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleClosed}},
			},
		}
		sibling := &model.Job{
			ID:     "notifier",
			Prompt: "x",
			Events: model.JobEvents{
				Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleClosed}},
			},
		}
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws"},
			Jobs:      []*model.Job{originator, sibling},
		})
		repo, c := setupCase(t, "ws")
		exec := &recordingExecutor{}
		runner := job.NewJobRunner(job.RunnerDeps{
			Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		})
		uc := job.NewUseCase(registry, runner)

		ctx := job.WithJobActor(context.Background(), job.JobActorMarker{JobID: "closer"})
		uc.Publish(ctx, closedEvent(c))
		async.Wait()
		gt.Array(t, exec.firedJobIDs()).Equal([]string{"notifier"})
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
