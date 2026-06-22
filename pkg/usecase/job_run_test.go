package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// seedLog stores one RUNNING-stage log and immediately marks it
// finished with the supplied stage. Used by the JobRunUseCase tests to
// build a deterministic per-case log history without going through the
// full agent runner.
func seedLog(t *testing.T, repo interfaces.Repository, ws string, caseID int64, jobID, runID string, started time.Time, stage model.JobRunStage, errMsg string) *model.JobRunLog {
	t.Helper()
	traceID := fmt.Sprintf("trace-%s", runID)
	log := &model.JobRunLog{
		WorkspaceID:     ws,
		CaseID:          caseID,
		JobID:           jobID,
		RunID:           runID,
		TraceID:         traceID,
		Stage:           model.JobRunStageRunning,
		StartedAt:       started,
		ExecutorKind:    "single_loop",
		ExecutorVersion: "test",
	}
	gt.NoError(t, repo.JobRunLog().Create(context.Background(), log)).Required()

	// Register the JobRun so per-case fan-out finds the JobID.
	gt.NoError(t, repo.JobRun().RecordRun(
		context.Background(),
		model.JobRunKey{WorkspaceID: ws, CaseID: caseID, JobID: jobID},
		model.JobRunStatusSuccess,
		started, runID, traceID, "",
	)).Required()

	if stage != model.JobRunStageRunning {
		log.Stage = stage
		log.EndedAt = started.Add(5 * time.Second)
		log.Error = errMsg
		gt.NoError(t, repo.JobRunLog().Finish(context.Background(), log)).Required()
	}
	return log
}

func setupJobRunTestCase(t *testing.T) (interfaces.Repository, *usecase.JobRunUseCase, string, *model.Case) {
	t.Helper()
	repo := memory.New()
	caseUC := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
	uc := usecase.NewJobRunUseCase(repo, nil)
	ws := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})
	c, err := caseUC.CreateCase(ctx, ws, "agent target", "", nil, nil, false, "", "")
	gt.NoError(t, err).Required()
	return repo, uc, ws, c
}

func TestJobRunUseCase_ListLogsByCase(t *testing.T) {
	t.Run("returns empty page when no logs exist", func(t *testing.T) {
		_, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		page, err := uc.ListLogsByCase(ctx, ws, c.ID, 10, nil)
		gt.NoError(t, err).Required()
		gt.Number(t, len(page.Items)).Equal(0)
		gt.Value(t, page.NextCursor).Nil()
	})

	t.Run("orders by StartedAt DESC across multiple jobs", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		base := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
		seedLog(t, repo, ws, c.ID, "incident-rca", "run-A", base.Add(2*time.Minute), model.JobRunStageSuccess, "")
		seedLog(t, repo, ws, c.ID, "incident-rca", "run-B", base.Add(4*time.Minute), model.JobRunStageFailed, "boom")
		seedLog(t, repo, ws, c.ID, "summarize-slack", "run-C", base.Add(1*time.Minute), model.JobRunStageSuccess, "")
		seedLog(t, repo, ws, c.ID, "knowledge-link", "run-D", base.Add(3*time.Minute), model.JobRunStageSuccess, "")

		page, err := uc.ListLogsByCase(ctx, ws, c.ID, 50, nil)
		gt.NoError(t, err).Required()
		gt.Array(t, page.Items).Length(4).Required()
		gt.String(t, page.Items[0].RunID).Equal("run-B")
		gt.String(t, page.Items[1].RunID).Equal("run-D")
		gt.String(t, page.Items[2].RunID).Equal("run-A")
		gt.String(t, page.Items[3].RunID).Equal("run-C")
		gt.Value(t, page.NextCursor).Nil()
	})

	t.Run("paginates through cursor", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		base := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
		for i := range 5 {
			runID := fmt.Sprintf("run-%02d", i)
			seedLog(t, repo, ws, c.ID, "incident-rca", runID, base.Add(time.Duration(i)*time.Minute), model.JobRunStageSuccess, "")
		}

		page1, err := uc.ListLogsByCase(ctx, ws, c.ID, 2, nil)
		gt.NoError(t, err).Required()
		gt.Array(t, page1.Items).Length(2).Required()
		gt.Value(t, page1.NextCursor).NotNil()
		gt.String(t, page1.Items[0].RunID).Equal("run-04")
		gt.String(t, page1.Items[1].RunID).Equal("run-03")

		page2, err := uc.ListLogsByCase(ctx, ws, c.ID, 2, page1.NextCursor)
		gt.NoError(t, err).Required()
		gt.Array(t, page2.Items).Length(2).Required()
		gt.Value(t, page2.NextCursor).NotNil()
		gt.String(t, page2.Items[0].RunID).Equal("run-02")
		gt.String(t, page2.Items[1].RunID).Equal("run-01")

		page3, err := uc.ListLogsByCase(ctx, ws, c.ID, 2, page2.NextCursor)
		gt.NoError(t, err).Required()
		gt.Array(t, page3.Items).Length(1).Required()
		gt.Value(t, page3.NextCursor).Nil()
		gt.String(t, page3.Items[0].RunID).Equal("run-00")
	})

	t.Run("rejects unknown cursor encoding", func(t *testing.T) {
		_, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		bogus := "!!! not base64 !!!"
		_, err := uc.ListLogsByCase(ctx, ws, c.ID, 10, &bogus)
		gt.Error(t, err).Is(usecase.ErrInvalidArgument)
	})

	t.Run("private case rejects non-member", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t)
		raw, err := repo.Case().Get(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		raw.IsPrivate = true
		raw.ChannelUserIDs = []string{"UREPORTER"}
		_, err = repo.Case().Update(context.Background(), ws, raw)
		gt.NoError(t, err).Required()

		other := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		_, err = uc.ListLogsByCase(other, ws, c.ID, 10, nil)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("system context without token bypasses access control", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t)
		raw, err := repo.Case().Get(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		raw.IsPrivate = true
		raw.ChannelUserIDs = []string{"UREPORTER"}
		_, err = repo.Case().Update(context.Background(), ws, raw)
		gt.NoError(t, err).Required()

		// background context has no auth token at all
		page, err := uc.ListLogsByCase(context.Background(), ws, c.ID, 10, nil)
		gt.NoError(t, err).Required()
		gt.Number(t, len(page.Items)).Equal(0)
	})
}

