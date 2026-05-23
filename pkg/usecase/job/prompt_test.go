package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

func newWorkspace(id, name string) *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: id, Name: name, Description: "test"},
	}
}

func newCase(id int64) *model.Case {
	return &model.Case{
		ID:             id,
		Title:          "Sample",
		Description:    "desc",
		Status:         types.CaseStatusOpen,
		ReporterID:     "U-REPORTER",
		AssigneeIDs:    []string{"U-A1", "U-A2"},
		SlackChannelID: "C-CASE",
		CreatedAt:      time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC),
	}
}

func TestBuildSystemPrompt_CaseCreatedEvent(t *testing.T) {
	j := &model.Job{
		ID:     "summarize",
		Prompt: "{{.Case.Title}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	ev := job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        42,
		Timestamp:     time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	}
	got, err := job.BuildSystemPrompt(job.PromptInputs{
		Job: j, Workspace: newWorkspace("ws", "WS"), Case: newCase(42), Event: ev,
	})
	gt.NoError(t, err).Required()

	mustContain(t, got, "# Role")
	mustContain(t, got, "# Workspace")
	mustContain(t, got, "- id: ws")
	mustContain(t, got, "# Case")
	mustContain(t, got, "- title: Sample")
	mustContain(t, got, "# Actions (existing, non-archived)")
	mustContain(t, got, "(none)")
	mustContain(t, got, "# Trigger condition")
	mustContain(t, got, "a new case is created")
	mustContain(t, got, "# Trigger reason (this invocation)")
	mustContain(t, got, "Case #42 was created by U-CALLER at 2026-05-23T12:00:00Z.")
	mustContain(t, got, "# Guardrails")
	mustContain(t, got, "Do not duplicate work")
	mustContain(t, got, "cannot close the case")
}

func TestBuildSystemPrompt_CaseClosedEvent(t *testing.T) {
	j := &model.Job{
		ID:     "on-close",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleClosed}},
		},
	}
	ev := job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        99,
		Timestamp:     time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		ActorUserID:   "U-OPS",
		CaseLifecycle: model.CaseLifecycleClosed,
	}
	got, err := job.BuildSystemPrompt(job.PromptInputs{
		Job: j, Workspace: newWorkspace("ws", "WS"), Case: newCase(99), Event: ev,
	})
	gt.NoError(t, err).Required()
	mustContain(t, got, "Case #99 status was transitioned to CLOSED by U-OPS at 2026-05-23T12:00:00Z.")
	mustContain(t, got, "the case status transitions to CLOSED")
}

func TestBuildSystemPrompt_ScheduledEvery(t *testing.T) {
	j := &model.Job{
		ID:     "stale",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Every: time.Hour},
		},
	}
	ev := job.Event{
		Domain:      model.JobEventDomainScheduled,
		WorkspaceID: "ws",
		CaseID:      1,
		Timestamp:   time.Date(2026, 5, 23, 10, 5, 0, 0, time.UTC),
		LastRunAt:   time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
	}
	got, err := job.BuildSystemPrompt(job.PromptInputs{
		Job: j, Workspace: newWorkspace("ws", "WS"), Case: newCase(1), Event: ev,
	})
	gt.NoError(t, err).Required()
	mustContain(t, got, "the time since the last run reaches 1h0m0s")
	mustContain(t, got, "every=1h0m0s")
	mustContain(t, got, "last_run_at=2026-05-23T09:00:00Z")
	mustContain(t, got, "now=2026-05-23T10:05:00Z")
}

func TestBuildSystemPrompt_ScheduledCron(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse("0 9 * * *")
	gt.NoError(t, err).Required()

	j := &model.Job{
		ID:     "daily",
		Prompt: "x",
		Events: model.JobEvents{
			Scheduled: &model.ScheduledEventConfig{Cron: sched, CronExpr: "0 9 * * *"},
		},
	}
	ev := job.Event{
		Domain:       model.JobEventDomainScheduled,
		WorkspaceID:  "ws",
		CaseID:       1,
		Timestamp:    time.Date(2026, 5, 23, 9, 5, 0, 0, time.UTC),
		LastRunAt:    time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		ScheduledFor: time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
	}
	got, err := job.BuildSystemPrompt(job.PromptInputs{
		Job: j, Workspace: newWorkspace("ws", "WS"), Case: newCase(1), Event: ev,
	})
	gt.NoError(t, err).Required()
	mustContain(t, got, "a cron tick of `0 9 * * *` arrives (UTC)")
	mustContain(t, got, "scheduled_for=2026-05-23T09:00:00Z")
}

func TestRenderUserPrompt_TemplateExpansion(t *testing.T) {
	j := &model.Job{
		ID:     "demo",
		Prompt: "case {{.Case.Title}} created by {{.Event.ActorUserID}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	out, err := job.RenderUserPrompt(job.PromptInputs{
		Job: j, Case: newCase(7),
		Event: job.Event{ActorUserID: "U-X", CaseLifecycle: model.CaseLifecycleCreated},
	})
	gt.NoError(t, err).Required()
	gt.String(t, out).Equal("case Sample created by U-X")
}

func TestRenderUserPrompt_TemplateError(t *testing.T) {
	j := &model.Job{
		ID:     "bad",
		Prompt: "{{.Missing.Field",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	_, err := job.RenderUserPrompt(job.PromptInputs{Job: j, Case: newCase(1)})
	gt.Error(t, err)
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("expected prompt to contain %q\n---\n%s\n---", sub, s)
	}
}
