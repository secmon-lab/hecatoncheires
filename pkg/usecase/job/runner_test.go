package job_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
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
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
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

// TestJobRunner_RunManual covers the web-UI trigger path: the Job is
// resolved from the registry by id, the run is tagged with the manual
// provenance, and a Job that is absent or disabled is refused without
// producing a run log.
func TestJobRunner_RunManual(t *testing.T) {
	newManualRunner := func(t *testing.T, jobs []*model.Job) (*job.JobRunner, interfaces.Repository, *model.Case, *recordingExecutor) {
		t.Helper()
		exec := &recordingExecutor{}
		repo, c := setupCase(t, "ws")
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: jobs})
		runner := job.NewJobRunner(job.RunnerDeps{
			Repo: repo, Registry: registry, LLMClient: inertLLM(),
			Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		})
		return runner, repo, c, exec
	}

	scheduledJob := func(id string, disabled bool) *model.Job {
		return &model.Job{
			ID:       id,
			Prompt:   "review {{.Case.Title}}",
			Disabled: disabled,
			Events: model.JobEvents{
				Scheduled: &model.ScheduledEventConfig{Every: 24 * time.Hour},
			},
		}
	}

	t.Run("runs the job and records manual provenance", func(t *testing.T) {
		j := scheduledJob("daily_review", false)
		runner, repo, c, exec := newManualRunner(t, []*model.Job{j})

		err := runner.RunManual(context.Background(), "ws", c.ID, j.ID, "U-OPERATOR")
		gt.NoError(t, err).Required()
		gt.Number(t, exec.calls.Load()).Equal(int32(1))

		key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
		logs, listErr := repo.JobRunLog().List(context.Background(), key, 0)
		gt.NoError(t, listErr).Required()
		gt.Array(t, logs).Length(1).Required()
		gt.Value(t, logs[0].Stage).Equal(model.JobRunStageSuccess)
		gt.String(t, logs[0].EventType).Equal("manual")
		gt.String(t, logs[0].JobID).Equal(j.ID)

		// The manual event is the run's trigger, so its timestamp must be
		// populated rather than left at the zero value.
		gt.Bool(t, logs[0].EventTriggerAt.IsZero()).False()

		run, getErr := repo.JobRun().Get(context.Background(), key)
		gt.NoError(t, getErr).Required()
		gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)
	})

	t.Run("refuses an unknown job id", func(t *testing.T) {
		j := scheduledJob("daily_review", false)
		runner, repo, c, exec := newManualRunner(t, []*model.Job{j})

		err := runner.RunManual(context.Background(), "ws", c.ID, "no_such_job", "U-OPERATOR")
		gt.Value(t, err).NotNil()
		gt.Number(t, exec.calls.Load()).Equal(int32(0))

		runs, listErr := repo.JobRun().ListByCase(context.Background(), "ws", c.ID)
		gt.NoError(t, listErr).Required()
		gt.Array(t, runs).Length(0)
	})

	t.Run("refuses a disabled job", func(t *testing.T) {
		j := scheduledJob("daily_review", true)
		runner, repo, c, exec := newManualRunner(t, []*model.Job{j})

		err := runner.RunManual(context.Background(), "ws", c.ID, j.ID, "U-OPERATOR")
		gt.Value(t, err).NotNil()
		gt.Number(t, exec.calls.Load()).Equal(int32(0))

		runs, listErr := repo.JobRun().ListByCase(context.Background(), "ws", c.ID)
		gt.NoError(t, listErr).Required()
		gt.Array(t, runs).Length(0)
	})

	t.Run("refuses an unknown workspace", func(t *testing.T) {
		j := scheduledJob("daily_review", false)
		runner, _, c, exec := newManualRunner(t, []*model.Job{j})

		err := runner.RunManual(context.Background(), "other-ws", c.ID, j.ID, "U-OPERATOR")
		gt.Value(t, err).NotNil()
		gt.Number(t, exec.calls.Load()).Equal(int32(0))
	})
}

// TestJobRunner_CanRunManual pins the admission rule behind the web UI's Run
// button. The rule must match what Run itself does: a live lease or a
// genuinely open question blocks a new run, while a stale or inconsistent
// suspension marker — which Run recovers from — must not.
func TestJobRunner_CanRunManual(t *testing.T) {
	const unansweredTimeout = 30 * time.Minute
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	setup := func(t *testing.T) (*job.JobRunner, interfaces.Repository, model.JobRunKey) {
		t.Helper()
		j := &model.Job{
			ID:          "ask_first",
			Prompt:      "x",
			Strategy:    model.JobStrategyPlanexec,
			Interactive: true,
			Events: model.JobEvents{
				Scheduled: &model.ScheduledEventConfig{Every: 24 * time.Hour},
			},
		}
		repo, c := setupCase(t, "ws")
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})
		runner := job.NewJobRunner(job.RunnerDeps{
			Repo: repo, Registry: registry, LLMClient: inertLLM(),
			Executors:         map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: &recordingExecutor{}},
			UnansweredTimeout: unansweredTimeout,
			Clock:             func() time.Time { return now },
		})
		return runner, repo, model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	}

	// seedAwaitingInput writes the AWAITING_INPUT run log a live suspension
	// points at, so suspensionIsActive can confirm the question is open.
	seedAwaitingInput := func(t *testing.T, repo interfaces.Repository, key model.JobRunKey, runID string) {
		t.Helper()
		log := &model.JobRunLog{
			WorkspaceID:  key.WorkspaceID,
			CaseID:       key.CaseID,
			JobID:        key.JobID,
			RunID:        runID,
			TraceID:      "trace-" + runID,
			Stage:        model.JobRunStageRunning,
			StartedAt:    now.Add(-time.Minute),
			ExecutorKind: model.ExecutorKindPlanexec,
		}
		gt.NoError(t, repo.JobRunLog().Create(context.Background(), log)).Required()
		log.Stage = model.JobRunStageAwaitingInput
		log.PendingInteraction = &model.PendingInteraction{
			PostedChannelID: "C1",
			PostedMessageTS: "1700000000.000100",
			Reason:          "need input",
			Items: []model.PendingInteractionItem{
				{ID: "q1", Text: "Which one?", Type: "free_text"},
			},
		}
		gt.NoError(t, repo.JobRunLog().Suspend(context.Background(), log)).Required()
	}

	t.Run("admits a job that has never run", func(t *testing.T) {
		runner, _, key := setup(t)
		ok, err := runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		gt.Bool(t, ok).True()
	})

	t.Run("refuses while a lease is live and admits once it expires", func(t *testing.T) {
		runner, repo, key := setup(t)
		acquired, err := repo.JobRun().TryAcquireLease(context.Background(), key, now, 10*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		ok, err := runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		gt.Bool(t, ok).False()

		gt.NoError(t, repo.JobRun().ReleaseLease(context.Background(), key)).Required()
		ok, err = runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		gt.Bool(t, ok).True()
	})

	t.Run("refuses while a question is genuinely open", func(t *testing.T) {
		runner, repo, key := setup(t)
		seedAwaitingInput(t, repo, key, "run-open")
		gt.NoError(t, repo.JobRun().Suspend(context.Background(), key, "run-open", now.Add(-time.Minute))).Required()

		ok, err := runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		gt.Bool(t, ok).False()
	})

	t.Run("admits when the question went unanswered past the timeout", func(t *testing.T) {
		runner, repo, key := setup(t)
		seedAwaitingInput(t, repo, key, "run-stale")
		gt.NoError(t, repo.JobRun().Suspend(context.Background(), key, "run-stale", now.Add(-unansweredTimeout-time.Minute))).Required()

		ok, err := runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		// Run itself recovers this marker, so the admission check must not
		// leave the manual path blocked on it.
		gt.Bool(t, ok).True()
	})

	t.Run("admits when the suspension marker points at no run log", func(t *testing.T) {
		runner, repo, key := setup(t)
		// A resume that crashed before writing its log leaves the marker with
		// nothing behind it.
		gt.NoError(t, repo.JobRun().Suspend(context.Background(), key, "run-vanished", now.Add(-time.Minute))).Required()

		ok, err := runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, key.JobID)
		gt.NoError(t, err).Required()
		gt.Bool(t, ok).True()
	})

	t.Run("rejects an incomplete key", func(t *testing.T) {
		runner, _, key := setup(t)
		_, err := runner.CanRunManual(context.Background(), "", key.CaseID, key.JobID)
		gt.Value(t, err).NotNil()
		_, err = runner.CanRunManual(context.Background(), key.WorkspaceID, 0, key.JobID)
		gt.Value(t, err).NotNil()
		_, err = runner.CanRunManual(context.Background(), key.WorkspaceID, key.CaseID, "")
		gt.Value(t, err).NotNil()
	})
}

