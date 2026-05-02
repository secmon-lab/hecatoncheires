package usecase

import (
	"fmt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	goslack "github.com/slack-go/slack"
)

const (
	blockIDDraftEditTitle        = "draft_edit_title"
	actionIDDraftEditTitle       = "draft_edit_title_input"
	blockIDDraftEditDescription  = "draft_edit_description"
	actionIDDraftEditDescription = "draft_edit_description_input"
)

// buildDraftEditModal constructs the dynamic Edit modal for the draft. The
// fixed inputs (title + description) are followed by one input block per
// custom field defined in the workspace's FieldSchema, with initial values
// drawn from the materialization.
func buildDraftEditModal(entry *model.WorkspaceEntry, mat *model.WorkspaceMaterialization, privateMetadata string) goslack.ModalViewRequest {
	blocks := []goslack.Block{
		goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType,
				fmt.Sprintf("*Workspace*: %s\n_To switch workspace, cancel this modal and use the selector in the preview._",
					fallbackText(entry.Workspace.Name, entry.Workspace.ID)), false, false),
			nil, nil,
		),
	}

	titleEl := goslack.NewPlainTextInputBlockElement(nil, actionIDDraftEditTitle)
	if mat != nil && mat.Title != "" {
		titleEl.InitialValue = mat.Title
	}
	blocks = append(blocks, goslack.NewInputBlock(
		blockIDDraftEditTitle,
		goslack.NewTextBlockObject(goslack.PlainTextType, "Title", false, false),
		nil,
		titleEl,
	))

	descEl := goslack.NewPlainTextInputBlockElement(nil, actionIDDraftEditDescription)
	descEl.Multiline = true
	if mat != nil && mat.Description != "" {
		descEl.InitialValue = mat.Description
	}
	descBlock := goslack.NewInputBlock(
		blockIDDraftEditDescription,
		goslack.NewTextBlockObject(goslack.PlainTextType, "Description", false, false),
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

	return goslack.ModalViewRequest{
		Type:            goslack.VTModal,
		CallbackID:      SlackCallbackIDDraftEdit,
		Title:           goslack.NewTextBlockObject(goslack.PlainTextType, "Edit Case Draft", false, false),
		Submit:          goslack.NewTextBlockObject(goslack.PlainTextType, "Submit", false, false),
		Close:           goslack.NewTextBlockObject(goslack.PlainTextType, "Cancel", false, false),
		PrivateMetadata: privateMetadata,
		Blocks:          goslack.Blocks{BlockSet: blocks},
	}
}
