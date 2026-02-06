package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

type CaseUseCase struct {
	repo           interfaces.Repository
	fieldSchema    *config.FieldSchema
	fieldValidator *model.FieldValidator
	slackService   slack.Service
}

func NewCaseUseCase(repo interfaces.Repository, fieldSchema *config.FieldSchema, slackService slack.Service) *CaseUseCase {
	var validator *model.FieldValidator
	if fieldSchema != nil {
		validator = model.NewFieldValidator(fieldSchema)
	}

	return &CaseUseCase{
		repo:           repo,
		fieldSchema:    fieldSchema,
		fieldValidator: validator,
		slackService:   slackService,
	}
}

func (uc *CaseUseCase) CreateCase(ctx context.Context, title, description string, assigneeIDs []string, fields []model.FieldValue) (*model.Case, error) {
	if title == "" {
		return nil, goerr.New("case title is required")
	}

	// Validate custom fields
	if uc.fieldValidator != nil {
		if err := uc.fieldValidator.ValidateCaseFields(fields); err != nil {
			return nil, goerr.Wrap(err, "field validation failed")
		}
	}

	// Create case
	caseModel := &model.Case{
		Title:       title,
		Description: description,
		AssigneeIDs: assigneeIDs,
	}

	created, err := uc.repo.Case().Create(ctx, caseModel)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case")
	}

	// Save custom field values
	for i := range fields {
		fields[i].CaseID = created.ID
		if err := uc.repo.CaseField().Save(ctx, &fields[i]); err != nil {
			// Rollback: delete the created case
			if delErr := uc.repo.Case().Delete(ctx, created.ID); delErr != nil {
				return nil, goerr.Wrap(err, "failed to save field values, and also failed to roll back case creation",
					goerr.V("rollback_error", delErr),
					goerr.V(CaseIDKey, created.ID))
			}
			return nil, goerr.Wrap(err, "failed to save field values", goerr.V(CaseIDKey, created.ID))
		}
	}

	// Create Slack channel if service is available
	if uc.slackService != nil {
		channelID, err := uc.slackService.CreateChannel(ctx, created.ID, created.Title)
		if err != nil {
			// Rollback: delete field values and case
			_ = uc.repo.CaseField().DeleteByCaseID(ctx, created.ID)
			if delErr := uc.repo.Case().Delete(ctx, created.ID); delErr != nil {
				return nil, goerr.Wrap(err, "failed to create Slack channel for case, and also failed to roll back case creation",
					goerr.V("rollback_error", delErr),
					goerr.V(CaseIDKey, created.ID))
			}
			return nil, goerr.Wrap(err, "failed to create Slack channel for case", goerr.V(CaseIDKey, created.ID))
		}

		// Invite creator and assignees to the channel
		usersToInvite := make([]string, 0, len(assigneeIDs)+1)
		if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
			usersToInvite = append(usersToInvite, token.Sub)
		}
		usersToInvite = append(usersToInvite, assigneeIDs...)
		usersToInvite = uniqueStrings(usersToInvite)

		if len(usersToInvite) > 0 {
			if inviteErr := uc.slackService.InviteUsersToChannel(ctx, channelID, usersToInvite); inviteErr != nil {
				errutil.Handle(ctx, inviteErr, "failed to invite users to Slack channel")
			}
		}

		// Update case with channel ID
		created.SlackChannelID = channelID
		updated, err := uc.repo.Case().Update(ctx, created)
		if err != nil {
			// Note: Channel is created but case update failed.
			// We don't attempt to delete the channel here as it might also fail.
			// The created channel is now orphaned and needs manual cleanup.
			return nil, goerr.Wrap(err, "failed to update case with Slack channel ID",
				goerr.V("orphaned_channel_id", channelID),
				goerr.V(CaseIDKey, created.ID))
		}
		return updated, nil
	}

	return created, nil
}

