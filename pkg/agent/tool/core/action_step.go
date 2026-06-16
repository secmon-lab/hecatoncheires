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

// actionStepToMap shapes an ActionStep for tool responses. Internal-only
// audit fields (CreatedBy / DoneBy) are exposed because the LLM may want to
// reference who set them, but they are intentionally NOT shown in the WebUI.
func actionStepToMap(s *model.ActionStep) map[string]any {
	m := map[string]any{
		"id":         s.ID,
		"action_id":  s.ActionID,
		"title":      s.Title,
		"done":       s.IsDone(),
		"created_by": s.CreatedBy,
		"created_at": s.CreatedAt.Format(time.RFC3339),
		"updated_at": s.UpdatedAt.Format(time.RFC3339),
	}
	if s.DoneAt != nil {
		m["done_at"] = s.DoneAt.Format(time.RFC3339)
	}
	if s.DoneBy != "" {
		m["done_by"] = s.DoneBy
	}
	return m
}

func actionStepNotWiredErr(workspaceID string) error {
	return goerr.New("action step tool is not wired to an ActionStepUseCase",
		goerr.V("workspaceID", workspaceID))
}

// listActionStepsTool returns the steps registered under an Action.
type listActionStepsTool struct {
	stepUC      ActionStepMutator
	workspaceID string
}

func (t *listActionStepsTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__list_action_steps",
		Description: "List the binary-state work items (steps) registered under an Action. Use this to see how progress on an Action is broken down.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action",
				Required:    true,
			},
		},
	}
}

func (t *listActionStepsTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.stepUC == nil {
		return nil, actionStepNotWiredErr(t.workspaceID)
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	tool.Update(ctx, fmt.Sprintf("Listing steps of action #%d...", actionID))
	steps, err := t.stepUC.List(ctx, t.workspaceID, actionID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list action steps",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID))
	}
	items := make([]map[string]any, len(steps))
	done := 0
	for i, s := range steps {
		items[i] = actionStepToMap(s)
		if s.IsDone() {
			done++
		}
	}
	return map[string]any{
		"steps":    items,
		"done":     done,
		"total":    len(steps),
		"complete": len(steps) > 0 && done == len(steps),
	}, nil
}

// addActionStepTool creates a new step under an Action.
type addActionStepTool struct {
	stepUC      ActionStepMutator
	workspaceID string
}

func (t *addActionStepTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__add_action_step",
		Description: "Add a new step (binary-state work item) under an Action. Steps appear in the Action detail and are notified to the Action's Slack thread.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "Short title describing the step",
				Required:    true,
			},
		},
	}
}

func (t *addActionStepTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.stepUC == nil {
		return nil, actionStepNotWiredErr(t.workspaceID)
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	tool.Update(ctx, fmt.Sprintf("Adding step to action #%d: %s", actionID, title))
	step, err := t.stepUC.Add(ctx, t.workspaceID, actionID, title)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to add action step",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID))
	}
	return actionStepToMap(step), nil
}

// setActionStepDoneTool toggles a step's done state.
type setActionStepDoneTool struct {
	stepUC      ActionStepMutator
	workspaceID string
}

func (t *setActionStepDoneTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__set_action_step_done",
		Description: "Mark an Action step as done or revert it to ongoing. Idempotent — calling with the current state is a no-op.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action",
				Required:    true,
			},
			"step_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the step (UUID)",
				Required:    true,
			},
			"done": {
				Type:        gollem.TypeBoolean,
				Description: "true to mark as done, false to revert to ongoing",
				Required:    true,
			},
		},
	}
}

func (t *setActionStepDoneTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.stepUC == nil {
		return nil, actionStepNotWiredErr(t.workspaceID)
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	stepID, _ := args["step_id"].(string)
	if stepID == "" {
		return nil, fmt.Errorf("step_id is required")
	}
	doneVal, ok := args["done"].(bool)
	if !ok {
		return nil, fmt.Errorf("done is required (boolean)")
	}
	tool.Update(ctx, fmt.Sprintf("Updating step %s on action #%d (done=%v)", stepID, actionID, doneVal))
	step, err := t.stepUC.SetDone(ctx, t.workspaceID, actionID, stepID, doneVal)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to set action step done state",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
			goerr.V("stepID", stepID))
	}
	return actionStepToMap(step), nil
}

// renameActionStepTool changes a step's title.
type renameActionStepTool struct {
	stepUC      ActionStepMutator
	workspaceID string
}

func (t *renameActionStepTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__rename_action_step",
		Description: "Rename an Action step. No-op when the title is unchanged.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action",
				Required:    true,
			},
			"step_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the step (UUID)",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "The new title",
				Required:    true,
			},
		},
	}
}

func (t *renameActionStepTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.stepUC == nil {
		return nil, actionStepNotWiredErr(t.workspaceID)
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	stepID, _ := args["step_id"].(string)
	if stepID == "" {
		return nil, fmt.Errorf("step_id is required")
	}
	title, _ := args["title"].(string)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	tool.Update(ctx, fmt.Sprintf("Renaming step %s on action #%d", stepID, actionID))
	step, err := t.stepUC.Rename(ctx, t.workspaceID, actionID, stepID, title)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to rename action step",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
			goerr.V("stepID", stepID))
	}
	return actionStepToMap(step), nil
}

// deleteActionStepTool removes a step.
type deleteActionStepTool struct {
	stepUC      ActionStepMutator
	workspaceID string
}

func (t *deleteActionStepTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "core__delete_action_step",
		Description: "Delete an Action step.",
		Parameters: map[string]*gollem.Parameter{
			"action_id": {
				Type:        gollem.TypeInteger,
				Description: "The ID of the parent action",
				Required:    true,
			},
			"step_id": {
				Type:        gollem.TypeString,
				Description: "The ID of the step to delete",
				Required:    true,
			},
		},
	}
}

func (t *deleteActionStepTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.stepUC == nil {
		return nil, actionStepNotWiredErr(t.workspaceID)
	}
	actionID, err := tool.ExtractInt64(args, "action_id")
	if err != nil {
		return nil, err
	}
	stepID, _ := args["step_id"].(string)
	if stepID == "" {
		return nil, fmt.Errorf("step_id is required")
	}
	tool.Update(ctx, fmt.Sprintf("Deleting step %s from action #%d", stepID, actionID))
	if err := t.stepUC.Delete(ctx, t.workspaceID, actionID, stepID); err != nil {
		return nil, goerr.Wrap(err, "failed to delete action step",
			goerr.V("workspaceID", t.workspaceID),
			goerr.V("actionID", actionID),
			goerr.V("stepID", stepID))
	}
	return map[string]any{"deleted": true, "step_id": stepID}, nil
}
