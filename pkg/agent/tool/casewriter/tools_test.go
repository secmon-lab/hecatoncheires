package casewriter_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type statusCall struct {
	boardStatus string
}

type mockCaseUC struct {
	calls       []casewriter.CaseUpdate
	statusCalls []statusCall
	resp        *model.Case
	err         error
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
	gt.Array(t, tools).Length(1).Required()

	out, err := tools[0].Run(context.Background(), map[string]any{
		"title": "new",
	})
	gt.NoError(t, err).Required()

	gt.Array(t, uc.calls).Length(1).Required()
	gt.Value(t, uc.calls[0].Title).NotNil()
	gt.String(t, *uc.calls[0].Title).Equal("new")
	gt.Value(t, uc.calls[0].Description).Nil()
	gt.Bool(t, uc.calls[0].HasAssign).False()

	gt.Number(t, out["id"].(int64)).Equal(int64(42))
	gt.String(t, out["title"].(string)).Equal("new")
}

func TestUpdateCaseTool_Assignees(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 1, AssigneeIDs: []string{"U1", "U2"}, Status: types.CaseStatusOpen}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})

	t.Run("set", func(t *testing.T) {
		uc.calls = nil
		_, err := tools[0].Run(context.Background(), map[string]any{
			"assignee_ids": []any{"U1", "U2"},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.calls).Length(1).Required()
		gt.Bool(t, uc.calls[0].HasAssign).True()
		gt.Array(t, uc.calls[0].AssigneeIDs).Length(2)
		gt.String(t, uc.calls[0].AssigneeIDs[0]).Equal("U1")
		gt.String(t, uc.calls[0].AssigneeIDs[1]).Equal("U2")
	})

	t.Run("clear", func(t *testing.T) {
		uc.calls = nil
		_, err := tools[0].Run(context.Background(), map[string]any{
			"assignee_ids": []any{},
		})
		gt.NoError(t, err).Required()
		gt.Array(t, uc.calls).Length(1).Required()
		gt.Bool(t, uc.calls[0].HasAssign).True()
		gt.Array(t, uc.calls[0].AssigneeIDs).Length(0)
	})

	t.Run("non-string element", func(t *testing.T) {
		uc.calls = nil
		_, err := tools[0].Run(context.Background(), map[string]any{
			"assignee_ids": []any{"U1", 42},
		})
		gt.Error(t, err)
		gt.Array(t, uc.calls).Length(0)
	})
}

func TestUpdateCaseTool_Fields(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 7, Status: types.CaseStatusOpen, FieldValues: map[string]model.FieldValue{
		"summary": {FieldID: "summary", Type: types.FieldTypeText, Value: "done"},
	}}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 7, Schema: testSchema()})

	t.Run("coerces scalar, number and multi values", func(t *testing.T) {
		uc.calls = nil
		_, err := tools[0].Run(context.Background(), map[string]any{
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
		_, err := tools[0].Run(context.Background(), map[string]any{
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
		_, err := noSchema[0].Run(context.Background(), map[string]any{
			"fields": []any{map[string]any{"field_id": "summary", "value": "x"}},
		})
		gt.Error(t, err)
		gt.Array(t, uc.calls).Length(0)
	})
}

func TestUpdateCaseTool_RejectsEmptyPatch(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	_, err := tools[0].Run(context.Background(), map[string]any{})
	gt.Error(t, err)
	gt.Array(t, uc.calls).Length(0)
}

func TestUpdateCaseTool_PropagatesUseCaseError(t *testing.T) {
	sentinel := goerr.New("boom")
	uc := &mockCaseUC{err: sentinel}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	_, err := tools[0].Run(context.Background(), map[string]any{
		"title": "x",
	})
	gt.Error(t, err).Is(sentinel)
}

func TestUpdateCaseTool_NilCaseUCErrors(t *testing.T) {
	tools := casewriter.New(casewriter.Deps{WorkspaceID: "ws", CaseID: 1})
	_, err := tools[0].Run(context.Background(), map[string]any{"title": "x"})
	gt.Error(t, err)
}

func TestUpdateCaseTool_HasNoStatusParameter(t *testing.T) {
	// Status transitions live in the separate case__update_case_status tool;
	// the field-update tool must NOT carry a status parameter.
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	spec := tools[0].Spec()
	_, has := spec.Parameters["status"]
	gt.Bool(t, has).False()
	_, hasFields := spec.Parameters["fields"]
	gt.Bool(t, hasFields).True()
}

func TestUpdateCaseTool_DescriptionWarnsAgainstBlindOverwrite(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	desc := tools[0].Spec().Description
	gt.String(t, desc).Contains("Do not overwrite blindly")
	gt.String(t, desc).Contains("FULL replacements")
}

func TestStatusTool_NotBuiltWithoutStatusSet(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	gt.Array(t, tools).Length(1)
	gt.Value(t, toolByName(t, tools, "case__update_case_status")).Nil()
}

func TestStatusTool_BuiltWithStatusSet(t *testing.T) {
	uc := &mockCaseUC{resp: &model.Case{ID: 5, Status: types.CaseStatusClosed, BoardStatus: "closed"}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 5, StatusSet: testStatusSet(t)})
	gt.Array(t, tools).Length(2)

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
