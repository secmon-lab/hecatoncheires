package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/slack-go/slack"
)

// Callback IDs for slash command modals
const (
	SlackCallbackIDSelectWorkspace = "hc_select_workspace"
	SlackCallbackIDCreateCase      = "hc_create_case"

	slackBlockIDWorkspaceSelect  = "hc_ws_select_block"
	slackActionIDWorkspaceRadio  = "hc_ws_radio"
	slackBlockIDCaseTitle        = "hc_case_title_block"
	slackActionIDCaseTitle       = "hc_case_title"
	slackBlockIDCaseDescription  = "hc_case_desc_block"
	slackActionIDCaseDescription = "hc_case_desc"

	// Prefix for custom field block/action IDs
	slackFieldBlockPrefix  = "hc_field_block_"
	slackFieldActionPrefix = "hc_field_action_"
)

// commandMetadata is stored in modal private_metadata as JSON
type commandMetadata struct {
	WorkspaceID string `json:"workspace_id"`
	ChannelID   string `json:"channel_id"`
}

// HandleSlashCommand handles a Slack slash command to create a case.
// If workspaceID is provided (from URL path), it opens the case creation modal directly.
// If workspaceID is empty, it shows a workspace selection modal (or skips if only one workspace).
func (uc *SlackUseCases) HandleSlashCommand(ctx context.Context, triggerID, userID, channelID, workspaceID string) error {
	if uc.slackService == nil {
		return goerr.New("slack service is not available")
	}
	if uc.registry == nil {
		return goerr.New("workspace registry is not available")
	}

	// If workspace ID is specified, validate and open case creation modal directly
	if workspaceID != "" {
		entry, err := uc.registry.Get(workspaceID)
		if err != nil {
			return goerr.Wrap(err, "invalid workspace ID",
				goerr.V("workspace_id", workspaceID))
		}
		return uc.openCaseCreationModal(ctx, triggerID, workspaceID, channelID, entry.FieldSchema)
	}

	// No workspace specified; decide based on workspace count
	workspaces := uc.registry.Workspaces()
	switch len(workspaces) {
	case 0:
		return goerr.New("no workspaces configured")
	case 1:
		entry, _ := uc.registry.Get(workspaces[0].ID)
		return uc.openCaseCreationModal(ctx, triggerID, workspaces[0].ID, channelID, entry.FieldSchema)
	default:
		return uc.openWorkspaceSelectModal(ctx, triggerID, channelID, workspaces)
	}
}

// HandleWorkspaceSelectSubmit processes the workspace selection modal submission.
// It returns the case creation modal view to replace the current modal via response_action: update.
func (uc *SlackUseCases) HandleWorkspaceSelectSubmit(callback *slack.InteractionCallback) (*slack.ModalViewRequest, error) {
	// Extract selected workspace from radio buttons
	blockValues := callback.View.State.Values
	radioBlock, ok := blockValues[slackBlockIDWorkspaceSelect]
	if !ok {
		return nil, goerr.New("workspace selection block not found")
	}
	radioAction, ok := radioBlock[slackActionIDWorkspaceRadio]
	if !ok {
		return nil, goerr.New("workspace radio action not found")
	}
	if radioAction.SelectedOption.Value == "" {
		return nil, goerr.New("no workspace selected")
	}

	workspaceID := radioAction.SelectedOption.Value

	// Extract channel_id from private_metadata
	var meta commandMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return nil, goerr.Wrap(err, "failed to parse private_metadata")
	}

	// Get field schema for the selected workspace
	var schema *config.FieldSchema
	if uc.registry != nil {
		if entry, err := uc.registry.Get(workspaceID); err == nil {
			schema = entry.FieldSchema
		}
	}

	view := buildCaseCreationModal(workspaceID, meta.ChannelID, schema)
	return &view, nil
}

