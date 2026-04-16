package usecase

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack"
)

// Callback IDs for slash command modals
const (
	SlackCallbackIDSelectWorkspace = "hc_select_workspace"
	SlackCallbackIDCreateCase      = "hc_create_case"
	SlackCallbackIDEditCase        = "hc_edit_case"

	SlackBlockIDWorkspaceSelect  = "hc_ws_select_block"
	SlackActionIDWorkspaceRadio  = "hc_ws_radio"
	SlackBlockIDCaseTitle        = "hc_case_title_block"
	SlackActionIDCaseTitle       = "hc_case_title"
	SlackBlockIDCaseDescription  = "hc_case_desc_block"
	SlackActionIDCaseDescription = "hc_case_desc"

	// Prefix for custom field block/action IDs
	slackFieldBlockPrefix  = "hc_field_block_"
	slackFieldActionPrefix = "hc_field_action_"
)

// commandMetadata is stored in modal private_metadata as JSON
type commandMetadata struct {
	WorkspaceID  string `json:"workspace_id"`
	ChannelID    string `json:"channel_id"`
	SourceTeamID string `json:"source_team_id,omitempty"` // Slack workspace ID where the slash command was invoked
	RequestKey   string `json:"request_key,omitempty"`    // UUID for preventing duplicate case creation
}

// editCommandMetadata is stored in edit modal private_metadata as JSON
type editCommandMetadata struct {
	WorkspaceID  string `json:"workspace_id"`
	ChannelID    string `json:"channel_id"`
	SourceTeamID string `json:"source_team_id,omitempty"`
	CaseID       int64  `json:"case_id"`
}

// HandleSlashCommand handles a Slack slash command.
// If the channel is linked to an existing case, it opens the case edit modal.
// Otherwise, it opens the case creation modal (or workspace selection if multiple workspaces).
func (uc *SlackUseCases) HandleSlashCommand(ctx context.Context, triggerID, userID, channelID, workspaceID, sourceTeamID string) error {
	if uc.slackService == nil {
		return goerr.New("slack service is not available")
	}
	if uc.registry == nil {
		return goerr.New("workspace registry is not available")
	}

	// Detect user's language from Slack locale
	ctx = uc.contextWithUserLang(ctx, userID)

	// Check if the channel is linked to an existing case
	if channelID != "" {
		if existingCase, wsID, schema := uc.findCaseByChannelID(ctx, channelID); existingCase != nil {
			// Check access control for private cases
			if !model.IsCaseAccessible(existingCase, userID) {
				if err := uc.slackService.PostEphemeral(ctx, channelID, userID, i18n.T(ctx, i18n.MsgErrCaseNotAccessible)); err != nil {
					errutil.Handle(ctx, err, "failed to post access denied ephemeral")
				}
				return nil
			}
			return uc.openCaseEditModal(ctx, triggerID, wsID, channelID, sourceTeamID, existingCase, schema)
		}
	}

	// No existing case found; fall through to create flow
	if workspaceID != "" {
		entry, err := uc.registry.Get(workspaceID)
		if err != nil {
			return goerr.Wrap(err, "invalid workspace ID",
				goerr.V("workspace_id", workspaceID))
		}
		return uc.openCaseCreationModal(ctx, triggerID, workspaceID, channelID, sourceTeamID, entry.FieldSchema)
	}

	workspaces := uc.registry.Workspaces()
	switch len(workspaces) {
	case 0:
		return goerr.New("no workspaces configured")
	case 1:
		entry, _ := uc.registry.Get(workspaces[0].ID)
		return uc.openCaseCreationModal(ctx, triggerID, workspaces[0].ID, channelID, sourceTeamID, entry.FieldSchema)
	default:
		return uc.openWorkspaceSelectModal(ctx, triggerID, channelID, sourceTeamID, workspaces)
	}
}

// HandleWorkspaceSelectSubmit processes the workspace selection modal submission.
// It returns the case creation modal view to replace the current modal via response_action: update.
func (uc *SlackUseCases) HandleWorkspaceSelectSubmit(ctx context.Context, callback *slack.InteractionCallback) (*slack.ModalViewRequest, error) {
	// Extract selected workspace from radio buttons
	blockValues := callback.View.State.Values
	radioBlock, ok := blockValues[SlackBlockIDWorkspaceSelect]
	if !ok {
		return nil, goerr.New("workspace selection block not found")
	}
	radioAction, ok := radioBlock[SlackActionIDWorkspaceRadio]
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

	view := uc.buildCaseCreationModal(ctx, workspaceID, meta.ChannelID, meta.SourceTeamID, schema)
	return &view, nil
}