// TestJobRunner_StrategyDispatchPicksRegisteredExecutor verifies that
// the runner picks the executor that matches Job.Strategy at Run time
// and writes the matching ExecutorKind onto the JobRunLog.
func TestJobRunner_StrategyDispatchPicksRegisteredExecutor(t *testing.T) {
	simpleExec := &recordingExecutor{}
	planexecExec := &recordingExecutor{}

	j := &model.Job{
		ID:       "planexec-job",
		Prompt:   "x",
		Strategy: model.JobStrategyPlanexec,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{
			model.JobStrategySimple:   simpleExec,
			model.JobStrategyPlanexec: planexecExec,
		},
	})
	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.NoError(t, err).Required()
	gt.Number(t, planexecExec.calls.Load()).Equal(int32(1))
	gt.Number(t, simpleExec.calls.Load()).Equal(int32(0))

	// ExecutorKind on the persisted JobRunLog reflects the chosen
	// strategy. Read it back through the repository (List for the
	// (workspace, case, job) key).
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	logs, listErr := repo.JobRunLog().List(context.Background(), key, 0)
	gt.NoError(t, listErr).Required()
	gt.Array(t, logs).Length(1).Required()
	gt.String(t, logs[0].ExecutorKind).Equal("plan_execute")
}

// TestJobRunner_StrategyDispatchFailsWhenExecutorMissing verifies that
// running a planexec-strategy Job without a registered executor records
// a prepare-stage failure rather than panicking.
func TestJobRunner_StrategyDispatchFailsWhenExecutorMissing(t *testing.T) {
	j := &model.Job{
		ID:       "planexec-job",
		Prompt:   "x",
		Strategy: model.JobStrategyPlanexec,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{
			model.JobStrategySimple: &recordingExecutor{},
			// JobStrategyPlanexec deliberately absent.
		},
	})
	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err)
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
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
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
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
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
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
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
		Repo:      nil, // unreachable: validation fires first
		Registry:  model.NewWorkspaceRegistry(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: &recordingExecutor{}},
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
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
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

// scriptedRunnerExecutor lets a test seed a list of trace events the
// executor will replay through the handler, simulating what a real
// gollem agent loop would produce. It also forwards an optional
// terminal error from the agent loop.
type scriptedRunnerExecutor struct {
	emit       func(ctx context.Context, h *job.JobRunTraceHandlerForTest)
	terminate  error
	gotRequest *jobagent.ExecuteRequest
}

