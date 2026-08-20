package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func validActionComment() *model.ActionComment {
	now := time.Now().UTC()
	return &model.ActionComment{
		ID:        "comment-1",
		ActionID:  7,
		AuthorID:  "U001",
		Body:      "looks like a false positive",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestActionComment_IsEdited(t *testing.T) {
	now := time.Now().UTC()

	t.Run("nil receiver is not edited", func(t *testing.T) {
		var c *model.ActionComment
		gt.Bool(t, c.IsEdited()).False()
	})

	t.Run("equal timestamps are not edited", func(t *testing.T) {
		c := &model.ActionComment{CreatedAt: now, UpdatedAt: now}
		gt.Bool(t, c.IsEdited()).False()
	})

	t.Run("later UpdatedAt is edited", func(t *testing.T) {
		c := &model.ActionComment{CreatedAt: now, UpdatedAt: now.Add(time.Nanosecond)}
		gt.Bool(t, c.IsEdited()).True()
	})
}

func TestActionComment_Validate(t *testing.T) {
	t.Run("valid comment passes", func(t *testing.T) {
		gt.NoError(t, validActionComment().Validate())
	})

	t.Run("nil comment is rejected", func(t *testing.T) {
		var c *model.ActionComment
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})

	t.Run("missing ID is rejected", func(t *testing.T) {
		c := validActionComment()
		c.ID = ""
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})

	t.Run("missing ActionID is rejected", func(t *testing.T) {
		c := validActionComment()
		c.ActionID = 0
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})

	t.Run("missing AuthorID is rejected", func(t *testing.T) {
		c := validActionComment()
		c.AuthorID = ""
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})

	t.Run("empty body is rejected", func(t *testing.T) {
		c := validActionComment()
		c.Body = ""
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})

	t.Run("body at the cap passes", func(t *testing.T) {
		c := validActionComment()
		c.Body = strings.Repeat("a", model.ActionCommentBodyMaxLen)
		gt.NoError(t, c.Validate())
	})

	t.Run("body over the cap is rejected", func(t *testing.T) {
		c := validActionComment()
		c.Body = strings.Repeat("a", model.ActionCommentBodyMaxLen+1)
		gt.Error(t, c.Validate()).Is(model.ErrActionCommentValidation)
	})
}
