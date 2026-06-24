package casebound_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/casebound"
)

// fakeCaseMutator satisfies casewriter.CaseMutator for tool-wiring tests.
type fakeCaseMutator struct{}

func (fakeCaseMutator) UpdateCase(context.Context, string, int64, casewriter.CaseUpdate) (*model.Case, error) {
	return &model.Case{}, nil
}

func (fakeCaseMutator) UpdateCaseStatus(context.Context, string, int64, string) (*model.Case, error) {
	return &model.Case{}, nil
}

func (fakeCaseMutator) CloseCase(context.Context, string, int64) (*model.Case, error) {
	return &model.Case{}, nil
}

func (fakeCaseMutator) AssignCase(_ context.Context, _ string, _ int64, _ []string) (*model.Case, error) {
	return &model.Case{}, nil
}

func (fakeCaseMutator) UnassignCase(_ context.Context, _ string, _ int64, _ []string) (*model.Case, error) {
	return &model.Case{}, nil
}

func toolNames(tools []gollem.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Spec().Name] = true
	}
	return out
}

func TestBuildSystemPrompt_CaseAndFieldValues(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity Level", Type: types.FieldTypeSelect},
			},
		},
	}
	c := &model.Case{
		Title:       "Important Case",
		Description: "This is very important",
		Status:      types.CaseStatusOpen,
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
		},
	}
	messages := []casebound.ConversationMessage{
		{UserID: "U001", UserName: "alice", Text: "Hello", Timestamp: "1234567890.000001"},
	}
	now := time.Date(2026, 5, 4, 12, 30, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, nil, nil, messages)

	gt.String(t, prompt).Contains("Important Case")
	gt.String(t, prompt).Contains("This is very important")
	gt.String(t, prompt).Contains("Severity Level")
	gt.String(t, prompt).Contains("high")
	gt.String(t, prompt).Contains("alice: Hello")
	gt.String(t, prompt).Contains("Slack's mrkdwn format")
	gt.String(t, prompt).Contains("Do NOT use Markdown headers")
}

func TestBuildSystemPrompt_ChannelIDAndTime(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
	}
	c := &model.Case{Title: "Test Case", Status: types.CaseStatusOpen}
	now := time.Date(2026, 5, 4, 12, 30, 45, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0123ABC", now, nil, nil, nil)

	gt.String(t, prompt).Contains("## Slack Context")
	gt.String(t, prompt).Contains("Channel ID: C0123ABC")
	gt.String(t, prompt).Contains("## Current Time")
	gt.String(t, prompt).Contains("2026-05-04T12:30:45Z")
}

func TestBuildSystemPrompt_CaseWideActionsTitleOnly(t *testing.T) {
	entry := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-test", Name: "Test"}}
	c := &model.Case{Title: "Test Case", Status: types.CaseStatusOpen}
	actions := []*model.Action{
		{ID: 1, Title: "Investigate the issue", Status: types.ActionStatusInProgress, AssigneeID: "U001"},
		{ID: 2, Title: "Write report", Status: types.ActionStatusTodo},
	}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, nil, actions, nil)

	gt.String(t, prompt).Contains("## Actions")
	gt.String(t, prompt).Contains("Investigate the issue")
	gt.String(t, prompt).Contains("Write report")
	// Status / Assignee detail must NOT leak into the case-wide list.
	gt.Bool(t, strings.Contains(prompt, "U001")).False()
	gt.Bool(t, strings.Contains(prompt, "IN_PROGRESS")).False()
	gt.Bool(t, strings.Contains(prompt, "TODO")).False()
	// And the Current Action section must be absent.
	gt.Bool(t, strings.Contains(prompt, "## Current Action")).False()
}

