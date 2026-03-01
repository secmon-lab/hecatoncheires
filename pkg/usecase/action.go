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
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	goslack "github.com/slack-go/slack"
)

// Slack interaction action IDs for Block Kit buttons
const (
	SlackActionIDAssign     = "hc_assign"
	SlackActionIDInProgress = "hc_in_progress"
	SlackActionIDComplete   = "hc_complete"
	slackActionBlockID      = "hc_action_buttons"
)

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

func (uc *ActionUseCase) CreateAction(ctx context.Context, workspaceID string, caseID int64, title, description string, assigneeIDs []string, slackMessageTS string, status types.ActionStatus, dueDate *time.Time) (*model.Action, error) {
	if title == "" {
		return nil, goerr.New("action title is required")
	}

	// Verify case exists and get case data for Slack notification
	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		return nil, goerr.Wrap(ErrCaseNotFound, "case not found", goerr.V(CaseIDKey, caseID))
	}

	// Access control for private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(caseModel, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot create action in private case",
			goerr.V(CaseIDKey, caseID), goerr.V("user_id", token.Sub))
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
		DueDate:        dueDate,
	}

	created, err := uc.repo.Action().Create(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action",
			goerr.V(CaseIDKey, caseID))
	}

	// Post Slack notification (best-effort: failure does not fail action creation)
	if uc.slackService != nil && caseModel.SlackChannelID != "" {
		actionURL := ""
		if uc.baseURL != "" {
			actionURL = fmt.Sprintf("%s/ws/%s/cases/%d/actions/%d", uc.baseURL, workspaceID, caseID, created.ID)
		}

		blocks := buildActionMessageBlocks(created, actionURL, workspaceID)
		fallbackText := fmt.Sprintf("New action: %s", created.Title)
		ts, postErr := uc.slackService.PostMessage(ctx, caseModel.SlackChannelID, blocks, fallbackText)
		if postErr != nil {
			errutil.Handle(ctx, postErr, "failed to post Slack notification for action")
		} else if ts != "" {
			// Store the message timestamp for future updates
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

func (uc *ActionUseCase) UpdateAction(ctx context.Context, workspaceID string, id int64, caseID *int64, title, description *string, assigneeIDs []string, slackMessageTS *string, status *types.ActionStatus, dueDate *time.Time, clearDueDate bool) (*model.Action, error) {
	// Get existing action
	existing, err := uc.repo.Action().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	// Access control: check parent case
	parentCase, err := uc.repo.Case().Get(ctx, workspaceID, existing.CaseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get parent case", goerr.V(CaseIDKey, existing.CaseID))
	}
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil && !model.IsCaseAccessible(parentCase, token.Sub) {
		return nil, goerr.Wrap(ErrAccessDenied, "cannot update action in private case",
			goerr.V(ActionIDKey, id), goerr.V("user_id", token.Sub))
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
		DueDate:        existing.DueDate,
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

	if clearDueDate {
		action.DueDate = nil
	} else if dueDate != nil {
		action.DueDate = dueDate
	}

	updated, err := uc.repo.Action().Update(ctx, workspaceID, action)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action", goerr.V(ActionIDKey, id))
	}

	// Update Slack message (best-effort)
	uc.updateSlackMessage(ctx, workspaceID, updated)

	return updated, nil
}

func (uc *ActionUseCase) DeleteAction(ctx context.Context, workspaceID string, id int64) error {
	// Get existing action for access control
	existing, err := uc.repo.Action().Get(ctx, workspaceID, id)
	if err != nil {
		return goerr.Wrap(ErrActionNotFound, "action not found", goerr.V(ActionIDKey, id))
	}

	// Access control: check parent case
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

	// Access control: filter out actions from inaccessible private cases
	token, tokenErr := auth.TokenFromContext(ctx)
	if tokenErr == nil {
		filtered := make([]*model.Action, 0, len(actions))
		for _, action := range actions {
			parentCase, caseErr := uc.repo.Case().Get(ctx, workspaceID, action.CaseID)
			if caseErr != nil {
				continue
			}
			if model.IsCaseAccessible(parentCase, token.Sub) {
				filtered = append(filtered, action)
			}
		}
		actions = filtered
	}

	return actions, nil
}

func (uc *ActionUseCase) GetActionsByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	// Access control: check parent case
	parentCase, err := uc.repo.Case().Get(ctx, workspaceID, caseID)
	if err != nil {
		// If case not found (deleted or doesn't exist), return empty list
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

// HandleSlackInteraction processes a button click from a Slack Block Kit message.
// It updates the action based on the actionType (assign, in_progress, complete)
// and updates the Slack message to reflect the new state.
func (uc *ActionUseCase) HandleSlackInteraction(ctx context.Context, workspaceID string, actionID int64, userID string, actionType string) error {
	existing, err := uc.repo.Action().Get(ctx, workspaceID, actionID)
	if err != nil {
		return goerr.Wrap(ErrActionNotFound, "action not found for interaction",
			goerr.V(ActionIDKey, actionID))
	}

	switch actionType {
	case SlackActionIDAssign:
		// Add user to assignees if not already present
		found := false
		for _, id := range existing.AssigneeIDs {
			if id == userID {
				found = true
				break
			}
		}
		if !found {
			existing.AssigneeIDs = append(existing.AssigneeIDs, userID)
		}

	case SlackActionIDInProgress:
		existing.Status = types.ActionStatusInProgress

	case SlackActionIDComplete:
		existing.Status = types.ActionStatusCompleted

	default:
		// Unknown action type, ignore
		return nil
	}

	updated, err := uc.repo.Action().Update(ctx, workspaceID, existing)
	if err != nil {
		return goerr.Wrap(err, "failed to update action from Slack interaction",
			goerr.V(ActionIDKey, actionID))
	}

	// Update the Slack message to reflect new state
	uc.updateSlackMessage(ctx, workspaceID, updated)

	return nil
}

// updateSlackMessage updates the Slack message for an action (best-effort).
func (uc *ActionUseCase) updateSlackMessage(ctx context.Context, workspaceID string, action *model.Action) {
	if uc.slackService == nil || action.SlackMessageTS == "" {
		return
	}

	caseModel, err := uc.repo.Case().Get(ctx, workspaceID, action.CaseID)
	if err != nil {
		errutil.Handle(ctx, err, "failed to get case for Slack message update")
		return
	}

	actionURL := ""
	if uc.baseURL != "" {
		actionURL = fmt.Sprintf("%s/ws/%s/cases/%d/actions/%d", uc.baseURL, workspaceID, action.CaseID, action.ID)
	}

	blocks := buildActionMessageBlocks(action, actionURL, workspaceID)
	fallbackText := fmt.Sprintf("Action updated: %s", action.Title)
	if updateErr := uc.slackService.UpdateMessage(ctx, caseModel.SlackChannelID, action.SlackMessageTS, blocks, fallbackText); updateErr != nil {
		errutil.Handle(ctx, updateErr, "failed to update Slack message for action")
	}
}

// buildActionMessageBlocks constructs Block Kit blocks for an action notification message.
func buildActionMessageBlocks(action *model.Action, actionURL string, workspaceID string) []goslack.Block {
	blocks := []goslack.Block{
		// Header: "Action: {emoji} {title}"
		goslack.NewHeaderBlock(
			goslack.NewTextBlockObject(goslack.PlainTextType, "Action: "+action.Status.Emoji()+" "+action.Title, true, false),
		),
	}

	// Description (if present)
	if action.Description != "" {
		blocks = append(blocks, goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType, action.Description, false, false),
			nil, nil,
		))
	}

	// Context: Assignees, status, and link
	contextParts := []string{}
	if len(action.AssigneeIDs) > 0 {
		mentions := make([]string, len(action.AssigneeIDs))
		for i, id := range action.AssigneeIDs {
			mentions[i] = fmt.Sprintf("<@%s>", id)
		}
		contextParts = append(contextParts, strings.Join(mentions, " "))
	} else {
		contextParts = append(contextParts, "No Assign")
	}
	contextParts = append(contextParts, fmt.Sprintf("Status: %s", action.Status))
	if actionURL != "" {
		contextParts = append(contextParts, fmt.Sprintf(":link: <%s|Link>", actionURL))
	}
	contextText := strings.Join(contextParts, "  |  ")

	blocks = append(blocks, goslack.NewContextBlock("",
		goslack.NewTextBlockObject(goslack.MarkdownType, contextText, false, false),
	))

	// Action buttons (conditionally shown based on current state)
	buttonValue := fmt.Sprintf("%s:%d", workspaceID, action.ID)

	var buttons []goslack.BlockElement
	if len(action.AssigneeIDs) == 0 {
		buttons = append(buttons, goslack.NewButtonBlockElement(SlackActionIDAssign, buttonValue,
			goslack.NewTextBlockObject(goslack.PlainTextType, "Assign to me", true, false),
		))
	}
	if action.Status != types.ActionStatusInProgress {
		btn := goslack.NewButtonBlockElement(SlackActionIDInProgress, buttonValue,
			goslack.NewTextBlockObject(goslack.PlainTextType, "In Progress", true, false),
		)
		btn.Style = goslack.StylePrimary
		buttons = append(buttons, btn)
	}
	if action.Status != types.ActionStatusCompleted {
		btn := goslack.NewButtonBlockElement(SlackActionIDComplete, buttonValue,
			goslack.NewTextBlockObject(goslack.PlainTextType, "Completed", true, false),
		)
		btn.Style = goslack.StyleDanger
		buttons = append(buttons, btn)
	}

	if len(buttons) > 0 {
		actionBlock := goslack.NewActionBlock(slackActionBlockID, buttons...)
		blocks = append(blocks, actionBlock)
	}

	return blocks
}

// ParseSlackActionValue parses a button value string "workspaceID:actionID" into its components.
func ParseSlackActionValue(value string) (workspaceID string, actionID int64, err error) {
	// Find the last colon to split (workspaceID may contain colons, but actionID is always a number)
	lastColon := strings.LastIndex(value, ":")
	if lastColon < 0 {
		return "", 0, goerr.New("invalid action value format", goerr.V("value", value))
	}

	workspaceID = value[:lastColon]
	var id int64
	if _, parseErr := fmt.Sscanf(value[lastColon+1:], "%d", &id); parseErr != nil {
		return "", 0, goerr.Wrap(parseErr, "failed to parse action ID from value", goerr.V("value", value))
	}

	return workspaceID, id, nil
}
