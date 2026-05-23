package casewriter_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type mockCaseUC struct {
	calls []casewriter.CaseUpdate
	resp  *model.Case
	err   error
}

func (m *mockCaseUC) UpdateCase(ctx context.Context, workspaceID string, id int64, patch casewriter.CaseUpdate) (*model.Case, error) {
	m.calls = append(m.calls, patch)
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func findTool(tools []interface {
	Spec() (s any)
}, name string) any {
	return nil
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

func TestUpdateCaseTool_NoStatusParameter(t *testing.T) {
	// The tool intentionally has no `status` parameter — status transitions
	// are not exposed to the agent. Lock the absence so it cannot be added
	// without an updated test.
	uc := &mockCaseUC{resp: &model.Case{}}
	tools := casewriter.New(casewriter.Deps{CaseUC: uc, WorkspaceID: "ws", CaseID: 1})
	spec := tools[0].Spec()
	_, has := spec.Parameters["status"]
	gt.Bool(t, has).False()
}

// unused but referenced earlier; keep for future expansion guard
var _ = findTool
