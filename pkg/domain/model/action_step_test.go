package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestActionStep_IsDone(t *testing.T) {
	now := time.Now().UTC()

	t.Run("nil receiver is not done", func(t *testing.T) {
		var s *model.ActionStep
		gt.Bool(t, s.IsDone()).False()
	})

	t.Run("nil DoneAt is not done", func(t *testing.T) {
		s := &model.ActionStep{}
		gt.Bool(t, s.IsDone()).False()
	})

	t.Run("non-nil DoneAt is done", func(t *testing.T) {
		s := &model.ActionStep{DoneAt: &now}
		gt.Bool(t, s.IsDone()).True()
	})
}