func (e *scriptedRunnerExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.gotRequest = &req
	if e.emit != nil && req.TraceHandler != nil {
		h, ok := req.TraceHandler.(*job.JobRunTraceHandlerForTest)
		if !ok {
			return nil, errors.New("scriptedRunnerExecutor: TraceHandler is not jobRunTraceHandler")
		}
		e.emit(ctx, h)
	}
	if e.terminate != nil {
		return nil, e.terminate
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// TestJobRunner_GoldenPath drives a complete success Run with a
// scripted agent loop and asserts the *entire* contents of JobRunLog,
// JobRunEvent list, and JobRun lock doc field-by-field. This is the
// canonical Layer 5 test for the trace persistence contract.
func TestJobRunner_GoldenPath(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-golden"
	j := &model.Job{
		ID:     "summarize",
		Prompt: "summary for {{.Case.Title}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})

	fixedT := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	runID := "run-fixed-id"
	traceID := "trace-fixed-id"
	triggeredAt := fixedT.Add(-time.Second)

	// Scripted agent loop: 1 LLM call + 1 tool call.
	exec := &scriptedRunnerExecutor{
		emit: func(ctx context.Context, h *job.JobRunTraceHandlerForTest) {
			llmCtx := h.StartLLMCall(ctx)
			h.EndLLMCall(llmCtx, &traceLLMCallDataForTest, nil)
			toolCtx := h.StartToolExec(ctx, "slack_search", map[string]any{"q": "foo"})
			h.EndToolExec(toolCtx, map[string]any{"hits": 3}, nil)
		},
	}

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		NewRunID:   func() string { return runID },
		NewTraceID: func() string { return traceID },
		Clock:      func() time.Time { return fixedT },
	})
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     triggeredAt,
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	// Assert JobRunLog: full field check.
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	log, err := repo.JobRunLog().Get(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.String(t, log.WorkspaceID).Equal(wsID)
	gt.Number(t, log.CaseID).Equal(c.ID)
	gt.String(t, log.JobID).Equal(j.ID)
	gt.String(t, log.RunID).Equal(runID)
	gt.String(t, log.TraceID).Equal(traceID)
	gt.Value(t, log.Stage).Equal(model.JobRunStageSuccess)
	gt.Bool(t, log.StartedAt.Equal(fixedT)).True()
	gt.Bool(t, log.EndedAt.Equal(fixedT)).True()
	gt.String(t, log.Error).Equal("")
	gt.String(t, log.ExecutorKind).Equal("single_loop")
	gt.String(t, log.EventType).Equal(string(model.JobEventDomainCase))
	gt.Bool(t, log.EventTriggerAt.Equal(triggeredAt.UTC())).True()
	gt.String(t, log.SystemPrompt).NotEqual("")
	// The run's totals come from the scripted agent loop below — one LLM call
	// (120 in / 60 out) and one tool execution — summed by the trace handler and
	// stamped on the log before Finish.
	gt.Number(t, log.InputTokens).Equal(120)
	gt.Number(t, log.OutputTokens).Equal(60)
	gt.Number(t, log.LLMCallCount).Equal(1)
	gt.Number(t, log.ToolCallCount).Equal(1)

	// Assert event list: LLM_REQUEST -> LLM_RESPONSE -> TOOL_CALL.
	events, err := repo.JobRunEvent().List(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()

	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Number(t, events[0].Sequence).Equal(1)
	gt.String(t, events[0].WorkspaceID).Equal(wsID)
	gt.Number(t, events[0].CaseID).Equal(c.ID)
	gt.String(t, events[0].JobID).Equal(j.ID)
	gt.String(t, events[0].RunID).Equal(runID)
	gt.String(t, events[0].TraceID).Equal(traceID)
	gt.String(t, events[0].Phase).Equal("execute")
	gt.String(t, events[0].LLMRequest.Model).Equal("claude-opus-4-7")
	gt.Array(t, events[0].LLMRequest.Tools).Length(1).Required()
	gt.String(t, events[0].LLMRequest.Tools[0].Name).Equal("slack_search")

	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Number(t, events[1].Sequence).Equal(2)
	gt.Array(t, events[1].LLMResponse.Texts).Length(1).Required()
	gt.String(t, events[1].LLMResponse.Texts[0]).Equal("let me search")
	gt.Array(t, events[1].LLMResponse.FunctionCalls).Length(1).Required()
	gt.String(t, events[1].LLMResponse.FunctionCalls[0].Name).Equal("slack_search")
	gt.Number(t, events[1].LLMResponse.InputTokens).Equal(120)
	gt.Number(t, events[1].LLMResponse.OutputTokens).Equal(60)

	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindToolCall)
	gt.Number(t, events[2].Sequence).Equal(3)
	gt.Number(t, events[2].ParentSequence).Equal(2)
	gt.String(t, events[2].ToolCall.ToolName).Equal("slack_search")
	gt.String(t, events[2].ToolCall.ArgumentsJSON).Equal(`{"q":"foo"}`)
	gt.String(t, events[2].ToolCall.ResultJSON).Equal(`{"hits":3}`)
	gt.Bool(t, events[2].ToolCall.IsError).False()
	gt.String(t, events[2].ToolCall.ErrorMessage).Equal("")
	gt.Bool(t, events[2].ToolCall.StartedAt.Equal(fixedT)).True()
	gt.Bool(t, events[2].ToolCall.EndedAt.Equal(fixedT)).True()

	// Assert JobRun lock doc updates.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.String(t, jr.LastRunID).Equal(runID)
	gt.String(t, jr.LastTraceID).Equal(traceID)
	gt.String(t, jr.LastError).Equal("")
	gt.Bool(t, jr.LastRunAt.Equal(fixedT)).True()
	gt.Bool(t, jr.LeaseUntil.IsZero()).True()
}

// traceLLMCallDataForTest is reused across runner tests to drive the
// handler's EndLLMCall hook with a known LLMCallData shape.
var traceLLMCallDataForTest = trace.LLMCallData{
	Model:        "claude-opus-4-7",
	InputTokens:  120,
	OutputTokens: 60,
	Request: &trace.LLMRequest{
		Messages: []trace.Message{
			{
				Role: "user",
				Contents: []trace.MessageContent{
					{Type: "text", Text: "investigate case"},
				},
			},
		},
		Tools: []trace.ToolSpec{
			{Name: "slack_search", Description: "search slack"},
		},
	},
	Response: &trace.LLMResponse{
		Texts: []string{"let me search"},
		FunctionCalls: []*trace.FunctionCall{
			{ID: "fc-1", Name: "slack_search", Arguments: map[string]any{"q": "foo"}},
		},
	},
}

// interactiveScriptedExecutor drives both halves of an interactive run without
// a live LLM. gollem's trace hooks are invoked by the provider clients, not by
// the agent loop, so a mock LLMClient never reaches EndLLMCall — this fake
// stands in by emitting a known LLMCallData through the trace handler the
// runner supplied, then asking the user via the runner's Interactor.
type interactiveScriptedExecutor struct {
	// firstTokens / secondTokens are emitted through the trace handler on the
	// pre-question turn and the resumed turn respectively.
	firstTokens  trace.LLMCallData
	secondTokens trace.LLMCallData
}

func (e *interactiveScriptedExecutor) emit(ctx context.Context, req jobagent.ExecuteRequest, data *trace.LLMCallData) error {
	h, ok := req.TraceHandler.(*job.JobRunTraceHandlerForTest)
	if !ok {
		return errors.New("interactiveScriptedExecutor: TraceHandler is not the job run trace handler")
	}
	h.EndLLMCall(h.StartLLMCall(ctx), data, nil)
	return nil
}

func (e *interactiveScriptedExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	if err := e.emit(ctx, req, &e.firstTokens); err != nil {
		return nil, err
	}
	if req.Interactor == nil {
		return nil, errors.New("interactiveScriptedExecutor: no interactor wired")
	}
	out, err := req.Interactor.Solicit(ctx, interaction.Request{
		Reason: "which environment?",
		Items: []interaction.Item{
			{ID: "env", Text: "Which environment?", Type: interaction.ItemSelect, Options: []string{"prod", "stg"}},
		},
	})
	if err != nil {
		return nil, err
	}
	if !out.Paused {
		return nil, errors.New("interactiveScriptedExecutor: expected the solicit to pause the run")
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusAwaitingInput}, nil
}

func (e *interactiveScriptedExecutor) Resume(ctx context.Context, req jobagent.ExecuteRequest, _ model.PendingInteraction, _ []interaction.Answer) (*jobagent.ExecuteResult, error) {
	if err := e.emit(ctx, req, &e.secondTokens); err != nil {
		return nil, err
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// TestJobRunner_InteractiveRun_TokenTotalsSpanSuspendAndResume pins the token
// accounting across a suspend/resume boundary. The resumed turn builds a FRESH
// trace handler, so the suspended turn's tokens survive only because Solicit
// persists them and finishRun adds to the persisted value instead of
// overwriting it. Without both halves the run under-reports its cost.
func TestJobRunner_InteractiveRun_TokenTotalsSpanSuspendAndResume(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-interactive-tokens"
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)

	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	j := &model.Job{
		ID:          "interactive_tokens",
		Prompt:      "x",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	gt.NoError(t, j.Validate()).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})

	exec := &interactiveScriptedExecutor{
		firstTokens:  trace.LLMCallData{Model: "m", InputTokens: 500, OutputTokens: 70},
		secondTokens: trace.LLMCallData{Model: "m", InputTokens: 300, OutputTokens: 25},
	}
	poster := &fakeQuestionPoster{returnTS: "FORM-TS-1"}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:              repo,
		Registry:          registry,
		LLMClient:         inertLLM(),
		Executors:         map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategyPlanexec: exec},
		InteractionPoster: poster,
		NewRunID:          func() string { return "RUN-1" },
		NewTraceID:        func() string { return "TRACE-1" },
		Clock:             func() time.Time { return now },
	})

	// --- Turn 1: the run suspends at the question ------------------------
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	suspended, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, suspended.Stage).Equal(model.JobRunStageAwaitingInput)
	// The pre-question turn's tokens are persisted at the suspend, not deferred
	// to the terminal write.
	gt.Number(t, suspended.InputTokens).Equal(500)
	gt.Number(t, suspended.OutputTokens).Equal(70)
	gt.Number(t, suspended.LLMCallCount).Equal(1)

	// --- Turn 2: the user answers and the run resumes to completion ------
	gt.Array(t, poster.posts).Length(1).Required()
	refValue := submitValueFromBlocks(t, poster.posts[0].blocks)
	callback := &goslack.InteractionCallback{
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				"job_question_item:env": {
					"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}},
				},
			},
		},
	}
	callback.Channel.ID = "C-CASE"
	callback.Message.Timestamp = "FORM-TS-1"
	gt.NoError(t, runner.HandleQuestionSubmit(ctx, callback, &goslack.BlockAction{Value: refValue})).Required()

	final, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, final.Stage).Equal(model.JobRunStageSuccess)
	// Both halves are billed to the same record: 500+300 in, 70+25 out, 2 calls.
	gt.Number(t, final.InputTokens).Equal(800)
	gt.Number(t, final.OutputTokens).Equal(95)
	gt.Number(t, final.LLMCallCount).Equal(2)
}

// TestJobRunner_LLMFailure_AppendsRunErrorAndFails verifies that when
// the executor returns an error, the runner emits a RUN_ERROR event
// (with Stage="execute") AND transitions the JobRunLog to FAILED with
// the error message preserved.
func TestJobRunner_LLMFailure_AppendsRunError(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-fail"
	j := &model.Job{
		ID:     "fail-job",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: wsID}, Jobs: []*model.Job{j}})

	fixedT := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	runID := "run-fail-id"
	traceID := "trace-fail-id"
	sentinel := errors.New("llm timeout")

	exec := &scriptedRunnerExecutor{
		emit: func(ctx context.Context, h *job.JobRunTraceHandlerForTest) {
			llmCtx := h.StartLLMCall(ctx)
			h.EndLLMCall(llmCtx, &traceLLMCallDataForTest, nil)
		},
		terminate: sentinel,
	}

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		NewRunID:   func() string { return runID },
		NewTraceID: func() string { return traceID },
		Clock:      func() time.Time { return fixedT },
	})
	err := runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     fixedT,
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err).Is(sentinel)

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}

	// JobRunLog: FAILED with error captured.
	log, err := repo.JobRunLog().Get(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Value(t, log.Stage).Equal(model.JobRunStageFailed)
	gt.String(t, log.Error).Equal("llm timeout")
	gt.Bool(t, log.EndedAt.Equal(fixedT)).True()

	// Events: LLM_REQUEST + LLM_RESPONSE + RUN_ERROR.
	events, err := repo.JobRunEvent().List(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindRunError)
	gt.Number(t, events[2].Sequence).Equal(3)
	gt.String(t, events[2].RunError.Stage).Equal("execute")
	gt.String(t, events[2].RunError.Message).Equal("llm timeout")

	// JobRun lock doc: FAILED with LastRunID/LastTraceID populated.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, jr.LastRunID).Equal(runID)
	gt.String(t, jr.LastTraceID).Equal(traceID)
	gt.String(t, jr.LastError).Equal("llm timeout")
}

// notifierCall records one Slack post made through fakeNotifier.
type notifierCall struct {
	method    string // "root" | "reply"
	channelID string
	threadTS  string
	text      string
}