// HandleCaseCreationSubmit processes the case creation modal submission.
// It creates a case using CaseUseCase and posts a confirmation message.
func (uc *SlackUseCases) HandleCaseCreationSubmit(ctx context.Context, caseUC *CaseUseCase, callback *slack.InteractionCallback) error {
	// Extract fields from view state
	blockValues := callback.View.State.Values

	title := ""
	if titleBlock, ok := blockValues[slackBlockIDCaseTitle]; ok {
		if titleAction, ok := titleBlock[slackActionIDCaseTitle]; ok {
			title = titleAction.Value
		}
	}

	description := ""
	if descBlock, ok := blockValues[slackBlockIDCaseDescription]; ok {
		if descAction, ok := descBlock[slackActionIDCaseDescription]; ok {
			description = descAction.Value
		}
	}

	// Extract metadata
	var meta commandMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return goerr.Wrap(err, "failed to parse private_metadata")
	}

	// Extract custom field values from the view state
	fieldValues := extractFieldValues(blockValues)

	userID := callback.User.ID

	// Create case using existing CaseUseCase
	created, err := caseUC.CreateCase(ctx, meta.WorkspaceID, title, description, []string{userID}, fieldValues, false)
	if err != nil {
		return goerr.Wrap(err, "failed to create case via slash command",
			goerr.V("workspace_id", meta.WorkspaceID),
			goerr.V("user_id", userID))
	}

	// Post confirmation message to the channel where the command was invoked
	if meta.ChannelID != "" && uc.slackService != nil {
		confirmText := fmt.Sprintf("Case #%d *%s* has been created.", created.ID, created.Title)
		if created.SlackChannelID != "" {
			confirmText += fmt.Sprintf(" Channel: <#%s>", created.SlackChannelID)
		}

		if _, err := uc.slackService.PostMessage(ctx, meta.ChannelID, nil, confirmText); err != nil {
			// Log but don't fail; the case was already created
			return goerr.Wrap(err, "failed to post confirmation message",
				goerr.V("channel_id", meta.ChannelID),
				goerr.V("case_id", created.ID))
		}
	}

	return nil
}

// openCaseCreationModal opens the case creation modal directly
func (uc *SlackUseCases) openCaseCreationModal(ctx context.Context, triggerID, workspaceID, channelID string, schema *config.FieldSchema) error {
	view := buildCaseCreationModal(workspaceID, channelID, schema)
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open case creation modal",
			goerr.V("workspace_id", workspaceID))
	}
	return nil
}

// openWorkspaceSelectModal opens the workspace selection modal
func (uc *SlackUseCases) openWorkspaceSelectModal(ctx context.Context, triggerID, channelID string, workspaces []model.Workspace) error {
	view := buildWorkspaceSelectModal(channelID, workspaces)
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open workspace select modal")
	}
	return nil
}

// buildCaseCreationModal constructs the Block Kit modal for case creation
func buildCaseCreationModal(workspaceID, channelID string, schema *config.FieldSchema) slack.ModalViewRequest {
	meta := commandMetadata{
		WorkspaceID: workspaceID,
		ChannelID:   channelID,
	}
	metaJSON, _ := json.Marshal(meta) //nolint:errcheck

	titleInput := slack.NewInputBlock(
		slackBlockIDCaseTitle,
		slack.NewTextBlockObject(slack.PlainTextType, "Title", false, false),
		nil,
		slack.NewPlainTextInputBlockElement(
			slack.NewTextBlockObject(slack.PlainTextType, "Enter case title", false, false),
			slackActionIDCaseTitle,
		),
	)

	descInput := slack.NewInputBlock(
		slackBlockIDCaseDescription,
		slack.NewTextBlockObject(slack.PlainTextType, "Description", false, false),
		nil,
		&slack.PlainTextInputBlockElement{
			Type:        slack.METPlainTextInput,
			ActionID:    slackActionIDCaseDescription,
			Multiline:   true,
			Placeholder: slack.NewTextBlockObject(slack.PlainTextType, "Enter case description (optional)", false, false),
		},
	)
	descInput.Optional = true

	blocks := []slack.Block{
		titleInput,
		descInput,
	}

	// Add custom field inputs from workspace schema
	if schema != nil {
		for _, field := range schema.Fields {
			if block := buildFieldInputBlock(field); block != nil {
				blocks = append(blocks, block)
			}
		}
	}

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      SlackCallbackIDCreateCase,
		Title:           slack.NewTextBlockObject(slack.PlainTextType, "Create Case", false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, "Create", false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, "Cancel", false, false),
		PrivateMetadata: string(metaJSON),
		Blocks: slack.Blocks{
			BlockSet: blocks,
		},
	}
}

