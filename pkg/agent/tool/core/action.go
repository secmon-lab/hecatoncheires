package core

import (
	"context"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// actionToMap converts an Action to a map for tool response
func actionToMap(a *model.Action) map[string]any {
	m := map[string]any{
		"id":          a.ID,
		"case_id":     a.CaseID,
		"title":       a.Title,
		"description": a.Description,
		"status":      a.Status.String(),
		"assignee_id": a.AssigneeID,
		"created_at":  a.CreatedAt.String(),
		"updated_at":  a.UpdatedAt.String(),
	}
	if a.DueDate != nil {
		m["due_date"] = a.DueDate.Format(time.RFC3339)
	}
	return m
}

// listActionsTool retrieves all actions for the current case
type listActionsTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
}

func (t *listActionsTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__list_actions",
		Description: "List all actions associated with the current case",
		Parameters:  map[string]*gollem.Parameter{},
	}
}

func (t *listActionsTool) Run(ctx context.Context, _ map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing actions...")
	actions, err := t.repo.Action().GetByCase(ctx, t.workspaceID, t.caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list actions",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("caseID", t.caseID),
		)
	}

	items := make([]map[string]any, len(actions))
	for i, a := range actions {
		item := map[string]any{
			"id":          a.ID,
			"title":       a.Title,
			"status":      a.Status.String(),
			"assignee_id": a.AssigneeID,
		}
		if a.DueDate != nil {
			item["due_date"] = a.DueDate.Format(time.RFC3339)
		}
		items[i] = item
	}
	return map[string]any{"actions": items}, nil
}

// getActionTool retrieves action details by ID
type getActionTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *getActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__get_action",
		Description: "Get details of a specific action by its ID",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to retrieve",
				Required:    true,
			},
		},
	}
}

func (t *getActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Getting action #%d...", actionID))
	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}
	return actionToMap(a), nil
}

// createActionTool creates a new action
type createActionTool struct {
	repo        interfaces.Repository
	workspaceID string
	caseID      int64
	statusSet   *model.ActionStatusSet
}

func (t *createActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__create_action",
		Description: "Create a new action associated with the current case",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "Title of the action",
				Required:    true,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "Detailed description of the action",
				Required:    false,
			},
			"assignee_id": {
				Type:        gollem.TypeString,
				Description: "Slack user ID to assign to this action (omit or empty for unassigned)",
				Required:    false,
			},
			"status": {
				Type:        gollem.TypeString,
				Description: fmt.Sprintf("Initial status of the action (default: %s)", t.statusSet.InitialID()),
				Required:    false,
				Enum:        t.statusSet.IDs(),
			},
			"due_date": {
				Type:        gollem.TypeString,
				Description: "Optional due date for the action in RFC3339 format (e.g. 2025-03-01T00:00:00Z)",
				Required:    false,
			},
		},
	}
}

func (t *createActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	description, _ := args["description"].(string)

	status := types.ActionStatus(t.statusSet.InitialID())
	if s, ok := args["status"].(string); ok && s != "" {
		if !t.statusSet.IsValid(s) {
			return nil, fmt.Errorf("invalid status %q: must be one of %v", s, t.statusSet.IDs())
		}
		status = types.ActionStatus(s)
	}

	var assigneeID string
	if v, ok := args["assignee_id"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid assignee_id: expected string, got %T", v)
		}
		assigneeID = s
	}

	var dueDate *time.Time
	if dueDateStr, ok := args["due_date"].(string); ok && dueDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, dueDateStr)
		if err != nil {
			return nil, fmt.Errorf("invalid due_date format %q: expected RFC3339 (e.g. 2025-03-01T00:00:00Z)", dueDateStr)
		}
		dueDate = &parsed
	}

	tool.Update(ctx, fmt.Sprintf("Creating action: %s", title))
	action := &model.Action{
		CaseID:      t.caseID,
		Title:       title,
		Description: description,
		Status:      status,
		AssigneeID:  assigneeID,
		DueDate:     dueDate,
	}

	created, err := t.repo.Action().Create(ctx, t.workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("caseID", t.caseID),
		)
	}
	return actionToMap(created), nil
}

