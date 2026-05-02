package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	goslack "github.com/slack-go/slack"
)

// Slack interaction action / block IDs for Action Block Kit messages.
const (
	SlackActionIDStatusSelect   = "hc_action_status"
	SlackActionIDAssigneeSelect = "hc_action_assignee"
	// slackActionAssigneeBlockIDPrefix is followed by ":{workspaceID}:{actionID}".
	// The status_select and users_select share a single actions block whose
	// block_id encodes (workspaceID, actionID), since users_select carries no
	// `value` for the handler to recover them from.
	slackActionAssigneeBlockIDPrefix = "hc_action_assignee_block"
)

// SlackSyncMode controls how UpdateAction interacts with Slack.
type SlackSyncMode int

const (
	// SlackSyncFull updates the existing Slack message and posts a thread
	// notification for visible field changes (default).
	SlackSyncFull SlackSyncMode = iota
	// SlackSyncMessageOnly only refreshes the existing Slack message; no
	// thread notification is posted.
	SlackSyncMessageOnly
	// SlackSyncSkip leaves Slack untouched.
	SlackSyncSkip
)

// ActorKind describes who triggered an UpdateAction call.
type ActorKind int

const (
	ActorKindSystem ActorKind = iota
	ActorKindSlackUser
)

// ActorRef identifies the actor that triggered an update, for change-notification rendering.
type ActorRef struct {
	Kind ActorKind
	ID   string // Slack user ID when Kind == ActorKindSlackUser
}

// UpdateActionInput is the unified input for ActionUseCase.UpdateAction.
type UpdateActionInput struct {
	ID             int64
	CaseID         *int64
	Title          *string
	Description    *string
	AssigneeID     *string // nil = no change; "" is not a valid clear, use ClearAssignee.
	Status         *types.ActionStatus
	DueDate        *time.Time
	ClearDueDate   bool
	ClearAssignee  bool
	SlackMessageTS *string

	SlackSync SlackSyncMode
	Actor     ActorRef
}

type ActionUseCase struct {
	repo         interfaces.Repository
	slackService slack.Service
	baseURL      string
}

func NewActionUseCase(repo interfaces.Repository, slackService slack.Service, baseURL string) *ActionUseCase {
	return &ActionUseCase{
		repo:         repo,
		slackService: slackService,
		baseURL:      baseURL,
	}
}

func (uc *ActionUseCase) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeID string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error) {
	if title == "" {
		return nil, goerr.New("action title is required")
	}

	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, caseID))
	}

	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(caseModel, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot create action in private case",
			goerr.V(CaseIDKey, caseID), goerr.V("user_id", token.Sub))
	}

	if status == "" {
		status = types.ActionStatusTodo
	}
	if !status.IsValid() {
		return nil, goerr.New("invalid action status", goerr.V("status", status))
	}

	action := &model.Action{
		CaseID:         caseID,
		Title:          title,
		Description:    description,
		AssigneeID:     assigneeID,
		SlackMessageTS: slackMessageTS,
		Status:         status,
		DueDate:        dueDate,
	}

	created, err := uc.repo.Action().Create(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action",
			goerr.V(CaseIDKey, caseID))
	}

	if uc.slackService != nil && caseModel.SlackChannelID != "" {
		actionURL := uc.actionWebURL(workspaceID, caseID, created.ID)
		blocks := uc.buildActionMessageBlocks(ctx, workspaceID, created, actionURL)
		fallbackText := i18n.T(ctx, i18n.MsgActionNew, created.Title)
		ts, postErr := uc.slackService.PostMessage(ctx, caseModel.SlackChannelID, blocks, fallbackText)
		if postErr != nil {
			errutil.Handle(ctx, postErr, "failed to post Slack notification for action")
		} else if ts != "" {
			created.SlackMessageTS = ts
			updated, updateErr := uc.repo.Action().Update(ctx, workspaceID, created)
			if updateErr != nil {
				errutil.Handle(ctx, updateErr, "failed to update action with Slack message timestamp")
			} else {
				created = updated
			}
		}
	}

	return created, nil
}

