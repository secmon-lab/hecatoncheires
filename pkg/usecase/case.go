package usecase

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

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

// CaseEventPublisher is the narrow surface of pkg/usecase/job.UseCase that
// CaseUseCase calls into after a lifecycle transition. The interface is
// defined here so this package does not import pkg/usecase/job (which
// would create a cycle: job → usecase → job).
type CaseEventPublisher interface {
	PublishCaseLifecycle(ctx context.Context, workspaceID string, c *model.Case, lifecycle model.CaseLifecycle, actorUserID string)
}

type CaseUseCase struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	slackService      slack.Service
	slackAdminService slack.AdminService
	baseURL           string
	welcomeRenderers  map[string]*welcomeRenderer
	eventPublisher    CaseEventPublisher
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

// SetEventPublisher wires the lifecycle event publisher. Called once at
// startup after the job dispatch UseCase has been constructed. nil is
// allowed (Job dispatch effectively disabled).
func (uc *CaseUseCase) SetEventPublisher(p CaseEventPublisher) {
	uc.eventPublisher = p
}

// publishLifecycle is a no-op when the publisher is unset. Suppression of
// self-firing (a Job actor mutation re-firing its own event) lives inside
// the publisher implementation; this method only forwards.
func (uc *CaseUseCase) publishLifecycle(ctx context.Context, workspaceID string, c *model.Case, lifecycle model.CaseLifecycle) {
	if uc == nil || uc.eventPublisher == nil || c == nil {
		return
	}
	actor := ""
	if tok, err := auth.TokenFromContext(ctx); err == nil {
		actor = tok.Sub
	}
	uc.eventPublisher.PublishCaseLifecycle(ctx, workspaceID, c, lifecycle, actor)
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

// fieldSchemaForWorkspace returns the configured FieldSchema for the
// workspace, or nil when none is registered. Used when callers need the
// raw definitions (e.g. enumerating required fields) rather than a
// validator wrapper.
func (uc *CaseUseCase) fieldSchemaForWorkspace(workspaceID string) *config.FieldSchema {
	if uc.workspaceRegistry == nil {
		return nil
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil {
		return nil
	}
	return entry.FieldSchema
}

// fieldValidationMode selects how strictly validateCaseWrite treats the
// supplied custom-field values. The three modes mirror the three legal
// shapes a case write can take.
type fieldValidationMode int

const (
	// validatePartial type-checks the submitted fields, PRESERVES unknown
	// ids (forward-compat for fields removed from the schema), and does not
	// require missing required fields. Used by the draft paths.
	validatePartial fieldValidationMode = iota
	// validatePartialStrict type-checks the submitted fields, REJECTS unknown
	// ids, and does not require missing required fields. Used by the partial
	// edit paths whose input is untrusted (UpdateCase / MaterializeThreadCase).
	validatePartialStrict
	// validateAll type-checks every field, REJECTS unknown ids, and requires
	// every required field. Used by the open-create paths.
	validateAll
)

// validateCaseWrite is the single validation gate every case write funnels
// through so the agent (UpdateCase / MaterializeThreadCase), GraphQL, and
// Slack paths enforce identical rules. It (1) runs the workspace field
// validator in the requested mode, enriching each value with its config Type,
// and (2) verifies that every referenced user id — assignees plus the values
// of user / multi-user fields — exists in the SlackUser store. A missing user
// is rejected with ErrUnknownUser (Slack sync delay is treated as
// non-existence per project policy). A nil workspace validator skips the field
// checks (no schema configured) but the user-existence check still runs.
// Returns the enriched field values (nil-safe: a nil fieldValues yields nil).
func (uc *CaseUseCase) validateCaseWrite(
	ctx context.Context,
	workspaceID string,
	mode fieldValidationMode,
	fieldValues map[string]model.FieldValue,
	assigneeIDs []string,
) (map[string]model.FieldValue, error) {
	enriched := fieldValues
	// Skip the field validator only for partial modes with no submitted fields
	// (an assignee-only / status-adjacent update must not touch untouched
	// fields). validateAll always runs so missing required fields are caught
	// even when the caller supplied none.
	if fieldValues != nil || mode == validateAll {
		if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
			var err error
			switch mode {
			case validateAll:
				enriched, err = validator.ValidateCaseFieldsAll(fieldValues)
			case validatePartialStrict:
				enriched, err = validator.ValidateCaseFieldsPartialStrict(fieldValues)
			default:
				enriched, err = validator.ValidateCaseFieldsPartial(fieldValues)
			}
			if err != nil {
				return nil, goerr.Wrap(err, "case field validation failed", goerr.V("workspace_id", workspaceID))
			}
		}
	}

	if err := uc.verifyUsersExist(ctx, assigneeIDs, enriched); err != nil {
		return nil, err
	}
	if err := uc.verifyCaseRefsExist(ctx, workspaceID, enriched); err != nil {
		return nil, err
	}
	return enriched, nil
}

// verifyCaseRefsExist confirms that every case_ref / multi-case-
// reference value points at a Case that exists in the field's configured
// reference_workspace and is neither private nor a draft. Private and draft
// Cases are never referenceable (project requirement), independent of the
// requesting user — so this check runs even in bot/system contexts that carry
// no auth token. Lookups are batched per reference_workspace (N+1-safe).
func (uc *CaseUseCase) verifyCaseRefsExist(ctx context.Context, workspaceID string, fieldValues map[string]model.FieldValue) error {
	if len(fieldValues) == 0 {
		return nil
	}
	schema := uc.fieldSchemaForWorkspace(workspaceID)
	if schema == nil {
		return nil
	}
	defByID := make(map[string]config.FieldDefinition, len(schema.Fields))
	for _, fd := range schema.Fields {
		defByID[fd.ID] = fd
	}

	// reference_workspace -> set of Case IDs to fetch.
	idsByWS := make(map[string]map[int64]struct{})
	// per (fieldID) -> its referenced (workspace, ids) for violation attribution.
	type fieldRefs struct {
		refWS string
		ids   []int64
	}
	var perField []struct {
		fieldID string
		refs    fieldRefs
	}

	for _, fv := range fieldValues {
		if !fv.Type.IsCaseRef() {
			continue
		}
		fd, ok := defByID[string(fv.FieldID)]
		if !ok || fd.ReferenceWorkspace == "" {
			// Unknown field id is reported by the field validator; a missing
			// reference_workspace is rejected at config load. Skip defensively.
			continue
		}
		ids, err := caseRefIDs(fv)
		if err != nil {
			return goerr.Wrap(err, "invalid case reference value",
				goerr.V("field_id", fv.FieldID))
		}
		if len(ids) == 0 {
			continue
		}
		if idsByWS[fd.ReferenceWorkspace] == nil {
			idsByWS[fd.ReferenceWorkspace] = make(map[int64]struct{})
		}
		for _, id := range ids {
			idsByWS[fd.ReferenceWorkspace][id] = struct{}{}
		}
		perField = append(perField, struct {
			fieldID string
			refs    fieldRefs
		}{fieldID: string(fv.FieldID), refs: fieldRefs{refWS: fd.ReferenceWorkspace, ids: ids}})
	}

	if len(idsByWS) == 0 {
		return nil
	}

	// Batch-fetch each reference workspace exactly once.
	fetched := make(map[string]map[int64]*model.Case, len(idsByWS))
	for refWS, idSet := range idsByWS {
		ids := make([]int64, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		found, err := uc.repo.Case().GetByIDs(ctx, refWS, ids)
		if err != nil {
			return goerr.Wrap(err, "failed to look up referenced cases",
				goerr.V("reference_workspace", refWS))
		}
		fetched[refWS] = found
	}

	var violations []string
	for _, pf := range perField {
		found := fetched[pf.refs.refWS]
		for _, id := range pf.refs.ids {
			c, ok := found[id]
			switch {
			case !ok:
				violations = append(violations, fmt.Sprintf(
					"field %q: referenced case #%d not found in workspace %q", pf.fieldID, id, pf.refs.refWS))
			case c.IsPrivate:
				violations = append(violations, fmt.Sprintf(
					"field %q: referenced case #%d is private and cannot be referenced", pf.fieldID, id))
			case c.Status == types.CaseStatusDraft:
				violations = append(violations, fmt.Sprintf(
					"field %q: referenced case #%d is a draft and cannot be referenced", pf.fieldID, id))
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return goerr.Wrap(model.ErrCaseFieldValidation,
			"case reference validation failed:\n- "+strings.Join(violations, "\n- "),
			goerr.V("violations", violations))
	}
	return nil
}

// caseRefIDs extracts the numeric Case IDs from a case_ref value.
// Single values arrive as a string, multi values as []string / []any (depending
// on whether they went through typed coercion or a raw decode).
func caseRefIDs(fv model.FieldValue) ([]int64, error) {
	// A cleared optional case_ref arrives with a nil value; treat it as no
	// references rather than a type error.
	if fv.Value == nil {
		return nil, nil
	}
	parse := func(s string) (int64, bool, error) {
		if s == "" {
			return 0, false, nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false, goerr.Wrap(err, "case reference must be a numeric case ID",
				goerr.V("value", s))
		}
		return n, true, nil
	}

	switch fv.Type {
	case types.FieldTypeCaseRef:
		s, ok := fv.Value.(string)
		if !ok {
			return nil, goerr.New("case_ref value must be a string")
		}
		n, present, err := parse(s)
		if err != nil || !present {
			return nil, err
		}
		return []int64{n}, nil
	case types.FieldTypeMultiCaseRef:
		var out []int64
		for _, s := range coerceUserIDSlice(fv.Value) {
			n, present, err := parse(s)
			if err != nil {
				return nil, err
			}
			if present {
				out = append(out, n)
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

// verifyUsersExist collects every user id referenced by the write — the
// assignees plus the values of user / multi-user custom fields — and confirms
// each exists in the SlackUser store with a single batch lookup (N+1-safe).
// Reporter is intentionally NOT checked: it is set from the auth context /
// inbound Slack event, where the user provably exists even if the periodic
// SlackUser sync has not caught up yet. Unknown ids are reported together via
// ErrUnknownUser.
func (uc *CaseUseCase) verifyUsersExist(ctx context.Context, assigneeIDs []string, fieldValues map[string]model.FieldValue) error {
	idSet := make(map[string]struct{}, len(assigneeIDs))
	for _, id := range assigneeIDs {
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	for _, fv := range fieldValues {
		switch fv.Type {
		case types.FieldTypeUser:
			if s, ok := fv.Value.(string); ok && s != "" {
				idSet[s] = struct{}{}
			}
		case types.FieldTypeMultiUser:
			for _, s := range coerceUserIDSlice(fv.Value) {
				if s != "" {
					idSet[s] = struct{}{}
				}
			}
		}
	}
	if len(idSet) == 0 {
		return nil
	}

	ids := make([]model.SlackUserID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, model.SlackUserID(id))
	}
	found, err := uc.repo.SlackUser().GetByIDs(ctx, ids)
	if err != nil {
		return goerr.Wrap(err, "failed to look up users for case write")
	}

	var missing []string
	for id := range idSet {
		if _, ok := found[model.SlackUserID(id)]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return goerr.Wrap(ErrUnknownUser,
			"unknown user id(s): "+strings.Join(missing, ", "),
			goerr.V("missing_user_ids", missing))
	}
	return nil
}

// coerceUserIDSlice extracts string ids from a multi-user field value, which
// may arrive as []string (typed coercion) or []any (raw decode).
func coerceUserIDSlice(v any) []string {
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// mergeFieldValues overlays the validated patch onto a copy of the existing
// field values. It is the shared merge behind UpdateCase / MaterializeThreadCase
// / SubmitDraft so the "preserve untouched fields, replace submitted ones"
// contract lives in one place. existing is never mutated.
func mergeFieldValues(existing, patch map[string]model.FieldValue) map[string]model.FieldValue {
	merged := make(map[string]model.FieldValue, len(existing)+len(patch))
	maps.Copy(merged, existing)
	maps.Copy(merged, patch)
	return merged
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
func (uc *CaseUseCase) CreateCase(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool, isTest bool, sourceTeamID string, requestKey string) (*model.Case, error) {
	created, err := uc.persistCase(ctx, workspaceID, persistCaseInput{
		Title:       title,
		Description: description,
		Status:      types.CaseStatusOpen,
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		IsTest:      isTest,
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

	activated, actErr := uc.activateCase(ctx, workspaceID, created, sourceTeamID)
	if actErr != nil {
		// CreateCase's rollback policy: discard the half-formed case so the
		// whole call appears atomic. SubmitDraft uses a different policy
		// (status flip back to DRAFT) — see its implementation.
		if delErr := uc.repo.Case().Delete(ctx, workspaceID, created.ID); delErr != nil {
			return nil, goerr.Wrap(actErr, "case activation failed and rollback delete also failed",
				goerr.V("rollback_error", delErr),
				goerr.V(CaseIDKey, created.ID))
		}
		return nil, actErr
	}

	// Fire the case lifecycle event AFTER activation succeeded. Failure
	// here must not roll back the case — the Job dispatch is fire-and-
	// forget by design.
	uc.publishLifecycle(ctx, workspaceID, activated, model.CaseLifecycleCreated)
	return activated, nil
}

// CreateDraft persists a case in status=DRAFT — i.e. an "in-progress" entry
// saved from the Slack creation modal's Save as Draft button. None of the
// activation side effects (Slack channel, invites, welcome, etc.) run; those
// fire only when the draft is later promoted via SubmitDraft.
//
// The reporter (auth-context Slack user) becomes the draft owner; the
// returned case carries the assigned ID so the caller can echo it back to
// the user.
func (uc *CaseUseCase) CreateDraft(ctx context.Context, workspaceID string, title, description string, assigneeIDs []string, fieldValues map[string]model.FieldValue, isPrivate bool, isTest bool) (*model.Case, error) {
	// Title is intentionally optional for drafts: half-written entries are
	// the whole point. We still validate field values to keep the draft
	// usable on Submit without surprise validation failures.
	return uc.persistCase(ctx, workspaceID, persistCaseInput{
		Title:       title,
		Description: description,
		Status:      types.CaseStatusDraft,
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		IsTest:      isTest,
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
	IsTest      bool
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

	// Validate and enrich custom fields with Type from config, and verify
	// every referenced user (assignees + user-field values) exists. Drafts use
	// the partial mode: supplied fields are type-checked, but missing required
	// fields do NOT fail — half-finished entries are the whole point of the
	// draft state. The full required-field check runs again in SubmitDraft
	// before promoting the case to OPEN.
	mode := validateAll
	if in.Status.IsDraft() {
		mode = validatePartial
	}
	enriched, err := uc.validateCaseWrite(ctx, workspaceID, mode, in.FieldValues, in.AssigneeIDs)
	if err != nil {
		return nil, goerr.Wrap(err, "case write validation failed")
	}
	in.FieldValues = enriched

	// Set reporter from auth context (immutable after creation).
	var reporterID string
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		reporterID = token.Sub
	}

	now := time.Now().UTC()
	caseModel := &model.Case{
		Title:       in.Title,
		Description: in.Description,
		Status:      in.Status,
		ReporterID:  reporterID,
		AssigneeIDs: in.AssigneeIDs,
		IsPrivate:   in.IsPrivate,
		IsTest:      in.IsTest,
		FieldValues: in.FieldValues,
		RequestKey:  in.RequestKey,
		CreatedAt:   now,
		UpdatedAt:   now,
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
// activateCase is intentionally non-destructive: on Slack channel creation
// failure it returns the error without touching the persisted case. The
// caller decides the rollback policy:
//
//   - CreateCase rolls back by deleting the just-persisted case so the
//     entire "create" call appears atomic to the user.
//   - SubmitDraft rolls back by flipping the case status back to DRAFT
//     so the user does not lose work they had saved.
//
// Activation is a no-op when no Slack service is configured.
func (uc *CaseUseCase) activateCase(ctx context.Context, workspaceID string, c *model.Case, sourceTeamID string) (*model.Case, error) {
	if uc.slackService == nil {
		return c, nil
	}

	prefix := uc.slackChannelPrefixForWorkspace(workspaceID)
	teamID := uc.slackTeamIDForWorkspace(workspaceID)
	channelID, err := uc.slackService.CreateChannel(ctx, c.ID, c.Title, prefix, c.IsPrivate, teamID)
	if err != nil {
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

	// Invite reporter, the actor (= auth-context user that triggered the
	// activation), assignees, and auto-invite users to the channel.
	//
	// The "actor" inclusion is what keeps the SubmitDraft path symmetric
	// with CreateCase: when Alice creates a draft and Bob promotes it,
	// the reporter (Alice) and the submitter (Bob) both need to be in
	// the channel — otherwise Bob, who just kicked off the case from
	// Web, would end up unable to follow it in Slack. For CreateCase the
	// actor and reporter are usually the same person, so the extra
	// append simply dedupes through uniqueStrings below.
	usersToInvite := make([]string, 0, len(c.AssigneeIDs)+2)
	if c.ReporterID != "" {
		usersToInvite = append(usersToInvite, c.ReporterID)
	}
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil && token.Sub != "" {
		usersToInvite = append(usersToInvite, token.Sub)
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
	c.UpdatedAt = time.Now().UTC()
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
//
// Assignees are intentionally NOT part of this patch. They are mutated only
// through the dedicated AssignCase / UnassignCase delta operations, so every
// entry point shares one race-free path and a full-list replace can never
// silently clobber a concurrent edit.
type CaseUpdate struct {
	Title       *string
	Description *string
	// IsTest, when non-nil, sets the test flag; nil preserves the stored value.
	// A pointer is required to distinguish "leave unchanged" from "set false".
	IsTest *bool
	// nil means "preserve all stored field values". A non-nil map merges its
	// entries on top of the existing values (callers cannot remove individual
	// entries via this API).
	Fields map[string]model.FieldValue
}

func (uc *CaseUseCase) UpdateCase(ctx context.Context, workspaceID string, id int64, patch CaseUpdate) (*model.Case, error) {
	// Get existing case so we can preserve every field the caller didn't touch.
	existingCase, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control. Private drafts have no Slack channel yet so the
	// ChannelUserIDs-based check would lock out the reporter too; fall
	// back to reporter for private drafts only. Public drafts are
	// workspace-shared and editable by anyone.
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		if existingCase.IsDraft() {
			if existingCase.IsPrivate && existingCase.ReporterID != token.Sub {
				return nil, goerr.Wrap(ErrAccessDenied, "cannot update private draft",
					goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
			}
		} else if !model.IsCaseAccessible(existingCase, token.Sub) {
			return nil, goerr.Wrap(ErrAccessDenied, "cannot update private case",
				goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
		}
	}

	// Apply the patch onto the loaded Case (PATCH, not PUT): only fields the
	// caller actually set are overwritten; everything else — SlackThreadTS,
	// BoardStatus, RequestKey, AgentAdditionalPrompt, AgentSourceIDs, assignees,
	// Status, timestamps — is left exactly as stored, then persisted verbatim.
	// Reconstructing a fresh &model.Case{...} (PUT) silently dropped every field
	// not re-listed: that is how a thread-mode Case lost its SlackThreadTS on any
	// edit, unbinding it from its Slack thread so GetBySlackThread stopped finding
	// it — the next mention then created a duplicate Case and the on-closed Job
	// posted to the channel root instead of the thread.
	// Drafts may carry an empty title — that's the whole point of the "save in
	// progress" state. The empty-title gate fires again at SubmitDraft time,
	// before promoting to OPEN.
	if patch.Title != nil && *patch.Title == "" && !existingCase.IsDraft() {
		return nil, goerr.New("case title cannot be empty", goerr.V(CaseIDKey, id))
	}

	if patch.Description != nil {
		existingCase.Description = *patch.Description
	}

	if patch.IsTest != nil {
		existingCase.IsTest = *patch.IsTest
	}

	// Validate the submitted fields through the shared gate, then merge the
	// enriched values onto the existing ones. Without a field patch, the map is
	// left untouched (no validator pass — stale option IDs from a prior config
	// must not cause an unrelated update to fail).
	if patch.Fields != nil {
		validated, err := uc.validateCaseWrite(ctx, workspaceID, validatePartialStrict, patch.Fields, nil)
		if err != nil {
			return nil, goerr.Wrap(err, "case write validation failed", goerr.V(CaseIDKey, id))
		}
		existingCase.FieldValues = mergeFieldValues(existingCase.FieldValues, validated)
	}

	// Rename the Slack channel only after EVERY validation has passed. Renaming
	// is an external side effect that cannot be rolled back, so a later field
	// validation failure must not leave the channel renamed while the DB still
	// holds the old title (state desync). existingCase.Title is still the old
	// title here — it is assigned below.
	if patch.Title != nil && uc.slackService != nil && existingCase.SlackChannelID != "" && existingCase.Title != *patch.Title {
		prefix := uc.slackChannelPrefixForWorkspace(workspaceID)
		if err := uc.slackService.RenameChannel(ctx, existingCase.SlackChannelID, id, *patch.Title, prefix); err != nil {
			return nil, goerr.Wrap(err, "failed to rename Slack channel",
				goerr.V(CaseIDKey, id),
				goerr.V("channel_id", existingCase.SlackChannelID))
		}
	}

	if patch.Title != nil {
		existingCase.Title = *patch.Title
	}

	existingCase.UpdatedAt = time.Now().UTC()

	updated, err := uc.repo.Case().Update(ctx, workspaceID, existingCase)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case", goerr.V(CaseIDKey, id))
	}

	return updated, nil
}

// assertCaseEditable loads the case and runs the same access-control gate as
// UpdateCase (draft-aware: private drafts fall back to a reporter check since
// they have no Slack channel yet). It is the shared precondition for the
// assignee mutators below.
func (uc *CaseUseCase) assertCaseEditable(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	existingCase, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		if existingCase.IsDraft() {
			if existingCase.IsPrivate && existingCase.ReporterID != token.Sub {
				return nil, goerr.Wrap(ErrAccessDenied, "cannot edit private draft",
					goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
			}
		} else if !model.IsCaseAccessible(existingCase, token.Sub) {
			return nil, goerr.Wrap(ErrAccessDenied, "cannot edit private case",
				goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
		}
	}
	return existingCase, nil
}

// AssignCase atomically adds the given Slack user IDs to the case's assignee
// set. Unlike UpdateCase — which replaces the whole assignee list and therefore
// loses a concurrent edit inside its read-modify-write window — the add is
// applied as a transactional set union in the repository, so two simultaneous
// "assign me" actions both land. IDs already assigned are ignored. New
// assignees must resolve to known Slack users. An empty userIDs slice is a
// no-op that returns the case unchanged.
func (uc *CaseUseCase) AssignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	existingCase, err := uc.assertCaseEditable(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	// An empty input is a guaranteed no-op; short-circuit before the user
	// existence check and the assignee transaction.
	if len(userIDs) == 0 {
		return existingCase, nil
	}

	if err := uc.verifyUsersExist(ctx, userIDs, nil); err != nil {
		return nil, goerr.Wrap(err, "assignee verification failed", goerr.V(CaseIDKey, id))
	}

	updated, err := uc.repo.Case().AddAssignees(ctx, workspaceID, id, userIDs, time.Now().UTC())
	if err != nil {
		return nil, goerr.Wrap(err, "failed to add assignees", goerr.V(CaseIDKey, id))
	}
	return updated, nil
}

// UnassignCase atomically removes the given Slack user IDs from the case's
// assignee set. IDs not currently assigned are ignored. Removal needs no user
// existence check (a since-deleted user must still be removable). An empty
// userIDs slice is a no-op that returns the case unchanged.
func (uc *CaseUseCase) UnassignCase(ctx context.Context, workspaceID string, id int64, userIDs []string) (*model.Case, error) {
	existingCase, err := uc.assertCaseEditable(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	// An empty input is a guaranteed no-op; short-circuit before the
	// assignee transaction.
	if len(userIDs) == 0 {
		return existingCase, nil
	}

	updated, err := uc.repo.Case().RemoveAssignees(ctx, workspaceID, id, userIDs, time.Now().UTC())
	if err != nil {
		return nil, goerr.Wrap(err, "failed to remove assignees", goerr.V(CaseIDKey, id))
	}
	return updated, nil
}

// diffAssignees computes the delta needed to turn the current assignee set
// into the desired one: toAdd are ids in desired but not current, toRemove are
// ids in current but not desired. It lets a full-selection UI (e.g. a Slack
// multi-user select) reconcile through the delta AssignCase / UnassignCase
// path instead of a full-list replace. Blank ids in either input are ignored.
func diffAssignees(current, desired []string) (toAdd, toRemove []string) {
	currentSet := make(map[string]struct{}, len(current))
	for _, id := range current {
		if id != "" {
			currentSet[id] = struct{}{}
		}
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, id := range desired {
		if id == "" {
			continue
		}
		desiredSet[id] = struct{}{}
		if _, ok := currentSet[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	for _, id := range current {
		if id == "" {
			continue
		}
		if _, ok := desiredSet[id]; !ok {
			toRemove = append(toRemove, id)
		}
	}
	return toAdd, toRemove
}

// UpdateAgentSettings replaces the Case-specific agent additional prompt
// and the AgentSourceIDs whitelist. enabledSourceIDs == nil or empty
// resets the selection to "use every Source". Non-empty IDs are
// validated against the Workspace's Source list — any unknown ID makes
// the whole update fail with ErrInvalidArgument (we never silently
// drop an ID the caller meant to keep). Order is preserved exactly as
// supplied so the UI selection round-trips unchanged.
func (uc *CaseUseCase) UpdateAgentSettings(ctx context.Context, workspaceID string, caseID int64, additionalPrompt string, enabledSourceIDs []model.SourceID) (*model.Case, error) {
	existing, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, caseID))
	}

	// Access control. Drafts cannot carry agent settings (no agent runs
	// against an unsubmitted draft anyway), so reject the call early.
	if existing.IsDraft() {
		return nil, goerr.Wrap(ErrCaseIsDraft,
			"agent settings are unavailable on drafts",
			goerr.V(CaseIDKey, caseID))
	}

	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(existing, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot update agent settings on private case",
			goerr.V(CaseIDKey, caseID), goerr.V("user_id", token.Sub))
	}

	// Validate Source IDs against the workspace catalogue. We load the
	// full list (small per workspace) once instead of N parallel Gets
	// because callers typically pick a handful.
	if len(enabledSourceIDs) > 0 {
		sources, err := uc.repo.Source().List(ctx, workspaceID)
		if err != nil {
			return nil, goerr.Wrap(err, "list sources for agent settings",
				goerr.V("workspace_id", workspaceID))
		}
		known := make(map[model.SourceID]struct{}, len(sources))
		for _, s := range sources {
			known[s.ID] = struct{}{}
		}
		seen := make(map[model.SourceID]struct{}, len(enabledSourceIDs))
		for _, id := range enabledSourceIDs {
			if id == "" {
				return nil, goerr.Wrap(ErrInvalidArgument,
					"source id is empty", goerr.V(CaseIDKey, caseID))
			}
			if _, ok := known[id]; !ok {
				return nil, goerr.Wrap(ErrInvalidArgument,
					"unknown source id",
					goerr.V("source_id", string(id)),
					goerr.V(CaseIDKey, caseID))
			}
			if _, dup := seen[id]; dup {
				return nil, goerr.Wrap(ErrInvalidArgument,
					"duplicate source id",
					goerr.V("source_id", string(id)),
					goerr.V(CaseIDKey, caseID))
			}
			seen[id] = struct{}{}
		}
	}

	// Mutate the existing pointer rather than rebuilding the struct: the
	// rebuild pattern silently drops any new field added to model.Case
	// later. The repository's Update is a pure Set; whatever we hand
	// it is what lands in storage.
	existing.AgentAdditionalPrompt = additionalPrompt
	if len(enabledSourceIDs) == 0 {
		existing.AgentSourceIDs = nil
	} else {
		existing.AgentSourceIDs = append([]model.SourceID(nil), enabledSourceIDs...)
	}
	existing.UpdatedAt = time.Now().UTC()

	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case agent settings",
			goerr.V(CaseIDKey, caseID))
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

	token, tokenErr := auth.TokenFromContext(ctx)

	// Private drafts have no Slack channel yet, so the usual
	// ChannelUserIDs-based access check would lock out the reporter too.
	// For private drafts we restrict visibility to the reporter; public
	// drafts behave like any other case (workspace-wide listing).
	if caseModel.IsDraft() && caseModel.IsPrivate && tokenErr == nil && caseModel.ReporterID != token.Sub {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for non-draft private cases.
	if tokenErr == nil && !caseModel.IsDraft() && !model.IsCaseAccessible(caseModel, token.Sub) {
		return model.RestrictCase(caseModel), nil
	}

	return caseModel, nil
}

// GetCases retrieves multiple cases by ID in a single batch, applying the same
// per-case access control as GetCase. It fetches all requested cases with one
// repository batch call (repo.Case().GetByIDs) to avoid N+1 round-trips, then:
//   - omits cases that do not exist;
//   - omits private drafts the caller did not author (their existence must not
//     leak);
//   - RestrictCases non-draft private cases the caller cannot access.
//
// Results preserve the order of ids; duplicate ids yield duplicate entries.
func (uc *CaseUseCase) GetCases(ctx context.Context, workspaceID string, ids []int64) ([]*model.Case, error) {
	if len(ids) == 0 {
		return []*model.Case{}, nil
	}

	found, err := uc.repo.Case().GetByIDs(ctx, workspaceID, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to batch get cases")
	}

	token, tokenErr := auth.TokenFromContext(ctx)
	out := make([]*model.Case, 0, len(ids))
	for _, id := range ids {
		caseModel, ok := found[id]
		if !ok {
			continue
		}
		// Mirrors GetCase: a private draft is visible only to its reporter.
		if caseModel.IsDraft() && caseModel.IsPrivate && tokenErr == nil && caseModel.ReporterID != token.Sub {
			continue
		}
		if tokenErr == nil && !caseModel.IsDraft() && !model.IsCaseAccessible(caseModel, token.Sub) {
			out = append(out, model.RestrictCase(caseModel))
			continue
		}
		out = append(out, caseModel)
	}
	return out, nil
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

	// The repositories return cases in an unspecified order (memory iterates a
	// map; Firestore orders by document ID). Sort newest-first here so every
	// caller gets a stable, intuitive listing without forcing a Firestore
	// composite index (Status filter + OrderBy would require one).
	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i].CreatedAt.After(cases[j].CreatedAt)
	})

	return cases, nil
}

// referenceableCasesLimit caps how many candidate cases a case_ref picker
// / agent search returns in one call. It also serves as the default when the
// caller passes a non-positive limit.
const referenceableCasesLimit = 50

// ListReferenceableCases returns the non-private, non-draft Cases of
// workspaceID as case_ref candidates. Unlike ListCases, private Cases are
// dropped entirely (never RestrictCase'd) so a picker cannot even reveal that a
// private Case exists — the project rule that private Cases are not
// referenceable. With an empty query, OPEN cases sort first, then by most
// recently updated. With a query, results match the Case title (substring,
// case-insensitive) or the Case ID ("#42" / "42"), sorted by most recently
// updated. At most limit (clamped to referenceableCasesLimit) rows are
// returned. DRAFT cases are excluded because List excludes them by default.
func (uc *CaseUseCase) ListReferenceableCases(ctx context.Context, workspaceID, query string, limit int) ([]model.CaseRef, error) {
	if limit <= 0 || limit > referenceableCasesLimit {
		limit = referenceableCasesLimit
	}

	cases, err := uc.repo.Case().List(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list referenceable cases",
			goerr.V("reference_workspace", workspaceID))
	}

	q := strings.TrimSpace(query)
	lowerQ := strings.ToLower(q)
	var idQuery int64 = -1
	if n, perr := strconv.ParseInt(strings.TrimPrefix(q, "#"), 10, 64); perr == nil {
		idQuery = n
	}

	matched := make([]*model.Case, 0, len(cases))
	for _, c := range cases {
		// List excludes drafts by default, but enforce the "drafts are never
		// referenceable" rule explicitly so it does not depend on that default.
		if c.IsPrivate || c.Status.Normalize() == types.CaseStatusDraft {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(c.Title), lowerQ) && c.ID != idQuery {
				continue
			}
		}
		matched = append(matched, c)
	}

	openFirst := q == ""
	sort.SliceStable(matched, func(i, j int) bool {
		if openFirst {
			oi := matched[i].Status.Normalize() == types.CaseStatusOpen
			oj := matched[j].Status.Normalize() == types.CaseStatusOpen
			if oi != oj {
				return oi // OPEN cases sort before non-OPEN
			}
		}
		return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}

	refs := make([]model.CaseRef, len(matched))
	for i, c := range matched {
		refs[i] = model.NewCaseRef(workspaceID, c)
	}
	return refs, nil
}

// ResolveCaseRefs resolves the given Case IDs in workspaceID to their CaseRef
// summaries, dropping any ID that is missing, private, or a draft (the caller
// renders those as "unavailable"). Used to label existing case_ref values
// for display. A single batch lookup keeps it N+1-safe.
func (uc *CaseUseCase) ResolveCaseRefs(ctx context.Context, workspaceID string, ids []int64) ([]model.CaseRef, error) {
	cases, err := uc.GetReferenceableCases(ctx, workspaceID, ids)
	if err != nil {
		return nil, err
	}
	refs := make([]model.CaseRef, len(cases))
	for i, c := range cases {
		refs[i] = model.NewCaseRef(workspaceID, c)
	}
	return refs, nil
}

// GetReferenceableCases batch-fetches the full Cases for the given IDs in
// workspaceID, returning only those that are referenceable (non-private,
// non-draft, existing). IDs that are missing / private / draft are omitted, so
// the caller can diff the input against the result to report them. Results
// preserve the input ID order. This backs the agent's detail-fetch tool.
func (uc *CaseUseCase) GetReferenceableCases(ctx context.Context, workspaceID string, ids []int64) ([]*model.Case, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	found, err := uc.repo.Case().GetByIDs(ctx, workspaceID, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch referenceable cases",
			goerr.V("reference_workspace", workspaceID))
	}
	out := make([]*model.Case, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		c, ok := found[id]
		if !ok || c.IsPrivate || c.Status.Normalize() == types.CaseStatusDraft {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// ReferenceWorkspaceForField returns the reference_workspace configured for the
// case_ref field fieldID in workspaceID. It errors when the field is
// unknown or is not a case_ref type, so an agent tool that was handed a
// wrong field id fails loudly instead of querying the wrong workspace.
func (uc *CaseUseCase) ReferenceWorkspaceForField(workspaceID, fieldID string) (string, error) {
	schema := uc.fieldSchemaForWorkspace(workspaceID)
	if schema == nil {
		return "", goerr.Wrap(ErrCaseNotFound, "workspace has no field schema",
			goerr.V("workspace_id", workspaceID))
	}
	for _, fd := range schema.Fields {
		if fd.ID == fieldID {
			if !fd.Type.IsCaseRef() {
				return "", goerr.New("field is not a case_ref field",
					goerr.V("field_id", fieldID), goerr.V("field_type", fd.Type))
			}
			return fd.ReferenceWorkspace, nil
		}
	}
	return "", goerr.New("field not defined in the workspace schema",
		goerr.V("field_id", fieldID), goerr.V("workspace_id", workspaceID))
}

// RenderCaseFieldValues flattens a Case's field values into a plain map for an
// agent tool response. Non-reference fields render their raw stored value (as
// the existing tool responses do). Case-reference fields are resolved one level
// — each referenced Case ID becomes {id, title, status} — using the field's
// configured reference_workspace. An ID that no longer resolves (the referenced
// Case was deleted, made private, or turned into a draft after it was stored)
// renders as {id, available:false}; this read-time drift is the only place an
// "unavailable" reference can appear, since writes are gated by
// verifyCaseRefsExist. A repository lookup failure is propagated, never
// swallowed.
func (uc *CaseUseCase) RenderCaseFieldValues(ctx context.Context, workspaceID string, fieldValues map[string]model.FieldValue) (map[string]any, error) {
	out := make(map[string]any, len(fieldValues))
	schema := uc.fieldSchemaForWorkspace(workspaceID)
	defByID := make(map[string]config.FieldDefinition)
	if schema != nil {
		for _, fd := range schema.Fields {
			defByID[fd.ID] = fd
		}
	}

	for id, fv := range fieldValues {
		if !fv.Type.IsCaseRef() {
			out[id] = fv.Value
			continue
		}
		fd, ok := defByID[id]
		if !ok || fd.ReferenceWorkspace == "" {
			// Field removed from the schema (drift) or with no target workspace:
			// fall back to the raw stored value instead of failing the whole
			// field_values render with a doomed cross-workspace lookup.
			out[id] = fv.Value
			continue
		}
		ids, err := caseRefIDs(fv)
		if err != nil {
			return nil, goerr.Wrap(err, "invalid stored case reference value",
				goerr.V("field_id", id))
		}
		resolved, err := uc.ResolveCaseRefs(ctx, fd.ReferenceWorkspace, ids)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]model.CaseRef, len(resolved))
		for _, r := range resolved {
			byID[r.ID] = r
		}
		render := func(refID int64) map[string]any {
			if r, ok := byID[refID]; ok {
				return map[string]any{"id": r.ID, "title": r.Title, "status": r.Status.String()}
			}
			return map[string]any{"id": refID, "available": false}
		}
		if fv.Type == types.FieldTypeCaseRef {
			if len(ids) == 0 {
				out[id] = nil
			} else {
				out[id] = render(ids[0])
			}
			continue
		}
		arr := make([]any, 0, len(ids))
		for _, refID := range ids {
			arr = append(arr, render(refID))
		}
		out[id] = arr
	}
	return out, nil
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

	// Thread-mode cases close by moving to a closed board status (UpdateCaseStatus),
	// which keeps BoardStatus and the lifecycle Status in sync. Closing one
	// directly here would desync them, so reject it at the boundary — this is the
	// usecase-level counterpart of UpdateCaseStatus rejecting channel-mode cases.
	if existing.IsThreadBound() {
		return nil, goerr.Wrap(ErrCaseThreadModeUseStatus, "thread-mode case cannot be closed directly", goerr.V(CaseIDKey, id))
	}

	status := existing.Status.Normalize()
	if status == types.CaseStatusDraft {
		return nil, goerr.Wrap(ErrCaseIsDraft, "draft case cannot be closed", goerr.V(CaseIDKey, id))
	}
	if status == types.CaseStatusClosed {
		return nil, goerr.Wrap(ErrCaseAlreadyClosed, "case is already closed", goerr.V(CaseIDKey, id))
	}

	existing.Status = types.CaseStatusClosed
	existing.UpdatedAt = time.Now().UTC()
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to close case", goerr.V(CaseIDKey, id))
	}

	uc.publishLifecycle(ctx, workspaceID, updated, model.CaseLifecycleClosed)
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

	// Thread-mode cases reopen by moving to a non-closed board status
	// (UpdateCaseStatus); reopening directly here would desync BoardStatus and
	// the lifecycle Status. Reject at the boundary, mirroring CloseCase.
	if existing.IsThreadBound() {
		return nil, goerr.Wrap(ErrCaseThreadModeUseStatus, "thread-mode case cannot be reopened directly", goerr.V(CaseIDKey, id))
	}

	status := existing.Status.Normalize()
	if status == types.CaseStatusDraft {
		return nil, goerr.Wrap(ErrCaseIsDraft, "draft case cannot be reopened", goerr.V(CaseIDKey, id))
	}
	if status == types.CaseStatusOpen {
		return nil, goerr.Wrap(ErrCaseAlreadyOpen, "case is already open", goerr.V(CaseIDKey, id))
	}

	existing.Status = types.CaseStatusOpen
	existing.UpdatedAt = time.Now().UTC()
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to reopen case", goerr.V(CaseIDKey, id))
	}

	return updated, nil
}

// caseStatusSetForWorkspace returns the configurable Case status set for the
// workspace (thread mode), or nil when the workspace is channel mode / has no
// case status set.
func (uc *CaseUseCase) caseStatusSetForWorkspace(workspaceID string) *model.ActionStatusSet {
	if uc.workspaceRegistry == nil {
		return nil
	}
	entry, err := uc.workspaceRegistry.Get(workspaceID)
	if err != nil {
		return nil
	}
	return entry.CaseStatusSet
}

// CreateThreadCase creates a thread-mode Case bound to (channelID, threadTS).
// It is idempotent: a re-delivered Slack message that maps to an existing
// thread returns the existing Case unchanged. The reporter is the posting
// user; the initial board status is the workspace's configured initial status.
// CaseLifecycleCreated is published so Jobs fire exactly as in channel mode.
func (uc *CaseUseCase) CreateThreadCase(ctx context.Context, workspaceID, channelID, threadTS, reporterID, title, description string) (*model.Case, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required for thread case")
	}

	existing, err := uc.repo.Case().GetBySlackThread(ctx, workspaceID, channelID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to check existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return existing, nil
	}

	set := uc.caseStatusSetForWorkspace(workspaceID)
	initialStatus := ""
	if set != nil {
		initialStatus = set.InitialID()
	}

	now := time.Now().UTC()
	c := &model.Case{
		Title:          title,
		Description:    description,
		Status:         types.CaseStatusOpen,
		ReporterID:     reporterID,
		SlackChannelID: channelID,
		SlackThreadTS:  threadTS,
		BoardStatus:    initialStatus,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	created, err := uc.repo.Case().Create(ctx, workspaceID, c)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}

	uc.publishLifecycle(ctx, workspaceID, created, model.CaseLifecycleCreated)
	return created, nil
}

// CreateThreadCaseWithFields creates a thread-mode Case with the supplied
// title / description / custom fields, running FULL validation (required
// fields, allowed options, value types) before persisting. All violations are
// aggregated (ValidateCaseFieldsAll), not fail-fast, so the caller — the
// thread-mode initialization (create) agent — can be told everything that is
// wrong in one shot and re-emit. Unlike CreateThreadCase, the case is only
// created once it satisfies the schema; there is no placeholder pass.
func (uc *CaseUseCase) CreateThreadCaseWithFields(ctx context.Context, workspaceID, channelID, threadTS, reporterID, title, description string, fieldValues map[string]model.FieldValue) (*model.Case, error) {
	if channelID == "" || threadTS == "" {
		return nil, goerr.New("channelID and threadTS are required for thread case")
	}

	existing, err := uc.repo.Case().GetBySlackThread(ctx, workspaceID, channelID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to check existing thread case",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	if existing != nil {
		return existing, nil
	}

	enriched, vErr := uc.validateCaseWrite(ctx, workspaceID, validateAll, fieldValues, nil)
	if vErr != nil {
		return nil, goerr.Wrap(vErr, "thread case field validation failed",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	fieldValues = enriched

	set := uc.caseStatusSetForWorkspace(workspaceID)
	initialStatus := ""
	if set != nil {
		initialStatus = set.InitialID()
	}

	now := time.Now().UTC()
	c := &model.Case{
		Title:          title,
		Description:    description,
		Status:         types.CaseStatusOpen,
		ReporterID:     reporterID,
		SlackChannelID: channelID,
		SlackThreadTS:  threadTS,
		BoardStatus:    initialStatus,
		FieldValues:    fieldValues,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	created, err := uc.repo.Case().Create(ctx, workspaceID, c)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create thread case with fields",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}

	uc.publishLifecycle(ctx, workspaceID, created, model.CaseLifecycleCreated)
	return created, nil
}

// MaterializeThreadCase applies the LLM-materialized title / description /
// custom field values onto a thread-mode Case. Empty title / description are
// ignored (the placeholder set at creation is kept). Field values are
// type-checked via the workspace validator before write.
func (uc *CaseUseCase) MaterializeThreadCase(ctx context.Context, workspaceID string, id int64, title, description string, fieldValues map[string]model.FieldValue) (*model.Case, error) {
	existing, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	if title != "" {
		existing.Title = title
	}
	if description != "" {
		existing.Description = description
	}
	if len(fieldValues) > 0 {
		validated, vErr := uc.validateCaseWrite(ctx, workspaceID, validatePartialStrict, fieldValues, nil)
		if vErr != nil {
			return nil, goerr.Wrap(vErr, "thread case field validation failed", goerr.V(CaseIDKey, id))
		}
		existing.FieldValues = mergeFieldValues(existing.FieldValues, validated)
	}

	existing.UpdatedAt = time.Now().UTC()
	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to materialize thread case", goerr.V(CaseIDKey, id))
	}
	return updated, nil
}

// UpdateCaseStatus sets the configurable board status of a thread-mode Case
// and synchronises the lifecycle Status (a closed board status closes the
// case). It is the single entry point for both the Kanban drag-and-drop and
// the agent's `close` decision; CaseLifecycleClosed is published only on the
// open→closed edge so Jobs fire once.
func (uc *CaseUseCase) UpdateCaseStatus(ctx context.Context, workspaceID string, id int64, boardStatus string) (*model.Case, error) {
	set := uc.caseStatusSetForWorkspace(workspaceID)
	if set == nil {
		return nil, goerr.New("workspace has no case status set (not thread mode)",
			goerr.V("workspace_id", workspaceID))
	}
	if !set.IsValid(boardStatus) {
		return nil, goerr.New("invalid board status id",
			goerr.V("workspace_id", workspaceID), goerr.V("board_status", boardStatus))
	}

	existing, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, id))
	}

	// Access control for private cases.
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil && !model.IsCaseAccessible(existing, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot update private case status",
			goerr.V(CaseIDKey, id), goerr.V("user_id", token.Sub))
	}

	wasClosed := existing.Status.Normalize() == types.CaseStatusClosed
	existing.BoardStatus = boardStatus
	existing.SyncLifecycleFromBoardStatus(set)
	existing.UpdatedAt = time.Now().UTC()

	updated, err := uc.repo.Case().Update(ctx, workspaceID, existing)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update case status", goerr.V(CaseIDKey, id))
	}

	if !wasClosed && updated.Status.Normalize() == types.CaseStatusClosed {
		uc.publishLifecycle(ctx, workspaceID, updated, model.CaseLifecycleClosed)
	}
	return updated, nil
}

// ListDrafts returns every draft case in the workspace. Drafts are
// workspace-wide so any team member can pick one up; private drafts are
// the exception and remain visible only to their reporter (a draft has
// no Slack channel yet, so the usual IsCaseAccessible check via
// ChannelUserIDs would lock everyone out — we use ReporterID instead).
func (uc *CaseUseCase) ListDrafts(ctx context.Context, workspaceID string) ([]*model.Case, error) {
	drafts, err := uc.repo.Case().ListDrafts(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list drafts")
	}

	// Apply private-draft access control. Callers without an auth token
	// (bots / system contexts) only see public drafts.
	var requesterID string
	if token, tokenErr := auth.TokenFromContext(ctx); tokenErr == nil {
		requesterID = token.Sub
	}

	visible := make([]*model.Case, 0, len(drafts))
	for _, d := range drafts {
		if d.IsPrivate && d.ReporterID != requesterID {
			continue
		}
		visible = append(visible, d)
	}

	// Match ListCases: newest-first, repository-order-independent.
	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].CreatedAt.After(visible[j].CreatedAt)
	})
	return visible, nil
}

// GetDraft returns a single draft case. Public drafts are visible
// workspace-wide so any team member can preview (and act on) an
// in-progress entry; private drafts remain reporter-only (the usual
// ChannelUserIDs check can't help yet — the draft has no Slack channel).
// Non-draft cases return ErrCaseNotDraft so callers cannot reuse the
// draft resolver to peek at submitted cases.
//
// Mutating actions (SubmitDraft, DiscardDraft) reach the draft through
// this method, so private-draft access control automatically extends to
// them: a non-reporter cannot even discover a private draft, let alone
// modify it. Public drafts are deliberately open — the team owns them.
func (uc *CaseUseCase) GetDraft(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	c, err := uc.repo.Case().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "draft not found", goerr.V(CaseIDKey, id))
	}
	if !c.IsDraft() {
		return nil, goerr.Wrap(ErrCaseNotDraft, "case is not a draft", goerr.V(CaseIDKey, id))
	}

	if c.IsPrivate {
		token, tokenErr := auth.TokenFromContext(ctx)
		if tokenErr != nil || c.ReporterID != token.Sub {
			return nil, goerr.Wrap(ErrCaseNotFound, "draft not found", goerr.V(CaseIDKey, id))
		}
	}
	return c, nil
}

// SubmitDraft promotes a draft case to OPEN and triggers the same activation
// side effects (Slack channel, invites, welcome, etc.) as a fresh CreateCase.
// The optional `patch` carries last-minute edits the caller wants to apply
// atomically before the promotion — passing them to this single usecase
// method (rather than separate UpdateCase + SubmitDraft calls from the
// controller) keeps the "save final edits and submit" business operation
// atomic: required-field validation, channel creation, and invites all see
// the same set of values, and a failure path leaves the draft consistent.
//
// If activation fails the draft is kept in DRAFT so the user can retry
// without losing the saved entry.
func (uc *CaseUseCase) SubmitDraft(ctx context.Context, workspaceID string, id int64, patch *CaseUpdate) (*model.Case, error) {
	c, err := uc.GetDraft(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	// Apply pre-submit edits in memory and persist them before strict
	// validation runs. We route through repo.Case().Update directly rather
	// than UpdateCase() so the activation path further down sees the
	// freshly-stored case (and so private-draft / draft-empty-title quirks
	// stay scoped to SubmitDraft instead of leaking into the generic
	// UpdateCase contract).
	if patch != nil {
		if patch.Title != nil {
			c.Title = *patch.Title
		}
		if patch.Description != nil {
			c.Description = *patch.Description
		}
		if patch.IsTest != nil {
			c.IsTest = *patch.IsTest
		}
		// Assignees are not part of the submit-draft patch; they are managed
		// via AssignCase / UnassignCase on the draft before promotion.
		if patch.Fields != nil {
			validated := patch.Fields
			// Drafts keep the lenient partial validation (preserve unknown ids,
			// no required check) and the ErrFieldValidationFailed wrapping so
			// the GraphQL FIELD_VALIDATION_FAILED code is preserved on the
			// pre-submit edit path. The strict / required enforcement happens
			// below, just before the draft is promoted to OPEN.
			if validator := uc.fieldValidatorForWorkspace(workspaceID); validator != nil {
				enriched, vErr := validator.ValidateCaseFieldsPartial(validated)
				if vErr != nil {
					return nil, goerr.Wrap(ErrFieldValidationFailed, vErr.Error(), goerr.V(CaseIDKey, id))
				}
				validated = enriched
			}
			c.FieldValues = mergeFieldValues(c.FieldValues, validated)
		}
		// Verify every referenced user exists (assignees + user-field values),
		// consistent with every other case write.
		if err := uc.verifyUsersExist(ctx, c.AssigneeIDs, c.FieldValues); err != nil {
			return nil, goerr.Wrap(err, "case write validation failed", goerr.V(CaseIDKey, id))
		}
		c.UpdatedAt = time.Now().UTC()
		persistedPatch, pErr := uc.repo.Case().Update(ctx, workspaceID, c)
		if pErr != nil {
			return nil, goerr.Wrap(pErr, "failed to persist pre-submit edits", goerr.V(CaseIDKey, id))
		}
		c = persistedPatch
	}

	// Drafts cannot be Submitted with an empty title — Slack channel naming
	// and listing both need at least a few chars. The Save as Draft path
	// allowed empty titles for partial entries; require one on Submit.
	if c.Title == "" {
		return nil, goerr.Wrap(ErrDraftTitleRequired,
			"draft title is required before submit",
			goerr.V(CaseIDKey, id))
	}

	// Re-run strict field validation now that the draft is being promoted
	// to OPEN — Save as Draft skipped the required-field check, so this is
	// the first time the workspace's full schema is enforced. Bail out
	// before flipping the status so the user can finish filling required
	// fields on the draft entry and resubmit. We collect *every* missing
	// required field so the UI can list them in one message instead of
	// surfacing them one redirect at a time.
	if schema := uc.fieldSchemaForWorkspace(workspaceID); schema != nil {
		var missingNames []string
		var missingIDs []string
		for _, fd := range schema.Fields {
			if !fd.Required {
				continue
			}
			if _, ok := c.FieldValues[fd.ID]; ok {
				continue
			}
			missingIDs = append(missingIDs, fd.ID)
			name := fd.Name
			if name == "" {
				name = fd.ID
			}
			missingNames = append(missingNames, name)
		}
		if len(missingNames) > 0 {
			return nil, goerr.Wrap(ErrMissingRequiredOnSubmit,
				fmt.Sprintf("required field(s) not filled: %s", strings.Join(missingNames, ", ")),
				goerr.V(CaseIDKey, id),
				goerr.V(MissingFieldIDsKey, missingIDs),
				goerr.V(MissingFieldNamesKey, missingNames),
			)
		}
	}

	if err := c.SubmitDraft(); err != nil {
		return nil, goerr.Wrap(err, "cannot submit draft",
			goerr.V(CaseIDKey, id),
			goerr.V(CurrentStatusKey, string(c.Status)),
		)
	}

	c.UpdatedAt = time.Now().UTC()
	updated, err := uc.repo.Case().Update(ctx, workspaceID, c)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to flip draft to open", goerr.V(CaseIDKey, id))
	}

	activated, actErr := uc.activateCase(ctx, workspaceID, updated, "")
	if actErr != nil {
		// SubmitDraft's rollback policy is "preserve the saved work": flip
		// the status back to DRAFT and keep the row so the user can retry.
		// activateCase is now non-destructive, so the case row is still
		// there waiting to be patched.
		if rolled, getErr := uc.repo.Case().Get(ctx, workspaceID, id); getErr == nil {
			rolled.Status = types.CaseStatusDraft
			rolled.UpdatedAt = time.Now().UTC()
			if _, undoErr := uc.repo.Case().Update(ctx, workspaceID, rolled); undoErr != nil {
				errutil.Handle(ctx, goerr.Wrap(undoErr, "failed to roll status back to draft after activation failure",
					goerr.V(CaseIDKey, id),
				), "failed to roll status back to draft after activation failure")
			}
		} else {
			errutil.Handle(ctx, goerr.Wrap(getErr, "draft case missing during rollback",
				goerr.V(CaseIDKey, id),
			), "draft case missing during rollback")
		}
		return nil, goerr.Wrap(ErrActivationFailed, actErr.Error(), goerr.V(CaseIDKey, id))
	}

	// A DRAFT-promoted-to-OPEN case is the first time the entity is
	// "real" — fire the created lifecycle event so Jobs that listen for
	// new cases run uniformly whether they came from CreateCase or
	// SubmitDraft.
	uc.publishLifecycle(ctx, workspaceID, activated, model.CaseLifecycleCreated)
	return activated, nil
}

// DiscardDraft permanently deletes a draft. Public drafts are team-wide
// shared so any workspace member may discard one; private drafts are
// hidden from non-reporters at the GetDraft layer, which naturally keeps
// them owner-only. Non-draft cases are rejected so callers cannot pivot
// this method into a "delete any case" shortcut.
func (uc *CaseUseCase) DiscardDraft(ctx context.Context, workspaceID string, id int64) error {
	c, err := uc.GetDraft(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if err := uc.repo.Case().Delete(ctx, workspaceID, c.ID); err != nil {
		return goerr.Wrap(err, "failed to discard draft", goerr.V(CaseIDKey, id))
	}
	return nil
}

// draftURL returns the web-UI URL for a specific draft, or an empty
// string when no baseURL has been configured. The URL format mirrors
// what the React app's router expects for the draft detail page.
func (uc *CaseUseCase) draftURL(workspaceID string, caseID int64) string {
	if uc.baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ws/%s/drafts/%d", uc.baseURL, workspaceID, caseID)
}

// CaseURL returns the web-UI URL for a specific case detail page, or an empty
// string when no baseURL has been configured.
func (uc *CaseUseCase) CaseURL(workspaceID string, caseID int64) string {
	if uc.baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ws/%s/cases/%d", uc.baseURL, workspaceID, caseID)
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
	existing.UpdatedAt = time.Now().UTC()
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

// GetCaseStatusSet returns the configurable Case status set (the Kanban
// columns) for a thread-mode workspace, or nil for channel-mode workspaces.
func (uc *CaseUseCase) GetCaseStatusSet(workspaceID string) *model.ActionStatusSet {
	return uc.caseStatusSetForWorkspace(workspaceID)
}
