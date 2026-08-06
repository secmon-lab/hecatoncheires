package job_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

func TestIsDue_Every(t *testing.T) {
	cfg := &model.ScheduledEventConfig{Every: time.Hour}
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)

	t.Run("first run (no prior)", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, time.Time{}, now)).True()
	})
	t.Run("just at duration", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, now.Add(-time.Hour), now)).True()
	})
	t.Run("just before duration", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, now.Add(-59*time.Minute), now)).False()
	})
}

func TestIsDue_Cron(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("0 9 * * *") // every day 09:00 UTC
	gt.NoError(t, err).Required()
	cfg := &model.ScheduledEventConfig{Cron: sched, CronExpr: "0 9 * * *"}

	t.Run("first run is due", func(t *testing.T) {
		gt.Bool(t, job.IsDue(cfg, time.Time{}, time.Now().UTC())).True()
	})
	t.Run("not yet fired today", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 8, 59, 0, 0, time.UTC)
		last := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
		gt.Bool(t, job.IsDue(cfg, last, now)).False()
	})
	t.Run("just past fire time", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
		last := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
		gt.Bool(t, job.IsDue(cfg, last, now)).True()
	})
}

func TestIsDue_NilOrUnset(t *testing.T) {
	gt.Bool(t, job.IsDue(nil, time.Time{}, time.Now())).False()
	gt.Bool(t, job.IsDue(&model.ScheduledEventConfig{}, time.Time{}, time.Now())).False()
}

// recordingPublisher records every Publish call without invoking any
// runner / executor / LLM.
type recordingPublisher struct {
	mu     sync.Mutex
	events []job.Event
}

func (p *recordingPublisher) Publish(_ context.Context, ev job.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
}

func (p *recordingPublisher) snapshot() []job.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]job.Event, len(p.events))
	copy(out, p.events)
	return out
}

func TestScheduledScanner_PublishesDueJobs(t *testing.T) {
	ctx := context.Background()
	repo, caseA := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	dueJob := &model.Job{
		ID:     "stale_check",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{dueJob},
	})

	// Seed a last-run timestamp two hours ago so the job is due.
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: dueJob.ID,
	}, model.JobRunStatusSuccess, time.Now().UTC().Add(-2*time.Hour), "", "", "")).Required()

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      repo,
		Registry:  registry,
		Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()

	events := pub.snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, events[0].Domain).Equal(model.JobEventDomainScheduled)
	gt.Value(t, events[0].WorkspaceID).Equal("ws")
	gt.Value(t, events[0].CaseID).Equal(caseA.ID)
	gt.Value(t, events[0].ActorUserID).Equal(model.SystemActorID)
	gt.Value(t, events[0].JobID).Equal(dueJob.ID)
}

// TestScheduledScanner_PublishesOnlyDueJob pins the per-Job addressing: a
// workspace with several scheduled Jobs publishes one event for the Job that
// came due, carrying that Job's own last-run / next-fire times. Due-ness is
// built from `every` plus a seeded last-run time because Scan reads the wall
// clock directly, which makes cron-based fixtures depend on the time of day.
// The sweep must close with a line that reconciles what became due against
// what actually ran — the reason the per-workspace due_published line alone was
// not enough.
func TestScheduledScanner_TickSummary(t *testing.T) {
	ctx, out := jsonLogContext(context.Background())
	// Two OPEN cases and one scheduled Job: the sweep raises two due events.
	repo, _ := setupCase(t, "ws")
	_, err := repo.Case().Create(ctx, "ws", &model.Case{
		Title: "T2", Status: types.CaseStatusOpen, ReporterID: "U-REP",
	})
	gt.NoError(t, err).Required()

	dueJob := &model.Job{
		ID:     "stale_check",
		Prompt: "x",
		Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
	}
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{dueJob},
	})

	exec := &recordingExecutor{}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
	})
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      repo,
		Registry:  registry,
		Publisher: job.NewUseCase(registry, runner),
	})

	gt.NoError(t, scanner.Scan(ctx)).Required()
	async.Wait()

	// Both cases were first-run due, and Scan waited for both runs before
	// summarising them.
	gt.Number(t, exec.calls.Load()).Equal(int32(2))
	rec, ok := findLogRecord(out.lines(), "job tick summary")
	gt.Bool(t, ok).True().Required()
	gt.Value(t, rec["due_total"]).Equal(float64(2))
	gt.Value(t, rec["started"]).Equal(float64(2))
	gt.Value(t, rec["completed"]).Equal(float64(2))
	gt.Value(t, rec["failed"]).Equal(float64(0))
	gt.Value(t, rec["suspended"]).Equal(float64(0))
	gt.Value(t, rec["skipped_slots_full"]).Equal(float64(0))
	gt.Value(t, rec["skipped_lease"]).Equal(float64(0))
	gt.Value(t, rec["skipped_suspended"]).Equal(float64(0))
	gt.Value(t, rec["settled"]).Equal(true)
	gt.Number(t, rec["elapsed_ms"].(float64)).GreaterOrEqual(0)

	// The gate is not wired, so no capacity figures are reported.
	for _, key := range []string{"slot_limit", "slot_busy_ms", "slot_idle_ms"} {
		_, present := rec[key]
		gt.Bool(t, present).False()
	}

	// The per-workspace line stays as it was.
	sweep, ok := findLogRecord(out.lines(), "scheduled sweep completed")
	gt.Bool(t, ok).True().Required()
	gt.Value(t, sweep["due_published"]).Equal(float64(2))
	gt.Value(t, sweep["open_cases"]).Equal(float64(2))
}

