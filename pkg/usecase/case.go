package usecase

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

type CaseUseCase struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	slackService      slack.Service
	baseURL           string
}

func NewCaseUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, slackService slack.Service, baseURL string) *CaseUseCase {
	return &CaseUseCase{
		repo:              repo,
		workspaceRegistry: registry,
		slackService:      slackService,
		baseURL:           baseURL,
	}
}

func (uc *CaseUseCase) fieldValidatorForWorkspace(workspaceID string) *model.FieldValidator {
	if uc.workspaceRegistry == nil {
		return nil
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil || entry.FieldSchema == nil {
		return nil
	}
	return model.NewFieldValidator(entry.FieldSchema)
}

func (uc *CaseUseCase) slackChannelPrefixForWorkspace(workspaceID string) string {
	if uc.workspaceRegistry == nil {
		return workspaceID
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil {
		return workspaceID
	}
	if entry.SlackChannelPrefix == "" {
		return workspaceID
	}
	return entry.SlackChannelPrefix
}

func (uc *CaseUseCase) CreateCase(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool) (*model.Case, error) {
	if title == "" {
		return nil, goerr.New("case title is required")
	}

	// Validate and enrich custom fields with Type from config
	if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
		enriched, err := validator.ValidateCaseFields(fieldValues)
		if err != nil {
			return nil, goerr.Wrap(err, "field validation failed")
		}
		fieldValues = enriched
	}

	// Create case with embedded field values
	caseModel := &model.Case{
		Title:       title,
		Description: description,
		Status:      types.CaseStatusOpen,
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		FieldValues: fieldValues,
	}

	created, err := uc.repo.Case().Create(ctx, workspaceID, caseModel)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case")
	}

	// Create Slack channel if service is available
	if uc.slackService != nil {
		prefix := uc.slackChannelPrefixForWorkspace(workspaceID)
		channelID, err := uc.slackService.CreateChannel(ctx, created.ID, created.Title, prefix, isPrivate)
		if err != nil {
			// Rollback: delete case
			if delErr := uc.repo.Case().Delete(ctx, workspaceID, created.ID); delErr != nil {
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

		// Add bookmark to the Slack channel linking to the case WebUI
		if uc.baseURL != "" {
			caseURL := fmt.Sprintf("%s/ws/%s/cases/%d", uc.baseURL, workspaceID, created.ID)
			if bookmarkErr := uc.slackService.AddBookmark(ctx, channelID, "Open Case", caseURL); bookmarkErr != nil {
				errutil.Handle(ctx, bookmarkErr, "failed to add bookmark to Slack channel")
			}
		}

		// Sync channel members (for both private and public cases)
		var channelUserIDs []string
		members, membersErr := uc.slackService.GetConversationMembers(ctx, channelID)
		if membersErr != nil {
			errutil.Handle(ctx, membersErr, "failed to get channel members during case creation")
		} else {
			channelUserIDs = filterHumanUsers(ctx, uc.repo, members)
		}

		// Update case with channel ID and members
		created.SlackChannelID = channelID
		created.ChannelUserIDs = channelUserIDs
		updated, err := uc.repo.Case().Update(ctx, workspaceID, created)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to update case with Slack channel ID",
				goerr.V("orphaned_channel_id", channelID),
				goerr.V(CaseIDKey, created.ID))
		}
		return updated, nil
	}

	return created, nil
}

func (uc *CaseUseCase) UpdateCase(ctx context.Context, workspaceID string, id int64, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue) (*model.Case, error) {
	if title == "" {
		return nil, goerr.New("case title is required")
	}

	// Get existing case to check if title changed
	existingCase, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existingCase, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot update private case",
			goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
	}

	// Validate and enrich custom fields with Type from config
	if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
		enriched, err := validator.ValidateCaseFields(fieldValues)
		if err != nil {
			return nil, goerr.Wrap(err, "field validation failed", goerr.V(CaseIDKey, id))
		}
		fieldValues = enriched
	}

	// Rename Slack channel if title changed and channel exists
	if uc.slackService != nil && existingCase.SlackChannelID != "" && existingCase.Title != title {
		prefix := uc.slackChannelPrefixForWorkspace(workspaceID)
		if err := uc.slackService.RenameChannel(ctx, existingCase.SlackChannelID, id, title, prefix); err != nil {
			return nil, goerr.Wrap(err, "failed to rename Slack channel",
				goerr.V(CaseIDKey, id),
				goerr.V("channel_id", existingCase.SlackChannelID))
		}
	}

	// Update case with embedded field values
	caseModel := &model.Case{
		ID:             id,
		Title:          title,
		Description:    description,
		Status:         existingCase.Status, // Preserve status
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: existingCase.SlackChannelID, // Preserve channel ID
		IsPrivate:      existingCase.IsPrivate,      // Preserve private mode
		ChannelUserIDs: existingCase.ChannelUserIDs, // Preserve channel users
		FieldValues:    fieldValues,
		CreatedAt:      existingCase.CreatedAt, // Preserve creation time
	}

	updated, err := uc.repo.Case().Update(ctx, workspaceID, caseModel)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V(CaseIDKey, id))
	}

	return updated, nil
}

