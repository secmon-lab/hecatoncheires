package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

type CaseUseCase struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	slackService      slack.Service
	slackAdminService slack.AdminService
	baseURL           string
	welcomeRenderers  map[string]*welcomeRenderer
}

func NewCaseUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, slackService slack.Service, slackAdminService slack.AdminService, baseURL string) *CaseUseCase {
	uc := &CaseUseCase{
		repo:              repo,
		workspaceRegistry: registry,
		slackService:      slackService,
		slackAdminService: slackAdminService,
		baseURL:           baseURL,
		welcomeRenderers:  make(map[string]*welcomeRenderer),
	}

	// Pre-parse welcome message templates per workspace. Configuration loading
	// already validated each template, so a parse failure here is unexpected
	// but treated as non-fatal: the workspace simply gets no welcome messages.
	if registry != nil {
		for _, entry := range registry.List() {
			renderer, err := newWelcomeRenderer(entry.SlackWelcomeMessages)
			if err != nil {
				errutil.Handle(context.Background(), goerr.Wrap(err, "failed to build welcome renderer; skipping welcome messages",
					goerr.V("workspaceID", entry.Workspace.ID),
				), "failed to build welcome renderer")
				continue
			}
			uc.welcomeRenderers[entry.Workspace.ID] = renderer
		}
	}

	return uc
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

func (uc *CaseUseCase) slackTeamIDForWorkspace(workspaceID string) string {
	if uc.workspaceRegistry == nil {
		return ""
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil {
		return ""
	}
	return entry.SlackTeamID
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

// CreateCase persists a brand-new case in status=OPEN and runs the full
// activation side effects (Slack channel, invites, welcome, etc.). It is the
// public entry point used by createCase mutation and by the slash-command
// "submit" flow; both share identical post-persistence behaviour.
func (uc *CaseUseCase) CreateCase(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool, sourceTeamID string, requestKey string) (*model.Case, error) {
	created, err := uc.persistCase(ctx, workspaceID, persistCaseInput{
		Title:       title,
		Description: description,
		Status:      types.CaseStatusOpen,
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		FieldValues: fieldValues,
		RequestKey:  requestKey,
	})
	if err != nil {
		return nil, err
	}

	// `persistCase` returns early when an existing requestKey matched; that
	// case is already active and must not be re-activated.
	if created.Status != types.CaseStatusOpen || created.SlackChannelID != "" {
		return created, nil
	}

	return uc.activateCase(ctx, workspaceID, created, sourceTeamID)
}

// CreateDraft persists a case in status=DRAFT — i.e. an "in-progress" entry
// saved from the Slack creation modal's Save as Draft button. None of the
// activation side effects (Slack channel, invites, welcome, etc.) run; those
// fire only when the draft is later promoted via SubmitDraft.
//
// The reporter (auth-context Slack user) becomes the draft owner; the
// returned case carries the assigned ID so the caller can echo it back to
// the user.
func (uc *CaseUseCase) CreateDraft(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool) (*model.Case, error) {
	// Title is intentionally optional for drafts: half-written entries are
	// the whole point. We still validate field values to keep the draft
	// usable on Submit without surprise validation failures.
	return uc.persistCase(ctx, workspaceID, persistCaseInput{
		Title:       title,
		Description: description,
		Status:      types.CaseStatusDraft,
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		FieldValues: fieldValues,
	})
}

// persistCaseInput is the shared input for persistCase, used by both the
// "create open case" and "create draft" flows.
type persistCaseInput struct {
	Title       string
	Description string
	Status      types.CaseStatus
	AssigneeIDs []string
	IsPrivate   bool
	FieldValues map[string]model.FieldValue
	RequestKey  string
}

// persistCase performs request-key deduplication, field validation, and
// repository write. It does NOT run any activation side effects — callers
// must invoke activateCase separately when those should fire.
func (uc *CaseUseCase) persistCase(ctx context.Context, workspaceID string, in persistCaseInput) (*model.Case, error) {
	// Title is required for OPEN cases (the human flow needs a meaningful
	// listing entry); drafts may be saved with an empty title so a partial
	// entry can be picked up later.
	if !in.Status.IsDraft() && in.Title == "" {
		return nil, goerr.New("case title is required")
	}

	// Check request key: if a case with this key already exists, return it.
	// RequestKey deduplication applies only to non-draft submissions; drafts
	// do not currently carry a request key.
	if in.RequestKey != "" {
		existing, err := uc.repo.Case().GetByRequestKey(ctx, workspaceID, in.RequestKey)
		if err != nil {
			errutil.Handle(ctx, err, "failed to check request key key")
		} else if existing != nil {
			return existing, nil
		}
	}

	// Validate and enrich custom fields with Type from config.
	if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
		enriched, err := validator.ValidateCaseFields(in.FieldValues)
		if err != nil {
			return nil, goerr.Wrap(err, "field validation failed")
		}
		in.FieldValues = enriched
	}

	// Set reporter from auth context (immutable after creation).
	var reporterID string
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		reporterID = token.Sub
	}

	caseModel := &model.Case{
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		ReporterID:  reporterID,
		AssigneeIDs: in.AssigneeIDs,
		IsPrivate:   in.IsPrivate,
		FieldValues: in.FieldValues,
		RequestKey:  in.RequestKey,
	}

	created, err := uc.repo.Case().Create(ctx, workspaceID, caseModel)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create case")
	}
	return created, nil
}

