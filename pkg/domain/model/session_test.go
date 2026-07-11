package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestSession_Validate(t *testing.T) {
	t.Run("valid session passes", func(t *testing.T) {
		s := &model.Session{ID: "sess-1", ChannelID: "C123", ThreadTS: "1700000000.000100"}
		gt.NoError(t, s.Validate())
	})

	t.Run("nil session is rejected", func(t *testing.T) {
		var s *model.Session
		gt.Error(t, s.Validate()).Is(model.ErrSessionValidation)
	})

	t.Run("missing ID is rejected", func(t *testing.T) {
		s := &model.Session{ChannelID: "C123", ThreadTS: "1700000000.000100"}
		gt.Error(t, s.Validate()).Is(model.ErrSessionValidation)
	})

	t.Run("missing ChannelID is rejected", func(t *testing.T) {
		s := &model.Session{ID: "sess-1", ThreadTS: "1700000000.000100"}
		gt.Error(t, s.Validate()).Is(model.ErrSessionValidation)
	})

	t.Run("missing ThreadTS is rejected", func(t *testing.T) {
		s := &model.Session{ID: "sess-1", ChannelID: "C123"}
		gt.Error(t, s.Validate()).Is(model.ErrSessionValidation)
	})
}