func TestJobRunUseCase_GetLog(t *testing.T) {
	t.Run("returns the log when present", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		base := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
		seedLog(t, repo, ws, c.ID, "incident-rca", "run-X", base, model.JobRunStageSuccess, "")

		got, err := uc.GetLog(ctx, ws, c.ID, "run-X")
		gt.NoError(t, err).Required()
		gt.String(t, got.RunID).Equal("run-X")
		gt.String(t, got.JobID).Equal("incident-rca")
	})

	t.Run("returns not found when missing", func(t *testing.T) {
		_, uc, ws, c := setupJobRunTestCase(t)
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		_, err := uc.GetLog(ctx, ws, c.ID, "run-missing")
		gt.Error(t, err).Is(interfaces.ErrJobRunLogNotFound)
	})
}

// caseJobsRegistry builds a registry with a representative spread of Job
// definitions for the given workspace: a case-created Job, two scheduled
// Jobs (interval + cron), a case-closed Job, and a disabled Job that must
// never surface.
func caseJobsRegistry(t *testing.T, ws string) *model.WorkspaceRegistry {
	t.Helper()
	cronSched, err := cron.ParseStandard("0 9 * * *")
	gt.NoError(t, err).Required()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: ws, Name: "WS"},
		Jobs: []*model.Job{
			{
				ID: "triage", Name: "Initial triage", Description: "evaluate on create",
				Prompt: "p", Strategy: model.JobStrategyPlanexec,
				Events: model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}}},
			},
			{
				ID: "stale", Name: "Stale check", Description: "remind", Prompt: "p", Quiet: true,
				Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Every: time.Hour}},
			},
			{
				ID: "daily", Name: "Daily summary", Description: "report", Prompt: "p", Strategy: model.JobStrategyPlanexec,
				Events: model.JobEvents{Scheduled: &model.ScheduledEventConfig{Cron: cronSched, CronExpr: "0 9 * * *"}},
			},
			{
				ID: "closed-notify", Name: "Close notice", Description: "wrap up", Prompt: "p",
				Events: model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleClosed}}},
			},
			{
				ID: "disabled-job", Name: "Disabled", Description: "never", Prompt: "p", Disabled: true,
				Events: model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}}},
			},
		},
	})
	return registry
}

func jobIDSet(jobs []*model.Job) map[string]*model.Job {
	out := make(map[string]*model.Job, len(jobs))
	for _, j := range jobs {
		out[j.ID] = j
	}
	return out
}

