package job_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/robfig/cron/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
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

func TestBuildSystemPrompt_ThreadModeOmitsActions(t *testing.T) {
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
	ws := newWorkspace("ws", "WS")
	ws.CaseMode = model.CaseModeThread

	// Even if actions were somehow passed in, a thread-mode prompt must not
	// surface them: the agent has no action tools there.
	actions := []*model.Action{{ID: 7, Title: "stray", Status: types.ActionStatusTodo}}

	got, err := job.BuildSystemPrompt(job.PromptInputs{
		Job: j, Workspace: ws, Case: newCase(42), Actions: actions, Event: ev,
	})
	gt.NoError(t, err).Required()

	// Action section and action-specific guardrails are gone.
	mustNotContain(t, got, "# Actions (existing, non-archived)")
	mustNotContain(t, got, "stray")
	mustNotContain(t, got, "archive actions")
	mustNotContain(t, got, "action list")
	mustContain(t, got, "you do not manage Actions")

	// Non-action sections still render.
	mustContain(t, got, "# Case")
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

func mustNotContain(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Errorf("expected prompt NOT to contain %q\n---\n%s\n---", sub, s)
	}
}

func TestBuildSystemPrompt_CaseAgentAdditionalPrompt(t *testing.T) {
	j := &model.Job{
		ID:     "incident-rca",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	ev := job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        7,
		Timestamp:     time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	}

	t.Run("renders Per-case operator notes when prompt is set", func(t *testing.T) {
		c := newCase(7)
		c.AgentAdditionalPrompt = "### Custom guidance\n- always cite source IDs\n- escalate to #incident-7"
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: j, Workspace: newWorkspace("ws", "WS"), Case: c, Event: ev,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Per-case operator notes")
		mustContain(t, got, "### Custom guidance")
		mustContain(t, got, "always cite source IDs")
		mustContain(t, got, "escalate to #incident-7")
		// Sanity: section comes before Actions so the operator notes
		// are read in the context of the Case body.
		idxNotes := strings.Index(got, "# Per-case operator notes")
		idxActions := strings.Index(got, "# Actions")
		gt.Number(t, idxNotes).LessOrEqual(idxActions - 1)
	})

	t.Run("section is omitted when prompt is empty", func(t *testing.T) {
		c := newCase(7)
		c.AgentAdditionalPrompt = ""
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: j, Workspace: newWorkspace("ws", "WS"), Case: c, Event: ev,
		})
		gt.NoError(t, err).Required()
		mustNotContain(t, got, "# Per-case operator notes")
	})
}

func TestBuildSystemPrompt_SourcesSection(t *testing.T) {
	j := &model.Job{
		ID:     "incident-rca",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	ev := job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        7,
		Timestamp:     time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		CaseLifecycle: model.CaseLifecycleCreated,
	}

	mkSource := func(id, name, desc string, t model.SourceType) *model.Source {
		return &model.Source{
			ID:          model.SourceID(id),
			Name:        name,
			SourceType:  t,
			Description: desc,
			Enabled:     true,
		}
	}

	t.Run("operator-narrowed phrasing when SourcesNarrowed is true", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       j,
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     ev,
			Sources: []*model.Source{
				mkSource("src-1", "AWS CloudTrail", "Audit log for AWS", model.SourceTypeSlack),
				mkSource("src-2", "Datadog Logs", "", model.SourceTypeGitHub),
			},
			SourcesNarrowed: true,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Sources")
		mustContain(t, got, "operator explicitly preferred")
		mustContain(t, got, "NOT a hard")
		mustContain(t, got, "src-1")
		mustContain(t, got, "AWS CloudTrail")
		mustContain(t, got, "Audit log for AWS")
		mustContain(t, got, "src-2")
		mustContain(t, got, "Datadog Logs")
	})

	t.Run("full-catalogue phrasing when SourcesNarrowed is false", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       j,
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     ev,
			Sources: []*model.Source{
				mkSource("src-1", "AWS CloudTrail", "Audit log for AWS", model.SourceTypeSlack),
			},
			SourcesNarrowed: false,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Sources")
		mustContain(t, got, "No per-case Source selection")
		mustContain(t, got, "src-1")
		mustContain(t, got, "AWS CloudTrail")
		mustNotContain(t, got, "operator explicitly preferred")
	})

	t.Run("section is omitted when Sources slice is empty", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       j,
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     ev,
		})
		gt.NoError(t, err).Required()
		mustNotContain(t, got, "# Sources")
	})
}