// activateCase runs all post-persistence side effects required to bring an
// OPEN case to life: Slack channel creation, optional cross-workspace
// connect, invites, bookmark, welcome messages, and channel-member sync.
// Returns the updated case (with SlackChannelID / ChannelUserIDs filled in).
//
// On Slack channel creation failure the just-persisted case is rolled back
// so the caller observes "creation failed" rather than a partial case.
// Activation is a no-op when no Slack service is configured.
func (uc *CaseUseCase) activateCase(ctx context.Context, workspaceID string, c *model.Case, sourceTeamID string) (*model.Case, error) {
	if uc.slackService == nil {
		return c, nil
	}

	prefix := uc.slackChannelPrefixForWorkspace(workspaceID)
	teamID := uc.slackTeamIDForWorkspace(workspaceID)
	channelID, err := uc.slackService.CreateChannel(ctx, c.ID, c.Title, prefix, c.IsPrivate, teamID)
	if err != nil {
		// Rollback: delete case.
		if delErr := uc.repo.Case().Delete(ctx, workspaceID, c.ID); delErr != nil {
			return nil, goerr.Wrap(err, "failed to create Slack channel for case, and also failed to roll back case creation",
				goerr.V("rollback_error", delErr),
				goerr.V(CaseIDKey, c.ID))
		}
		return nil, goerr.Wrap(err, "failed to create Slack channel for case", goerr.V(CaseIDKey, c.ID))
	}

	// Connect channel to the source workspace if it differs from the configured team.
	if sourceTeamID != "" && sourceTeamID != teamID {
		if uc.slackAdminService != nil {
			if connectErr := uc.slackAdminService.ConnectChannelToWorkspace(ctx, channelID, []string{teamID, sourceTeamID}); connectErr != nil {
				errutil.Handle(ctx, connectErr, "failed to connect channel to source workspace")
			}
		}
	}

	// Invite reporter, assignees, and auto-invite users to the channel.
	usersToInvite := make([]string, 0, len(c.AssigneeIDs)+1)
	if c.ReporterID != "" {
		usersToInvite = append(usersToInvite, c.ReporterID)
	}
	usersToInvite = append(usersToInvite, c.AssigneeIDs...)
	autoInviteUsers := uc.resolveAutoInviteUsers(ctx, workspaceID)
	usersToInvite = append(usersToInvite, autoInviteUsers...)
	usersToInvite = uniqueStrings(usersToInvite)

	if len(usersToInvite) > 0 {
		if inviteErr := uc.slackService.InviteUsersToChannel(ctx, channelID, usersToInvite); inviteErr != nil {
			errutil.Handle(ctx, inviteErr, "failed to invite users to Slack channel")
		}
	}

	// Add bookmark to the Slack channel linking to the case WebUI.
	caseURL := ""
	if uc.baseURL != "" {
		caseURL = fmt.Sprintf("%s/ws/%s/cases/%d", uc.baseURL, workspaceID, c.ID)
		if bookmarkErr := uc.slackService.AddBookmark(ctx, channelID, i18n.T(ctx, i18n.MsgBookmarkOpenCase), caseURL); bookmarkErr != nil {
			errutil.Handle(ctx, bookmarkErr, "failed to add bookmark to Slack channel")
		}
	}

	// Post welcome messages defined in workspace configuration. The Case
	// passed to the renderer carries the freshly-assigned channel ID so
	// templates can reference it.
	c.SlackChannelID = channelID
	uc.postWelcomeMessages(ctx, workspaceID, c, channelID, caseURL)

	// Sync channel members (for both private and public cases).
	var channelUserIDs []string
	members, membersErr := uc.slackService.GetConversationMembers(ctx, channelID)
	if membersErr != nil {
		errutil.Handle(ctx, membersErr, "failed to get channel members during case creation")
	} else {
		channelUserIDs = filterHumanUsers(ctx, uc.repo, members)
	}

	c.ChannelUserIDs = channelUserIDs
	updated, err := uc.repo.Case().Update(ctx, workspaceID, c)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case with Slack channel ID",
			goerr.V("orphaned_channel_id", channelID),
			goerr.V(CaseIDKey, c.ID))
	}
	return updated, nil
}

