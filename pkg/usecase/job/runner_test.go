package job_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

func newRunner(t *testing.T, wsID string, jobs []*model.Job, exec jobagent.JobExecutor) (*job.JobRunner, *model.WorkspaceRegistry, *model.Case) {
	t.Helper()
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      jobs,
	})

	r := job.NewJobRunner(job.RunnerDeps{
		Repo:      repo,
		Registry:  registry,
		LLMClient: inertLLM(),
		Executor:  exec,
	})
	_ = repo
	return r, registry, c
}

func TestJobRunner_HappyPath(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "summary for {{.Case.Title}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	runner, _, c := newRunner(t, "ws", []*model.Job{j}, exec)

	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.NoError(t, err).Required()
	gt.Number(t, exec.calls.Load()).Equal(int32(1))
}

func TestJobRunner_SkipsWhenLeaseHeld(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	// Pre-acquire the lease so the runner sees it held.
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	got, err := repo.JobRun().TryAcquireLease(context.Background(), key, time.Now().UTC(), 5*time.Minute)
	gt.NoError(t, err).Required()
	gt.Bool(t, got).True()

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	err = runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.NoError(t, err).Required()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
}

// failingExecutor lets the runner record a failure path.
type failingExecutor struct {
	calls atomic.Int32
	err   error
}

func (f *failingExecutor) Execute(_ context.Context, _ jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	f.calls.Add(1)
	return nil, f.err
}

func TestJobRunner_FailureIsRecorded(t *testing.T) {
	sentinel := goerr.New("llm down")
	exec := &failingExecutor{err: sentinel}
	j := &model.Job{
		ID:     "fail-job",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err).Is(sentinel)

	run, err := repo.JobRun().Get(context.Background(), model.JobRunKey{
		WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, run.LastError).Contains("llm down")
}

func TestJobRunner_SuccessClearsLease(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	gt.NoError(t, runner.Run(context.Background(), j, job.Event{
		Domain: model.JobEventDomainCase, WorkspaceID: "ws", CaseID: c.ID,
		Timestamp: time.Now().UTC(), CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	run, err := repo.JobRun().Get(context.Background(), model.JobRunKey{
		WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.Bool(t, run.LeaseUntil.IsZero()).True()
}

func TestJobRunner_InvalidJob(t *testing.T) {
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:     nil, // unreachable: validation fires first
		Registry: model.NewWorkspaceRegistry(),
		Executor: &recordingExecutor{},
	})
	err := runner.Run(context.Background(), &model.Job{}, job.Event{})
	gt.Error(t, err)
}

// toolCapturingExecutor records the resolved tool list it was handed so
// the test can assert the ToolBuilder ran.
type toolCapturingExecutor struct {
	tools []gollem.Tool
}

func (e *toolCapturingExecutor) Execute(_ context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.tools = req.Tools
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

func TestJobRunner_PassesBuilderTools(t *testing.T) {
	exec := &toolCapturingExecutor{}
	j := &model.Job{
		ID:     "with-tools",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	stubTool := &stubTool{name: "stub__t"}
	builder := job.ToolBuilderFunc(func(_ context.Context, _ *model.Case, _ *model.WorkspaceEntry) []gollem.Tool {
		return []gollem.Tool{stubTool}
	})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
		ToolBuilder: builder,
	})
	gt.NoError(t, runner.Run(context.Background(), j, job.Event{
		Domain: model.JobEventDomainCase, WorkspaceID: "ws", CaseID: c.ID,
		Timestamp: time.Now().UTC(), CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()
	gt.Array(t, exec.tools).Length(1).Required()
	gt.String(t, exec.tools[0].Spec().Name).Equal("stub__t")
}

type stubTool struct {
	name string
}

func (s *stubTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: s.name, Description: "stub"}
}
func (s *stubTool) Run(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, errors.New("stub not invoked in test")
}
