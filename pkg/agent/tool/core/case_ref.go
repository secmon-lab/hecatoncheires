package core

import (
	"context"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// searchReferenceableCasesTool lets the agent search the workspace that a
// case_ref field points at, so it can find the Case ID to set on that
// field. Private and draft Cases are never returned.
type searchReferenceableCasesTool struct {
	uc          CaseRefReader
	workspaceID string
}

func (t *searchReferenceableCasesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "core__search_referenceable_cases",
		Description: "Search the cases that a case_ref (or multi_case_ref) " +
			"custom field is allowed to reference, to find the case ID to set on that " +
			"field. Pass the field's id; the target workspace is resolved from the field " +
			"definition. Private and draft cases are never returned. With an empty query, " +
			"open cases are listed first, most recently updated first. A query matches the " +
			"case title (substring) or the case ID (\"#42\" or \"42\"). Returns case summaries " +
			"(id, title, status); use core__get_referenceable_cases for full details.",
		Parameters: map[string]*gollem.Parameter{
			"field_id": {
				Type:        gollem.TypeString,
				Description: "The id of the case_ref / multi_case_ref field whose target workspace to search.",
				Required:    true,
			},
			"query": {
				Type:        gollem.TypeString,
				Description: "Optional filter: matched against case title (substring) or case ID. Empty lists recent open cases.",
			},
			"limit": {
				Type:        gollem.TypeInteger,
				Description: "Optional maximum number of cases to return (default and maximum 50).",
			},
		},
	}
}

func (t *searchReferenceableCasesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	fieldID, err := extractRequiredString(args, "field_id")
	if err != nil {
		return nil, err
	}
	refWS, err := t.uc.ReferenceWorkspaceForField(t.workspaceID, fieldID)
	if err != nil {
		return nil, goerr.Wrap(err, "resolve reference workspace", goerr.V("field_id", fieldID))
	}

	query, _ := args["query"].(string)
	limit := 0
	if _, ok := args["limit"]; ok {
		n, err := tool.ExtractInt64(args, "limit")
		if err != nil {
			return nil, err
		}
		limit = int(n)
	}

	tool.Update(ctx, fmt.Sprintf("Searching referenceable cases in %q...", refWS))
	refs, err := t.uc.ListReferenceableCases(ctx, refWS, query, limit)
	if err != nil {
		return nil, goerr.Wrap(err, "search referenceable cases", goerr.V("reference_workspace", refWS))
	}

	cases := make([]map[string]any, len(refs))
	for i, r := range refs {
		cases[i] = caseRefToMap(r)
	}
	return map[string]any{
		"reference_workspace": refWS,
		"cases":               cases,
	}, nil
}

// getReferenceableCasesTool batch-fetches full details (including custom field
// values) for specific cases in a case_ref field's target workspace.
type getReferenceableCasesTool struct {
	uc          CaseRefReader
	workspaceID string
}

func (t *getReferenceableCasesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "core__get_referenceable_cases",
		Description: "Fetch full details (title, description, status, reporter, assignees, " +
			"custom field values, timestamps) for one or more cases referenced by a " +
			"case_ref field, in a single batch. Pass the field's id (the target " +
			"workspace is resolved from it) and the list of case ids. Private, draft, and " +
			"non-existent ids are not returned; they are listed under not_found.",
		Parameters: map[string]*gollem.Parameter{
			"field_id": {
				Type:        gollem.TypeString,
				Description: "The id of the case_ref / multi_case_ref field whose target workspace the case ids belong to.",
				Required:    true,
			},
			"ids": {
				Type:        gollem.TypeArray,
				Description: "Case ids to fetch in one batch.",
				Items:       &gollem.Parameter{Type: gollem.TypeInteger},
				Required:    true,
			},
		},
	}
}

func (t *getReferenceableCasesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	fieldID, err := extractRequiredString(args, "field_id")
	if err != nil {
		return nil, err
	}
	refWS, err := t.uc.ReferenceWorkspaceForField(t.workspaceID, fieldID)
	if err != nil {
		return nil, goerr.Wrap(err, "resolve reference workspace", goerr.V("field_id", fieldID))
	}

	ids, err := extractInt64Slice(args, "ids")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Fetching %d referenceable case(s) from %q...", len(ids), refWS))
	cases, err := t.uc.GetReferenceableCases(ctx, refWS, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "get referenceable cases", goerr.V("reference_workspace", refWS))
	}

	foundIDs := make(map[int64]struct{}, len(cases))
	items := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		foundIDs[c.ID] = struct{}{}
		fieldValues, err := t.uc.RenderCaseFieldValues(ctx, refWS, c.FieldValues)
		if err != nil {
			return nil, goerr.Wrap(err, "render referenced case field values", goerr.V("case_id", c.ID))
		}
		items = append(items, map[string]any{
			"id":           c.ID,
			"title":        c.Title,
			"description":  c.Description,
			"status":       c.Status.String(),
			"reporter_id":  c.ReporterID,
			"assignee_ids": c.AssigneeIDs,
			"field_values": fieldValues,
			"created_at":   c.CreatedAt.Format(time.RFC3339),
			"updated_at":   c.UpdatedAt.Format(time.RFC3339),
		})
	}

	notFound := make([]int64, 0)
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := foundIDs[id]; !ok {
			notFound = append(notFound, id)
		}
	}

	return map[string]any{
		"reference_workspace": refWS,
		"cases":               items,
		"not_found":           notFound,
	}, nil
}

func caseRefToMap(r model.CaseRef) map[string]any {
	return map[string]any{
		"id":     r.ID,
		"title":  r.Title,
		"status": r.Status.String(),
	}
}

// extractRequiredString reads a required non-empty string argument.
func extractRequiredString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %T", key, v)
	}
	if s == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	return s, nil
}

// extractInt64Slice reads a required array of integers. gollem decodes arrays
// as []any whose elements are float64 / int, but concrete []int64 / []int are
// also accepted so non-LLM callers and tests need not box every element.
func extractInt64Slice(args map[string]any, key string) ([]int64, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	switch arr := v.(type) {
	case []int64:
		return arr, nil
	case []int:
		out := make([]int64, len(arr))
		for i, n := range arr {
			out[i] = int64(n)
		}
		return out, nil
	case []any:
		out := make([]int64, 0, len(arr))
		for i, item := range arr {
			switch n := item.(type) {
			case int:
				out = append(out, int64(n))
			case int64:
				out = append(out, n)
			case float64:
				out = append(out, int64(n))
			default:
				return nil, fmt.Errorf("%s[%d] must be an integer, got %T", key, i, item)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of integers, got %T", key, v)
	}
}