// UpdateAction is the single entry point for mutating an Action. All transports
// (GraphQL/WebUI, Slack interactivity, internal callers) funnel through this
// method; Slack side-effects are controlled by in.SlackSync and in.Actor.
func (uc *ActionUseCase) UpdateAction(ctx context.Context, workspaceID string, in UpdateActionInput) (*model.Action, error) {
	existing, err := uc.repo.Action().Get(ctx, workspaceID, in.ID)
	if err != nil {
		return nil, goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, in.ID))
	}

	parentCase, err := uc.repo.Case().Get(ctx, workspaceID, existing.CaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get parent case", goerr.V(CaseIDKey, existing.CaseID))
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(parentCase, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot update action in private case",
			goerr.V(ActionIDKey, in.ID), goerr.V("user_id", token.Sub))
	}

	if in.CaseID != nil && *in.CaseID != existing.CaseID {
		if _, err := uc.repo.Case().Get(ctx, workspaceID, *in.CaseID); err != nil {
			return nil, goerr.Wrap(ErrCaseNotFound, "new case not found",
				goerr.V(CaseIDKey, *in.CaseID),
				goerr.V(ActionIDKey, in.ID))
		}
	}

	action := &model.Action{
		ID:             existing.ID,
		CaseID:         existing.CaseID,
		Title:          existing.Title,
		Description:    existing.Description,
		AssigneeID:     existing.AssigneeID,
		SlackMessageTS: existing.SlackMessageTS,
		Status:         existing.Status,
		DueDate:        existing.DueDate,
		CreatedAt:      existing.CreatedAt,
	}

	if in.CaseID != nil {
		action.CaseID = *in.CaseID
	}
	if in.Title != nil {
		if *in.Title == "" {
			return nil, goerr.New("action title cannot be empty", goerr.V(ActionIDKey, in.ID))
		}
		action.Title = *in.Title
	}
	if in.Description != nil {
		action.Description = *in.Description
	}
	if in.ClearAssignee {
		action.AssigneeID = ""
	} else if in.AssigneeID != nil {
		action.AssigneeID = *in.AssigneeID
	}
	if in.SlackMessageTS != nil {
		action.SlackMessageTS = *in.SlackMessageTS
	}
	if in.Status != nil {
		if !in.Status.IsValid() {
			return nil, goerr.New("invalid action status",
				goerr.V("status", *in.Status),
				goerr.V(ActionIDKey, in.ID))
		}
		action.Status = *in.Status
	}
	if in.ClearDueDate {
		action.DueDate = nil
	} else if in.DueDate != nil {
		action.DueDate = in.DueDate
	}

	updated, err := uc.repo.Action().Update(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action", goerr.V(ActionIDKey, in.ID))
	}

	switch in.SlackSync {
	case SlackSyncSkip:
		// no Slack side effects
	case SlackSyncMessageOnly:
		uc.refreshSlackMessage(ctx, workspaceID, updated)
	case SlackSyncFull:
		uc.refreshSlackMessage(ctx, workspaceID, updated)
		uc.postActionChangeNotification(ctx, workspaceID, existing, updated, in.Actor)
	}

	return updated, nil
}

