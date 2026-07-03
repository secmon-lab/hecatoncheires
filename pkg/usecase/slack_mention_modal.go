package usecase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	goslack "github.com/slack-go/slack"
)

const (
	blockIDDraftEditTitle        = "draft_edit_title"
	actionIDDraftEditTitle       = "draft_edit_title_input"
	blockIDDraftEditDescription  = "draft_edit_description"
	actionIDDraftEditDescription = "draft_edit_description_input"
	blockIDDraftEditTest         = "draft_edit_test"
	actionIDDraftEditTest        = "draft_edit_test_input"
)

// buildDraftEditModal constructs the dynamic Edit modal for the draft. The
// fixed inputs (title + description) are followed by one input block per
// custom field defined in the workspace's FieldSchema, with initial values
// drawn from the materialization.
func buildDraftEditModal(ctx context.Context, entry *model.WorkspaceEntry, mat *model.WorkspaceMaterialization, privateMetadata string) goslack.ModalViewRequest {
	blocks := []goslack.Block{
		goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType,
				i18n.T(ctx, i18n.MsgMentionEditWorkspace, fallbackText(entry.Workspace.Name, entry.Workspace.ID)), false, false),
			nil, nil,
		),
	}

	titleEl := goslack.NewPlainTextInputBlockElement(nil, actionIDDraftEditTitle)
	if mat != nil && mat.Title != "" {
		// Title is not clamped here: the planner is instructed to keep it
		// under ~80 characters, and Slack's 3000-rune ceiling on
		// initial_value is far above that, so a stray long title still
		// renders without raising invalid_arguments.
		titleEl.InitialValue = mat.Title
	}
	blocks = append(blocks, goslack.NewInputBlock(
		blockIDDraftEditTitle,
		goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTitle), false, false),
		nil,
		titleEl,
	))

	descEl := goslack.NewPlainTextInputBlockElement(nil, actionIDDraftEditDescription)
	descEl.Multiline = true
	if mat != nil && mat.Description != "" {
		descEl.InitialValue = clampPlainText(mat.Description, true)
	}
	descBlock := goslack.NewInputBlock(
		blockIDDraftEditDescription,
		goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgFieldDescription), false, false),
		nil,
		descEl,
	)
	descBlock.Optional = true
	blocks = append(blocks, descBlock)

	if entry.FieldSchema != nil {
		for _, fd := range entry.FieldSchema.Fields {
			var fv *model.FieldValue
			if mat != nil && mat.CustomFieldValues != nil {
				if v, ok := mat.CustomFieldValues[fd.ID]; ok {
					fv = &v
				}
			}
			if block := buildFieldInputBlockWithValue(fd, fv); block != nil {
				blocks = append(blocks, block)
			}
		}
	}

	// Test-case flag, pre-ticked from the agent's suggestion so the human can
	// confirm or override it before the case is created.
	testOption := goslack.NewOptionBlockObject(
		caseOptionValueTest,
		goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTestCase), false, false),
		goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgFieldTestCaseDesc), false, false),
	)
	testEl := goslack.NewCheckboxGroupsBlockElement(actionIDDraftEditTest, testOption)
	if mat != nil && mat.IsTest {
		testEl.InitialOptions = []*goslack.OptionBlockObject{testOption}
	}
	testBlock := goslack.NewInputBlock(
		blockIDDraftEditTest,
		goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgFieldCaseOptions), false, false),
		nil,
		testEl,
	)
	testBlock.Optional = true
	blocks = append(blocks, testBlock)

	return goslack.ModalViewRequest{
		Type:            goslack.VTModal,
		CallbackID:      SlackCallbackIDDraftEdit,
		Title:           goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgMentionEditModalTitle), false, false),
		Submit:          goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgMentionBtnSubmit), false, false),
		Close:           goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgMentionBtnCancel), false, false),
		PrivateMetadata: privateMetadata,
		Blocks:          goslack.Blocks{BlockSet: blocks},
	}
}
