// Package casewriter exposes the case-mutation gollem tools available to
// event-driven Agent Jobs and the case-bound mention agent. Field-style
// updates (title / description / custom fields) go through case__update_case;
// assignee changes go through case__assign / case__unassign (delta add/remove,
// applied atomically so concurrent edits never clobber). Marking a case done
// is mode-specific: thread-mode workspaces (a configured board status set)
// close by moving to a closed board status via case__update_case_status, while
// channel-mode cases (no board status) close via case__close_case. Archive and
// delete are intentionally absent.
package casewriter

import (
	"context"
	"fmt"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// CaseMutator is the narrow surface of CaseUseCase the casewriter tools
// depend on. Defined here so the package does not import pkg/usecase and
// create a cycle.
type CaseMutator interface {
	UpdateCase(ctx context.Context, workspaceID string, id int64, patch CaseUpdate) (*model.Case, error)
	UpdateCaseStatus(ctx context.Context, workspaceID string, id int64, boardStatus string) (*model.Case, error)
	CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error)
	AssignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error)
	UnassignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error)
}

// CaseUpdate mirrors the partial-update shape of usecase.CaseUpdate. Nil
// pointer / unset slice means "preserve the existing value". Status and
// assignees are intentionally absent — board status moves via the separate
// case__update_case_status tool, and assignees move via case__assign /
// case__unassign, each owning their own (lifecycle / atomic-delta) semantics.
type CaseUpdate struct {
	Title       *string
	Description *string
	Fields      map[string]model.FieldValue
}

// Deps groups the dependencies the casewriter tools need.
type Deps struct {
	CaseUC      CaseMutator
	WorkspaceID string
	CaseID      int64
	// Schema resolves field types for the case__update_case `fields` parameter
	// coercion. nil disables custom-field updates (the fields input then errors
	// out at runtime). Validation of the coerced values happens in the usecase.
	Schema *config.FieldSchema
	// StatusSet, when non-nil, enables case__update_case_status and lets its
	// Spec enumerate the valid board status ids. nil (a non-thread-mode
	// workspace, which has no board status) means the status tool is not built.
	StatusSet *model.ActionStatusSet
}

// New builds the writer-side case tools. case__update_case / case__assign /
// case__unassign are always present. The "mark done" tool is mode-specific and
// exactly one is built: thread-mode workspaces (a configured board status set)
// get case__update_case_status (closing by moving to a closed board status),
// while channel-mode cases (no board status) get case__close_case. Offering
// only one keeps the LLM from facing two redundant ways to close a case.
func New(deps Deps) []gollem.Tool {
	tools := []gollem.Tool{
		&updateCaseTool{deps: deps},
		&assignCaseTool{deps: deps},
		&unassignCaseTool{deps: deps},
	}
	tools = append(tools, statusTools(deps)...)
	return tools
}

// NewStatusTool builds ONLY the status-change tool — case__update_case_status
// when a board status set is configured (thread-mode), otherwise case__close_case
// (channel-mode). It deliberately excludes case__update_case (title / description
// / field edits are "materialize", owned by the host, not the sub-agent) and
// case__assign / case__unassign. This is the subset wired into a planexec
// sub-agent so it can close / transition the case it is investigating while the
// host keeps ownership of content materialization.
func NewStatusTool(deps Deps) []gollem.Tool {
	return statusTools(deps)
}

// statusTools returns the single mode-appropriate "mark done" tool. Shared by
// New (full writer set) and NewStatusTool (status-only subset) so both stay in
// sync on the thread-mode vs channel-mode selection.
func statusTools(deps Deps) []gollem.Tool {
	if deps.StatusSet != nil {
		return []gollem.Tool{&updateCaseStatusTool{deps: deps}}
	}
	return []gollem.Tool{&closeCaseTool{deps: deps}}
}

type updateCaseTool struct {
	deps Deps
}

