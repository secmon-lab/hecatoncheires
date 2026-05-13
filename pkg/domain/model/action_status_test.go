package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewActionStatusSet_valid(t *testing.T) {
	defs := []model.ActionStatusDefinition{
		{ID: "todo", Name: "Todo"},
		{ID: "doing", Name: "Doing", Color: "active"},
		{ID: "done", Name: "Done", Color: "success", Emoji: "✅"},
	}
	set := gt.R1(model.NewActionStatusSet("todo", []string{"done"}, defs)).NoError(t)

	gt.Equal(t, set.InitialID(), "todo")
	gt.Equal(t, set.Initial().ID, "todo")
	gt.True(t, set.IsValid("doing"))
	gt.False(t, set.IsValid("missing"))
	gt.True(t, set.IsClosed("done"))
	gt.False(t, set.IsClosed("doing"))
	gt.Equal(t, set.ClosedIDs(), []string{"done"})
	gt.Equal(t, set.IDs(), []string{"todo", "doing", "done"})

	def, ok := set.Get("done")
	gt.True(t, ok)
	gt.Equal(t, def.Name, "Done")

	gt.Equal(t, set.Emoji("done"), "✅")
	gt.Equal(t, set.Emoji("todo"), "❓") // no emoji set → fallback
}

func TestNewActionStatusSet_errors(t *testing.T) {
	t.Run("empty statuses", func(t *testing.T) {
		_, err := model.NewActionStatusSet("a", nil, nil)
		gt.Error(t, err)
	})

	t.Run("invalid id pattern", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "bad id!", Name: "X"}}
		_, err := model.NewActionStatusSet("bad id!", nil, defs)
		gt.Error(t, err)
	})

	t.Run("missing name", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.Error(t, err)
	})

	t.Run("duplicate id", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{
			{ID: "x", Name: "X"},
			{ID: "x", Name: "X2"},
		}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.Error(t, err)
	})

	t.Run("initial empty", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X"}}
		_, err := model.NewActionStatusSet("", nil, defs)
		gt.Error(t, err)
	})

	t.Run("initial not found", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X"}}
		_, err := model.NewActionStatusSet("y", nil, defs)
		gt.Error(t, err)
	})

	t.Run("closed id not found", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X"}}
		_, err := model.NewActionStatusSet("x", []string{"y"}, defs)
		gt.Error(t, err)
	})

	t.Run("invalid color preset", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X", Color: "rainbow"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.Error(t, err)
	})

	t.Run("rejects css var", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X", Color: "var(--ok)"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.Error(t, err)
	})

	t.Run("rejects css color keyword", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X", Color: "red"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.Error(t, err)
	})

	t.Run("accepts hex color", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{
			{ID: "x", Name: "X", Color: "#5EAEDC"},
			{ID: "y", Name: "Y", Color: "#abc"},
		}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.NoError(t, err)
	})

	t.Run("accepts empty color", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.NoError(t, err)
	})

	t.Run("accepts color preset case-insensitively", func(t *testing.T) {
		defs := []model.ActionStatusDefinition{{ID: "x", Name: "X", Color: "Active"}}
		_, err := model.NewActionStatusSet("x", nil, defs)
		gt.NoError(t, err)
	})

	t.Run("rejects status id over 32 characters", func(t *testing.T) {
		// 33-char id: pattern-valid but too long.
		longID := "abcdefghij_abcdefghij_abcdefghij_x"
		defs := []model.ActionStatusDefinition{{ID: longID, Name: "X"}}
		_, err := model.NewActionStatusSet(longID, nil, defs)
		gt.Error(t, err)
	})
}

func TestActionStatusDefinition_SlackColor(t *testing.T) {
	t.Run("empty color returns empty", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: ""}
		gt.Equal(t, def.SlackColor(), "")
	})

	t.Run("six-digit hex passes through", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: "#5EAEDC"}
		gt.Equal(t, def.SlackColor(), "#5EAEDC")
	})

	t.Run("three-digit hex passes through", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: "#abc"}
		gt.Equal(t, def.SlackColor(), "#abc")
	})

	t.Run("preset name resolves to expected hex", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: "active"}
		gt.Equal(t, def.SlackColor(), "#3B82F6")
	})

	t.Run("preset name is case-insensitive", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: "Success"}
		gt.Equal(t, def.SlackColor(), "#22C55E")
	})

	t.Run("each declared preset resolves to a non-empty hex", func(t *testing.T) {
		presets := []string{
			"idle", "active", "waiting", "paused", "attention",
			"blocked", "success", "neutral_done", "failure",
		}
		for _, p := range presets {
			def := model.ActionStatusDefinition{Color: p}
			gt.String(t, def.SlackColor()).NotEqual("")
		}
	})

	t.Run("unknown value yields empty (graceful, no panic)", func(t *testing.T) {
		def := model.ActionStatusDefinition{Color: "rainbow"}
		gt.Equal(t, def.SlackColor(), "")
	})
}

func TestDefaultActionStatusSet(t *testing.T) {
	set := model.DefaultActionStatusSet()
	gt.Equal(t, set.InitialID(), "BACKLOG")
	gt.True(t, set.IsValid("BACKLOG"))
	gt.True(t, set.IsValid("TODO"))
	gt.True(t, set.IsValid("IN_PROGRESS"))
	gt.True(t, set.IsValid("BLOCKED"))
	gt.True(t, set.IsValid("COMPLETED"))
	gt.True(t, set.IsClosed("COMPLETED"))
	gt.False(t, set.IsClosed("IN_PROGRESS"))
	gt.Equal(t, set.Emoji("COMPLETED"), "✅")
}
