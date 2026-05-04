package usecase

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// actionToolAdapter wraps an *ActionUseCase so that agent / assist tools see
// it through the narrow core.ActionMutator surface. The adapter is the
// single place that decides:
//
//   - tool-driven mutations are attributed to ActorKindSystem (the LLM is
//     not a Slack user, so notifications must not @-mention anyone), and
//   - tool-driven mutations always trigger SlackSyncFull (the same Slack
//     message refresh + thread-summary post that GraphQL/Slack-modal
//     mutations get — without it, tool edits would silently diverge from
//     the visible state in the channel).
//
// Code outside this file should not reach into the existing
// UpdateActionInput / Actor / SlackSync knobs from core tools; that is
// what the adapter exists to hide.
type actionToolAdapter struct {
	uc *ActionUseCase
}

// NewActionToolAdapter returns a core.ActionMutator backed by the supplied
// ActionUseCase. Returns nil when uc is nil, matching the contract that
// core.Deps.ActionUC == nil → tool fails loudly.
func NewActionToolAdapter(uc *ActionUseCase) core.ActionMutator {
	if uc == nil {
		return nil
	}
	return &actionToolAdapter{uc: uc}
}

// CreateAction delegates straight through; CreateAction's signature already
// uses primitives, so no translation is needed.
func (a *actionToolAdapter) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description, assigneeID, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error) {
	return a.uc.CreateAction(ctx, workspaceID, caseID, title, description, assigneeID, slackMessageTS, status, dueDate)
}

// UpdateAction translates a core.UpdateActionParams into the canonical
// UpdateActionInput shape and pins Actor / SlackSync to the values that
// tool-driven edits must always carry.
func (a *actionToolAdapter) UpdateAction(ctx context.Context, workspaceID string, actionID int64, params core.UpdateActionParams) (*model.Action, error) {
	return a.uc.UpdateAction(ctx, workspaceID, UpdateActionInput{
		ID:            actionID,
		Title:         params.Title,
		Description:   params.Description,
		AssigneeID:    params.AssigneeID,
		Status:        params.Status,
		DueDate:       params.DueDate,
		ClearAssignee: params.ClearAssignee,
		ClearDueDate:  params.ClearDueDate,
		Actor:         ActorRef{Kind: ActorKindSystem},
		SlackSync:     SlackSyncFull,
	})
}