// A run still in flight when the settle timeout expires must be reported as a
// partial count, not silently under-reported.
func TestScheduledScanner_TickSummaryReportsUnsettled(t *testing.T) {
	ctx, out := jsonLogContext(context.Background())
	repo, _ := setupCase(t, "ws")

	dueJob := &model.Job{
		ID:     "stale_check",
		Prompt: "x",
		Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
	}
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{dueJob},
	})

	release := make(chan struct{})
	exec := &blockingExecutor{release: release}
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(),
		Executors: map[model.JobStrategy]jobagent.JobExecutor{model.JobStrategySimple: exec},
	})
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:             repo,
		Registry:         registry,
		Publisher:        job.NewUseCase(registry, runner),
		RunSettleTimeout: 20 * time.Millisecond,
	})

	gt.NoError(t, scanner.Scan(ctx)).Required()

	rec, ok := findLogRecord(out.lines(), "job tick summary")
	gt.Bool(t, ok).True().Required()
	gt.Value(t, rec["settled"]).Equal(false)
	gt.Value(t, rec["due_total"]).Equal(float64(1))
	gt.Value(t, rec["started"]).Equal(float64(0)) // still running, not yet reported

	close(release)
	async.Wait()
}

// listErrCase wraps a CaseRepository and forces List to fail, standing in for
// a backend read error mid-sweep.
type listErrCase struct {
	interfaces.CaseRepository
}

func (listErrCase) List(_ context.Context, _ string, _ ...interfaces.ListCaseOption) ([]*model.Case, error) {
	return nil, goerr.New("transient backend error")
}

// caseListFailingRepo is a Repository whose Case().List always fails.
type caseListFailingRepo struct {
	interfaces.Repository
}

func (r *caseListFailingRepo) Case() interfaces.CaseRepository {
	return listErrCase{CaseRepository: r.Repository.Case()}
}

// blockingExecutor holds the run open until released, so a test can observe the
// sweep giving up on the wait.
type blockingExecutor struct {
	release chan struct{}
}

func (e *blockingExecutor) Execute(_ context.Context, _ jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	<-e.release
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// A sweep that fails before it can summarise must not block for the settle
// timeout, and must not emit a summary describing a sweep that did not happen.
func TestScheduledScanner_NoTickSummaryOnScanFailure(t *testing.T) {
	ctx, out := jsonLogContext(context.Background())
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs: []*model.Job{{
			ID:     "stale_check",
			Prompt: "x",
			Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
		}},
	})
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      &caseListFailingRepo{Repository: memory.New()},
		Registry:  registry,
		Publisher: &recordingPublisher{},
	})

	gt.Error(t, scanner.Scan(ctx))
	_, ok := findLogRecord(out.lines(), "job tick summary")
	gt.Bool(t, ok).False()
}

func TestScheduledScanner_PublishesOnlyDueJob(t *testing.T) {
	ctx, out := jsonLogContext(context.Background())
	repo, caseA := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	hourly := &model.Job{
		ID:     "hourly",
		Prompt: "x",
		Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
	}
	daily := &model.Job{
		ID:     "daily",
		Prompt: "x",
		Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: 24 * time.Hour}},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{hourly, daily},
	})

	hourlyLast := time.Now().UTC().Add(-2 * time.Hour) // due: every=1h
	dailyLast := time.Now().UTC().Add(-time.Hour)      // not due: every=24h
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: hourly.ID,
	}, model.JobRunStatusSuccess, hourlyLast, "", "", "")).Required()
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: daily.ID,
	}, model.JobRunStatusSuccess, dailyLast, "", "", "")).Required()

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()

	events := pub.snapshot()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, events[0].JobID).Equal(hourly.ID)
	gt.Bool(t, events[0].LastRunAt.Sub(hourlyLast).Abs() < time.Second).True()
	gt.Bool(t, events[0].ScheduledFor.Sub(hourlyLast.Add(time.Hour)).Abs() < time.Second).True()

	// The sweep summary makes the same fact readable from the log: 2 scheduled
	// Jobs over 1 OPEN case produced exactly 1 due event.
	rec, ok := findLogRecord(out.lines(), "scheduled sweep completed")
	gt.Bool(t, ok).True().Required()
	gt.Value(t, rec["workspace_id"]).Equal("ws")
	gt.Value(t, rec["scheduled_jobs"]).Equal(float64(2))
	gt.Value(t, rec["open_cases"]).Equal(float64(1))
	gt.Value(t, rec["due_published"]).Equal(float64(1))
}