// fakeNotifier records every job.SlackNotifier call so tests can assert
// count, ordering, and exact field values. Optional errors let tests drive
// the non-fatal failure paths.
type fakeNotifier struct {
	mu       sync.Mutex
	calls    []notifierCall
	rootErr  error
	replyErr error
	rootTS   string
}

func (f *fakeNotifier) PostMessage(_ context.Context, channelID, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifierCall{method: "root", channelID: channelID, text: text})
	if f.rootErr != nil {
		return "", f.rootErr
	}
	if f.rootTS == "" {
		return "root-ts", nil
	}
	return f.rootTS, nil
}

func (f *fakeNotifier) PostThreadReply(_ context.Context, channelID, threadTS, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, notifierCall{method: "reply", channelID: channelID, threadTS: threadTS, text: text})
	if f.replyErr != nil {
		return "", f.replyErr
	}
	return "reply-ts", nil
}

func (f *fakeNotifier) snapshot() []notifierCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]notifierCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// traceDrivingExecutor emits one tool span through the trace handler it
// receives, then optionally returns a terminal error. Tool spans are
// recorded to the Firestore trace handler only — they never surface into
// the Slack session log — so this exercises that the session log carries
// just the lifecycle markers (starting / completed / failed).
type traceDrivingExecutor struct {
	toolName string
	err      error
}

func (e *traceDrivingExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	if req.TraceHandler != nil && e.toolName != "" {
		tctx := req.TraceHandler.StartToolExec(ctx, e.toolName, map[string]any{"q": "x"})
		req.TraceHandler.EndToolExec(tctx, map[string]any{"ok": true}, nil)
	}
	if e.err != nil {
		return nil, e.err
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

func runNotifyJob(t *testing.T, repo interfaces.Repository, wsID string, j *model.Job, c *model.Case, notifier job.SlackNotifier, exec jobagent.JobExecutor) error {
	t.Helper()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: wsID, Name: "WS"}, Jobs: []*model.Job{j}})
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors:     map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		SlackNotifier: notifier,
	})
	return runner.Run(context.Background(), j, job.Event{
		Domain: model.JobEventDomainCase, WorkspaceID: wsID, CaseID: c.ID,
		Timestamp: time.Now().UTC(), CaseLifecycle: model.CaseLifecycleCreated,
	})
}

func notifyJob(id string) *model.Job {
	return &model.Job{
		ID:     id,
		Prompt: "x",
		Events: model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}}},
	}
}

// TestJobRunner_ChannelModeSessionLog: starting marker roots a channel-root
// thread; completion replies into it. Tool spans driven through the trace
// handler do NOT surface into the session log.
func TestJobRunner_ChannelModeSessionLog(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCaseWithSlack(t, "ws", "C1", "")
	j := notifyJob("triage")
	fake := &fakeNotifier{rootTS: "root-123"}

	gt.NoError(t, runNotifyJob(t, repo, "ws", j, c, fake, &traceDrivingExecutor{toolName: "slack_search"})).Required()

	calls := fake.snapshot()
	gt.Array(t, calls).Length(2).Required()

	gt.String(t, calls[0].method).Equal("root")
	gt.String(t, calls[0].channelID).Equal("C1")
	gt.String(t, calls[0].text).Equal(i18n.T(ctx, i18n.MsgJobRunStarting, "triage"))

	gt.String(t, calls[1].method).Equal("reply")
	gt.String(t, calls[1].threadTS).Equal("root-123")
	gt.String(t, calls[1].text).Equal(i18n.T(ctx, i18n.MsgJobRunCompleted, "triage"))
}

// TestJobRunner_ThreadModeSessionLog: thread-mode Case reuses its own thread
// for the starting marker and completion (no root post, no tool lines).
func TestJobRunner_ThreadModeSessionLog(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCaseWithSlack(t, "ws", "Cmon", "TT")
	j := notifyJob("triage")
	fake := &fakeNotifier{}

	gt.NoError(t, runNotifyJob(t, repo, "ws", j, c, fake, &traceDrivingExecutor{toolName: "case_writer"})).Required()

	calls := fake.snapshot()
	gt.Array(t, calls).Length(2).Required()
	for _, call := range calls {
		gt.String(t, call.method).Equal("reply")
		gt.String(t, call.channelID).Equal("Cmon")
		gt.String(t, call.threadTS).Equal("TT")
	}
	gt.String(t, calls[0].text).Equal(i18n.T(ctx, i18n.MsgJobRunStarting, "triage"))
	gt.String(t, calls[1].text).Equal(i18n.T(ctx, i18n.MsgJobRunCompleted, "triage"))
}

// TestJobRunner_QuietSuppressesSessionLog: quiet=true emits no operational
// Slack traffic at all, even with a wired notifier.
func TestJobRunner_QuietSuppressesSessionLog(t *testing.T) {
	repo, c := setupCaseWithSlack(t, "ws", "C1", "")
	j := notifyJob("triage")
	j.Quiet = true
	fake := &fakeNotifier{rootTS: "root-123"}

	gt.NoError(t, runNotifyJob(t, repo, "ws", j, c, fake, &traceDrivingExecutor{toolName: "slack_search"})).Required()
	gt.Array(t, fake.snapshot()).Length(0)
}

// TestJobRunner_StartingPostFailureDegrades: a failed starting-marker post
// disables the session thread but the run still completes successfully.
func TestJobRunner_StartingPostFailureDegrades(t *testing.T) {
	repo, c := setupCaseWithSlack(t, "ws", "C1", "")
	j := notifyJob("triage")
	fake := &fakeNotifier{rootErr: errors.New("slack down")}

	gt.NoError(t, runNotifyJob(t, repo, "ws", j, c, fake, &traceDrivingExecutor{toolName: "slack_search"})).Required()

	// Only the (failed) root attempt happened; no thread replies because the
	// session thread was never established.
	calls := fake.snapshot()
	gt.Array(t, calls).Length(1).Required()
	gt.String(t, calls[0].method).Equal("root")

	// Run still recorded success.
	jr, err := repo.JobRun().Get(context.Background(), model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID})
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusSuccess)
}

// TestJobRunner_FailureMarkerPosted: a failed run posts the failure marker
// (with the error text) into the session thread.
func TestJobRunner_FailureMarkerPosted(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCaseWithSlack(t, "ws", "C1", "")
	j := notifyJob("triage")
	fake := &fakeNotifier{rootTS: "root-123"}
	sentinel := errors.New("boom")

	err := runNotifyJob(t, repo, "ws", j, c, fake, &traceDrivingExecutor{err: sentinel})
	gt.Error(t, err).Is(sentinel)

	calls := fake.snapshot()
	gt.Array(t, calls).Length(2).Required()
	gt.String(t, calls[0].method).Equal("root")
	gt.String(t, calls[1].method).Equal("reply")
	gt.String(t, calls[1].threadTS).Equal("root-123")
	gt.String(t, calls[1].text).Equal(i18n.T(ctx, i18n.MsgJobRunFailed, "triage", "boom"))
}

// TestJobRunner_NilNotifierNoPanic: with no notifier wired the run executes
// (and emits tool spans) without panicking or posting.
func TestJobRunner_NilNotifierNoPanic(t *testing.T) {
	repo, c := setupCaseWithSlack(t, "ws", "C1", "")
	j := notifyJob("triage")
	gt.NoError(t, runNotifyJob(t, repo, "ws", j, c, nil, &traceDrivingExecutor{toolName: "slack_search"})).Required()
}

// TestJobRunner_WorkspaceLoadFailure_NoLog asserts that prepare-stage
// failures (here: missing workspace) do NOT leave a JobRunLog behind.
// The JobRun lock doc still records FAILED for the lifecycle, but no
// RunID was ever allocated so events are not attributable.
func TestJobRunner_WorkspaceLoadFailure_NoLog(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-missing"
	j := &model.Job{
		ID:     "j",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	// Note: NewWorkspaceRegistry is empty — Registry.Get returns an error.
	registry := model.NewWorkspaceRegistry()

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: &recordingExecutor{}},
	})
	err := runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err)

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}

	// No JobRunLog written.
	logs, err := repo.JobRunLog().List(ctx, key, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(0)

	// JobRun lock doc transitioned to FAILED.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, jr.LastRunID).Equal("")
	gt.String(t, jr.LastTraceID).Equal("")
}

