package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNotificationSlot_Validate(t *testing.T) {
	t.Run("valid slot passes", func(t *testing.T) {
		s := &model.NotificationSlot{ChannelID: "C123", MessageTS: "1700000000.000100"}
		gt.NoError(t, s.Validate())
	})

	t.Run("nil slot is rejected", func(t *testing.T) {
		var s *model.NotificationSlot
		gt.Error(t, s.Validate()).Is(model.ErrNotificationSlotValidation)
	})

	t.Run("missing ChannelID is rejected", func(t *testing.T) {
		s := &model.NotificationSlot{MessageTS: "1700000000.000100"}
		gt.Error(t, s.Validate()).Is(model.ErrNotificationSlotValidation)
	})
}