// HandleCaseCreationSubmit processes the case creation modal submission.
// It creates a case using CaseUseCase and posts a confirmation message.
func (uc *SlackUseCases) HandleCaseCreationSubmit(ctx context.Context, caseUC *CaseUseCase, callback *slack.InteractionCallback) error {
	// Extract fields from view state
	blockValues := callback.View.State.Values

	title := ""
	if titleBlock, ok := blockValues[SlackBlockIDCaseTitle]; ok {
		if titleAction, ok := titleBlock[SlackActionIDCaseTitle]; ok {
			title = titleAction.Value
		}
	}

	description := ""
	if descBlock, ok := blockValues[SlackBlockIDCaseDescription]; ok {
		if descAction, ok := descBlock[SlackActionIDCaseDescription]; ok {
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
	created, err := caseUC.CreateCase(ctx, meta.WorkspaceID, title, description, []string{userID}, fieldValues, false, meta.SourceTeamID, meta.RequestKey)
	if err != nil {
		return goerr.Wrap(err, "failed to create case via slash command",
			goerr.V("workspace_id", meta.WorkspaceID),
			goerr.V("user_id", userID))
	}

	// Notify the creator if cross-workspace connect was needed but not available
	if meta.ChannelID != "" && meta.SourceTeamID != "" && uc.slackService != nil {
		configuredTeamID := caseUC.slackTeamIDForWorkspace(meta.WorkspaceID)
		if meta.SourceTeamID != configuredTeamID && caseUC.slackAdminService == nil {
			msg := i18n.T(ctx, i18n.MsgCrossWorkspaceConnectUnavailable)
			if ephErr := uc.slackService.PostEphemeral(ctx, meta.ChannelID, userID, msg); ephErr != nil {
				errutil.Handle(ctx, ephErr, "failed to post cross-workspace connect notification")
			}
		}
	}

	// Post confirmation message to the channel where the command was invoked
	if meta.ChannelID != "" && uc.slackService != nil {
		var confirmText string
		if created.SlackChannelID != "" {
			confirmText = i18n.T(ctx, i18n.MsgCaseCreatedWithChannel, created.ID, created.Title, created.SlackChannelID)
		} else {
			confirmText = i18n.T(ctx, i18n.MsgCaseCreated, created.ID, created.Title)
		}

		if _, err := uc.slackService.PostMessage(ctx, meta.ChannelID, nil, confirmText); err != nil {
			// Log but don't fail; the case was already created
			logging.From(ctx).Error("failed to post confirmation message",
				"error", err,
				"channel_id", meta.ChannelID,
				"case_id", created.ID)
		}
	}

	return nil
}

// openCaseCreationModal opens the case creation modal directly
func (uc *SlackUseCases) openCaseCreationModal(ctx context.Context, triggerID, workspaceID, channelID, sourceTeamID string, schema *config.FieldSchema) error {
	view := uc.buildCaseCreationModal(ctx, workspaceID, channelID, sourceTeamID, schema)
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open case creation modal",
			goerr.V("workspace_id", workspaceID))
	}
	return nil
}

// openWorkspaceSelectModal opens the workspace selection modal
func (uc *SlackUseCases) openWorkspaceSelectModal(ctx context.Context, triggerID, channelID, sourceTeamID string, workspaces []model.Workspace) error {
	view := uc.buildWorkspaceSelectModal(ctx, channelID, sourceTeamID, workspaces)
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open workspace select modal")
	}
	return nil
}

// buildCaseCreationModal constructs the Block Kit modal for case creation
func (uc *SlackUseCases) buildCaseCreationModal(ctx context.Context, workspaceID, channelID, sourceTeamID string, schema *config.FieldSchema) slack.ModalViewRequest {
	meta := commandMetadata{
		WorkspaceID:  workspaceID,
		ChannelID:    channelID,
		SourceTeamID: sourceTeamID,
		RequestKey:   uuid.New().String(),
	}
	metaJSON, _ := json.Marshal(meta) //nolint:errcheck

	titleInput := slack.NewInputBlock(
		SlackBlockIDCaseTitle,
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTitle), false, false),
		nil,
		slack.NewPlainTextInputBlockElement(
			slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTitlePlaceholder), false, false),
			SlackActionIDCaseTitle,
		),
	)

	descInput := slack.NewInputBlock(
		SlackBlockIDCaseDescription,
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldDescription), false, false),
		nil,
		&slack.PlainTextInputBlockElement{
			Type:        slack.METPlainTextInput,
			ActionID:    SlackActionIDCaseDescription,
			Multiline:   true,
			Placeholder: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldDescPlaceholder), false, false),
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
		Title:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseTitle), false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseSubmit), false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
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
			Type:             slack.METNumber,
			ActionID:         actionID,
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

