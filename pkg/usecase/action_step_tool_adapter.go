package usecase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// actionStepToolAdapter wraps an *ActionStepUseCase so that agent / assist
// tools see it through the narrow core.ActionStepMutator surface. The
// adapter is the single place that decides:
//
//   - tool-driven mutations are attributed to ActorKindSystem (the LLM is
//     not a Slack user, so notifications must not @-mention anyone), and
//   - tool callers do not need to know about the AddActionStepInput /
//     SetActionStepDoneInput / RenameActionStepInput / DeleteActionStepInput
//     usecase structs.
type actionStepToolAdapter struct {
	uc *ActionStepUseCase
}

// NewActionStepToolAdapter returns a core.ActionStepMutator backed by the
// supplied ActionStepUseCase. Returns nil when uc is nil, matching the
// contract that core.Deps.ActionStepUC == nil → tool fails loudly.
func NewActionStepToolAdapter(uc *ActionStepUseCase) core.ActionStepMutator {
	if uc == nil {
		return nil
	}
	return &actionStepToolAdapter{uc: uc}
}

func (a *actionStepToolAdapter) List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error) {
	return a.uc.List(ctx, workspaceID, actionID)
}

func (a *actionStepToolAdapter) Add(ctx context.Context, workspaceID string, actionID int64, title string) (*model.ActionStep, error) {
	return a.uc.Add(ctx, AddActionStepInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		Title:       title,
		Actor:       ActorRef{Kind: ActorKindSystem},
	})
}

func (a *actionStepToolAdapter) SetDone(ctx context.Context, workspaceID string, actionID int64, stepID string, done bool) (*model.ActionStep, error) {
	return a.uc.SetDone(ctx, SetActionStepDoneInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		StepID:      stepID,
		Done:        done,
		Actor:       ActorRef{Kind: ActorKindSystem},
	})
}

func (a *actionStepToolAdapter) Rename(ctx context.Context, workspaceID string, actionID int64, stepID string, title string) (*model.ActionStep, error) {
	return a.uc.Rename(ctx, RenameActionStepInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		StepID:      stepID,
		Title:       title,
		Actor:       ActorRef{Kind: ActorKindSystem},
	})
}

func (a *actionStepToolAdapter) Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error {
	return a.uc.Delete(ctx, DeleteActionStepInput{
		WorkspaceID: workspaceID,
		ActionID:    actionID,
		StepID:      stepID,
		Actor:       ActorRef{Kind: ActorKindSystem},
	})
}