// fakeReflector records every Reflect call it receives.
type fakeReflector struct {
	calls []jobagent.ReflectRequest
	err   error
	// emitTokens, when non-nil, is pushed through the trace handler the runner
	// handed to the reflection pass. It stands in for the reflection agent's own
	// LLM calls, which a mock LLMClient never reports (gollem's trace hooks live
	// in the provider clients, not the agent loop).
	emitTokens *trace.LLMCallData
}

func (f *fakeReflector) Reflect(ctx context.Context, req jobagent.ReflectRequest) error {
	f.calls = append(f.calls, req)
	if f.emitTokens != nil {
		h, ok := req.TraceHandler.(*job.JobRunTraceHandlerForTest)
		if !ok {
			return errors.New("fakeReflector: TraceHandler is not the job run trace handler")
		}
		h.EndLLMCall(h.StartLLMCall(ctx), f.emitTokens, nil)
	}
	return f.err
}

// historyWritingExecutor is an executor that saves a non-nil history to the
// HistoryRepository before returning success. This is necessary because
// maybeReflect skips reflection when the loaded history is nil.
type historyWritingExecutor struct {
	calls atomic.Int32
	// emitTokens, when non-nil, is pushed through the run's trace handler so a
	// test can distinguish the executor's token usage from the reflection pass's.
	emitTokens *trace.LLMCallData
}

func (e *historyWritingExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.calls.Add(1)
	if e.emitTokens != nil {
		h, ok := req.TraceHandler.(*job.JobRunTraceHandlerForTest)
		if !ok {
			return nil, errors.New("historyWritingExecutor: TraceHandler is not the job run trace handler")
		}
		h.EndLLMCall(h.StartLLMCall(ctx), e.emitTokens, nil)
	}
	if req.HistoryRepository != nil && req.HistoryKey != "" {
		// Save a minimal non-nil history so maybeReflect can load it.
		if err := req.HistoryRepository.Save(ctx, req.HistoryKey, &gollem.History{
			Version: gollem.HistoryVersion,
		}); err != nil {
			return nil, err
		}
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

func reflectionJob(id string, reflection bool) *model.Job {
	return &model.Job{
		ID:         id,
		Prompt:     "x",
		Reflection: reflection,
		Events:     model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}}},
	}
}

func runReflectionJob(
	t *testing.T,
	wsID string,
	j *model.Job,
	c *model.Case,
	repo interfaces.Repository,
	reflector jobagent.Reflector,
	histRepo gollem.HistoryRepository,
	exec jobagent.JobExecutor,
) error {
	t.Helper()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:        repo,
		Registry:    registry,
		LLMClient:   inertLLM(),
		Executors:   map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		Reflector:   reflector,
		HistoryRepo: histRepo,
	})
	return runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
}

// TestJobRunner_Reflection_CalledOnSuccess verifies that when reflection=true
// and the executor succeeds, the reflector is invoked with the correct
// WorkspaceID, CaseID, JobID, and a non-nil History.
func TestJobRunner_Reflection_CalledOnSuccess(t *testing.T) {
	wsID := "ws-reflect"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{}

	err := runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)
	gt.NoError(t, err).Required()

	gt.Array(t, fake.calls).Length(1).Required()
	gt.String(t, fake.calls[0].WorkspaceID).Equal(wsID)
	gt.Number(t, fake.calls[0].CaseID).Equal(c.ID)
	gt.String(t, fake.calls[0].JobID).Equal(j.ID)
	gt.Value(t, fake.calls[0].History).NotNil()
}

// TestJobRunner_Reflection_TokensIncludeReflectionPass pins that the run's
// persisted token totals cover the reflection agent too. Reflection shares the
// run's trace handler and runs AFTER the executor returns, so reading the
// totals any earlier than the terminal write would silently drop its calls.
func TestJobRunner_Reflection_TokensIncludeReflectionPass(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-reflect-tokens"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{emitTokens: &trace.LLMCallData{Model: "m", InputTokens: 40, OutputTokens: 9}}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{emitTokens: &trace.LLMCallData{Model: "m", InputTokens: 200, OutputTokens: 30}}

	gt.NoError(t, runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)).Required()
	gt.Array(t, fake.calls).Length(1).Required()

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	logs, err := repo.JobRunLog().List(ctx, key, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(1).Required()
	// The executor's call (200/30) plus the reflection call (40/9).
	gt.Number(t, logs[0].InputTokens).Equal(240)
	gt.Number(t, logs[0].OutputTokens).Equal(39)
	gt.Number(t, logs[0].LLMCallCount).Equal(2)
}

// TestJobRunner_Reflection_SkippedWhenFlagFalse verifies that a job with
// reflection=false never invokes the reflector even when all deps are wired.
func TestJobRunner_Reflection_SkippedWhenFlagFalse(t *testing.T) {
	wsID := "ws-no-reflect"
	j := reflectionJob("summarize", false)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{}

	err := runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)
	gt.NoError(t, err).Required()
	gt.Array(t, fake.calls).Length(0)
}

// TestJobRunner_Reflection_SkippedForPrivateCase verifies that reflection is
// not triggered for a private case (IsPrivate=true), since private case
// contents must not leak into shared workspace knowledge.
func TestJobRunner_Reflection_SkippedForPrivateCase(t *testing.T) {
	wsID := "ws-private"
	j := reflectionJob("summarize", true)
	repo := memory.New() // from event_test.go helpers (uses memory import)
	created, err := repo.Case().Create(context.Background(), wsID, &model.Case{
		Title:      "Private",
		Status:     types.CaseStatusOpen,
		ReporterID: "U-REP",
		IsPrivate:  true,
	})
	gt.NoError(t, err).Required()
	c := created

	fake := &fakeReflector{}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{}

	err = runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)
	gt.NoError(t, err).Required()
	gt.Array(t, fake.calls).Length(0)
}

// TestJobRunner_Reflection_SkippedWhenReflectorNil verifies that a nil
// Reflector causes no panic and reflection is simply skipped.
func TestJobRunner_Reflection_SkippedWhenReflectorNil(t *testing.T) {
	wsID := "ws-nil-reflector"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{}

	// Pass nil Reflector.
	err := runReflectionJob(t, wsID, j, c, repo, nil, histRepo, exec)
	gt.NoError(t, err).Required()
}

// TestJobRunner_Reflection_SkippedWhenHistoryRepoNil verifies that a nil
// HistoryRepo prevents reflection (there is no history to load).
func TestJobRunner_Reflection_SkippedWhenHistoryRepoNil(t *testing.T) {
	wsID := "ws-nil-history"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{}
	exec := &historyWritingExecutor{}

	// Pass nil HistoryRepo.
	err := runReflectionJob(t, wsID, j, c, repo, fake, nil, exec)
	gt.NoError(t, err).Required()
	gt.Array(t, fake.calls).Length(0)
}

// TestJobRunner_Reflection_SkippedOnExecutorFailure verifies that when the
// executor fails, reflection is not attempted (reflection only runs on success).
func TestJobRunner_Reflection_SkippedOnExecutorFailure(t *testing.T) {
	wsID := "ws-exec-fail"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	sentinel := errors.New("executor failed")
	exec := &failingExecutor{err: sentinel}

	err := runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)
	gt.Error(t, err).Is(sentinel)
	gt.Array(t, fake.calls).Length(0)
}

// TestJobRunner_Reflection_ErrorIsNonFatal verifies that when the reflector
// returns an error, the run is still recorded as SUCCESS (reflection errors are
// non-fatal by design).
func TestJobRunner_Reflection_ErrorIsNonFatal(t *testing.T) {
	wsID := "ws-reflect-fail"
	j := reflectionJob("summarize", true)
	repo, c := setupCase(t, wsID)

	fake := &fakeReflector{err: errors.New("reflection exploded")}
	histRepo := agentarchive.NewMemoryHistoryRepository()
	exec := &historyWritingExecutor{}

	// Run must succeed even though the reflector returned an error.
	err := runReflectionJob(t, wsID, j, c, repo, fake, histRepo, exec)
	gt.NoError(t, err).Required()

	// Reflector was called.
	gt.Array(t, fake.calls).Length(1)

	// JobRun lock doc still records success.
	jr, getErr := repo.JobRun().Get(context.Background(), model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID})
	gt.NoError(t, getErr).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusSuccess)
}