// updateActionTool updates the title, description, and/or assignee of an action
type updateActionTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *updateActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__update_action",
		Description: "Update the title, description, and/or assignee of an existing action",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to update",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "New title for the action (omit to keep current value)",
				Required:    false,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "New description for the action (omit or empty string to keep current value)",
				Required:    false,
			},
			"assignee_id": {
				Type:        gollem.TypeString,
				Description: "New Slack user ID to assign (replaces existing). Set to empty string to clear the assignee. Omit to keep current value.",
				Required:    false,
			},
			"due_date": {
				Type:        gollem.TypeString,
				Description: "New due date in RFC3339 format (e.g. 2025-03-01T00:00:00Z). Set to empty string to clear the due date. Omit to keep current value.",
				Required:    false,
			},
		},
	}
}

func (t *updateActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Updating action #%d...", actionID))
	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action for update",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}

	if title, ok := args["title"].(string); ok && title != "" {
		a.Title = title
	}
	if desc, ok := args["description"].(string); ok && desc != "" {
		a.Description = desc
	}
	if v, ok := args["assignee_id"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid assignee_id: expected string, got %T", v)
		}
		a.AssigneeID = s
	}
	if dueDateStr, ok := args["due_date"].(string); ok {
		if dueDateStr == "" {
			a.DueDate = nil
		} else {
			parsed, err := time.Parse(time.RFC3339, dueDateStr)
			if err != nil {
				return nil, fmt.Errorf("invalid due_date format %q: expected RFC3339 (e.g. 2025-03-01T00:00:00Z)", dueDateStr)
			}
			a.DueDate = &parsed
		}
	}

	updated, err := t.repo.Action().Update(ctx, t.workspaceID, a)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	return actionToMap(updated), nil
}

// updateActionStatusTool updates the status of an action
type updateActionStatusTool struct {
	repo        interfaces.Repository
	workspaceID string
	statusSet   *model.ActionStatusSet
}

func (t *updateActionStatusTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__update_action_status",
		Description: "Update the status of an existing action",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action to update",
				Required:    true,
			},
			"status": {
				Type:        gollem.TypeString,
				Description: "New status for the action",
				Required:    true,
				Enum:        t.statusSet.IDs(),
			},
		},
	}
}

func (t *updateActionStatusTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	statusStr, _ := args["status"].(string)
	if !t.statusSet.IsValid(statusStr) {
		return nil, fmt.Errorf("invalid status %q: must be one of %v", statusStr, t.statusSet.IDs())
	}
	status := types.ActionStatus(statusStr)

	tool.Update(ctx, fmt.Sprintf("Updating action #%d status -> %s", actionID, status))
	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action for status update",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}

	a.Status = status
	updated, err := t.repo.Action().Update(ctx, t.workspaceID, a)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action status",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	return actionToMap(updated), nil
}

// setActionAssigneeTool sets (or clears) the assignee of an action
type setActionAssigneeTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *setActionAssigneeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__set_action_assignee",
		Description: "Set or clear the assignee of an action. Pass an empty string to clear.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action",
				Required:    true,
			},
			"assignee_id": {
				Type:        gollem.TypeString,
				Description: "Slack user ID of the assignee. Empty string clears the assignee.",
				Required:    true,
			},
		},
	}
}

func (t *setActionAssigneeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	v, ok := args["assignee_id"]
	if !ok {
		return nil, fmt.Errorf("assignee_id is required")
	}
	assigneeID, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("assignee_id must be a string, got %T", v)
	}

	if assigneeID == "" {
		tool.Update(ctx, fmt.Sprintf("Clearing assignee on action #%d", actionID))
	} else {
		tool.Update(ctx, fmt.Sprintf("Setting assignee %s on action #%d", assigneeID, actionID))
	}

	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action for assignee update",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}

	a.AssigneeID = assigneeID
	updated, err := t.repo.Action().Update(ctx, t.workspaceID, a)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action assignee",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	return actionToMap(updated), nil
}

