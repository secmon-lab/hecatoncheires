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

func TestActionStep_Validate(t *testing.T) {
	t.Run("valid step passes", func(t *testing.T) {
		s := &model.ActionStep{ID: "step-1", ActionID: 7, Title: "gather evidence"}
		gt.NoError(t, s.Validate())
	})

	t.Run("nil step is rejected", func(t *testing.T) {
		var s *model.ActionStep
		gt.Error(t, s.Validate()).Is(model.ErrActionStepValidation)
	})

	t.Run("missing ID is rejected", func(t *testing.T) {
		s := &model.ActionStep{ActionID: 7}
		gt.Error(t, s.Validate()).Is(model.ErrActionStepValidation)
	})

	t.Run("missing ActionID is rejected", func(t *testing.T) {
		s := &model.ActionStep{ID: "step-1"}
		gt.Error(t, s.Validate()).Is(model.ErrActionStepValidation)
	})
}