func TestBuildSystemPrompt_CurrentActionInActionThread(t *testing.T) {
	entry := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-test", Name: "Test"}}
	c := &model.Case{Title: "Test Case", Status: types.CaseStatusOpen}
	due := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	currentAction := &model.Action{
		ID:          7,
		Title:       "Patch the vulnerable library",
		Description: "Bump dep to 1.2.3 and rerun integration tests.",
		Status:      types.ActionStatusInProgress,
		AssigneeID:  "U777",
		DueDate:     &due,
	}
	others := []*model.Action{{ID: 8, Title: "Sibling action"}}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, currentAction, others, nil)

	gt.String(t, prompt).Contains("## Current Action")
	gt.String(t, prompt).Contains("Patch the vulnerable library")
	gt.String(t, prompt).Contains("IN_PROGRESS")
	gt.String(t, prompt).Contains("Assignee: U777")
	gt.String(t, prompt).Contains("Bump dep to 1.2.3")
	gt.String(t, prompt).Contains("2026-06-01T09:00:00Z")
	// Case-wide actions must be suppressed in this mode.
	gt.Bool(t, strings.Contains(prompt, "## Actions")).False()
	gt.Bool(t, strings.Contains(prompt, "Sibling action")).False()
}

func TestBuildSystemPrompt_CurrentActionWithoutAssignee(t *testing.T) {
	entry := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-test", Name: "Test"}}
	c := &model.Case{Title: "Test Case", Status: types.CaseStatusOpen}
	currentAction := &model.Action{ID: 9, Title: "Triage", Status: types.ActionStatusTodo}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, currentAction, nil, nil)

	gt.String(t, prompt).Contains("Assignee: unassigned")
	gt.Bool(t, strings.Contains(prompt, "- Due:")).False()
	gt.Bool(t, strings.Contains(prompt, "### Description")).False()
}

func TestBuildSystemPrompt_NoActionsSection(t *testing.T) {
	entry := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-test", Name: "Test"}}
	c := &model.Case{Title: "Test Case", Status: types.CaseStatusOpen}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, nil, nil, nil)

	gt.Bool(t, strings.Contains(prompt, "## Actions")).False()
	gt.Bool(t, strings.Contains(prompt, "## Current Action")).False()
}

func TestBuildUserInput_NoDelta(t *testing.T) {
	got := casebound.BuildUserInputForTest(nil, "@bot ping", "1700000000.000001")
	gt.String(t, got).Equal("@bot ping")
}

func TestBuildUserInput_WithDelta(t *testing.T) {
	delta := []casebound.ConversationMessage{
		{UserID: "U1", UserName: "alice", Text: "still here", Timestamp: "1700000005.000001"},
		{UserID: "U2", Text: "no name", Timestamp: "1700000006.000001"},
	}
	got := casebound.BuildUserInputForTest(delta, "@bot follow up", "1700000010.000001")
	gt.String(t, got).Contains("# Unprocessed thread messages since last mention")
	gt.String(t, got).Contains("alice: still here")
	gt.String(t, got).Contains("U2: no name")
	gt.String(t, got).Contains("# Current mention")
	gt.String(t, got).Contains("@bot follow up")
}

func TestBuildUserInput_SkipsCurrentMentionInDelta(t *testing.T) {
	currentTS := "1700000020.000001"
	delta := []casebound.ConversationMessage{
		{UserID: "U1", UserName: "alice", Text: "older", Timestamp: "1700000015.000001"},
		{UserID: "U1", UserName: "alice", Text: "current message text", Timestamp: currentTS},
	}
	got := casebound.BuildUserInputForTest(delta, "current message text", currentTS)
	// The delta line for the current mention TS must not be duplicated.
	occurrences := strings.Count(got, "current message text")
	gt.Number(t, occurrences).Equal(1)
}