// capturingExecutor records the SystemPrompt of the last Execute call so a
// test can assert on the prompt the runner assembled and handed to the agent.
type capturingExecutor struct {
	systemPrompt string
}

func (e *capturingExecutor) Execute(_ context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.systemPrompt = req.SystemPrompt
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// putMsg persists one case-scoped Slack message with an explicit CreatedAt so
// tests can place messages inside or outside the 24h window deterministically.
func putMsg(t *testing.T, repo interfaces.Repository, wsID string, caseID int64, text string, createdAt time.Time) {
	t.Helper()
	m := slack.NewMessageFromData(createdAt.Format("20060102.150405"), "C-CASE", "1700000000.0001", "T1", "U-1", "Alice", text, "", createdAt, nil)
	gt.NoError(t, repo.CaseMessage().Put(context.Background(), wsID, caseID, m)).Required()
}

// TestJobRunner_ThreadModeIncludesRecentMessages verifies that a thread-mode
// Job's system prompt embeds the case thread's recent messages, bounded to the
// last 24h and the newest 32, oldest-first, with out-of-window messages dropped.
func TestJobRunner_ThreadModeIncludesRecentMessages(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-thread-msgs"
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		CaseMode:  model.CaseModeThread,
		Jobs:      []*model.Job{j},
	})

	// Two in-window messages and one just outside the 24h window.
	putMsg(t, repo, wsID, c.ID, "older-window-msg", now.Add(-20*time.Hour))
	putMsg(t, repo, wsID, c.ID, "newer-window-msg", now.Add(-1*time.Hour))
	putMsg(t, repo, wsID, c.ID, "stale-msg", now.Add(-25*time.Hour))

	exec := &capturingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		Clock:     func() time.Time { return now },
	})

	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	sp := exec.systemPrompt
	gt.String(t, sp).Contains("# Recent thread messages (last 24h, up to 32)")
	gt.String(t, sp).Contains("older-window-msg")
	gt.String(t, sp).Contains("newer-window-msg")
	// Outside the 24h window → excluded.
	gt.Bool(t, strings.Contains(sp, "stale-msg")).False()
	// Oldest-first ordering: the -20h message precedes the -1h message.
	gt.Number(t, strings.Index(sp, "older-window-msg")).LessOrEqual(strings.Index(sp, "newer-window-msg"))
}

// TestJobRunner_ThreadModeCapsRecentMessages verifies the newest-32 cap: with
// more than 32 in-window messages, only the newest 32 reach the prompt.
func TestJobRunner_ThreadModeCapsRecentMessages(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-thread-cap"
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		CaseMode:  model.CaseModeThread,
		Jobs:      []*model.Job{j},
	})

	// 33 messages all within the window; msg-00 is the oldest, msg-32 newest.
	// The newest-32 cap must drop exactly msg-00.
	for i := range 33 {
		putMsg(t, repo, wsID, c.ID, fmt.Sprintf("msg-%02d", i), now.Add(-time.Duration(33-i)*time.Minute))
	}

	exec := &capturingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		Clock:     func() time.Time { return now },
	})

	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	sp := exec.systemPrompt
	gt.Bool(t, strings.Contains(sp, "msg-00")).False() // dropped by the 32 cap
	gt.String(t, sp).Contains("msg-01")                // oldest survivor
	gt.String(t, sp).Contains("msg-32")                // newest
}

// TestJobRunner_ChannelModeOmitsRecentMessages verifies that a channel-mode
// Job never gets the recent-messages section, even when the case has messages.
func TestJobRunner_ChannelModeOmitsRecentMessages(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-channel-msgs"
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"}, // default channel mode
		Jobs:      []*model.Job{j},
	})

	putMsg(t, repo, wsID, c.ID, "should-not-appear-body", now.Add(-1*time.Hour))

	exec := &capturingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		Clock:     func() time.Time { return now },
	})

	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	sp := exec.systemPrompt
	gt.Bool(t, strings.Contains(sp, "# Recent thread messages")).False()
	gt.Bool(t, strings.Contains(sp, "should-not-appear-body")).False()
}

// inputCapturingLLM returns canned responses in order and records the
// user-text input of every Generate call so a test can assert what each
// round was asked. Unlike inertLLM it actually drives the planexec loop.
type inputCapturingLLM struct {
	mu        sync.Mutex
	responses []string
	idx       int
	inputs    []string
}

func (l *inputCapturingLLM) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					l.mu.Lock()
					defer l.mu.Unlock()
					var sb strings.Builder
					for _, in := range input {
						if txt, ok := in.(gollem.Text); ok {
							sb.WriteString(string(txt))
							sb.WriteString("\n")
						}
					}
					l.inputs = append(l.inputs, sb.String())
					if l.idx >= len(l.responses) {
						return &gollem.Response{}, nil
					}
					next := l.responses[l.idx]
					l.idx++
					return &gollem.Response{Texts: []string{next}}, nil
				},
			}, nil
		},
	}
}

// TestLifecycle_InteractiveJobQuestionThenResume drives the full interactive
// Job lifecycle through the public entry points: a planexec Job asks the user
// a question (Run suspends at AWAITING_INPUT), the user submits an answer
// (HandleQuestionSubmit), and the run resumes to completion. The decisive
// assertion is HISTORY CONTINUITY: the run's gollem conversation (keyed by the
// SAME RunID across suspend and resume) accumulates BOTH the sub-agent
// observation from before the question AND the user's answer after it — proving
// the resumed planner sees the prior context rather than restarting.
func TestLifecycle_InteractiveJobQuestionThenResume(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-interactive"
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)

	const observationMarker = "OBSERVATION-MARKER-9f3a"

	llm := &inputCapturingLLM{responses: []string{
		// Turn 1, round 1: plan one task.
		`{"message":"start","tasks":[{"id":"t1","title":"A","description":"investigate","acceptance_criteria":"a","tools":["default"]}]}`,
		// Turn 1: sub-agent observation (carries the marker we assert on later).
		observationMarker + " was found in the prod logs.",
		// Turn 1, replan: ask the user which environment.
		`{"message":"need input","question":{"reason":"which environment?","items":[{"id":"env","text":"Which environment?","type":"select","options":["prod","stg"]}]}}`,
		// Turn 2 (resume), replan: terminate via explicit finalize.
		`{"message":"done","finalize":{"reason":"goal met"}}`,
		// Turn 2: final response.
		"Concluded: the prod environment was affected.",
	}}

	historyRepo := agentarchive.NewMemoryHistoryRepository()
	peRunner, err := planexecRunnerForTest(llm.client(), historyRepo)
	gt.NoError(t, err).Required()
	planexecExec, err := jobagent.NewPlanexecJobExecutor(peRunner)
	gt.NoError(t, err).Required()

	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      nil, // set below after we build the Job
	})

	j := &model.Job{
		ID:          "interactive_triage",
		Prompt:      "Investigate {{.Case.Title}} and ask if unclear.",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	gt.NoError(t, j.Validate()).Required()
	// Re-register the workspace with the job so Resume can find it.
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})

	poster := &fakeQuestionPoster{returnTS: "FORM-TS-1"}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:              repo,
		Registry:          registry,
		LLMClient:         llm.client(),
		Executors:         map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategyPlanexec: planexecExec},
		HistoryRepo:       historyRepo,
		InteractionPoster: poster,
		NewRunID:          func() string { return "RUN-1" },
		NewTraceID:        func() string { return "TRACE-1" },
		Clock:             func() time.Time { return now },
	})

	// --- Turn 1: Run suspends at the question ----------------------------
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     now.Add(-time.Second),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	suspendedLog, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, suspendedLog.Stage).Equal(model.JobRunStageAwaitingInput)
	gt.Value(t, suspendedLog.PendingInteraction).NotNil().Required()
	gt.Array(t, poster.posts).Length(1).Required()

	runDoc, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, runDoc.SuspendedRunID).Equal("RUN-1")

	// Extract the resume-context value from the posted form's Submit button.
	refValue := submitValueFromBlocks(t, poster.posts[0].blocks)

	// --- Turn 2: user answers; HandleQuestionSubmit resumes --------------
	callback := &goslack.InteractionCallback{
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				"job_question_item:env": {
					"job_question_choice": {SelectedOption: goslack.OptionBlockObject{Value: "prod"}},
				},
			},
		},
	}
	callback.Channel.ID = "C-CASE"
	callback.Message.Timestamp = "FORM-TS-1"
	action := &goslack.BlockAction{Value: refValue}

	gt.NoError(t, runner.HandleQuestionSubmit(ctx, callback, action)).Required()

	// The run completed under the SAME RunID (no fresh run minted).
	finalLog, err := repo.JobRunLog().Get(ctx, key, "RUN-1")
	gt.NoError(t, err).Required()
	gt.Value(t, finalLog.Stage).Equal(model.JobRunStageSuccess)
	gt.Value(t, finalLog.PendingInteraction).Nil()

	finalRun, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, finalRun.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.String(t, finalRun.LastRunID).Equal("RUN-1")
	// Suspension marker cleared on terminal record.
	gt.String(t, finalRun.SuspendedRunID).Equal("")

	// The form was swapped to the answered view.
	gt.Number(t, len(poster.updates)).GreaterOrEqual(1)

	// --- The decisive assertions: history continuity --------------------
	// gollem persists the conversation under HistoryKey, and the JobRunner
	// sets HistoryKey == RunID for BOTH the initial run and the resume (the
	// run completed under the same "RUN-1" asserted above), so the resumed
	// planner loads the same conversation. We verify the Job-level halves of
	// that contract on the captured planner inputs:
	//   1. the pre-question sub-agent observation entered the conversation
	//      (turn-1 replan input), and
	//   2. the post-question answer is folded into the resumed planner input
	//      under the same run — proving resume continued the dialogue rather
	//      than restarting from a blank round-0 plan.
	gt.Number(t, len(llm.inputs)).GreaterOrEqual(4)
	gt.Bool(t, strings.Contains(llm.inputs[2], observationMarker)).True()
	gt.Bool(t, strings.Contains(llm.inputs[3], "User answers")).True()
	gt.Bool(t, strings.Contains(llm.inputs[3], "prod")).True()
	// The marker is NOT re-stated in the resumed input — it lives in the
	// loaded gollem history, not the fresh turn input — confirming the resume
	// relies on the persisted conversation rather than re-sending observations.
	gt.Bool(t, strings.Contains(llm.inputs[3], observationMarker)).False()
}