// fieldSchemaFixture is the workspace FieldSchema used by the
// custom-fields / field_values tests below. It exercises required,
// description, select / multi-select options, option descriptions, and
// option metadata so a single fixture covers every template branch.
func fieldSchemaFixture() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:          "severity",
				Name:        "Severity",
				Type:        types.FieldTypeSelect,
				Required:    true,
				Description: "How severe the case is.",
				Options: []config.FieldOption{
					{ID: "low", Name: "Low", Description: "Minor impact", Metadata: map[string]any{"score": 1}},
					{ID: "high", Name: "High", Description: "Severe impact", Metadata: map[string]any{"score": 4}},
				},
			},
			{
				ID:   "affected_systems",
				Name: "Affected systems",
				Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{
					{ID: "prod", Name: "Production"},
					{ID: "staging", Name: "Staging"},
				},
			},
			{
				ID:   "notes",
				Name: "Notes",
				Type: types.FieldTypeText,
			},
		},
	}
}

func newCustomFieldsWorkspace() *model.WorkspaceEntry {
	ws := newWorkspace("ws", "WS")
	ws.FieldSchema = fieldSchemaFixture()
	return ws
}

func caseCreatedEvent() job.Event {
	return job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        7,
		Timestamp:     time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	}
}

func caseCreatedJob() *model.Job {
	return &model.Job{
		ID:     "j",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
}

func TestBuildSystemPrompt_CustomFieldsSchema(t *testing.T) {
	t.Run("required marker and description are rendered", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- custom fields:")
		mustContain(t, got, "- severity (select): Severity [required]")
		mustContain(t, got, "description: How severe the case is.")
		// Non-required field has no [required] marker.
		mustContain(t, got, "- affected_systems (multi-select): Affected systems")
		mustNotContain(t, got, "Affected systems [required]")
		// Text field with no description, no options: just the header
		// line is emitted, no trailing metadata lines.
		mustContain(t, got, "- notes (text): Notes")
	})

	t.Run("select options carry name, description, and metadata", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "options:")
		mustContain(t, got, "- low — Low (Minor impact) [score=1]")
		mustContain(t, got, "- high — High (Severe impact) [score=4]")
		// Multi-select option with no description / metadata renders
		// only the ID and Name.
		mustContain(t, got, "- prod — Production")
		mustNotContain(t, got, "Production (")
		mustNotContain(t, got, "Production [")
	})

	t.Run("custom fields section is omitted when schema is nil", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustNotContain(t, got, "- custom fields:")
	})

	t.Run("custom fields section is omitted when schema has no fields", func(t *testing.T) {
		ws := newWorkspace("ws", "WS")
		ws.FieldSchema = &config.FieldSchema{}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: ws,
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustNotContain(t, got, "- custom fields:")
	})
}