// findCaseByChannelID searches all workspaces for a case associated with the given channel ID.
// Returns the case, workspace ID, and field schema if found; otherwise returns nil.
func (uc *SlackUseCases) findCaseByChannelID(ctx context.Context, channelID string) (*model.Case, string, *config.FieldSchema) {
	if uc.registry == nil {
		return nil, "", nil
	}
	for _, entry := range uc.registry.List() {
		c, err := uc.repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, channelID)
		if err != nil {
			errutil.Handle(ctx, err, "failed to look up case by slack channel ID for edit")
			continue
		}
		if c != nil {
			return c, entry.Workspace.ID, entry.FieldSchema
		}
	}
	return nil, "", nil
}

// openCaseEditModal opens the case edit modal with prefilled values
func (uc *SlackUseCases) openCaseEditModal(ctx context.Context, triggerID, workspaceID, channelID, sourceTeamID string, existingCase *model.Case, schema *config.FieldSchema) error {
	view := uc.buildCaseEditModal(ctx, workspaceID, channelID, sourceTeamID, existingCase, schema)
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open case edit modal",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", existingCase.ID))
	}
	return nil
}

// buildCaseEditModal constructs the Block Kit modal for case editing with prefilled values
func (uc *SlackUseCases) buildCaseEditModal(ctx context.Context, workspaceID, channelID, sourceTeamID string, existingCase *model.Case, schema *config.FieldSchema) slack.ModalViewRequest {
	meta := editCommandMetadata{
		WorkspaceID:  workspaceID,
		ChannelID:    channelID,
		SourceTeamID: sourceTeamID,
		CaseID:       existingCase.ID,
	}
	metaJSON, _ := json.Marshal(meta) //nolint:errcheck

	titleElement := slack.NewPlainTextInputBlockElement(
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTitlePlaceholder), false, false),
		SlackActionIDCaseTitle,
	)
	titleElement.InitialValue = existingCase.Title

	titleInput := slack.NewInputBlock(
		SlackBlockIDCaseTitle,
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTitle), false, false),
		nil,
		titleElement,
	)

	descElement := &slack.PlainTextInputBlockElement{
		Type:        slack.METPlainTextInput,
		ActionID:    SlackActionIDCaseDescription,
		Multiline:   true,
		Placeholder: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldDescPlaceholder), false, false),
	}
	if existingCase.Description != "" {
		descElement.InitialValue = existingCase.Description
	}

	descInput := slack.NewInputBlock(
		SlackBlockIDCaseDescription,
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldDescription), false, false),
		nil,
		descElement,
	)
	descInput.Optional = true

	blocks := []slack.Block{
		titleInput,
		descInput,
	}

	// Add custom field inputs with prefilled values
	if schema != nil {
		for _, field := range schema.Fields {
			var fv *model.FieldValue
			if existingCase.FieldValues != nil {
				if v, ok := existingCase.FieldValues[field.ID]; ok {
					fv = &v
				}
			}
			if block := buildFieldInputBlockWithValue(field, fv); block != nil {
				blocks = append(blocks, block)
			}
		}
	}

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      SlackCallbackIDEditCase,
		Title:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalEditCaseTitle), false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalEditCaseSubmit), false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
		PrivateMetadata: string(metaJSON),
		Blocks: slack.Blocks{
			BlockSet: blocks,
		},
	}
}

// HandleCaseEditSubmit processes the case edit modal submission.
// It updates the case using CaseUseCase and posts a confirmation message.
func (uc *SlackUseCases) HandleCaseEditSubmit(ctx context.Context, caseUC *CaseUseCase, callback *slack.InteractionCallback) error {
	blockValues := callback.View.State.Values

	title := ""
	if titleBlock, ok := blockValues[SlackBlockIDCaseTitle]; ok {
		if titleAction, ok := titleBlock[SlackActionIDCaseTitle]; ok {
			title = titleAction.Value
		}
	}

	description := ""
	if descBlock, ok := blockValues[SlackBlockIDCaseDescription]; ok {
		if descAction, ok := descBlock[SlackActionIDCaseDescription]; ok {
			description = descAction.Value
		}
	}

	var meta editCommandMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return goerr.Wrap(err, "failed to parse edit private_metadata")
	}

	fieldValues := extractFieldValues(blockValues)
	userID := callback.User.ID

	// Get existing case to preserve assignees
	existingCase, err := caseUC.repo.Case().Get(ctx, meta.WorkspaceID, meta.CaseID)
	if err != nil {
		return goerr.Wrap(err, "failed to get existing case for edit",
			goerr.V("case_id", meta.CaseID))
	}

	updated, err := caseUC.UpdateCase(ctx, meta.WorkspaceID, meta.CaseID, title, description, existingCase.AssigneeIDs, fieldValues)
	if err != nil {
		return goerr.Wrap(err, "failed to update case via slash command",
			goerr.V("workspace_id", meta.WorkspaceID),
			goerr.V("case_id", meta.CaseID),
			goerr.V("user_id", userID))
	}

	// Post confirmation message to the case channel
	if meta.ChannelID != "" && uc.slackService != nil {
		confirmText := i18n.T(ctx, i18n.MsgCaseUpdated, updated.ID, updated.Title)
		if _, err := uc.slackService.PostMessage(ctx, meta.ChannelID, nil, confirmText); err != nil {
			errutil.Handle(ctx, err, "failed to post case update confirmation message")
		}
	}

	return nil
}