func (uc *ActionUseCase) DeleteAction(ctx context.Context, workspaceID string, id int64) error {
	existing, err := uc.repo.Action().Get(ctx, workspaceID, id)
	if err != nil {
		return goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	parentCase, err := uc.repo.Case().Get(ctx, workspaceID, existing.CaseID)
	if err != nil {
		return goerr.Wrap(err, "failed to get parent case", goerr.V(CaseIDKey, existing.CaseID))
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(parentCase, token.Sub) {
		return goerr.Wrap(ErrAccessDenied, "cannot delete action in private case",
			goerr.V(ActionIDKey, id), goerr.V("user_id", token.Sub))
	}

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

	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		caseIDSet := make(map[int64]struct{})
		for _, action := range actions {
			caseIDSet[action.CaseID] = struct{}{}
		}
		accessibleCases := make(map[int64]bool, len(caseIDSet))
		for caseID := range caseIDSet {
			parentCase, caseErr := uc.repo.Case().Get(ctx, workspaceID, caseID)
			if caseErr != nil {
				continue
			}
			accessibleCases[caseID] = model.IsCaseAccessible(parentCase, token.Sub)
		}
		filtered := make([]*model.Action, 0, len(actions))
		for _, action := range actions {
			if accessibleCases[action.CaseID] {
				filtered = append(filtered, action)
			}
		}
		actions = filtered
	}
	return actions, nil
}

func (uc *ActionUseCase) GetActionsByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	parentCase, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return []*model.Action{}, nil
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(parentCase, token.Sub) {
		return []*model.Action{}, nil
	}

	actions, err := uc.repo.Action().GetByCase(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get actions by case", goerr.V(CaseIDKey, caseID))
	}
	return actions, nil
}

// refreshSlackMessage rebuilds and updates the Action's primary Slack message.
// It is best-effort: failures are logged but do not abort the caller.
func (uc *ActionUseCase) refreshSlackMessage(ctx context.Context, workspaceID string, action *model.Action) {
	if uc.slackService == nil || action.SlackMessageTS == "" {
		return
	}

	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, action.CaseID)
	if err != nil {
		errutil.Handle(ctx, err, "failed to get case for Slack message update")
		return
	}
	if caseModel.SlackChannelID == "" {
		return
	}

	actionURL := uc.actionWebURL(workspaceID, action.CaseID, action.ID)
	blocks := uc.buildActionMessageBlocks(ctx, workspaceID, action, actionURL)
	fallbackText := i18n.T(ctx, i18n.MsgActionUpdated, action.Title)
	if updateErr := uc.slackService.UpdateMessage(ctx, caseModel.SlackChannelID, action.SlackMessageTS, blocks, fallbackText); updateErr != nil {
		errutil.Handle(ctx, updateErr, "failed to update Slack message for action")
	}
}

// postActionChangeNotification posts a context-block thread reply summarising
// changes to title / status / assignee. Best-effort.
func (uc *ActionUseCase) postActionChangeNotification(ctx context.Context, workspaceID string, before, after *model.Action, actor ActorRef) {
	if uc.slackService == nil || after.SlackMessageTS == "" {
		return
	}

	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, after.CaseID)
	if err != nil {
		errutil.Handle(ctx, err, "failed to get case for Slack change notification")
		return
	}
	if caseModel.SlackChannelID == "" {
		return
	}

	actorMention := renderActor(ctx, actor)

	var lines []string
	if before.Title != after.Title {
		lines = append(lines, i18n.T(ctx, i18n.MsgActionChangeTitle, actorMention, before.Title, after.Title))
	}
	if before.Status != after.Status {
		lines = append(lines, i18n.T(ctx, i18n.MsgActionChangeStatus, actorMention, before.Status.String(), after.Status.String()))
	}
	if before.AssigneeID != after.AssigneeID {
		switch {
		case before.AssigneeID == "" && after.AssigneeID != "":
			lines = append(lines, i18n.T(ctx, i18n.MsgActionChangeAssigneeAssigned, actorMention, mentionUser(after.AssigneeID)))
		case before.AssigneeID != "" && after.AssigneeID == "":
			lines = append(lines, i18n.T(ctx, i18n.MsgActionChangeAssigneeUnassigned, actorMention, mentionUser(before.AssigneeID)))
		default:
			lines = append(lines, i18n.T(ctx, i18n.MsgActionChangeAssigneeReplaced, actorMention, mentionUser(before.AssigneeID), mentionUser(after.AssigneeID)))
		}
	}
	if len(lines) == 0 {
		return
	}

	body := strings.Join(lines, "\n")
	blocks := []goslack.Block{
		goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, body, false, false),
		),
	}
	if _, postErr := uc.slackService.PostThreadMessage(ctx, caseModel.SlackChannelID, after.SlackMessageTS, blocks, body); postErr != nil {
		errutil.Handle(ctx, postErr, "failed to post action change notification")
	}
}