// buildFieldInputBlock creates a Slack input block for a custom field definition
func buildFieldInputBlock(field config.FieldDefinition) slack.Block {
	blockID := slackFieldBlockPrefix + field.ID
	actionID := slackFieldActionPrefix + field.ID

	label := slack.NewTextBlockObject(slack.PlainTextType, field.Name, false, false)

	var inputBlock *slack.InputBlock

	switch field.Type {
	case types.FieldTypeText:
		element := slack.NewPlainTextInputBlockElement(nil, actionID)
		if field.Description != "" {
			element.Placeholder = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeNumber:
		element := &slack.NumberInputBlockElement{
			Type:     slack.METNumber,
			ActionID: actionID,
			IsDecimalAllowed: true,
		}
		if field.Description != "" {
			element.Placeholder = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeSelect:
		options := buildFieldOptions(field.Options)
		if len(options) == 0 {
			return nil
		}
		element := slack.NewOptionsSelectBlockElement(
			slack.OptTypeStatic,
			nil,
			actionID,
			options...,
		)
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeMultiSelect:
		options := buildFieldOptions(field.Options)
		if len(options) == 0 {
			return nil
		}
		element := slack.NewOptionsMultiSelectBlockElement(
			slack.MultiOptTypeStatic,
			nil,
			actionID,
			options...,
		)
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeUser:
		element := slack.NewOptionsSelectBlockElement(
			slack.OptTypeUser,
			nil,
			actionID,
		)
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeMultiUser:
		element := slack.NewOptionsMultiSelectBlockElement(
			slack.MultiOptTypeUser,
			nil,
			actionID,
		)
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeDate:
		element := slack.NewDatePickerBlockElement(actionID)
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeURL:
		element := &slack.URLTextInputBlockElement{
			Type:     slack.METURLTextInput,
			ActionID: actionID,
		}
		if field.Description != "" {
			element.Placeholder = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	default:
		return nil
	}

	if inputBlock != nil && !field.Required {
		inputBlock.Optional = true
	}

	if inputBlock != nil && field.Description != "" {
		inputBlock.Hint = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
	}

	return inputBlock
}

// buildFieldOptions converts config field options to Slack option block objects
func buildFieldOptions(options []config.FieldOption) []*slack.OptionBlockObject {
	result := make([]*slack.OptionBlockObject, 0, len(options))
	for _, opt := range options {
		desc := opt.Description
		var descObj *slack.TextBlockObject
		if desc != "" {
			descObj = slack.NewTextBlockObject(slack.PlainTextType, desc, false, false)
		}
		result = append(result, slack.NewOptionBlockObject(
			opt.ID,
			slack.NewTextBlockObject(slack.PlainTextType, opt.Name, false, false),
			descObj,
		))
	}
	return result
}

// extractFieldValues extracts custom field values from the Slack view state
func extractFieldValues(blockValues map[string]map[string]slack.BlockAction) map[string]model.FieldValue {
	fieldValues := make(map[string]model.FieldValue)

	for blockID, actions := range blockValues {
		if !strings.HasPrefix(blockID, slackFieldBlockPrefix) {
			continue
		}
		fieldID := strings.TrimPrefix(blockID, slackFieldBlockPrefix)
		actionID := slackFieldActionPrefix + fieldID

		action, ok := actions[actionID]
		if !ok {
			continue
		}

		var value any
		hasValue := false

		switch action.Type {
		case "plain_text_input":
			if action.Value != "" {
				value = action.Value
				hasValue = true
			}
		case "number_input":
			if action.Value != "" {
				value = action.Value
				hasValue = true
			}
		case "static_select":
			if action.SelectedOption.Value != "" {
				value = action.SelectedOption.Value
				hasValue = true
			}
		case "multi_static_select":
			if len(action.SelectedOptions) > 0 {
				selected := make([]string, len(action.SelectedOptions))
				for i, opt := range action.SelectedOptions {
					selected[i] = opt.Value
				}
				value = selected
				hasValue = true
			}
		case "users_select":
			if action.SelectedUser != "" {
				value = action.SelectedUser
				hasValue = true
			}
		case "multi_users_select":
			if len(action.SelectedUsers) > 0 {
				value = action.SelectedUsers
				hasValue = true
			}
		case "datepicker":
			if action.SelectedDate != "" {
				value = action.SelectedDate
				hasValue = true
			}
		case "url_text_input":
			if action.Value != "" {
				value = action.Value
				hasValue = true
			}
		}

		if hasValue {
			fieldValues[fieldID] = model.FieldValue{
				FieldID: types.FieldID(fieldID),
				Value:   value,
			}
		}
	}

	if len(fieldValues) == 0 {
		return nil
	}
	return fieldValues
}

// buildWorkspaceSelectModal constructs the Block Kit modal for workspace selection
func buildWorkspaceSelectModal(channelID string, workspaces []model.Workspace) slack.ModalViewRequest {
	meta := commandMetadata{
		ChannelID: channelID,
	}
	metaJSON, _ := json.Marshal(meta) //nolint:errcheck

	options := make([]*slack.OptionBlockObject, len(workspaces))
	for i, ws := range workspaces {
		options[i] = slack.NewOptionBlockObject(
			ws.ID,
			slack.NewTextBlockObject(slack.PlainTextType, ws.Name, false, false),
			nil,
		)
	}

	radioGroup := slack.NewRadioButtonsBlockElement(slackActionIDWorkspaceRadio, options...)
	radioInput := slack.NewInputBlock(
		slackBlockIDWorkspaceSelect,
		slack.NewTextBlockObject(slack.PlainTextType, "Workspace", false, false),
		nil,
		radioGroup,
	)

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      SlackCallbackIDSelectWorkspace,
		Title:           slack.NewTextBlockObject(slack.PlainTextType, "Create Case", false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, "Next", false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, "Cancel", false, false),
		PrivateMetadata: string(metaJSON),
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				radioInput,
			},
		},
	}
}