// planexecRunnerForTest builds a planexec.Runner sharing the given history
// repository (so the JobRunner and the planner persist into the same store).
func planexecRunnerForTest(llm gollem.LLMClient, historyRepo gollem.HistoryRepository) (*planexec.Runner, error) {
	return planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: historyRepo,
		TraceRepo:   agentarchive.NewMemoryTraceRepository(),
		Budget: planexec.BudgetConfig{
			PlannerLoopMax:  8,
			SubAgentLoopMax: 20,
		},
	})
}

// submitValueFromBlocks pulls the Submit button's value out of a posted
// question form so the test can replay it as the interaction callback action.
func submitValueFromBlocks(t *testing.T, blocks []goslack.Block) string {
	t.Helper()
	for _, b := range blocks {
		ab, ok := b.(*goslack.ActionBlock)
		if !ok || ab.Elements == nil {
			continue
		}
		for _, el := range ab.Elements.ElementSet {
			if btn, ok := el.(*goslack.ButtonBlockElement); ok && btn.ActionID == job.ActionIDJobQuestionSubmit {
				return btn.Value
			}
		}
	}
	t.Fatal("submit button not found in posted blocks")
	return ""
}

// interactivePlanexecJob is a valid interactive (planexec) Job for recovery tests.
func interactivePlanexecJob() *model.Job {
	return &model.Job{
		ID:          "interactive_triage",
		Prompt:      "x",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
}

func newInteractiveRecoveryRunner(repo interfaces.Repository, j *model.Job, exec jobagent.JobExecutor, now time.Time) *job.JobRunner {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})
	return job.NewJobRunner(job.RunnerDeps{
		Repo:      repo,
		Registry:  registry,
		LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategyPlanexec: exec},
		Clock:     func() time.Time { return now },
	})
}

func interactiveCreatedEvent(caseID int64, now time.Time) job.Event {
	return job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        caseID,
		Timestamp:     now,
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	}
}

func TestJobRunner_SkipsGenuinelyActiveSuspendedRun(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo, c := setupCase(t, "ws")
	j := interactivePlanexecJob()
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	// Suspended just now (within the 24h timeout) with an AWAITING_INPUT log.
	suspendRun(t, repo, key, "RUN-ACTIVE", now)

	exec := &recordingExecutor{}
	runner := newInteractiveRecoveryRunner(repo, j, exec, now)
	gt.NoError(t, runner.Run(ctx, j, interactiveCreatedEvent(c.ID, now))).Required()

	// The pending question owns the slot: no new run started.
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
	log, err := repo.JobRunLog().Get(ctx, key, "RUN-ACTIVE")
	gt.NoError(t, err).Required()
	gt.Value(t, log.Stage).Equal(model.JobRunStageAwaitingInput)
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, run.SuspendedRunID).Equal("RUN-ACTIVE")
}

func TestJobRunner_RecoversStaleSuspendedRunAndProceeds(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo, c := setupCase(t, "ws")
	j := interactivePlanexecJob()
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	// Suspended 48h ago — past the 24h timeout, so it is stale.
	suspendRun(t, repo, key, "RUN-STALE", now.Add(-48*time.Hour))

	exec := &recordingExecutor{}
	runner := newInteractiveRecoveryRunner(repo, j, exec, now)
	gt.NoError(t, runner.Run(ctx, j, interactiveCreatedEvent(c.ID, now))).Required()

	// The stale suspension was recovered and a fresh run executed.
	gt.Number(t, exec.calls.Load()).Equal(int32(1))
	// The orphaned run log was failed (not left perpetually AWAITING_INPUT).
	oldLog, err := repo.JobRunLog().Get(ctx, key, "RUN-STALE")
	gt.NoError(t, err).Required()
	gt.Value(t, oldLog.Stage).Equal(model.JobRunStageFailed)
	gt.Value(t, oldLog.PendingInteraction).Nil()
	// The slot is free again: suspension cleared, last run succeeded.
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, run.SuspendedRunID).Equal("")
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)
}

func TestJobRunner_RecoversInconsistentSuspension(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	repo, c := setupCase(t, "ws")
	j := interactivePlanexecJob()
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}

	// Inconsistent state: SuspendedRunID set (recently) but the run log is
	// still RUNNING — what a crash mid-resume leaves behind. Even within the
	// timeout, this must be recovered (the question is not actually open).
	gt.NoError(t, repo.JobRunLog().Create(ctx, newRunningLog(key, "RUN-CRASH", now))).Required()
	gt.NoError(t, repo.JobRun().Suspend(ctx, key, "RUN-CRASH", now)).Required()

	exec := &recordingExecutor{}
	runner := newInteractiveRecoveryRunner(repo, j, exec, now)
	gt.NoError(t, runner.Run(ctx, j, interactiveCreatedEvent(c.ID, now))).Required()

	gt.Number(t, exec.calls.Load()).Equal(int32(1))
	oldLog, err := repo.JobRunLog().Get(ctx, key, "RUN-CRASH")
	gt.NoError(t, err).Required()
	gt.Value(t, oldLog.Stage).Equal(model.JobRunStageFailed)
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, run.SuspendedRunID).Equal("")
}

// getErrJobRun wraps a JobRunRepository and forces Get to fail, simulating a
// transient backend read error.
type getErrJobRun struct {
	interfaces.JobRunRepository
}

func (g getErrJobRun) Get(_ context.Context, _ model.JobRunKey) (*model.JobRun, error) {
	return nil, errors.New("transient backend error")
}

// getErrRepo is a Repository whose JobRun().Get always fails.
type getErrRepo struct {
	interfaces.Repository
	jr interfaces.JobRunRepository
}

func (r getErrRepo) JobRun() interfaces.JobRunRepository { return r.jr }