func renderActor(ctx context.Context, actor ActorRef) string {
	if actor.Kind == ActorKindSlackUser && actor.ID != "" {
		return mentionUser(actor.ID)
	}
	return i18n.T(ctx, i18n.MsgActionChangeActorSystem)
}

func mentionUser(slackUserID string) string {
	return fmt.Sprintf("<@%s>", slackUserID)
}

func (uc *ActionUseCase) actionWebURL(workspaceID string, caseID, actionID int64) string {
	if uc.baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ws/%s/cases/%d/actions/%d", uc.baseURL, workspaceID, caseID, actionID)
}

// buildActionMessageBlocks constructs the Block Kit blocks for the action's
// primary Slack message. Layout:
//   - section: bold title that links to the WebUI (or plain title when no URL),
//     so the user can jump to the action from the title itself.
//   - section: optional description.
//   - actions: status_select and assignee static_select side-by-side. Both
//     elements carry value="{workspaceID}:{actionID}:{payload}" so the
//     handler can recover identity from the callback. The assignee dropdown
//     uses static_select instead of users_select so we can omit bots from
//     the candidate list (Slack's users_select has no built-in bot filter).
func (uc *ActionUseCase) buildActionMessageBlocks(ctx context.Context, workspaceID string, action *model.Action, actionURL string) []goslack.Block {
	titleText := fmt.Sprintf("%s *%s*", action.Status.Emoji(), action.Title)
	if actionURL != "" {
		titleText = fmt.Sprintf("%s *<%s|%s>*", action.Status.Emoji(), actionURL, action.Title)
	}
	blocks := []goslack.Block{
		goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, titleText, false, false),
			nil, nil,
		),
	}

	if action.Description != "" {
		blocks = append(blocks, goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, action.Description, false, false),
			nil, nil,
		))
	}

	statusSelect := buildStatusSelect(ctx, workspaceID, action)
	assigneeSelect := uc.buildAssigneeSelect(ctx, workspaceID, action)
	// One actions block carries both selects so they render side-by-side.
	blocks = append(blocks, goslack.NewActionBlock(
		SlackActionAssigneeBlockID(workspaceID, action.ID),
		statusSelect,
		assigneeSelect,
	))

	return blocks
}

func buildStatusSelect(ctx context.Context, workspaceID string, action *model.Action) *goslack.SelectBlockElement {
	options := make([]*goslack.OptionBlockObject, 0, len(types.AllActionStatuses()))
	var initial *goslack.OptionBlockObject
	for _, s := range types.AllActionStatuses() {
		label := goslack.NewTextBlockObject(goslack.PlainTextType, statusLabel(ctx, s), true, false)
		opt := goslack.NewOptionBlockObject(
			fmt.Sprintf("%s:%d:%s", workspaceID, action.ID, s),
			label,
			nil,
		)
		options = append(options, opt)
		if s == action.Status {
			initial = opt
		}
	}
	placeholder := goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgActionStatusPlaceholder), true, false)
	sel := goslack.NewOptionsSelectBlockElement(goslack.OptTypeStatic, placeholder, SlackActionIDStatusSelect, options...)
	if initial != nil {
		sel.InitialOption = initial
	}
	return sel
}