func TestJobRunUseCase_ListCaseJobs(t *testing.T) {
	t.Run("open case returns enabled case-event and scheduled jobs", func(t *testing.T) {
		repo, _, ws, c := setupJobRunTestCase(t)
		uc := usecase.NewJobRunUseCase(repo, caseJobsRegistry(t, ws))
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})

		jobs, err := uc.ListCaseJobs(ctx, ws, c.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, jobs).Length(4).Required()
		got := jobIDSet(jobs)
		gt.Map(t, got).HasKey("triage")
		gt.Map(t, got).HasKey("stale")
		gt.Map(t, got).HasKey("daily")
		gt.Map(t, got).HasKey("closed-notify")
		// Disabled jobs never surface.
		_, hasDisabled := got["disabled-job"]
		gt.Bool(t, hasDisabled).False()
		// Field fidelity: the scheduled interval Job round-trips its quiet flag.
		gt.Bool(t, got["stale"].Quiet).True()
		gt.Value(t, got["triage"].Strategy).Equal(model.JobStrategyPlanexec)
	})

	t.Run("closed case drops scheduled jobs but keeps case-event jobs", func(t *testing.T) {
		repo, _, ws, c := setupJobRunTestCase(t)
		uc := usecase.NewJobRunUseCase(repo, caseJobsRegistry(t, ws))

		raw, err := repo.Case().Get(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		raw.Status = types.CaseStatusClosed
		_, err = repo.Case().Update(context.Background(), ws, raw)
		gt.NoError(t, err).Required()

		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})
		jobs, err := uc.ListCaseJobs(ctx, ws, c.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, jobs).Length(2).Required()
		got := jobIDSet(jobs)
		gt.Map(t, got).HasKey("triage")
		gt.Map(t, got).HasKey("closed-notify")
		_, hasStale := got["stale"]
		gt.Bool(t, hasStale).False()
		_, hasDaily := got["daily"]
		gt.Bool(t, hasDaily).False()
	})

	t.Run("private case rejects non-member", func(t *testing.T) {
		repo, _, ws, c := setupJobRunTestCase(t)
		uc := usecase.NewJobRunUseCase(repo, caseJobsRegistry(t, ws))

		raw, err := repo.Case().Get(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		raw.IsPrivate = true
		raw.ChannelUserIDs = []string{"UREPORTER"}
		_, err = repo.Case().Update(context.Background(), ws, raw)
		gt.NoError(t, err).Required()

		other := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "USTRANGER"})
		_, err = uc.ListCaseJobs(other, ws, c.ID)
		gt.Error(t, err).Is(usecase.ErrAccessDenied)
	})

	t.Run("system context without token bypasses access control", func(t *testing.T) {
		repo, _, ws, c := setupJobRunTestCase(t)
		uc := usecase.NewJobRunUseCase(repo, caseJobsRegistry(t, ws))

		raw, err := repo.Case().Get(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		raw.IsPrivate = true
		raw.ChannelUserIDs = []string{"UREPORTER"}
		_, err = repo.Case().Update(context.Background(), ws, raw)
		gt.NoError(t, err).Required()

		jobs, err := uc.ListCaseJobs(context.Background(), ws, c.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, jobs).Length(4)
	})

	t.Run("nil registry yields empty slice", func(t *testing.T) {
		repo, uc, ws, c := setupJobRunTestCase(t) // setup wires registry=nil
		ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "UREPORTER"})
		jobs, err := uc.ListCaseJobs(ctx, ws, c.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, jobs).Length(0)
		_ = repo
	})
}

func TestJobRunUseCase_ResolveJobName(t *testing.T) {
	t.Run("falls back to job id when registry is nil", func(t *testing.T) {
		uc := usecase.NewJobRunUseCase(memory.New(), nil)
		gt.String(t, uc.ResolveJobName("ws", "the-job")).Equal("the-job")
	})

	t.Run("returns Name from registered Job", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws", Name: "WS"},
			Jobs: []*model.Job{
				{ID: "incident-rca", Name: "Incident RCA", Prompt: "p", Events: model.JobEvents{Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}}}},
			},
		})
		uc := usecase.NewJobRunUseCase(repo, registry)
		gt.String(t, uc.ResolveJobName("ws", "incident-rca")).Equal("Incident RCA")
		gt.String(t, uc.ResolveJobName("ws", "unknown-job")).Equal("unknown-job")
	})
}