func (uc *CaseUseCase) DeleteCase(ctx context.Context, workspaceID string, id int64) error {
	// Get existing case for access control
	existingCase, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existingCase, token.Sub) {
		return goerr.Wrap(ErrAccessDenied, "cannot delete private case",
			goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
	}

	// Delete actions associated with this case
	actions, err := uc.repo.Action().GetByCase(ctx, workspaceID, id)
	if err != nil {
		return goerr.Wrap(err, "failed to get actions for case", goerr.V(CaseIDKey, id))
	}

	for _, action := range actions {
		if err := uc.repo.Action().Delete(ctx, workspaceID, action.ID); err != nil {
			return goerr.Wrap(err, "failed to delete action",
				goerr.V(CaseIDKey, id),
				goerr.V(ActionIDKey, action.ID))
		}
	}

	// Delete case (field values are embedded, so they are deleted with the case)
	if err := uc.repo.Case().Delete(ctx, workspaceID, id); err != nil {
		return goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	return nil
}

func (uc *CaseUseCase) GetCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(caseModel, token.Sub) {
		return model.RestrictCase(caseModel), nil
	}

	return caseModel, nil
}

func (uc *CaseUseCase) ListCases(ctx context.Context, workspaceID string, status *types.CaseStatus) ([]*model.Case, error) {
	var opts []interfaces.ListCaseOption
	if status != nil {
		opts = append(opts, interfaces.WithStatus(*status))
	}

	cases, err := uc.repo.Case().List(ctx, workspaceID, opts...)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list cases")
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		for i, c := range cases {
			if !model.IsCaseAccessible(c, token.Sub) {
				cases[i] = model.RestrictCase(c)
			}
		}
	}

	return cases, nil
}

func (uc *CaseUseCase) CloseCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	existing, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existing, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot close private case",
			goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
	}

	status := existing.Status.Normalize()
	if status == types.CaseStatusClosed {
		return nil, goerr.Wrap(ErrCaseAlreadyClosed, "case is already closed", goerr.V(CaseIDKey, id))
	}

	existing.Status = types.CaseStatusClosed
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to close case", goerr.V(CaseIDKey, id))
	}

	return updated, nil
}

func (uc *CaseUseCase) ReopenCase(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	existing, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existing, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot reopen private case",
			goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
	}

	status := existing.Status.Normalize()
	if status == types.CaseStatusOpen {
		return nil, goerr.Wrap(ErrCaseAlreadyOpen, "case is already open", goerr.V(CaseIDKey, id))
	}

	existing.Status = types.CaseStatusOpen
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to reopen case", goerr.V(CaseIDKey, id))
	}

	return updated, nil
}

// SyncCaseChannelUsers synchronizes channel members from Slack API to the case
func (uc *CaseUseCase) SyncCaseChannelUsers(ctx context.Context, workspaceID string, caseID int64) (*model.Case, error) {
	existing, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, caseID))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existing, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot sync private case members",
			goerr.V(CaseIDKey, caseID), goerr.V("user_id", token.Sub))
	}

	if existing.SlackChannelID == "" {
		return nil, goerr.New("case has no Slack channel", goerr.V(CaseIDKey, caseID))
	}

	if uc.slackService == nil {
		return nil, goerr.New("Slack service is not available")
	}

	members, err := uc.slackService.GetConversationMembers(ctx, existing.SlackChannelID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get channel members",
			goerr.V(CaseIDKey, caseID),
			goerr.V("channel_id", existing.SlackChannelID))
	}

	existing.ChannelUserIDs = filterHumanUsers(ctx, uc.repo, members)
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case with channel members",
			goerr.V(CaseIDKey, caseID))
	}

	return updated, nil
}

// filterHumanUsers filters out bot/unknown user IDs by checking against the SlackUser DB cache.
// Only IDs that exist in the cache (i.e., real human users synced via ListUsers) are returned.
// This avoids additional Slack API calls since ListUsers already excludes bots.
func filterHumanUsers(ctx context.Context, repo interfaces.Repository, userIDs []string) []string {
	if len(userIDs) == 0 {
		return userIDs
	}

	slackUserIDs := make([]model.SlackUserID, len(userIDs))
	for i, id := range userIDs {
		slackUserIDs[i] = model.SlackUserID(id)
	}

	known, err := repo.SlackUser().GetByIDs(ctx, slackUserIDs)
	if err != nil {
		// On error, return all IDs (don't lose data)
		return userIDs
	}

	filtered := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if _, ok := known[model.SlackUserID(id)]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered
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

func (uc *CaseUseCase) GetFieldConfiguration(workspaceID string) *config.FieldSchema {
	if uc.workspaceRegistry != nil {
		entry, err := uc.workspaceRegistry.Get(workspaceID)
		if err == nil && entry.FieldSchema != nil {
			return entry.FieldSchema
		}
	}
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{},
		Labels: config.EntityLabels{
			Case: "Case",
		},
	}
}