func (t *updateCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__update_case",
		Description: "Update the current case's title, description, or custom " +
			"field values. This tool cannot change the case status (use " +
			"case__update_case_status), cannot change assignees (use case__assign " +
			"/ case__unassign), and cannot delete the case.\n\n" +
			"IMPORTANT: Do not overwrite blindly. Before changing any field, " +
			"review the case's CURRENT values shown in the system prompt and " +
			"confirm what is already there — ESPECIALLY title and description, " +
			"which are FULL replacements that discard the existing text. Submit " +
			"only the fields you intend to change; omit the rest to preserve them.",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "New title for the case (full replacement). Omit to preserve the existing title.",
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "New description (full replacement). Omit to preserve the existing description.",
			},
			"fields": {
				Type: gollem.TypeArray,
				Description: "Custom field assignments. Each entry sets one field defined in " +
					"the workspace field schema (see the system prompt for ids, types, and " +
					"option ids). Submitted entries are merged onto existing values; omitted " +
					"fields are preserved.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"field_id": {Type: gollem.TypeString, Description: "The field id from the workspace schema.", Required: true},
						"value":    {Type: gollem.TypeString, Description: "Scalar value (text / number / url / date / single select option id / single user id)."},
						"values":   {Type: gollem.TypeArray, Description: "Multi value (multi-select option ids / multi-user ids).", Items: &gollem.Parameter{Type: gollem.TypeString}},
					},
				},
			},
		},
	}
}

func (t *updateCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating case fields...")

	if t.deps.CaseUC == nil {
		return nil, goerr.New("casewriter: CaseUC is not configured")
	}

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

	if v, ok := args["fields"]; ok && v != nil {
		if t.deps.Schema == nil {
			return nil, goerr.New("this workspace has no custom fields; the fields parameter is not supported")
		}
		inputs, err := parseFieldInputs(v)
		if err != nil {
			return nil, goerr.Wrap(err, "fields invalid")
		}
		coerced, violations := model.CoerceFieldInputs(t.deps.Schema, inputs)
		if len(violations) > 0 {
			return nil, goerr.New("invalid field value(s):\n- " + strings.Join(violations, "\n- "))
		}
		patch.Fields = coerced
		hasUpdate = true
	}

	if !hasUpdate {
		return nil, goerr.New("update_case requires at least one of title, description, fields")
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
		"field_values": renderFieldValues(updated.FieldValues),
	}, nil
}

type assignCaseTool struct {
	deps Deps
}

func (t *assignCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__assign",
		Description: "Add one or more assignees to the current case. This is a " +
			"delta add: the listed user IDs are unioned onto the existing " +
			"assignees, so already-assigned users are left untouched and other " +
			"assignees are preserved. Use case__unassign to remove. New assignees " +
			"must be known Slack users.",
		Parameters: map[string]*gollem.Parameter{
			"user_ids": {
				Type:        gollem.TypeArray,
				Description: "Slack user IDs to add as assignees.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
				Required:    true,
			},
		},
	}
}

func (t *assignCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Assigning users to case...")

	if t.deps.CaseUC == nil {
		return nil, goerr.New("casewriter: CaseUC is not configured")
	}

	ids, err := assigneeIDsArg(args)
	if err != nil {
		return nil, err
	}

	updated, err := t.deps.CaseUC.AssignCase(ctx, t.deps.WorkspaceID, t.deps.CaseID, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "assign case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", t.deps.CaseID))
	}

	return map[string]any{
		"id":           updated.ID,
		"assignee_ids": updated.AssigneeIDs,
	}, nil
}

type unassignCaseTool struct {
	deps Deps
}

func (t *unassignCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__unassign",
		Description: "Remove one or more assignees from the current case. This is a " +
			"delta remove: the listed user IDs are dropped from the existing " +
			"assignees and the rest are preserved. Removing a user who is not " +
			"assigned is a no-op.",
		Parameters: map[string]*gollem.Parameter{
			"user_ids": {
				Type:        gollem.TypeArray,
				Description: "Slack user IDs to remove from the assignees.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
				Required:    true,
			},
		},
	}
}

func (t *unassignCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Unassigning users from case...")

	if t.deps.CaseUC == nil {
		return nil, goerr.New("casewriter: CaseUC is not configured")
	}

	ids, err := assigneeIDsArg(args)
	if err != nil {
		return nil, err
	}

	updated, err := t.deps.CaseUC.UnassignCase(ctx, t.deps.WorkspaceID, t.deps.CaseID, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "unassign case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", t.deps.CaseID))
	}

	return map[string]any{
		"id":           updated.ID,
		"assignee_ids": updated.AssigneeIDs,
	}, nil
}

// assigneeIDsArg extracts and validates the required non-empty user_ids array
// shared by case__assign / case__unassign.
func assigneeIDsArg(args map[string]any) ([]string, error) {
	v, ok := args["user_ids"]
	if !ok || v == nil {
		return nil, goerr.New("user_ids is required")
	}
	ids, err := toStringSlice(v)
	if err != nil {
		return nil, goerr.Wrap(err, "user_ids invalid")
	}
	if len(ids) == 0 {
		return nil, goerr.New("user_ids must not be empty")
	}
	return ids, nil
}