func TestBuildSystemPrompt_CaseFieldValuesResolution(t *testing.T) {
	t.Run("select value resolves to option name", func(t *testing.T) {
		c := newCase(7)
		c.FieldValues = map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      c,
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- severity: high (High)")
	})

	t.Run("multi-select []string is joined and resolved", func(t *testing.T) {
		c := newCase(7)
		c.FieldValues = map[string]model.FieldValue{
			"affected_systems": {
				FieldID: "affected_systems",
				Type:    types.FieldTypeMultiSelect,
				Value:   []string{"prod", "staging"},
			},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      c,
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- affected_systems: prod, staging (Production, Staging)")
	})

	t.Run("multi-select []any (from Firestore decode) is joined and resolved", func(t *testing.T) {
		c := newCase(7)
		c.FieldValues = map[string]model.FieldValue{
			"affected_systems": {
				FieldID: "affected_systems",
				Type:    types.FieldTypeMultiSelect,
				Value:   []any{"staging", "prod"},
			},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      c,
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- affected_systems: staging, prod (Staging, Production)")
	})

	t.Run("unknown option id falls back to raw value with no parenthetical", func(t *testing.T) {
		c := newCase(7)
		c.FieldValues = map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "ghost"},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      c,
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- severity: ghost")
		// No parenthetical because the option ID did not resolve.
		mustNotContain(t, got, "severity: ghost (")
	})

	t.Run("text field renders verbatim with no resolution", func(t *testing.T) {
		c := newCase(7)
		c.FieldValues = map[string]model.FieldValue{
			"notes": {FieldID: "notes", Type: types.FieldTypeText, Value: "free text here"},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newCustomFieldsWorkspace(),
			Case:      c,
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "- notes: free text here")
		mustNotContain(t, got, "notes: free text here (")
	})
}

func TestBuildSystemPrompt_CurrentTime(t *testing.T) {
	t.Run("renders the turn start time normalised to UTC RFC3339", func(t *testing.T) {
		// Deliberately non-UTC input to prove the section is normalised.
		jst := time.FixedZone("JST", 9*60*60)
		now := time.Date(2026, 5, 23, 21, 30, 0, 0, jst)
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
			Now:       now,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Current time")
		mustContain(t, got, "The current time (this turn's execution start) is 2026-05-23T12:30:00Z (UTC).")
	})

	t.Run("omits the section when Now is the zero value", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job:       caseCreatedJob(),
			Workspace: newWorkspace("ws", "WS"),
			Case:      newCase(7),
			Event:     caseCreatedEvent(),
		})
		gt.NoError(t, err).Required()
		mustNotContain(t, got, "# Current time")
	})
}

func newWorkspaceWithMemo(id, name string) *model.WorkspaceEntry {
	ws := newWorkspace(id, name)
	ws.MemoConfig = &config.MemoConfig{
		Description: "Investigation memory for this case.",
		FieldSchema: &config.FieldSchema{Fields: []config.FieldDefinition{
			{ID: "memo_type", Name: "Type", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{
				{ID: "fact", Name: "Fact"},
			}},
			{ID: "body", Name: "Body", Type: types.FieldTypeText},
		}},
	}
	return ws
}

func TestBuildSystemPrompt_MemoSection(t *testing.T) {
	ev := job.Event{Domain: model.JobEventDomainCase, WorkspaceID: "ws", CaseID: 42}

	t.Run("renders definition, fields and active memos", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
		memos := []*model.Memo{
			{ID: "m-1", WorkspaceID: "ws", CaseID: 42, Title: "first memory", CreatedAt: now, UpdatedAt: now},
			{ID: "m-2", WorkspaceID: "ws", CaseID: 42, Title: "second memory", CreatedAt: now, UpdatedAt: now},
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: &model.Job{ID: "j"}, Workspace: newWorkspaceWithMemo("ws", "WS"), Case: newCase(42), Memos: memos, Event: ev,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Memos (case-scoped memory)")
		mustContain(t, got, "Investigation memory for this case.")
		mustContain(t, got, "memo_type (select): Type [required]")
		mustContain(t, got, "Current memos (2 total)")
		mustContain(t, got, "`m-1` first memory")
		mustContain(t, got, "`m-2` second memory")
	})

	t.Run("caps preview at 20 and reports overflow", func(t *testing.T) {
		now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
		var memos []*model.Memo
		for i := range 25 {
			memos = append(memos, &model.Memo{ID: model.MemoID("m-" + string(rune('a'+i))), WorkspaceID: "ws", CaseID: 42, Title: "memory", CreatedAt: now, UpdatedAt: now})
		}
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: &model.Job{ID: "j"}, Workspace: newWorkspaceWithMemo("ws", "WS"), Case: newCase(42), Memos: memos, Event: ev,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "Current memos (25 total, showing first 20)")
		mustContain(t, got, "more memos exist; use memo__list_memos")
	})

	t.Run("empty active memos render none-yet", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: &model.Job{ID: "j"}, Workspace: newWorkspaceWithMemo("ws", "WS"), Case: newCase(42), Event: ev,
		})
		gt.NoError(t, err).Required()
		mustContain(t, got, "# Memos (case-scoped memory)")
		mustContain(t, got, "(none yet)")
	})

	t.Run("memo-disabled workspace omits the section", func(t *testing.T) {
		got, err := job.BuildSystemPrompt(job.PromptInputs{
			Job: &model.Job{ID: "j"}, Workspace: newWorkspace("ws", "WS"), Case: newCase(42), Event: ev,
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, strings.Contains(got, "# Memos (case-scoped memory)")).False()
	})
}