func TestScheduledScanner_SkipsNotYetDue(t *testing.T) {
	ctx := context.Background()
	repo, caseA := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "stale_check",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})

	// Last run was just now — not due.
	gt.NoError(t, repo.JobRun().RecordRun(ctx, model.JobRunKey{
		WorkspaceID: "ws", CaseID: caseA.ID, JobID: j.ID,
	}, model.JobRunStatusSuccess, time.Now().UTC(), "", "", "")).Required()

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	gt.Array(t, pub.snapshot()).Length(0)
}

func TestScheduledScanner_SkipsDisabledJobs(t *testing.T) {
	ctx := context.Background()
	repo, _ := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs: []*model.Job{{
			ID:       "disabled",
			Prompt:   "x",
			Disabled: true,
			Events:   model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
		}},
	})

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: pub,
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	gt.Array(t, pub.snapshot()).Length(0)
}

func TestScheduledScanner_FirstRunImmediatelyDue(t *testing.T) {
	ctx := context.Background()
	repo, _ := setupCase(t, "ws")

	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:     "stale",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})

	pub := &recordingPublisher{}
	scanner := job.NewScheduledScanner(job.ScannerDeps{Repo: repo, Registry: registry, Publisher: pub})
	gt.NoError(t, scanner.Scan(ctx)).Required()
	// No prior JobRun → due on first scan.
	gt.Array(t, pub.snapshot()).Length(1)
}

// suspendRun puts a (job, case) into the AWAITING_INPUT state with the given
// SuspendedAt, mirroring what JobInteractor.Solicit persists at runtime.
func suspendRun(t *testing.T, repo interfaces.Repository, key model.JobRunKey, runID string, suspendedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	log := &model.JobRunLog{
		WorkspaceID:  key.WorkspaceID,
		CaseID:       key.CaseID,
		JobID:        key.JobID,
		RunID:        runID,
		TraceID:      "trace-" + runID,
		Stage:        model.JobRunStageRunning,
		StartedAt:    suspendedAt,
		ExecutorKind: "planexec",
	}
	gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()
	log.Stage = model.JobRunStageAwaitingInput
	log.PendingInteraction = &model.PendingInteraction{
		PostedChannelID: "C1",
		PostedMessageTS: "1700000000.0001",
		Reason:          "which env?",
		Items: []model.PendingInteractionItem{
			{ID: "env", Text: "Which environment?", Type: "select", Options: []string{"prod", "stg"}},
		},
	}
	gt.NoError(t, repo.JobRunLog().Suspend(ctx, log)).Required()
	gt.NoError(t, repo.JobRun().Suspend(ctx, key, runID, suspendedAt)).Required()
}

func interactiveScannerWorkspace() (*model.WorkspaceRegistry, *model.Job) {
	registry := model.NewWorkspaceRegistry()
	j := &model.Job{
		ID:          "interactive_triage",
		Prompt:      "x",
		Strategy:    model.JobStrategyPlanexec,
		Interactive: true,
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws"},
		Jobs:      []*model.Job{j},
	})
	return registry, j
}

func TestScheduledScanner_ExpiresStaleSuspendedRun(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCase(t, "ws")
	registry, j := interactiveScannerWorkspace()
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}

	// Suspended well past the default 24h timeout.
	suspendRun(t, repo, key, "RUN-STALE", time.Now().UTC().Add(-48*time.Hour))

	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: &recordingPublisher{},
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()

	// The run was expired: log FAILED, suspension marker cleared.
	log, err := repo.JobRunLog().Get(ctx, key, "RUN-STALE")
	gt.NoError(t, err).Required()
	gt.Value(t, log.Stage).Equal(model.JobRunStageFailed)
	gt.String(t, log.Error).NotEqual("")
	gt.Value(t, log.PendingInteraction).Nil()

	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, run.SuspendedRunID).Equal("")
	gt.Bool(t, run.IsSuspended()).False()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusFailed)
}

func TestScheduledScanner_KeepsFreshSuspendedRun(t *testing.T) {
	ctx := context.Background()
	repo, c := setupCase(t, "ws")
	registry, j := interactiveScannerWorkspace()
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}

	// Suspended just now — well within the timeout.
	suspendRun(t, repo, key, "RUN-FRESH", time.Now().UTC())

	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo: repo, Registry: registry, Publisher: &recordingPublisher{},
	})
	gt.NoError(t, scanner.Scan(ctx)).Required()

	// Still suspended — the sweep left it alone.
	log, err := repo.JobRunLog().Get(ctx, key, "RUN-FRESH")
	gt.NoError(t, err).Required()
	gt.Value(t, log.Stage).Equal(model.JobRunStageAwaitingInput)

	run, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.String(t, run.SuspendedRunID).Equal("RUN-FRESH")
}
