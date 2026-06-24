package interaction_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
)

func TestItemType_IsValid(t *testing.T) {
	gt.Bool(t, interaction.ItemSelect.IsValid()).True()
	gt.Bool(t, interaction.ItemMultiSelect.IsValid()).True()
	gt.Bool(t, interaction.ItemFreeText.IsValid()).True()
	gt.Bool(t, interaction.ItemType("approve").IsValid()).False()
	gt.Bool(t, interaction.ItemType("").IsValid()).False()
}

func TestRequest_Validate(t *testing.T) {
	t.Run("ok free_text", func(t *testing.T) {
		r := &interaction.Request{
			Reason: "need more info",
			Items: []interaction.Item{
				{ID: "q1", Text: "What is the impact?", Type: interaction.ItemFreeText},
			},
		}
		gt.NoError(t, r.Validate())
	})

	t.Run("ok select with options", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "Severity?", Type: interaction.ItemSelect, Options: []string{"high", "low"}},
			},
		}
		gt.NoError(t, r.Validate())
	})

	t.Run("nil request", func(t *testing.T) {
		var r *interaction.Request
		gt.Error(t, r.Validate())
	})

	t.Run("no items", func(t *testing.T) {
		r := &interaction.Request{Items: nil}
		gt.Error(t, r.Validate())
	})

	t.Run("too many items", func(t *testing.T) {
		items := make([]interaction.Item, 0, 6)
		for i := range 6 {
			items = append(items, interaction.Item{
				ID:   "q" + string(rune('a'+i)),
				Text: "x",
				Type: interaction.ItemFreeText,
			})
		}
		r := &interaction.Request{Items: items}
		gt.Error(t, r.Validate())
	})

	t.Run("empty id", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{{ID: "", Text: "x", Type: interaction.ItemFreeText}},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("duplicate id", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "dup", Text: "a", Type: interaction.ItemFreeText},
				{ID: "dup", Text: "b", Type: interaction.ItemFreeText},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("empty text", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{{ID: "q1", Text: "", Type: interaction.ItemFreeText}},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("invalid type", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{{ID: "q1", Text: "x", Type: interaction.ItemType("bogus")}},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("select with too few options", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "x", Type: interaction.ItemSelect, Options: []string{"only"}},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("multi_select needs options", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "x", Type: interaction.ItemMultiSelect, Options: nil},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("multibyte option within the 75-char limit is accepted", func(t *testing.T) {
		// 70 Japanese runes = 210 bytes. Slack counts characters, so this is
		// within the 75-char option limit; a byte-based check would wrongly
		// reject it.
		jp := strings.Repeat("あ", 70)
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "環境は?", Type: interaction.ItemSelect, Options: []string{jp, "stg"}},
			},
		}
		gt.NoError(t, r.Validate())
	})

	t.Run("option longer than Slack's 75 is rejected", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "x", Type: interaction.ItemSelect, Options: []string{"ok", strings.Repeat("a", 76)}},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("empty option is rejected", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: "x", Type: interaction.ItemSelect, Options: []string{"ok", ""}},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("item id over the block_id budget is rejected", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: strings.Repeat("x", 201), Text: "x", Type: interaction.ItemFreeText},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("item text over the label limit is rejected", func(t *testing.T) {
		r := &interaction.Request{
			Items: []interaction.Item{
				{ID: "q1", Text: strings.Repeat("t", 2001), Type: interaction.ItemFreeText},
			},
		}
		gt.Error(t, r.Validate())
	})

	t.Run("reason over the section limit is rejected", func(t *testing.T) {
		r := &interaction.Request{
			Reason: strings.Repeat("r", 3001),
			Items:  []interaction.Item{{ID: "q1", Text: "x", Type: interaction.ItemFreeText}},
		}
		gt.Error(t, r.Validate())
	})
}
