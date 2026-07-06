package casewriter_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type statusCall struct {
	boardStatus string
}

type assignCall struct {
	userIDs []string
}

type mockCaseUC struct {
	calls         []casewriter.CaseUpdate
	statusCalls   []statusCall
	assignCalls   []assignCall
	unassignCalls []assignCall
	closeCalls    int
	resp          *model.Case
	err           error
}

func (m *mockCaseUC) UpdateCase(ctx context.Context, workspaceID string, id int64, patch casewriter.CaseUpdate) (*model.Case, error) {
	m.calls = append(m.calls, patch)
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockCaseUC) UpdateCaseStatus(ctx context.Context, workspaceID string, id int64, boardStatus string) (*model.Case, error) {
	m.statusCalls = append(m.statusCalls, statusCall{boardStatus: boardStatus})
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockCaseUC) CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	m.closeCalls++
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockCaseUC) AssignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	m.assignCalls = append(m.assignCalls, assignCall{userIDs: userIDs})
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func (m *mockCaseUC) UnassignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	m.unassignCalls = append(m.unassignCalls, assignCall{userIDs: userIDs})
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func toolByName(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	for _, tl := range tools {
		if tl.Spec().Name == name {
			return tl
		}
	}
	return nil
}

func testSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "summary", Name: "Summary", Type: types.FieldTypeText},
			{ID: "score", Name: "Score", Type: types.FieldTypeNumber},
			{ID: "tags", Name: "Tags", Type: types.FieldTypeMultiSelect, Options: []config.FieldOption{{ID: "a"}, {ID: "b"}}},
		},
	}
}

func testStatusSet(t *testing.T) *model.ActionStatusSet {
	t.Helper()
	set, err := model.NewActionStatusSet("open", []string{"closed"}, []model.ActionStatusDefinition{
		{ID: "open", Name: "Open"},
		{ID: "in_progress", Name: "In Progress"},
		{ID: "closed", Name: "Closed"},
	})
	gt.NoError(t, err).Required()
	return set
}

func TestUpdateCaseTool_Title(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 42, Title: "new", Status: types.CaseStatusOpen}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 42})
	// New returns update + assign + unassign + close (4 tools when no StatusSet:
	// channel-mode closes via case__close_case).
	gt.Array(t, tools).Length(4).Required()

	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()

	out, err := updateTool.Run(context.Background(), map[string]any{
		"title": "new",
	})
	gt.NoError(t, err).Required()

	gt.Array(t, uc.calls).Length(1).Required()
	gt.Value(t, uc.calls[0].Title).NotNil()
	gt.String(t, *uc.calls[0].Title).Equal("new")
	gt.Value(t, uc.calls[0].Description).Nil()

	gt.Number(t, out["id"].(int64)).Equal(int64(42))
	gt.String(t, out["title"].(string)).Equal("new")
}

func TestUpdateCaseTool_NoAssigneeIDsParam(t *testing.T) {
	// case__update_case must NOT expose an assignee_ids parameter; assignees
	// are mutated exclusively through case__assign / case__unassign.
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	_, has := updateTool.Spec().Parameters["assignee_ids"]
	gt.Bool(t, has).False()
}

func TestAssignCaseTool(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 1, AssigneeIDs: []string{"U1", "U2"}, Status: types.CaseStatusOpen}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	assignTool := toolByName(t, tools, "case__assign")
	gt.Value(t, assignTool).NotNil().Required()

	t.Run("calls AssignCase with provided user IDs and returns assignee_ids", func(t *testing.T) {
		uc.assignCalls = nil
		out, err := assignTool.Run(context.Background(), map[string]any{
			"user_ids": []any{"U1", "U2"},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.assignCalls).Length(1).Required()
		gt.Array(t, uc.assignCalls[0].userIDs).Length(2).Required()
		gt.String(t, uc.assignCalls[0].userIDs[0]).Equal("U1")
		gt.String(t, uc.assignCalls[0].userIDs[1]).Equal("U2")
		gt.Value(t, out["assignee_ids"]).Equal([]string{"U1", "U2"})
	})

	t.Run("missing user_ids errors", func(t *testing.T) {
		uc.assignCalls = nil
		_, err := assignTool.Run(context.Background(), map[string]any{})
		gt.Error(t, err)
		gt.Array(t, uc.assignCalls).Length(0)
	})

	t.Run("empty user_ids errors", func(t *testing.T) {
		uc.assignCalls = nil
		_, err := assignTool.Run(context.Background(), map[string]any{
			"user_ids": []any{},
		})
		gt.Error(t, err)
		gt.Array(t, uc.assignCalls).Length(0)
	})
}

