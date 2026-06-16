// Package core contains gollem tools that operate on the case's domain state
// — currently actions. Slack and Notion integrations live in their own tool
// packages (pkg/agent/tool/slack, pkg/agent/tool/notion); this package
// intentionally has no dependency on either external service.
package core

import (
	"context"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ActionMutator is the narrow surface of the ActionUseCase that the action
// mutation core tools depend on. Defined here so each tool can route through
// the unified usecase entry point (which handles Slack post / refresh,
// ActionEvent recording, access control, and any future side-effects)
// without taking a dependency on the entire usecase package — that would
// create an import cycle, since pkg/usecase already imports
// pkg/agent/tool/core.
type ActionMutator interface {
	// CreateAction is invoked by core__create_action.
	CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error)
	// UpdateAction is invoked by core__update_action,
	// core__update_action_status and core__set_action_assignee. The caller
	// fills only the fields it intends to change; the implementation must
	// translate this into the full UpdateAction usecase contract (system
	// actor, full Slack sync) so tool-driven edits behave identically to
	// GraphQL / Slack-modal edits.
	UpdateAction(ctx context.Context, workspaceID string, actionID int64, params UpdateActionParams) (*model.Action, error)
	// ArchiveAction is invoked by core__archive_action.
	ArchiveAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error)
	// UnarchiveAction is invoked by core__unarchive_action.
	UnarchiveAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error)
}

// ActionStepMutator is the narrow surface of ActionStepUseCase that the
// step mutation core tools depend on. The caller does not pass an actor;
// the adapter pins it to ActorKindSystem so tool-driven changes are not
// @-mentioning anyone.
type ActionStepMutator interface {
	List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error)
	Add(ctx context.Context, workspaceID string, actionID int64, title string) (*model.ActionStep, error)
	SetDone(ctx context.Context, workspaceID string, actionID int64, stepID string, done bool) (*model.ActionStep, error)
	Rename(ctx context.Context, workspaceID string, actionID int64, stepID string, title string) (*model.ActionStep, error)
	Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error
}

// UpdateActionParams describes a partial Action update from the agent tool
// path. nil pointer means "no change". Empty pointer plus its corresponding
// Clear* flag is how the caller asks for an explicit clear (e.g. the user
// wants to unassign an action, not just leave the field alone).
type UpdateActionParams struct {
	Title         *string
	Description   *string
	AssigneeID    *string
	Status        *types.ActionStatus
	DueDate       *time.Time
	ClearAssignee bool
	ClearDueDate  bool
}

// Deps groups the dependencies the core tool factories need.
type Deps struct {
	Repo        interfaces.Repository
	WorkspaceID string
	CaseID      int64
	StatusSet   *model.ActionStatusSet
	// ActionUC routes core__create_action / core__update_action /
	// core__update_action_status / core__set_action_assignee through the
	// unified usecase entry points. Required: tools fail loudly when this
	// is nil rather than silently degrade to the legacy repository-direct
	// path, which would skip Slack notifications and ActionEvent records.
	ActionUC ActionMutator
	// ActionStepUC routes the core__*_action_step tools through the unified
	// ActionStepUseCase entry points. Required for the same reason as
	// ActionUC; tools fail loudly when nil.
	ActionStepUC ActionStepMutator
}

// New builds core tools for the agent mention use case: action management.
// deps.StatusSet may be nil; it falls back to model.DefaultActionStatusSet().
func New(deps Deps) []gollem.Tool {
	statusSet := deps.StatusSet
	if statusSet == nil {
		statusSet = model.DefaultActionStatusSet()
	}

	return []gollem.Tool{
		&listActionsTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
		&getActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
		&createActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, statusSet: statusSet},
		&updateActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&updateActionStatusTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID, statusSet: statusSet},
		&setActionAssigneeTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&archiveActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&unarchiveActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&listActionStepsTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&addActionStepTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&setActionStepDoneTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&renameActionStepTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&deleteActionStepTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
	}
}

// NewForAssist builds tools for the assist use case. Currently identical to
// New(); kept as a separate factory so future assist-only tools can be added
// without touching the mention flow.
func NewForAssist(deps Deps) []gollem.Tool {
	return New(deps)
}

// NewReadOnly builds the read-only subset of the core tool family. This is
// what investigation sub-agents (draft mode) get: list / get tools only,
// no creation, no status mutation, no archival.
//
// ActionUC is not used and may be nil. ActionStepUC is consulted only for
// list_action_steps (a read operation in mutator clothing); the step tool is
// included only when the mutator is wired so we never crash a sub-agent that
// has no step backend hooked up.
func NewReadOnly(deps Deps) []gollem.Tool {
	tools := []gollem.Tool{
		&listActionsTool{repo: deps.Repo, workspaceID: deps.WorkspaceID, caseID: deps.CaseID},
		&getActionTool{repo: deps.Repo, workspaceID: deps.WorkspaceID},
	}
	if deps.ActionStepUC != nil {
		tools = append(tools, &listActionStepsTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID})
	}
	return tools
}

// NewWriterForJob builds the writer subset of the core tool family suitable
// for event-driven Agent Jobs. It exposes creation and edit tools but
// withholds the destructive variants (archive/unarchive/delete_action_step)
// — Jobs run unattended and an auto-archive flag from a misjudgement is
// strictly worse than leaving the action in place for a human to triage.
//
// Read-only list / get / list_action_steps are not included; combine with
// NewReadOnly when both sides are needed.
func NewWriterForJob(deps Deps) []gollem.Tool {
	statusSet := deps.StatusSet
	if statusSet == nil {
		statusSet = model.DefaultActionStatusSet()
	}
	return []gollem.Tool{
		&createActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID, caseID: deps.CaseID, statusSet: statusSet},
		&updateActionTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&updateActionStatusTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID, statusSet: statusSet},
		&setActionAssigneeTool{actionUC: deps.ActionUC, workspaceID: deps.WorkspaceID},
		&addActionStepTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&setActionStepDoneTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
		&renameActionStepTool{stepUC: deps.ActionStepUC, workspaceID: deps.WorkspaceID},
	}
}
