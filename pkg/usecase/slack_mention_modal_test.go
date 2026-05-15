package usecase_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// findInputBlock walks the rendered modal blocks and returns the first
// InputBlock whose BlockID matches. The test fails fast if none is found
// so the assertion downstream can dereference safely.
func findInputBlock(t *testing.T, view goslack.ModalViewRequest, blockID string) *goslack.InputBlock {
	t.Helper()
	for _, b := range view.Blocks.BlockSet {
		ib, ok := b.(*goslack.InputBlock)
		if !ok {
			continue
		}
		if ib.BlockID == blockID {
			return ib
		}
	}
	t.Fatalf("input block %q not found in view", blockID)
	return nil
}

func TestBuildDraftEditModal_DescriptionClampedForLongInput(t *testing.T) {
	long := strings.Repeat("d", 5000)
	entry := &model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       "short title",
		Description: long,
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	desc := findInputBlock(t, view, "draft_edit_description")
	descEl, ok := desc.Element.(*goslack.PlainTextInputBlockElement)
	gt.Bool(t, ok).True()

	// initial_value must fit comfortably under Slack's 3000-rune ceiling.
	gt.Number(t, len([]rune(descEl.InitialValue))).LessOrEqual(
		usecase.SlackPlainTextMaxRunesForTest + len([]rune(usecase.ClampSuffixMultiLineForTest)),
	)
	gt.Bool(t, strings.HasSuffix(descEl.InitialValue, usecase.ClampSuffixMultiLineForTest)).True()
}

func TestBuildDraftEditModal_TitlePassesThroughEvenWhenLong(t *testing.T) {
	// Title clamping is deliberately not applied — the planner is steered
	// to keep titles short via prompt guidance, and Slack's 3000-rune
	// ceiling still leaves plenty of headroom for an over-long title.
	longTitle := strings.Repeat("t", 500)
	entry := &model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       longTitle,
		Description: "ok",
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	title := findInputBlock(t, view, "draft_edit_title")
	titleEl, ok := title.Element.(*goslack.PlainTextInputBlockElement)
	gt.Bool(t, ok).True()
	gt.String(t, titleEl.InitialValue).Equal(longTitle)
}

func TestBuildDraftEditModal_ShortDescriptionUnchanged(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       "title",
		Description: "small description body",
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	desc := findInputBlock(t, view, "draft_edit_description")
	descEl, ok := desc.Element.(*goslack.PlainTextInputBlockElement)
	gt.Bool(t, ok).True()
	gt.String(t, descEl.InitialValue).Equal("small description body")
}

func TestBuildDraftEditModal_UserFieldRejectsNonSlackID(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
			},
		},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       "title",
		Description: "desc",
		CustomFieldValues: map[string]model.FieldValue{
			// AI emitted an email-shaped value instead of a Slack ID.
			"owner": {Type: types.FieldTypeUser, Value: "alice@example.com"},
		},
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	ownerBlock := findInputBlock(t, view, "hc_field_block_owner")
	sel, ok := ownerBlock.Element.(*goslack.SelectBlockElement)
	gt.Bool(t, ok).True()
	gt.String(t, sel.InitialUser).Equal("")
}

func TestBuildDraftEditModal_UserFieldAcceptsSlackID(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
			},
		},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       "title",
		Description: "desc",
		CustomFieldValues: map[string]model.FieldValue{
			"owner": {Type: types.FieldTypeUser, Value: "U01ABC234"},
		},
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	ownerBlock := findInputBlock(t, view, "hc_field_block_owner")
	sel, ok := ownerBlock.Element.(*goslack.SelectBlockElement)
	gt.Bool(t, ok).True()
	gt.String(t, sel.InitialUser).Equal("U01ABC234")
}

func TestBuildDraftEditModal_MultiUserFieldFiltersMalformed(t *testing.T) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws", Name: "WS"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "owners", Name: "Owners", Type: types.FieldTypeMultiUser},
			},
		},
	}
	mat := &model.WorkspaceMaterialization{
		Title:       "title",
		Description: "desc",
		CustomFieldValues: map[string]model.FieldValue{
			"owners": {
				Type:  types.FieldTypeMultiUser,
				Value: []string{"U01ABC", "alice@example.com", "W02DEF"},
			},
		},
	}

	view := usecase.BuildDraftEditModalForTest(entry, mat, "{}")
	ownersBlock := findInputBlock(t, view, "hc_field_block_owners")
	sel, ok := ownersBlock.Element.(*goslack.MultiSelectBlockElement)
	gt.Bool(t, ok).True()
	gt.Array(t, sel.InitialUsers).Length(2).Required()
	gt.String(t, sel.InitialUsers[0]).Equal("U01ABC")
	gt.String(t, sel.InitialUsers[1]).Equal("W02DEF")
}