func TestJobRunner_FailsClosedWhenSuspensionCheckErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 9, 0, 0, 0, time.UTC)
	base, c := setupCase(t, "ws")
	repo := getErrRepo{Repository: base, jr: getErrJobRun{JobRunRepository: base.JobRun()}}

	j := interactivePlanexecJob()
	exec := &recordingExecutor{}
	runner := newInteractiveRecoveryRunner(repo, j, exec, now)

	// A transient Get error must NOT let a new run proceed (which could
	// clobber a suspended one). Run fails closed.
	err := runner.Run(ctx, j, interactiveCreatedEvent(c.ID, now))
	gt.Error(t, err)
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
}

// --- deployment-wide concurrency gate ---------------------------------

// scheduledSlotJob is a scheduled Job used by the concurrency-gate tests.
func scheduledSlotJob() *model.Job {
	return &model.Job{
		ID:     "daily_sweep",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
			Case:      &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
}

// newSlotGatedRunner wires a runner whose scheduled runs go through limiter.
func newSlotGatedRunner(
	repo interfaces.Repository,
	j *model.Job,
	exec jobagent.JobExecutor,
	limiter *job.ConcurrencyLimiter,
	wsID string,
) *job.JobRunner {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})
	return job.NewJobRunner(job.RunnerDeps{
		Repo:        repo,
		Registry:    registry,
		LLMClient:   inertLLM(),
		Executors:   map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
		SlotLimiter: limiter,
	})
}

// occupyAllSlots takes every slot in [0, limit) on behalf of another instance.
func occupyAllSlots(t *testing.T, repo interfaces.Repository, limit int, now time.Time) {
	t.Helper()
	for i := 0; i < limit; i++ {
		acquired, err := repo.JobSlot().TryAcquire(context.Background(), &model.JobSlot{
			Index:       i,
			HolderID:    fmt.Sprintf("other-instance-%d", i),
			WorkspaceID: "ws-other",
			CaseID:      int64(1000 + i),
			JobID:       "daily_sweep",
			AcquiredAt:  now,
			ExpiresAt:   now.Add(testSlotTTL),
		}, now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True().Required()
	}
}

func scheduledSlotEvent(wsID string, caseID int64, now time.Time) job.Event {
	return job.Event{
		Domain:      model.JobEventDomainScheduled,
		WorkspaceID: wsID,
		CaseID:      caseID,
		Timestamp:   now,
		ActorUserID: model.SystemActorID,
	}
}

func TestJobRunner_ScheduledRunSkippedWhenSlotsFull(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-full"
	repo, c := setupCase(t, wsID)
	clock := newTestClock()
	occupyAllSlots(t, repo, 2, clock.Now())

	j := scheduledSlotJob()
	exec := &recordingExecutor{}
	runner := newSlotGatedRunner(repo, j, exec, newLimiter(t, repo.JobSlot(), 2, clock), wsID)

	// Skipping is not a failure: Run returns nil so the dispatcher stays quiet.
	gt.NoError(t, runner.Run(ctx, j, scheduledSlotEvent(wsID, c.ID, clock.Now())))
	async.Wait()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))

	// No outcome is recorded, so LastRunAt stays zero and the next tick still
	// finds the Job due — the skip is a postponement, not a lost run.
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Bool(t, run.LastRunAt.IsZero()).True()
	gt.Value(t, string(run.LastStatus)).Equal("")

	// No run log either: the run never reached the execute stage.
	logs, err := repo.JobRunLog().List(ctx, key, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(0)

	// The other instance's slots are untouched.
	stored, err := repo.JobSlot().List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(2).Required()
	gt.Value(t, stored[0].HolderID).Equal("other-instance-0")
	gt.Value(t, stored[1].HolderID).Equal("other-instance-1")
}

func TestJobRunner_ScheduledRunTakesAndReleasesSlot(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-free"
	repo, c := setupCase(t, wsID)
	clock := newTestClock()

	j := scheduledSlotJob()
	exec := &recordingExecutor{}
	runner := newSlotGatedRunner(repo, j, exec, newLimiter(t, repo.JobSlot(), 1, clock), wsID)

	gt.NoError(t, runner.Run(ctx, j, scheduledSlotEvent(wsID, c.ID, clock.Now())))
	async.Wait()

	gt.Value(t, exec.firedJobIDs()).Equal([]string{j.ID})

	// The slot is handed back, so the next tick can use it.
	stored, err := repo.JobSlot().List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.Bool(t, run.LastRunAt.IsZero()).False()
}

func TestJobRunner_NonScheduledRunIgnoresSlotLimit(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-lifecycle"
	repo, c := setupCase(t, wsID)
	clock := newTestClock()
	occupyAllSlots(t, repo, 1, clock.Now())

	j := scheduledSlotJob()
	exec := &recordingExecutor{}
	runner := newSlotGatedRunner(repo, j, exec, newLimiter(t, repo.JobSlot(), 1, clock), wsID)

	// A lifecycle event is a single user-visible action with no retry, so the
	// gate does not apply even with every slot occupied.
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		CaseLifecycle: model.CaseLifecycleCreated,
		Timestamp:     clock.Now(),
	}))
	async.Wait()
	gt.Value(t, exec.firedJobIDs()).Equal([]string{j.ID})

	// The occupied slot was neither taken nor released by this run.
	stored, err := repo.JobSlot().List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(1).Required()
	gt.Value(t, stored[0].HolderID).Equal("other-instance-0")
}

func TestJobRunner_ScheduledRunWithoutLimiterIsUngated(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-nil-limiter"
	repo, c := setupCase(t, wsID)
	now := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)

	j := scheduledSlotJob()
	exec := &recordingExecutor{}
	runner := newSlotGatedRunner(repo, j, exec, nil, wsID)

	gt.NoError(t, runner.Run(ctx, j, scheduledSlotEvent(wsID, c.ID, now)))
	async.Wait()
	gt.Value(t, exec.firedJobIDs()).Equal([]string{j.ID})

	// Nothing was written to the slot store at all.
	stored, err := repo.JobSlot().List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}

func TestJobRunner_ScheduledRunFailsClosedWhenSlotStateUnreadable(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-unreadable"
	repo, c := setupCase(t, wsID)
	clock := newTestClock()

	slotRepo := newFakeSlotRepo()
	slotRepo.listErr = goerr.New("firestore unavailable")

	j := scheduledSlotJob()
	exec := &recordingExecutor{}
	runner := newSlotGatedRunner(repo, j, exec, newLimiter(t, slotRepo, 2, clock), wsID)

	// With the slot state unreadable we cannot tell how many runs are in
	// flight, so the run is refused rather than started unbounded.
	gt.Error(t, runner.Run(ctx, j, scheduledSlotEvent(wsID, c.ID, clock.Now())))
	async.Wait()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))

	// No outcome recorded: the next tick retries.
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Bool(t, run.LastRunAt.IsZero()).True()
	logs, err := repo.JobRunLog().List(ctx, key, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(0)
}

func TestJobRunner_ScheduledInteractiveRunReleasesSlotOnSuspend(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-slot-suspend"
	now := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	repo, c := setupCaseWithSlack(t, wsID, "C-CASE", "1700000000.0001")
	clock := newTestClock()

	j := &model.Job{
		ID:          "interactive_sweep",
		Prompt:      "x",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	gt.NoError(t, j.Validate()).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})
	exec := &interactiveScriptedExecutor{
		firstTokens: trace.LLMCallData{Model: "m", InputTokens: 10, OutputTokens: 2},
	}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:              repo,
		Registry:          registry,
		LLMClient:         inertLLM(),
		Executors:         map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategyPlanexec: exec},
		InteractionPoster: &fakeQuestionPoster{returnTS: "FORM-TS-1"},
		NewRunID:          func() string { return "RUN-SLOT-1" },
		NewTraceID:        func() string { return "TRACE-SLOT-1" },
		Clock:             func() time.Time { return now },
		SlotLimiter:       newLimiter(t, repo.JobSlot(), 1, clock),
	})

	gt.NoError(t, runner.Run(ctx, j, scheduledSlotEvent(wsID, c.ID, now)))
	async.Wait()

	// The run is parked awaiting the user's answer...
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	logRec, err := repo.JobRunLog().Get(ctx, key, "RUN-SLOT-1")
	gt.NoError(t, err).Required()
	gt.Value(t, logRec.Stage).Equal(model.JobRunStageAwaitingInput)

	// ...but its slot is already free: a human wait must not pin the limit.
	stored, err := repo.JobSlot().List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}