// CaseUpdate represents a partial update to a Case. Each pointer/slice is
// nil-vs-set: nil means "preserve the existing value", a non-nil pointer
// means "set to this value (including empty string)". For Fields the nil
// case preserves all stored field values; a non-nil map merges the supplied
// entries on top of the existing ones (entries are not removed individually
// — clients should send the empty value to clear a field if needed).
type CaseUpdate struct {
	Title       *string
	Description *string
	// nil means "preserve existing assignees"; a non-nil slice (including an
	// empty one) means "replace assignees with this list".
	AssigneeIDs []string
	// nil means "preserve all stored field values". A non-nil map merges its
	// entries on top of the existing values (callers cannot remove individual
	// entries via this API).
	Fields    map[string]model.FieldValue
	hasAssign bool
}

// SetAssignees marks the patch as "replacing assignees with ids" (which may
// be empty). Use this rather than assigning the field directly so that the
// nil-vs-empty distinction is preserved through callers that may construct
// an empty slice for a missing input.
func (p *CaseUpdate) SetAssignees(ids []string) {
	if ids == nil {
		ids = []string{}
	}
	p.AssigneeIDs = ids
	p.hasAssign = true
}

func (uc *CaseUseCase) UpdateCase(ctx context.Context, workspaceID string, id int64, patch CaseUpdate) (*model.Case, error) {
	// Get existing case so we can preserve every field the caller didn't touch.
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

	title := existingCase.Title
	if patch.Title != nil {
		t := *patch.Title
		if t == "" {
			return nil, goerr.New("case title cannot be empty", goerr.V(CaseIDKey, id))
		}
		title = t
	}

	description := existingCase.Description
	if patch.Description != nil {
		description = *patch.Description
	}

	assigneeIDs := existingCase.AssigneeIDs
	if patch.hasAssign {
		assigneeIDs = patch.AssigneeIDs
	}

	// Build the field-value map. Without a patch, preserve the existing map
	// verbatim (no validator pass — stale option IDs from a prior config must
	// not cause an unrelated update to fail).
	fieldValues := existingCase.FieldValues
	if patch.Fields != nil {
		// Partial validation: only the submitted entries are type-checked.
		validated := patch.Fields
		if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
			enriched, err := validator.ValidateCaseFieldsPartial(validated)
			if err != nil {
				return nil, goerr.Wrap(err, "field validation failed", goerr.V(CaseIDKey, id))
			}
			validated = enriched
		}
		merged := make(map[string]model.FieldValue, len(existingCase.FieldValues)+len(validated))
		for k, v := range existingCase.FieldValues {
			merged[k] = v
		}
		for k, v := range validated {
			merged[k] = v
		}
		fieldValues = merged
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

	caseModel := &model.Case{
		ID:             id,
		Title:          title,
		Description:    description,
		Status:         existingCase.Status,
		ReporterID:     existingCase.ReporterID,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: existingCase.SlackChannelID,
		IsPrivate:      existingCase.IsPrivate,
		ChannelUserIDs: existingCase.ChannelUserIDs,
		FieldValues:    fieldValues,
		CreatedAt:      existingCase.CreatedAt,
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

	// Cascade-delete actions associated with this case. We pull every
	// action (archived included) because the case itself is being removed,
	// and orphaned action documents would otherwise leak. The repository's
	// Delete is INTERNAL to this cascade — public callers archive instead.
	actions, err := uc.repo.Action().GetByCase(ctx, workspaceID, id, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeAll})
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

	// Drafts are reporter-only. When the auth context is present, only the
	// reporter may see (or even discover) their own draft; for anyone else
	// the case is reported as not found. Without an auth context (system /
	// bot calls) we fall through to the normal access path so internal
	// flows that work with full models keep functioning.
	token, tokenErr := auth.TokenFromContext(ctx)
	if caseModel.IsDraft() && tokenErr == nil && caseModel.ReporterID != token.Sub {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases
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
	if status == types.CaseStatusDraft {
		return nil, goerr.Wrap(ErrCaseIsDraft, "draft case cannot be closed", goerr.V(CaseIDKey, id))
	}
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
	if status == types.CaseStatusDraft {
		return nil, goerr.Wrap(ErrCaseIsDraft, "draft case cannot be reopened", goerr.V(CaseIDKey, id))
	}
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

// ListDrafts returns the auth-context user's own drafts in the workspace.
// Drafts are author-scoped; callers without an auth token receive an empty
// list (drafts cannot be browsed anonymously / from a bot context).
func (uc *CaseUseCase) ListDrafts(ctx context.Context, workspaceID string) ([]*model.Case, error) {
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr != nil {
		return []*model.Case{}, nil
	}
	drafts, err := uc.repo.Case().ListDrafts(ctx, workspaceID, token.Sub)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list drafts")
	}
	return drafts, nil
}

// GetDraft returns a single draft case to its reporter. Other users see
// ErrCaseNotFound (drafts must never leak existence to non-reporters);
// non-draft cases return ErrCaseNotDraft so callers cannot reuse the draft
// resolver to peek at submitted cases.
func (uc *CaseUseCase) GetDraft(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	c, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "draft not found", goerr.V(CaseIDKey, id))
	}
	if !c.IsDraft() {
		return nil, goerr.Wrap(ErrCaseNotDraft, "case is not a draft", goerr.V(CaseIDKey, id))
	}

	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr != nil || c.ReporterID != token.Sub {
		// Hide existence from non-reporters.
		return nil, goerr.Wrap(ErrCaseNotFound, "draft not found", goerr.V(CaseIDKey, id))
	}
	return c, nil
}

