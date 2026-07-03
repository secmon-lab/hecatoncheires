package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestCaseTrigger_IsValid(t *testing.T) {
	gt.Bool(t, model.CaseTriggerInstant.IsValid()).True()
	gt.Bool(t, model.CaseTriggerMention.IsValid()).True()
	gt.Bool(t, model.CaseTrigger("").IsValid()).False()
	gt.Bool(t, model.CaseTrigger("bogus").IsValid()).False()
}

func TestCaseTrigger_Normalize(t *testing.T) {
	gt.Value(t, model.CaseTrigger("").Normalize()).Equal(model.CaseTriggerInstant)
	gt.Value(t, model.CaseTriggerInstant.Normalize()).Equal(model.CaseTriggerInstant)
	gt.Value(t, model.CaseTriggerMention.Normalize()).Equal(model.CaseTriggerMention)
}

func TestCaseTrigger_IsMention(t *testing.T) {
	gt.Bool(t, model.CaseTriggerMention.IsMention()).True()
	gt.Bool(t, model.CaseTriggerInstant.IsMention()).False()
	// Empty normalises to instant, so it is not mention mode.
	gt.Bool(t, model.CaseTrigger("").IsMention()).False()
}
