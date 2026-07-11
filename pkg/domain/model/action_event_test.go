package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestActionEvent_Validate(t *testing.T) {
	t.Run("valid event passes", func(t *testing.T) {
		e := &model.ActionEvent{ID: "evt-1", ActionID: 9, Kind: types.ActionEventCreated}
		gt.NoError(t, e.Validate())
	})

	t.Run("nil event is rejected", func(t *testing.T) {
		var e *model.ActionEvent
		gt.Error(t, e.Validate()).Is(model.ErrActionEventValidation)
	})

	t.Run("missing ID is rejected", func(t *testing.T) {
		e := &model.ActionEvent{ActionID: 9, Kind: types.ActionEventCreated}
		gt.Error(t, e.Validate()).Is(model.ErrActionEventValidation)
	})

	t.Run("missing ActionID is rejected", func(t *testing.T) {
		e := &model.ActionEvent{ID: "evt-1", Kind: types.ActionEventCreated}
		gt.Error(t, e.Validate()).Is(model.ErrActionEventValidation)
	})
}