func TestUnassignCaseTool(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 2, AssigneeIDs: []string{"U3"}, Status: types.CaseStatusOpen}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 2})
	unassignTool := toolByName(t, tools, "case__unassign")
	gt.Value(t, unassignTool).NotNil().Required()

	t.Run("calls UnassignCase with provided user IDs and returns assignee_ids", func(t *testing.T) {
		uc.unassignCalls = nil
		out, err := unassignTool.Run(context.Background(), map[string]any{
			"user_ids": []any{"U3"},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.unassignCalls).Length(1).Required()
		gt.Array(t, uc.unassignCalls[0].userIDs).Length(1).Required()
		gt.String(t, uc.unassignCalls[0].userIDs[0]).Equal("U3")
		gt.Value(t, out["assignee_ids"]).Equal([]string{"U3"})
	})

	t.Run("missing user_ids errors", func(t *testing.T) {
		uc.unassignCalls = nil
		_, err := unassignTool.Run(context.Background(), map[string]any{})
		gt.Error(t, err)
		gt.Array(t, uc.unassignCalls).Length(0)
	})

	t.Run("empty user_ids errors", func(t *testing.T) {
		uc.unassignCalls = nil
		_, err := unassignTool.Run(context.Background(), map[string]any{
			"user_ids": []any{},
		})
		gt.Error(t, err)
		gt.Array(t, uc.unassignCalls).Length(0)
	})
}

func TestUpdateCaseTool_Fields(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 7, Status: types.CaseStatusOpen, FieldValues: map[string]model.FieldValue{
		"summary": {FieldID: "summary", Type: types.FieldTypeText, Value: "done"},
	}}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 7, Schema: testSchema()})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()

	t.Run("coerces scalar, number and multi values", func(t *testing.T) {
		uc.calls = nil
		_, err := updateTool.Run(context.Background(), map[string]any{
			"fields": []any{
				map[string]any{"field_id": "summary", "value": "done"},
				map[string]any{"field_id": "score", "value": "42"},
				map[string]any{"field_id": "tags", "values": []any{"a", "b"}},
			},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.calls).Length(1).Required()
		f := uc.calls[0].Fields
		gt.Value(t, f["summary"].Value).Equal("done")
		gt.Value(t, f["score"].Value).Equal(float64(42))
		gt.Value(t, f["tags"].Value).Equal([]string{"a", "b"})
	})

	t.Run("unparseable number is rejected before reaching the usecase", func(t *testing.T) {
		uc.calls = nil
		_, err := updateTool.Run(context.Background(), map[string]any{
			"fields": []any{
				map[string]any{"field_id": "score", "value": "not-a-number"},
			},
		})
		gt.Error(t, err)
		gt.Array(t, uc.calls).Length(0)
	})

	t.Run("fields without a schema errors", func(t *testing.T) {
		uc.calls = nil
		noSchema := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 7})
		noSchemaTool := toolByName(t, noSchema, "case__update_case")
		gt.Value(t, noSchemaTool).NotNil().Required()
		_, err := noSchemaTool.Run(context.Background(), map[string]any{
			"fields": []any{map[string]any{"field_id": "summary", "value": "x"}},
		})
		gt.Error(t, err)
		gt.Array(t, uc.calls).Length(0)
	})
}

func TestUpdateCaseTool_RejectsEmptyPatch(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	_, err := updateTool.Run(context.Background(), map[string]any{})
	gt.Error(t, err)
	gt.Array(t, uc.calls).Length(0)
}

func TestUpdateCaseTool_PropagatesUseCaseError(t *testing.T) {
	sentinel := goerr.New("boom")
	uc := &mockCaseUC{err: sentinel}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	_, err := updateTool.Run(context.Background(), map[string]any{
		"title": "x",
	})
	gt.Error(t, err).Is(sentinel)
}

func TestUpdateCaseTool_NilCaseUCErrors(t *testing.T) {
	tools := casewriter.New(casewriter.Deps{WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	_, err := updateTool.Run(context.Background(), map[string]any{"title": "x"})
	gt.Error(t, err)
}

func TestUpdateCaseTool_HasNoStatusParameter(t *testing.T) {
	// Status transitions live in the separate case__update_case_status tool;
	// the field-update tool must NOT carry a status parameter.
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	spec := updateTool.Spec()
	_, has := spec.Parameters["status"]
	gt.Bool(t, has).False()
	_, hasFields := spec.Parameters["fields"]
	gt.Bool(t, hasFields).True()
}

func TestUpdateCaseTool_DescriptionWarnsAgainstBlindOverwrite(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	updateTool := toolByName(t, tools, "case__update_case")
	gt.Value(t, updateTool).NotNil().Required()
	desc := updateTool.Spec().Description
	gt.String(t, desc).Contains("Do not overwrite blindly")
	gt.String(t, desc).Contains("FULL replacements")
}

func TestStatusTool_NotBuiltWithoutStatusSet(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	// Without a StatusSet: update + assign + unassign + close = 4 tools. The
	// board-status tool is absent and case__close_case takes its place as the
	// channel-mode "mark done" path.
	gt.Array(t, tools).Length(4)
	gt.Value(t, toolByName(t, tools, "case__update_case_status")).Nil()
	gt.Value(t, toolByName(t, tools, "case__close_case")).NotNil()
}

func TestCloseTool_NotBuiltWithStatusSet(t *testing.T) {
	// Thread-mode workspaces close via the board-status tool, so case__close_case
	// must NOT be offered alongside it (one "mark done" path per mode).
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1, StatusSet: testStatusSet(t)})
	gt.Value(t, toolByName(t, tools, "case__close_case")).Nil()
	gt.Value(t, toolByName(t, tools, "case__update_case_status")).NotNil()
}

