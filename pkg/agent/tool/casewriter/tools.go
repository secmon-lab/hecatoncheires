// Package casewriter exposes the case-mutation gollem tools available to
// event-driven Agent Jobs. Only field-style updates (title / description /
// assignees / custom fields) are permitted; status transitions, archive,
// and delete are intentionally absent — close is a human decision and a
// Job that auto-closes a case would short-circuit human review.
package casewriter

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// CaseMutator is the narrow surface of CaseUseCase the casewriter tools
// depend on. Defined here so the package does not import pkg/usecase and
// create a cycle.
type CaseMutator interface {
	UpdateCase(ctx context.Context, workspaceID string, id int64, patch CaseUpdate) (*model.Case, error)
}

// CaseUpdate mirrors the partial-update shape of usecase.CaseUpdate. Nil
// pointer / unset slice means "preserve the existing value". Status is
// intentionally absent — see package doc.
type CaseUpdate struct {
	Title       *string
	Description *string
	AssigneeIDs []string
	HasAssign   bool
	Fields      map[string]model.FieldValue
}

// Deps groups the dependencies the casewriter tools need.
type Deps struct {
	CaseUC      CaseMutator
	WorkspaceID string
	CaseID      int64
}

// New builds the writer-side case tools available to Jobs.
func New(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&updateCaseTool{deps: deps},
	}
}

type updateCaseTool struct {
	deps Deps
}

func (t *updateCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__update_case",
		Description: "Update the current case's title, description, or assignee list. " +
			"This tool cannot change the case status (close/reopen is a human decision) " +
			"and cannot delete the case.",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "New title for the case. Omit to preserve the existing title.",
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "New description (full replacement). Omit to preserve the existing description.",
			},
			"assignee_ids": {
				Type:        gollem.TypeArray,
				Description: "New assignee user IDs (full replacement). Pass an empty array to clear assignees. Omit to preserve.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
			},
		},
	}
}

func (t *updateCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating case fields...")

	var patch CaseUpdate
	hasUpdate := false

	if v, ok := args["title"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("title must be a string", goerr.V("type", typeOf(v)))
		}
		patch.Title = &s
		hasUpdate = true
	}

	if v, ok := args["description"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("description must be a string", goerr.V("type", typeOf(v)))
		}
		patch.Description = &s
		hasUpdate = true
	}

	if v, ok := args["assignee_ids"]; ok && v != nil {
		ids, err := toStringSlice(v)
		if err != nil {
			return nil, goerr.Wrap(err, "assignee_ids invalid")
		}
		patch.AssigneeIDs = ids
		patch.HasAssign = true
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, goerr.New("update_case requires at least one of title, description, assignee_ids")
	}

	updated, err := t.deps.CaseUC.UpdateCase(ctx, t.deps.WorkspaceID, t.deps.CaseID, patch)
	if err != nil {
		return nil, goerr.Wrap(err, "update case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", t.deps.CaseID))
	}

	return map[string]any{
		"id":           updated.ID,
		"title":        updated.Title,
		"description":  updated.Description,
		"status":       updated.Status.String(),
		"assignee_ids": updated.AssigneeIDs,
	}, nil
}

// toStringSlice coerces a tool argument value into []string. gollem decodes
// arrays as []any, so we accept that shape plus the rare backend that
// returns []string directly.
func toStringSlice(v any) ([]string, error) {
	switch a := v.(type) {
	case []string:
		return a, nil
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			s, ok := item.(string)
			if !ok {
				return nil, goerr.New("array item must be string", goerr.V("type", typeOf(item)))
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, goerr.New("value must be an array of strings", goerr.V("type", typeOf(v)))
	}
}

func typeOf(v any) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", v)
}
