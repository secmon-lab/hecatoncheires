package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewAssistLogID(t *testing.T) {
	id1 := model.NewAssistLogID()
	id2 := model.NewAssistLogID()

	gt.Value(t, string(id1)).NotEqual("")
	gt.Value(t, string(id2)).NotEqual("")
	gt.Value(t, id1).NotEqual(id2)
}

func TestAssistLog_Validate(t *testing.T) {
	t.Run("valid log passes", func(t *testing.T) {
		l := &model.AssistLog{ID: model.NewAssistLogID(), CaseID: 3, Summary: "did stuff"}
		gt.NoError(t, l.Validate())
	})

	t.Run("nil log is rejected", func(t *testing.T) {
		var l *model.AssistLog
		gt.Error(t, l.Validate()).Is(model.ErrAssistLogValidation)
	})

	t.Run("missing CaseID is rejected", func(t *testing.T) {
		l := &model.AssistLog{ID: model.NewAssistLogID(), Summary: "orphan"}
		gt.Error(t, l.Validate()).Is(model.ErrAssistLogValidation)
	})
}