func TestBuildSystemPrompt_EditableFieldsAndStatuses(t *testing.T) {
	statusSet, err := model.NewActionStatusSet("open", []string{"closed"}, []model.ActionStatusDefinition{
		{ID: "open", Name: "Open", Description: "Work has not started"},
		{ID: "closed", Name: "Closed", Description: "Work is fully resolved"},
	})
	gt.NoError(t, err).Required()

	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Description: "How urgent the case is", Options: []config.FieldOption{
					{ID: "high", Name: "High", Description: "Needs immediate attention"},
					{ID: "low", Name: "Low", Description: "Can wait"},
				}},
			},
		},
		CaseStatusSet: statusSet,
	}
	c := &model.Case{Title: "Case", Status: types.CaseStatusOpen}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	prompt := casebound.BuildSystemPromptForTest(c, entry, "C0TEST", now, nil, nil, nil)

	gt.String(t, prompt).Contains("Editable Custom Fields")
	gt.String(t, prompt).Contains("`severity`")
	// The field-level description must reach the agent.
	gt.String(t, prompt).Contains("How urgent the case is")
	gt.String(t, prompt).Contains("(required)")
	// Each select option must surface its id, name, and description.
	gt.String(t, prompt).Contains("`high`")
	gt.String(t, prompt).Contains(`name="High"`)
	gt.String(t, prompt).Contains("Needs immediate attention")
	gt.String(t, prompt).Contains("`low`")
	gt.String(t, prompt).Contains("Can wait")
	gt.String(t, prompt).Contains("Board Statuses")
	gt.String(t, prompt).Contains("`closed`")
	gt.String(t, prompt).Contains("(closed)")
	// Status descriptions must reach the agent so it knows when to pick one.
	gt.String(t, prompt).Contains("Work has not started")
	gt.String(t, prompt).Contains("Work is fully resolved")
}

func TestBuildTools_CaseWriterWiring(t *testing.T) {
	statusSet, err := model.NewActionStatusSet("open", []string{"closed"}, []model.ActionStatusDefinition{
		{ID: "open", Name: "Open"},
		{ID: "closed", Name: "Closed"},
	})
	gt.NoError(t, err).Required()

	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect}},
		},
		CaseStatusSet: statusSet,
	}
	req := casebound.TurnRequest{Workspace: entry, Case: &model.Case{ID: 1}}

	t.Run("no CaseUC means no case-write tools", func(t *testing.T) {
		tools := casebound.BuildToolsForTest(&agent.CommonDeps{}, req)
		names := toolNames(tools)
		gt.Bool(t, names["case__update_case"]).False()
		gt.Bool(t, names["case__update_case_status"]).False()
	})

	t.Run("with CaseUC and a status set, the board-status tool closes (no close tool)", func(t *testing.T) {
		tools := casebound.BuildToolsForTest(&agent.CommonDeps{CaseUC: fakeCaseMutator{}}, req)
		names := toolNames(tools)
		gt.Bool(t, names["case__update_case"]).True()
		gt.Bool(t, names["case__update_case_status"]).True()
		gt.Bool(t, names["case__close_case"]).False()
	})

	t.Run("with CaseUC but no status set, close tool replaces the status tool", func(t *testing.T) {
		noStatus := &model.WorkspaceEntry{
			Workspace:   model.Workspace{ID: "ws-test", Name: "Test"},
			FieldSchema: entry.FieldSchema,
		}
		reqNoStatus := casebound.TurnRequest{Workspace: noStatus, Case: &model.Case{ID: 1}}
		tools := casebound.BuildToolsForTest(&agent.CommonDeps{CaseUC: fakeCaseMutator{}}, reqNoStatus)
		names := toolNames(tools)
		gt.Bool(t, names["case__update_case"]).True()
		gt.Bool(t, names["case__update_case_status"]).False()
		gt.Bool(t, names["case__close_case"]).True()
	})
}

func TestBuildTools_ActionToolsByMode(t *testing.T) {
	entry := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws-test", Name: "Test"}}

	t.Run("channel-mode case gets the action tools", func(t *testing.T) {
		req := casebound.TurnRequest{Workspace: entry, Case: &model.Case{ID: 1}}
		names := toolNames(casebound.BuildToolsForTest(&agent.CommonDeps{}, req))
		gt.Bool(t, names["core__create_action"]).True()
		gt.Bool(t, names["core__update_action"]).True()
	})

	t.Run("thread-mode case omits the action tools", func(t *testing.T) {
		// Thread-mode cases have no Actions; offering tools the usecase boundary
		// would reject (ErrCaseThreadModeNoActions) is withheld here, mirroring
		// the Job runtime exclusion.
		req := casebound.TurnRequest{Workspace: entry, Case: &model.Case{ID: 1, SlackThreadTS: "1700000000.000001"}}
		names := toolNames(casebound.BuildToolsForTest(&agent.CommonDeps{}, req))
		gt.Bool(t, names["core__create_action"]).False()
		gt.Bool(t, names["core__update_action"]).False()
	})
}
