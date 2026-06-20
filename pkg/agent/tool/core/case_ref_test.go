package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// mockCaseRefReader is a hand-written fake of core.CaseRefReader. Each
// method delegates to a func field so individual tests control behaviour and
// capture arguments.
type mockCaseRefReader struct {
	refWSFn  func(workspaceID, fieldID string) (string, error)
	listFn   func(ctx context.Context, workspaceID, query string, limit int) ([]model.CaseRef, error)
	getFn    func(ctx context.Context, workspaceID string, ids []int64) ([]*model.Case, error)
	renderFn func(ctx context.Context, workspaceID string, fv map[string]model.FieldValue) (map[string]any, error)
}

func (m *mockCaseRefReader) ReferenceWorkspaceForField(workspaceID, fieldID string) (string, error) {
	return m.refWSFn(workspaceID, fieldID)
}

func (m *mockCaseRefReader) ListReferenceableCases(ctx context.Context, workspaceID, query string, limit int) ([]model.CaseRef, error) {
	return m.listFn(ctx, workspaceID, query, limit)
}

func (m *mockCaseRefReader) GetReferenceableCases(ctx context.Context, workspaceID string, ids []int64) ([]*model.Case, error) {
	return m.getFn(ctx, workspaceID, ids)
}

func (m *mockCaseRefReader) RenderCaseFieldValues(ctx context.Context, workspaceID string, fv map[string]model.FieldValue) (map[string]any, error) {
	if m.renderFn != nil {
		return m.renderFn(ctx, workspaceID, fv)
	}
	out := make(map[string]any, len(fv))
	for id, v := range fv {
		out[id] = v.Value
	}
	return out, nil
}

func TestCaseRefTools_OnlyWiredWhenReaderPresent(t *testing.T) {
	t.Run("absent when CaseRefUC is nil", func(t *testing.T) {
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, ActionUC: &mockActionMutator{}})
		gt.Value(t, findTool(tools, "core__search_referenceable_cases")).Nil()
		gt.Value(t, findTool(tools, "core__get_referenceable_cases")).Nil()
	})

	t.Run("present when CaseRefUC is wired", func(t *testing.T) {
		reader := &mockCaseRefReader{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, ActionUC: &mockActionMutator{}, CaseRefUC: reader})
		gt.Value(t, findTool(tools, "core__search_referenceable_cases")).NotNil()
		gt.Value(t, findTool(tools, "core__get_referenceable_cases")).NotNil()
	})

	t.Run("present in read-only set", func(t *testing.T) {
		reader := &mockCaseRefReader{}
		tools := core.NewReadOnly(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})
		gt.Value(t, findTool(tools, "core__search_referenceable_cases")).NotNil()
		gt.Value(t, findTool(tools, "core__get_referenceable_cases")).NotNil()
	})
}

func TestSearchReferenceableCasesTool(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves reference workspace from field id and returns summaries", func(t *testing.T) {
		var gotWS, gotQuery string
		var gotLimit int
		reader := &mockCaseRefReader{
			refWSFn: func(workspaceID, fieldID string) (string, error) {
				gt.Value(t, workspaceID).Equal(testWorkspaceID)
				gt.Value(t, fieldID).Equal("related_incident")
				return "incident-response", nil
			},
			listFn: func(_ context.Context, workspaceID, query string, limit int) ([]model.CaseRef, error) {
				gotWS, gotQuery, gotLimit = workspaceID, query, limit
				return []model.CaseRef{
					{ID: 42, Title: "DB outage", Status: types.CaseStatusOpen, WorkspaceID: "incident-response"},
				}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})

		result, err := findTool(tools, "core__search_referenceable_cases").Run(ctx, map[string]any{
			"field_id": "related_incident",
			"query":    "DB",
			"limit":    float64(10),
		})
		gt.NoError(t, err).Required()
		gt.Value(t, gotWS).Equal("incident-response")
		gt.Value(t, gotQuery).Equal("DB")
		gt.Value(t, gotLimit).Equal(10)
		gt.Value(t, result["reference_workspace"]).Equal("incident-response")
		cases := result["cases"].([]map[string]any)
		gt.Array(t, cases).Length(1).Required()
		gt.Value(t, cases[0]["id"]).Equal(int64(42))
		gt.Value(t, cases[0]["title"]).Equal("DB outage")
		gt.Value(t, cases[0]["status"]).Equal("OPEN")
	})

	t.Run("errors when field_id is missing", func(t *testing.T) {
		reader := &mockCaseRefReader{}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})
		_, err := findTool(tools, "core__search_referenceable_cases").Run(ctx, map[string]any{})
		gt.Error(t, err)
	})

	t.Run("propagates field resolution error", func(t *testing.T) {
		reader := &mockCaseRefReader{
			refWSFn: func(_, _ string) (string, error) { return "", errors.New("not a case_ref field") },
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})
		_, err := findTool(tools, "core__search_referenceable_cases").Run(ctx, map[string]any{"field_id": "bad"})
		gt.Error(t, err)
	})
}

