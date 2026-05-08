package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestAction_IsArchived(t *testing.T) {
	t.Run("nil action returns false", func(t *testing.T) {
		var a *model.Action
		gt.Bool(t, a.IsArchived()).False()
	})

	t.Run("action without ArchivedAt is not archived", func(t *testing.T) {
		a := &model.Action{ID: 1}
		gt.Bool(t, a.IsArchived()).False()
	})

	t.Run("action with ArchivedAt is archived", func(t *testing.T) {
		ts := time.Now()
		a := &model.Action{ID: 1, ArchivedAt: &ts}
		gt.Bool(t, a.IsArchived()).True()
	})
}