// buildAssigneeSelect renders a static_select whose options are non-bot
// members of the case channel (plus an "Unassigned" sentinel). We use a
// static_select rather than users_select because Slack's users_select has
// no way to exclude bots / apps from the candidate list.
func (uc *ActionUseCase) buildAssigneeSelect(ctx context.Context, workspaceID string, action *model.Action) *goslack.SelectBlockElement {
	placeholder := goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgActionAssigneePlaceholder), true, false)

	// Sentinel for clearing the assignee.
	unassignedLabel := goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgActionNoAssign), true, false)
	unassignedOpt := goslack.NewOptionBlockObject(
		fmt.Sprintf("%s:%d:", workspaceID, action.ID),
		unassignedLabel, nil,
	)
	options := []*goslack.OptionBlockObject{unassignedOpt}

	candidates := uc.assigneeCandidates(ctx, workspaceID, action)
	// Slack caps static_select options at 100; cap defensively.
	const maxOptions = 99
	if len(candidates) > maxOptions {
		candidates = candidates[:maxOptions]
	}

	var initial *goslack.OptionBlockObject
	if action.AssigneeID == "" {
		initial = unassignedOpt
	}
	for _, u := range candidates {
		label := assigneeOptionLabel(u)
		text := goslack.NewTextBlockObject(goslack.PlainTextType, label, true, false)
		opt := goslack.NewOptionBlockObject(
			fmt.Sprintf("%s:%d:%s", workspaceID, action.ID, u.ID),
			text, nil,
		)
		options = append(options, opt)
		if string(u.ID) == action.AssigneeID {
			initial = opt
		}
	}

	sel := goslack.NewOptionsSelectBlockElement(goslack.OptTypeStatic, placeholder, SlackActionIDAssigneeSelect, options...)
	if initial != nil {
		sel.InitialOption = initial
	}
	return sel
}

// assigneeCandidates returns non-bot users that may be assigned to the
// action: the case channel members that exist in the SlackUser DB (which
// itself excludes bots during sync). On any lookup error we fall back to
// the empty list — the dropdown still gets the Unassigned sentinel.
func (uc *ActionUseCase) assigneeCandidates(ctx context.Context, workspaceID string, action *model.Action) []*model.SlackUser {
	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, action.CaseID)
	if err != nil || caseModel == nil {
		return nil
	}
	if len(caseModel.ChannelUserIDs) == 0 {
		return nil
	}
	ids := make([]model.SlackUserID, 0, len(caseModel.ChannelUserIDs))
	for _, id := range caseModel.ChannelUserIDs {
		ids = append(ids, model.SlackUserID(id))
	}
	userMap, err := uc.repo.SlackUser().GetByIDs(ctx, ids)
	if err != nil {
		errutil.Handle(ctx, err, "failed to load slack users for assignee select")
		return nil
	}
	// Iterate ChannelUserIDs to keep a stable, channel-membership-ordered list.
	users := make([]*model.SlackUser, 0, len(userMap))
	for _, id := range caseModel.ChannelUserIDs {
		if u, ok := userMap[model.SlackUserID(id)]; ok && u != nil {
			users = append(users, u)
		}
	}
	// If the current assignee is not a current channel member, surface them
	// at the top so the dropdown can render the existing selection.
	if action.AssigneeID != "" {
		found := false
		for _, u := range users {
			if string(u.ID) == action.AssigneeID {
				found = true
				break
			}
		}
		if !found {
			if u, ok := userMap[model.SlackUserID(action.AssigneeID)]; ok && u != nil {
				users = append([]*model.SlackUser{u}, users...)
			}
		}
	}
	return users
}

func assigneeOptionLabel(u *model.SlackUser) string {
	if u.RealName != "" && u.Name != "" && u.RealName != u.Name {
		return fmt.Sprintf("%s (%s)", u.RealName, u.Name)
	}
	if u.RealName != "" {
		return u.RealName
	}
	return u.Name
}

// SlackActionAssigneeBlockID returns the block_id that wraps the users_select
// element. We encode (workspaceID, actionID) into the block_id because
// users_select callbacks carry no `value` field.
func SlackActionAssigneeBlockID(workspaceID string, actionID int64) string {
	return fmt.Sprintf("%s:%s:%d", slackActionAssigneeBlockIDPrefix, workspaceID, actionID)
}