// SubmitDraft promotes a draft case to OPEN and triggers the same activation
// side effects (Slack channel, invites, welcome, etc.) as a fresh CreateCase.
// Only the draft's reporter may submit it. If activation fails, the draft is
// kept in DRAFT so the user can retry without losing the saved entry.
func (uc *CaseUseCase) SubmitDraft(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	c, err := uc.GetDraft(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	// Drafts cannot be Submitted with an empty title — Slack channel naming
	// and listing both need at least a few chars. The Save as Draft path
	// allowed empty titles for partial entries; require one on Submit.
	if c.Title == "" {
		return nil, goerr.New("draft title is required before submit",
			goerr.V(CaseIDKey, id))
	}

	if err := c.SubmitDraft(); err != nil {
		return nil, goerr.Wrap(err, "cannot submit draft", goerr.V(CaseIDKey, id))
	}

	updated, err := uc.repo.Case().Update(ctx, workspaceID, c)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to flip draft to open", goerr.V(CaseIDKey, id))
	}

	activated, actErr := uc.activateCase(ctx, workspaceID, updated, "")
	if actErr != nil {
		// activateCase already rolls back the case on Slack failure (Delete).
		// If the rollback path fired, the case is gone; otherwise we need to
		// flip the status back to DRAFT so the user can retry from the same
		// entry rather than starting over.
		if rolled, getErr := uc.repo.Case().Get(ctx, workspaceID, id); getErr == nil {
			rolled.Status = types.CaseStatusDraft
			if _, undoErr := uc.repo.Case().Update(ctx, workspaceID, rolled); undoErr != nil {
				errutil.Handle(ctx, undoErr, "failed to roll status back to draft after activation failure")
			}
		}
		return nil, actErr
	}
	return activated, nil
}

