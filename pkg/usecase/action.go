package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type ActionUseCase struct {
	repo interfaces.Repository
}

func NewActionUseCase(repo interfaces.Repository) *ActionUseCase {
	return &ActionUseCase{
		repo: repo,
	}
}

func (uc *ActionUseCase) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeIDs []string, slackMessageTS string, status types.ActionStatus) (*model.Action, error) {
	if title == "" {
		return nil, goerr.New("action title is required")
	}

	// Verify case exists
	if _, err := uc.repo.Case().Get(ctx, workspaceID, caseID); err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, caseID))
	}

	// Default status to todo if not provided
	if status == "" {
		status = types.ActionStatusTodo
	}

	// Validate status
	if !status.IsValid() {
		return nil, goerr.New("invalid action status", goerr.V("status", status))
	}

	// Ensure assigneeIDs is not nil
	if assigneeIDs == nil {
		assigneeIDs = []string{}
	}

	action := &model.Action{
		CaseID:         caseID,
		Title:          title,
		Description:    description,
		AssigneeIDs:    assigneeIDs,
		SlackMessageTS: slackMessageTS,
		Status:         status,
	}

	created, err := uc.repo.Action().Create(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action",
			goerr.V(CaseIDKey, caseID))
	}

	return created, nil
}

func (uc *ActionUseCase) UpdateAction(ctx context.Context, workspaceID string, id int64, caseID *int64, title, description *string, assigneeIDs []string, slackMessageTS *string, status *types.ActionStatus) (*model.Action, error) {
	// Get existing action
	existing, err := uc.repo.Action().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	// Verify new case exists if caseID is being updated
	if caseID != nil && *caseID != existing.CaseID {
		if _, err := uc.repo.Case().Get(ctx, workspaceID, *caseID); err != nil {
			return nil, goerr.Wrap(ErrCaseNotFound, "new case not found",
				goerr.V(CaseIDKey, *caseID),
				goerr.V(ActionIDKey, id))
		}
	}

	// Build action with only updated fields
	action := &model.Action{
		ID:             id,
		CaseID:         existing.CaseID,
		Title:          existing.Title,
		Description:    existing.Description,
		AssigneeIDs:    existing.AssigneeIDs,
		SlackMessageTS: existing.SlackMessageTS,
		Status:         existing.Status,
		CreatedAt:      existing.CreatedAt,
	}

	// Update only provided fields
	if caseID != nil {
		action.CaseID = *caseID
	}

	if title != nil {
		if *title == "" {
			return nil, goerr.New("action title cannot be empty", goerr.V(ActionIDKey, id))
		}
		action.Title = *title
	}

	if description != nil {
		action.Description = *description
	}

	if assigneeIDs != nil {
		action.AssigneeIDs = assigneeIDs
	}

	if slackMessageTS != nil {
		action.SlackMessageTS = *slackMessageTS
	}

	if status != nil {
		if !status.IsValid() {
			return nil, goerr.New("invalid action status",
				goerr.V("status", *status),
				goerr.V(ActionIDKey, id))
		}
		action.Status = *status
	}

	updated, err := uc.repo.Action().Update(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action", goerr.V(ActionIDKey, id))
	}

	return updated, nil
}

func (uc *ActionUseCase) DeleteAction(ctx context.Context, workspaceID string, id int64) error {
	if err := uc.repo.Action().Delete(ctx, workspaceID, id); err != nil {
		return goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	return nil
}

func (uc *ActionUseCase) GetAction(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
	action, err := uc.repo.Action().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	return action, nil
}

func (uc *ActionUseCase) ListActions(ctx context.Context, workspaceID string) ([]*model.Action, error) {
	actions, err := uc.repo.Action().List(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list actions")
	}

	return actions, nil
}

func (uc *ActionUseCase) GetActionsByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	actions, err := uc.repo.Action().GetByCase(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get actions by case", goerr.V(CaseIDKey, caseID))
	}

	return actions, nil
}