// ParseSlackAssigneeSelectValue parses a static_select option value of the
// form "{workspaceID}:{actionID}:{slackUserID}" used by the assignee
// dropdown. An empty trailing segment means "Unassigned".
func ParseSlackAssigneeSelectValue(value string) (workspaceID string, actionID int64, slackUserID string, err error) {
	lastColon := strings.LastIndex(value, ":")
	if lastColon < 0 {
		return "", 0, "", goerr.New("invalid assignee_select value: missing user_id separator", goerr.V("value", value))
	}
	slackUserID = value[lastColon+1:]
	rest := value[:lastColon]

	mid := strings.LastIndex(rest, ":")
	if mid < 0 {
		return "", 0, "", goerr.New("invalid assignee_select value: missing action_id separator", goerr.V("value", value))
	}
	idPart := rest[mid+1:]
	wsPart := rest[:mid]

	var id int64
	if _, parseErr := fmt.Sscanf(idPart, "%d", &id); parseErr != nil {
		return "", 0, "", goerr.Wrap(parseErr, "failed to parse action_id", goerr.V("value", value))
	}
	return wsPart, id, slackUserID, nil
}

// ParseSlackAssigneeBlockID parses a block_id of the form
// "{prefix}:{workspaceID}:{actionID}" into its components.
func ParseSlackAssigneeBlockID(blockID string) (workspaceID string, actionID int64, err error) {
	prefix := slackActionAssigneeBlockIDPrefix + ":"
	if !strings.HasPrefix(blockID, prefix) {
		return "", 0, goerr.New("block_id missing assignee prefix", goerr.V("block_id", blockID))
	}
	rest := strings.TrimPrefix(blockID, prefix)
	lastColon := strings.LastIndex(rest, ":")
	if lastColon < 0 {
		return "", 0, goerr.New("invalid assignee block_id", goerr.V("block_id", blockID))
	}
	workspaceID = rest[:lastColon]
	idPart := rest[lastColon+1:]
	if _, parseErr := fmt.Sscanf(idPart, "%d", &actionID); parseErr != nil {
		return "", 0, goerr.Wrap(parseErr, "failed to parse action_id from block_id", goerr.V("block_id", blockID))
	}
	return workspaceID, actionID, nil
}

func statusLabel(ctx context.Context, s types.ActionStatus) string {
	switch s {
	case types.ActionStatusBacklog:
		return i18n.T(ctx, i18n.MsgActionStatusBacklog)
	case types.ActionStatusTodo:
		return i18n.T(ctx, i18n.MsgActionStatusTodo)
	case types.ActionStatusInProgress:
		return i18n.T(ctx, i18n.MsgActionStatusInProgressLabel)
	case types.ActionStatusBlocked:
		return i18n.T(ctx, i18n.MsgActionStatusBlocked)
	case types.ActionStatusCompleted:
		return i18n.T(ctx, i18n.MsgActionStatusCompletedLabel)
	}
	return s.String()
}

// ParseSlackStatusSelectValue parses a status_select option value of the form
// "{workspaceID}:{actionID}:{status}" and returns its components.
func ParseSlackStatusSelectValue(value string) (workspaceID string, actionID int64, status types.ActionStatus, err error) {
	// status is ALL_CAPS_WITH_UNDERSCORES; split from the right.
	lastColon := strings.LastIndex(value, ":")
	if lastColon < 0 {
		return "", 0, "", goerr.New("invalid status_select value: missing status separator", goerr.V("value", value))
	}
	statusPart := value[lastColon+1:]
	rest := value[:lastColon]

	mid := strings.LastIndex(rest, ":")
	if mid < 0 {
		return "", 0, "", goerr.New("invalid status_select value: missing action_id separator", goerr.V("value", value))
	}
	idPart := rest[mid+1:]
	wsPart := rest[:mid]

	var id int64
	if _, parseErr := fmt.Sscanf(idPart, "%d", &id); parseErr != nil {
		return "", 0, "", goerr.Wrap(parseErr, "failed to parse action_id", goerr.V("value", value))
	}
	parsed, parseErr := types.ParseActionStatus(statusPart)
	if parseErr != nil {
		return "", 0, "", goerr.Wrap(parseErr, "failed to parse status", goerr.V("value", value))
	}
	return wsPart, id, parsed, nil
}
