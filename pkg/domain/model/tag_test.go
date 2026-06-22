package model_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewTagID(t *testing.T) {
	a := model.NewTagID()
	b := model.NewTagID()
	gt.Value(t, a).NotEqual(model.TagID(""))
	gt.Value(t, a).NotEqual(b)
	gt.String(t, a.String()).Equal(string(a))
}

func TestTagValidate(t *testing.T) {
	valid := func() *model.Tag {
		return &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: "ws-1",
			Name:        "ops",
		}
	}

	t.Run("valid tag", func(t *testing.T) {
		gt.NoError(t, valid().Validate())
	})

	t.Run("valid tag with empty name", func(t *testing.T) {
		tag := valid()
		tag.Name = ""
		gt.NoError(t, tag.Validate())
	})

	t.Run("valid tag at max name length", func(t *testing.T) {
		tag := valid()
		tag.Name = strings.Repeat("a", model.MaxTagNameLength)
		gt.NoError(t, tag.Validate())
	})

	t.Run("nil receiver", func(t *testing.T) {
		var tag *model.Tag
		gt.Error(t, tag.Validate()).Is(model.ErrTagValidation)
	})

	t.Run("missing ID", func(t *testing.T) {
		tag := valid()
		tag.ID = ""
		gt.Error(t, tag.Validate()).Is(model.ErrTagValidation)
	})

	t.Run("missing WorkspaceID", func(t *testing.T) {
		tag := valid()
		tag.WorkspaceID = ""
		gt.Error(t, tag.Validate()).Is(model.ErrTagValidation)
	})

	t.Run("name too long", func(t *testing.T) {
		tag := valid()
		tag.Name = strings.Repeat("a", model.MaxTagNameLength+1)
		gt.Error(t, tag.Validate()).Is(model.ErrTagValidation)
	})

	t.Run("name too long with multibyte runes", func(t *testing.T) {
		tag := valid()
		// Each "あ" is one rune, so 65 runes exceeds MaxTagNameLength (64).
		tag.Name = strings.Repeat("a", model.MaxTagNameLength+1)
		gt.Error(t, tag.Validate()).Is(model.ErrTagValidation)
	})
}
