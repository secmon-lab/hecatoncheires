package core

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// actionToMap converts an Action to a map for tool response
func actionToMap(a *model.Action) map[string]any {
	return map[string]any{
		"id":           a.ID,
		"case_id":      a.CaseID,
		"title":        a.Title,
		"description":  a.Description,
		"status":       a.Status.String(),
		"assignee_ids": a.AssigneeIDs,
		"created_at":   a.CreatedAt.String(),
		"updated_at":   a.UpdatedAt.String(),
	}
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
		items[i] = map[string]any{
			"id":           a.ID,
			"title":        a.Title,
			"status":       a.Status.String(),
			"assignee_ids": a.AssigneeIDs,
		}
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
	actionID, err := extractInt64(args, "action_id")
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
			"assignee_ids": {
				Type:        gollem.TypeArray,
				Description: "List of Slack user IDs to assign to this action",
				Required:    false,
				Items: &gollem.Parameter{
					Type: gollem.TypeString,
				},
			},
			"status": {
				Type:        gollem.TypeString,
				Description: "Initial status of the action (default: TODO)",
				Required:    false,
				Enum: []string{
					types.ActionStatusBacklog.String(),
					types.ActionStatusTodo.String(),
					types.ActionStatusInProgress.String(),
					types.ActionStatusBlocked.String(),
					types.ActionStatusCompleted.String(),
				},
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

	status := types.ActionStatusTodo
	if s, ok := args["status"].(string); ok && s != "" {
		parsed, err := types.ParseActionStatus(s)
		if err != nil {
			return nil, fmt.Errorf("invalid status %q: must be one of BACKLOG, TODO, IN_PROGRESS, BLOCKED, COMPLETED", s)
		}
		status = parsed
	}

	var assigneeIDs []string
	if rawIDs, ok := args["assignee_ids"].([]any); ok {
		for _, id := range rawIDs {
			if s, ok := id.(string); ok {
				assigneeIDs = append(assigneeIDs, s)
			}
		}
	}

	tool.Update(ctx, fmt.Sprintf("Creating action: %s", title))
	action := &model.Action{
		CaseID:      t.caseID,
		Title:       title,
		Description: description,
		Status:      status,
		AssigneeIDs: assigneeIDs,
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

// updateActionTool updates the title, description, and/or assignees of an action
type updateActionTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *updateActionTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__update_action",
		Description: "Update the title, description, and/or assignees of an existing action",
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
			"assignee_ids": {
				Type:        gollem.TypeArray,
				Description: "New list of Slack user IDs to assign to this action (replaces existing assignees; omit to keep current value)",
				Required:    false,
				Items: &gollem.Parameter{
					Type: gollem.TypeString,
				},
			},
		},
	}
}

func (t *updateActionTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := extractInt64(args, "action_id")
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
	if rawIDs, ok := args["assignee_ids"].([]any); ok {
		assigneeIDs := make([]string, 0, len(rawIDs))
		for _, id := range rawIDs {
			if s, ok := id.(string); ok {
				assigneeIDs = append(assigneeIDs, s)
			}
		}
		a.AssigneeIDs = assigneeIDs
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
				Enum: []string{
					types.ActionStatusBacklog.String(),
					types.ActionStatusTodo.String(),
					types.ActionStatusInProgress.String(),
					types.ActionStatusBlocked.String(),
					types.ActionStatusCompleted.String(),
				},
			},
		},
	}
}

func (t *updateActionStatusTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := extractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	statusStr, _ := args["status"].(string)
	status, err := types.ParseActionStatus(statusStr)
	if err != nil {
		return nil, fmt.Errorf("invalid status %q: must be one of BACKLOG, TODO, IN_PROGRESS, BLOCKED, COMPLETED", statusStr)
	}

	tool.Update(ctx, fmt.Sprintf("Updating action #%d status → %s", actionID, status))
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

// addActionAssigneeTool adds an assignee to an action
type addActionAssigneeTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *addActionAssigneeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__add_action_assignee",
		Description: "Add an assignee to an action",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action",
				Required:    true,
			},
			"assignee_id": {
				Type:        gollem.TypeString,
				Description: "Slack user ID of the assignee to add",
				Required:    true,
			},
		},
	}
}

func (t *addActionAssigneeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := extractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	assigneeID, _ := args["assignee_id"].(string)
	if assigneeID == "" {
		return nil, fmt.Errorf("assignee_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Adding assignee %s to action #%d", assigneeID, actionID))
	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action for assignee addition",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}

	// Avoid duplicate assignees
	for _, id := range a.AssigneeIDs {
		if id == assigneeID {
			return actionToMap(a), nil
		}
	}

	a.AssigneeIDs = append(a.AssigneeIDs, assigneeID)
	updated, err := t.repo.Action().Update(ctx, t.workspaceID, a)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to add assignee to action",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	return actionToMap(updated), nil
}

// removeActionAssigneeTool removes an assignee from an action
type removeActionAssigneeTool struct {
	repo        interfaces.Repository
	workspaceID string
}

func (t *removeActionAssigneeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__remove_action_assignee",
		Description: "Remove an assignee from an action",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the action",
				Required:    true,
			},
			"assignee_id": {
				Type:        gollem.TypeString,
				Description: "Slack user ID of the assignee to remove",
				Required:    true,
			},
		},
	}
}

func (t *removeActionAssigneeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	actionID, err := extractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}

	assigneeID, _ := args["assignee_id"].(string)
	if assigneeID == "" {
		return nil, fmt.Errorf("assignee_id is required")
	}

	tool.Update(ctx, fmt.Sprintf("Removing assignee %s from action #%d", assigneeID, actionID))
	a, err := t.repo.Action().Get(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get action for assignee removal",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	if a == nil {
		return nil, fmt.Errorf("action not found: id=%d", actionID)
	}

	filtered := a.AssigneeIDs[:0]
	for _, id := range a.AssigneeIDs {
		if id != assigneeID {
			filtered = append(filtered, id)
		}
	}
	a.AssigneeIDs = filtered

	updated, err := t.repo.Action().Update(ctx, t.workspaceID, a)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to remove assignee from action",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
		)
	}
	return actionToMap(updated), nil
}

// extractInt64 extracts an int64 value from args map, accepting int, int64, or float64
func extractInt64(args map[string]any, key string) (int64, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", key, v)
	}
}
