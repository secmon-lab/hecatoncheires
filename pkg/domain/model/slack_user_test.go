package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestSlackUser_Validate(t *testing.T) {
	t.Run("valid user passes", func(t *testing.T) {
		u := &model.SlackUser{ID: model.SlackUserID("U123"), Name: "john.doe"}
		gt.NoError(t, u.Validate())
	})

	t.Run("nil user is rejected", func(t *testing.T) {
		var u *model.SlackUser
		gt.Error(t, u.Validate()).Is(model.ErrSlackUserValidation)
	})

	t.Run("missing ID is rejected", func(t *testing.T) {
		u := &model.SlackUser{Name: "no-id"}
		gt.Error(t, u.Validate()).Is(model.ErrSlackUserValidation)
	})
}