type updateCaseStatusTool struct {
	deps Deps
}

func (t *updateCaseStatusTool) Spec() gollem.ToolSpec {
	var statusIDs []string
	if t.deps.StatusSet != nil {
		statusIDs = t.deps.StatusSet.IDs()
	}
	return gollem.ToolSpec{
		Name: "case__update_case_status",
		Description: "Move the case to a different board status (workflow column). " +
			"Transitioning to a status configured as closed will close the case, " +
			"so only do this when the work is genuinely resolved. Choose one of the " +
			"status ids listed below.",
		Parameters: map[string]*gollem.Parameter{
			"status": {
				Type:        gollem.TypeString,
				Description: "Target board status id.",
				Enum:        statusIDs,
				Required:    true,
			},
		},
	}
}

func (t *updateCaseStatusTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating case status...")

	if t.deps.CaseUC == nil {
		return nil, goerr.New("casewriter: CaseUC is not configured")
	}

	v, ok := args["status"]
	if !ok || v == nil {
		return nil, goerr.New("status is required")
	}
	status, ok := v.(string)
	if !ok {
		return nil, goerr.New("status must be a string", goerr.V("type", typeOf(v)))
	}
	if status == "" {
		return nil, goerr.New("status must not be empty")
	}

	updated, err := t.deps.CaseUC.UpdateCaseStatus(ctx, t.deps.WorkspaceID, t.deps.CaseID, status)
	if err != nil {
		return nil, goerr.Wrap(err, "update case status",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", t.deps.CaseID),
			goerr.V("status", status))
	}

	return map[string]any{
		"id":           updated.ID,
		"status":       updated.Status.String(),
		"board_status": updated.BoardStatus,
	}, nil
}

// closeCaseTool marks a channel-mode case done. Thread-mode workspaces close via
// the board-status tool instead (see New), so this tool is built only when no
// board status set is configured.
type closeCaseTool struct {
	deps Deps
}

func (t *closeCaseTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "case__close_case",
		Description: "Mark the current case as done by closing it (lifecycle " +
			"OPEN -> CLOSED). Only do this when the work is genuinely resolved. " +
			"Closing a case that is already closed, or one still in draft, is " +
			"rejected. This tool takes no parameters.",
	}
}

func (t *closeCaseTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Closing case...")

	if t.deps.CaseUC == nil {
		return nil, goerr.New("casewriter: CaseUC is not configured")
	}

	updated, err := t.deps.CaseUC.CloseCase(ctx, t.deps.WorkspaceID, t.deps.CaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "close case",
			goerr.V("workspace_id", t.deps.WorkspaceID),
			goerr.V("case_id", t.deps.CaseID))
	}

	return map[string]any{
		"id":     updated.ID,
		"status": updated.Status.String(),
	}, nil
}

// parseFieldInputs converts the gollem-decoded `fields` argument (a []any of
// per-entry maps) into model.FieldInput. gollem decodes arrays as []any and
// objects as map[string]any.
func parseFieldInputs(v any) ([]model.FieldInput, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, goerr.New("fields must be an array", goerr.V("type", typeOf(v)))
	}
	out := make([]model.FieldInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, goerr.New("each field entry must be an object", goerr.V("type", typeOf(item)))
		}
		fieldID, ok := m["field_id"].(string)
		if !ok || fieldID == "" {
			return nil, goerr.New("each field entry requires a non-empty field_id")
		}
		fi := model.FieldInput{FieldID: fieldID}
		if val, ok := m["value"]; ok && val != nil {
			s, ok := val.(string)
			if !ok {
				return nil, goerr.New("field value must be a string", goerr.V("field_id", fieldID), goerr.V("type", typeOf(val)))
			}
			fi.Value = s
		}
		if vals, ok := m["values"]; ok && vals != nil {
			ss, err := toStringSlice(vals)
			if err != nil {
				return nil, goerr.Wrap(err, "field values invalid", goerr.V("field_id", fieldID))
			}
			fi.Values = ss
		}
		out = append(out, fi)
	}
	return out, nil
}

// renderFieldValues flattens the stored field values into a plain map the tool
// returns to the LLM so it can see what was actually persisted.
func renderFieldValues(fields map[string]model.FieldValue) map[string]any {
	out := make(map[string]any, len(fields))
	for id, fv := range fields {
		out[id] = fv.Value
	}
	return out
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