// DiscardDraft permanently deletes the caller's draft. Non-draft cases are
// rejected so callers cannot pivot this method into a "delete any case I
// reported" shortcut.
func (uc *CaseUseCase) DiscardDraft(ctx context.Context, workspaceID string, id int64) error {
	// Reuse GetDraft for the reporter-only / draft-only checks.
	c, err := uc.GetDraft(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if err := uc.repo.Case().Delete(ctx, workspaceID, c.ID); err != nil {
		return goerr.Wrap(err, "failed to discard draft", goerr.V(CaseIDKey, id))
	}
	return nil
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

// resolveAutoInviteUsers resolves auto-invite users from workspace config.
// It collects direct user IDs and resolves user group members.
// Errors during group resolution are logged but do not stop the process.
func (uc *CaseUseCase) resolveAutoInviteUsers(ctx context.Context, workspaceID string) []string {
	if uc.workspaceRegistry == nil || uc.slackService == nil {
		return nil
	}

	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil {
		errutil.Handle(ctx, err, "failed to get workspace entry for auto-invite")
		return nil
	}

	if len(entry.SlackInviteUsers) == 0 && len(entry.SlackInviteGroups) == 0 {
		return nil
	}

	users := make([]string, 0, len(entry.SlackInviteUsers))
	users = append(users, entry.SlackInviteUsers...)

	// Resolve group members
	if len(entry.SlackInviteGroups) > 0 {
		groupMembers := uc.resolveGroupMembers(ctx, entry.SlackInviteGroups, entry.SlackTeamID)
		users = append(users, groupMembers...)
	}

	return users
}

// resolveGroupMembers resolves user group identifiers (IDs or handle names) to member user IDs.
// Handle names are prefixed with "@" (e.g., "@security-team"); everything else is treated as a group ID.
// teamID is passed to ListUserGroups for org-level app support (empty string for WS-level apps).
func (uc *CaseUseCase) resolveGroupMembers(ctx context.Context, groups []string, teamID string) []string {
	var groupIDs []string
	var handleNames []string

	for _, g := range groups {
		if handle, ok := strings.CutPrefix(g, "@"); ok {
			handleNames = append(handleNames, handle)
		} else {
			groupIDs = append(groupIDs, g)
		}
	}

	// Resolve handle names to group IDs via full group list
	if len(handleNames) > 0 {
		allGroups, err := uc.slackService.ListUserGroups(ctx, teamID)
		if err != nil {
			errutil.Handle(ctx, err, "failed to list user groups for handle resolution")
		} else {
			handleToID := make(map[string]string, len(allGroups))
			for _, g := range allGroups {
				if g.Handle != "" {
					handleToID[g.Handle] = g.ID
				}
			}
			for _, handle := range handleNames {
				if id, ok := handleToID[handle]; ok {
					groupIDs = append(groupIDs, id)
				} else {
					// Unknown handle: usually a workspace config typo or a
					// handle that was renamed/deleted in Slack. Surface so
					// the operator can fix the configuration.
					errutil.Handle(ctx, goerr.New("user group handle not found", goerr.V("handle", handle)), "user group handle not found")
				}
			}
		}
	}

	// Resolve group IDs to member user IDs
	groupIDs = uniqueStrings(groupIDs)
	var members []string
	for _, gid := range groupIDs {
		m, err := uc.slackService.GetUserGroupMembers(ctx, gid)
		if err != nil {
			errutil.Handle(ctx, err, "failed to get user group members")
			continue
		}
		members = append(members, m...)
	}

	return members
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
		// On error, return all IDs to avoid data loss; report so the
		// degraded mode is visible.
		errutil.Handle(ctx, goerr.Wrap(err, "failed to get slack users for bot filtering, returning all IDs",
			goerr.V("userIDs", userIDs),
		), "failed to get slack users for bot filtering")
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

// GetActionStatusSet returns the resolved ActionStatusSet for the workspace,
// falling back to the legacy default when the workspace is unknown or has no
// custom configuration. This is the canonical accessor for any layer that
// needs to render or validate action statuses outside ActionUseCase.
func (uc *CaseUseCase) GetActionStatusSet(workspaceID string) *model.ActionStatusSet {
	return resolveActionStatusSet(uc.workspaceRegistry, workspaceID)
}