func TestCloseTool(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 9, Status: types.CaseStatusClosed}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 9})
	closeTool := toolByName(t, tools, "case__close_case")
	gt.Value(t, closeTool).NotNil().Required()

	t.Run("spec takes no parameters", func(t *testing.T) {
		gt.Number(t, len(closeTool.Spec().Parameters)).Equal(0)
	})

	t.Run("run closes the case and returns the closed status", func(t *testing.T) {
		uc.closeCalls = 0
		out, err := closeTool.Run(context.Background(), map[string]any{})
		gt.NoError(t, err).Required()
		gt.Number(t, uc.closeCalls).Equal(1)
		gt.Number(t, out["id"].(int64)).Equal(int64(9))
		gt.String(t, out["status"].(string)).Equal(types.CaseStatusClosed.String())
	})

	t.Run("propagates the usecase error", func(t *testing.T) {
		sentinel := goerr.New("already closed")
		failing := &mockCaseUC{err: sentinel}
		failTools := casewriter.New(casewriter.Deps{CaseUC: failing, WorkspaceID: "ws", CaseID: 9})
		failClose := toolByName(t, failTools, "case__close_case")
		gt.Value(t, failClose).NotNil().Required()
		_, err := failClose.Run(context.Background(), map[string]any{})
		gt.Error(t, err).Is(sentinel)
	})

	t.Run("nil CaseUC errors", func(t *testing.T) {
		nilTools := casewriter.New(casewriter.Deps{WorkspaceID: "ws", CaseID: 9})
		nilClose := toolByName(t, nilTools, "case__close_case")
		gt.Value(t, nilClose).NotNil().Required()
		_, err := nilClose.Run(context.Background(), map[string]any{})
		gt.Error(t, err)
	})
}

func TestStatusTool_BuiltWithStatusSet(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 5, Status: types.CaseStatusClosed, BoardStatus: "closed"}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 5, StatusSet: testStatusSet(t)})
	// With StatusSet: update + assign + unassign + status = 4 tools.
	gt.Array(t, tools).Length(4)

	statusTool := toolByName(t, tools, "case__update_case_status")
	gt.Value(t, statusTool).NotNil().Required()

	t.Run("spec enumerates the configured status ids", func(t *testing.T) {
		enum := statusTool.Spec().Parameters["status"].Enum
		gt.Array(t, enum).Equal([]string{"open", "in_progress", "closed"})
	})

	t.Run("run forwards the status to the usecase", func(t *testing.T) {
		uc.statusCalls = nil
		out, err := statusTool.Run(context.Background(), map[string]any{"status": "closed"})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.statusCalls).Length(1).Required()
		gt.String(t, uc.statusCalls[0].boardStatus).Equal("closed")
		gt.String(t, out["board_status"].(string)).Equal("closed")
	})

	t.Run("missing status errors", func(t *testing.T) {
		uc.statusCalls = nil
		_, err := statusTool.Run(context.Background(), map[string]any{})
		gt.Error(t, err)
		gt.Array(t, uc.statusCalls).Length(0)
	})
}

func TestNewStatusTool_ThreadMode(t *testing.T) {
	// A board status set means thread-mode: the ONLY tool is
	// case__update_case_status. The content / assignee tools are deliberately
	// excluded — a sub-agent may close/transition but never materialize.
	uc := &mockCaseUC{resp: &model.Case{ID: 5, Status: types.CaseStatusClosed, BoardStatus: "closed"}}
	tools := casewriter.NewStatusTool(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 5, StatusSet: testStatusSet(t)})
	gt.Array(t, tools).Length(1).Required()
	gt.String(t, tools[0].Spec().Name).Equal("case__update_case_status")
	gt.Value(t, toolByName(t, tools, "case__update_case")).Nil()
	gt.Value(t, toolByName(t, tools, "case__assign")).Nil()
	gt.Value(t, toolByName(t, tools, "case__unassign")).Nil()

	out, err := tools[0].Run(context.Background(), map[string]any{"status": "closed"})
	gt.NoError(t, err).Required()
	gt.Array(t, uc.statusCalls).Length(1).Required()
	gt.String(t, uc.statusCalls[0].boardStatus).Equal("closed")
	gt.String(t, out["board_status"].(string)).Equal("closed")
}

func TestNewStatusTool_ChannelMode(t *testing.T) {
	// No board status set means channel-mode: the ONLY tool is case__close_case.
	uc := &mockCaseUC{resp: &model.Case{ID: 9, Status: types.CaseStatusClosed}}
	tools := casewriter.NewStatusTool(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 9})
	gt.Array(t, tools).Length(1).Required()
	gt.String(t, tools[0].Spec().Name).Equal("case__close_case")

	_, err := tools[0].Run(context.Background(), map[string]any{})
	gt.NoError(t, err).Required()
	gt.Number(t, uc.closeCalls).Equal(1)
}