// buildFieldInputBlockWithValue creates a Slack input block for a custom field with an optional prefilled value
func buildFieldInputBlockWithValue(field config.FieldDefinition, fv *model.FieldValue) slack.Block {
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
		if fv != nil {
			if s, ok := fv.Value.(string); ok {
				element.InitialValue = s
			}
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeNumber:
		element := &slack.NumberInputBlockElement{
			Type:             slack.METNumber,
			ActionID:         actionID,
			IsDecimalAllowed: true,
		}
		if field.Description != "" {
			element.Placeholder = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
		}
		if fv != nil {
			if s, ok := fv.Value.(string); ok {
				element.InitialValue = s
			}
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
		if fv != nil {
			if s, ok := fv.Value.(string); ok {
				for _, opt := range options {
					if opt.Value == s {
						element.InitialOption = opt
						break
					}
				}
			}
		}
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
		if fv != nil {
			selectedValues := fieldValueToStringSlice(fv.Value)
			if len(selectedValues) > 0 {
				optionMap := make(map[string]*slack.OptionBlockObject, len(options))
				for _, opt := range options {
					optionMap[opt.Value] = opt
				}
				initialOptions := make([]*slack.OptionBlockObject, 0, len(selectedValues))
				for _, v := range selectedValues {
					if opt, ok := optionMap[v]; ok {
						initialOptions = append(initialOptions, opt)
					}
				}
				if len(initialOptions) > 0 {
					element.InitialOptions = initialOptions
				}
			}
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeUser:
		element := slack.NewOptionsSelectBlockElement(
			slack.OptTypeUser,
			nil,
			actionID,
		)
		if fv != nil {
			if s, ok := fv.Value.(string); ok && s != "" {
				element.InitialUser = s
			}
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeMultiUser:
		element := slack.NewOptionsMultiSelectBlockElement(
			slack.MultiOptTypeUser,
			nil,
			actionID,
		)
		if fv != nil {
			users := fieldValueToStringSlice(fv.Value)
			if len(users) > 0 {
				element.InitialUsers = users
			}
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeDate:
		element := slack.NewDatePickerBlockElement(actionID)
		if fv != nil {
			if s, ok := fv.Value.(string); ok && s != "" {
				element.InitialDate = s
			}
		}
		inputBlock = slack.NewInputBlock(blockID, label, nil, element)

	case types.FieldTypeURL:
		element := &slack.URLTextInputBlockElement{
			Type:     slack.METURLTextInput,
			ActionID: actionID,
		}
		if field.Description != "" {
			element.Placeholder = slack.NewTextBlockObject(slack.PlainTextType, field.Description, false, false)
		}
		if fv != nil {
			if s, ok := fv.Value.(string); ok {
				element.InitialValue = s
			}
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

// fieldValueToStringSlice converts a FieldValue's Value to []string.
// Handles both []string and []interface{} (from JSON deserialization).
func fieldValueToStringSlice(v any) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, elem := range val {
			if s, ok := elem.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// buildWorkspaceSelectModal constructs the Block Kit modal for workspace selection
func (uc *SlackUseCases) buildWorkspaceSelectModal(ctx context.Context, channelID, sourceTeamID string, workspaces []model.Workspace) slack.ModalViewRequest {
	meta := commandMetadata{
		ChannelID:    channelID,
		SourceTeamID: sourceTeamID,
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

	radioGroup := slack.NewRadioButtonsBlockElement(SlackActionIDWorkspaceRadio, options...)
	radioInput := slack.NewInputBlock(
		SlackBlockIDWorkspaceSelect,
		slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgFieldWorkspace), false, false),
		nil,
		radioGroup,
	)

	return slack.ModalViewRequest{
		Type:            slack.VTModal,
		CallbackID:      SlackCallbackIDSelectWorkspace,
		Title:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseTitle), false, false),
		Submit:          slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalNextButton), false, false),
		Close:           slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
		PrivateMetadata: string(metaJSON),
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				radioInput,
			},
		},
	}
}