func TestGetReferenceableCasesTool(t *testing.T) {
	ctx := context.Background()

	t.Run("batch-fetches full details and reports not_found", func(t *testing.T) {
		var gotIDs []int64
		reader := &mockCaseRefReader{
			refWSFn: func(_, _ string) (string, error) { return "incident-response", nil },
			getFn: func(_ context.Context, workspaceID string, ids []int64) ([]*model.Case, error) {
				gt.Value(t, workspaceID).Equal("incident-response")
				gotIDs = ids
				// Only id 42 is referenceable; 99 is private/draft/missing.
				return []*model.Case{
					{ID: 42, Title: "DB outage", Description: "root cause", Status: types.CaseStatusOpen, ReporterID: "U1"},
				}, nil
			},
			renderFn: func(_ context.Context, _ string, fv map[string]model.FieldValue) (map[string]any, error) {
				return map[string]any{"severity": "high"}, nil
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})

		result, err := findTool(tools, "core__get_referenceable_cases").Run(ctx, map[string]any{
			"field_id": "related_incident",
			"ids":      []any{float64(42), float64(99)},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, gotIDs).Equal([]int64{42, 99})
		gt.Value(t, result["reference_workspace"]).Equal("incident-response")

		cases := result["cases"].([]map[string]any)
		gt.Array(t, cases).Length(1).Required()
		gt.Value(t, cases[0]["id"]).Equal(int64(42))
		gt.Value(t, cases[0]["title"]).Equal("DB outage")
		gt.Value(t, cases[0]["description"]).Equal("root cause")
		gt.Value(t, cases[0]["status"]).Equal("OPEN")
		fieldValues := cases[0]["field_values"].(map[string]any)
		gt.Value(t, fieldValues["severity"]).Equal("high")

		notFound := result["not_found"].([]int64)
		gt.Array(t, notFound).Length(1).Required()
		gt.Value(t, notFound[0]).Equal(int64(99))
	})

	t.Run("errors when ids is missing", func(t *testing.T) {
		reader := &mockCaseRefReader{
			refWSFn: func(_, _ string) (string, error) { return "incident-response", nil },
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})
		_, err := findTool(tools, "core__get_referenceable_cases").Run(ctx, map[string]any{"field_id": "related_incident"})
		gt.Error(t, err)
	})

	t.Run("propagates GetReferenceableCases error", func(t *testing.T) {
		reader := &mockCaseRefReader{
			refWSFn: func(_, _ string) (string, error) { return "incident-response", nil },
			getFn: func(_ context.Context, _ string, _ []int64) ([]*model.Case, error) {
				return nil, errors.New("database unavailable")
			},
		}
		tools := core.New(core.Deps{Repo: newMockRepo(nil), WorkspaceID: testWorkspaceID, CaseID: testCaseID, CaseRefUC: reader})
		_, err := findTool(tools, "core__get_referenceable_cases").Run(ctx, map[string]any{
			"field_id": "related_incident",
			"ids":      []any{float64(1)},
		})
		gt.Error(t, err)
	})
}