func (uc *CaseUseCase) UpdateCase(ctx context.Context, id int64, title, description string, assigneeIDs []string, fields []model.FieldValue) (*model.Case, error) {
	if title == "" {
		return nil, goerr.New("case title is required")
	}

	// Get existing case to check if title changed
	existingCase, err := uc.repo.Case().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Validate custom fields
	if uc.fieldValidator != nil {
		if err := uc.fieldValidator.ValidateCaseFields(fields); err != nil {
			return nil, goerr.Wrap(err, "field validation failed", goerr.V(CaseIDKey, id))
		}
	}

	// Rename Slack channel if title changed and channel exists
	if uc.slackService != nil && existingCase.SlackChannelID != "" && existingCase.Title != title {
		if err := uc.slackService.RenameChannel(ctx, existingCase.SlackChannelID, id, title); err != nil {
			return nil, goerr.Wrap(err, "failed to rename Slack channel",
				goerr.V(CaseIDKey, id),
				goerr.V("channel_id", existingCase.SlackChannelID))
		}
	}

	// Update case
	caseModel := &model.Case{
		ID:             id,
		Title:          title,
		Description:    description,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: existingCase.SlackChannelID, // Preserve channel ID
		CreatedAt:      existingCase.CreatedAt,      // Preserve creation time
	}

	updated, err := uc.repo.Case().Update(ctx, caseModel)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V(CaseIDKey, id))
	}

	// Delete existing field values and save new ones
	if err := uc.repo.CaseField().DeleteByCaseID(ctx, id); err != nil {
		return nil, goerr.Wrap(err, "failed to delete existing field values", goerr.V(CaseIDKey, id))
	}

	for i := range fields {
		fields[i].CaseID = id
		if err := uc.repo.CaseField().Save(ctx, &fields[i]); err != nil {
			return nil, goerr.Wrap(err, "failed to save field values", goerr.V(CaseIDKey, id))
		}
	}

	return updated, nil
}

func (uc *CaseUseCase) DeleteCase(ctx context.Context, id int64) error {
	// Delete field values first
	if err := uc.repo.CaseField().DeleteByCaseID(ctx, id); err != nil {
		return goerr.Wrap(err, "failed to delete case field values", goerr.V(CaseIDKey, id))
	}

	// Delete actions associated with this case
	actions, err := uc.repo.Action().GetByCase(ctx, id)
	if err != nil {
		return goerr.Wrap(err, "failed to get actions for case", goerr.V(CaseIDKey, id))
	}

	for _, action := range actions {
		if err := uc.repo.Action().Delete(ctx, action.ID); err != nil {
			return goerr.Wrap(err, "failed to delete action",
				goerr.V(CaseIDKey, id),
				goerr.V(ActionIDKey, action.ID))
		}
	}

	// Delete case
	if err := uc.repo.Case().Delete(ctx, id); err != nil {
		return goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	return nil
}

func (uc *CaseUseCase) GetCase(ctx context.Context, id int64) (*model.Case, error) {
	caseModel, err := uc.repo.Case().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	return caseModel, nil
}

func (uc *CaseUseCase) GetCaseFieldValues(ctx context.Context, caseID int64) ([]model.FieldValue, error) {
	fieldValues, err := uc.repo.CaseField().GetByCaseID(ctx, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get case field values", goerr.V(CaseIDKey, caseID))
	}

	return fieldValues, nil
}

func (uc *CaseUseCase) ListCases(ctx context.Context) ([]*model.Case, error) {
	cases, err := uc.repo.Case().List(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list cases")
	}

	return cases, nil
}

// uniqueStrings removes duplicate strings while preserving order
func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	result := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

func (uc *CaseUseCase) GetFieldConfiguration() *config.FieldSchema {
	if uc.fieldSchema == nil {
		return &config.FieldSchema{
			Fields: []config.FieldDefinition{},
			Labels: config.EntityLabels{
				Case: "Case",
			},
		}
	}
	return uc.fieldSchema
}
