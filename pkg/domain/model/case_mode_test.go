package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestCaseMode_IsValid(t *testing.T) {
	gt.Bool(t, model.CaseModeChannel.IsValid()).True()
	gt.Bool(t, model.CaseModeThread.IsValid()).True()
	gt.Bool(t, model.CaseMode("").IsValid()).False()
	gt.Bool(t, model.CaseMode("bogus").IsValid()).False()
}

func TestCaseMode_Normalize(t *testing.T) {
	gt.Value(t, model.CaseMode("").Normalize()).Equal(model.CaseModeChannel)
	gt.Value(t, model.CaseModeChannel.Normalize()).Equal(model.CaseModeChannel)
	gt.Value(t, model.CaseModeThread.Normalize()).Equal(model.CaseModeThread)
}

func TestCaseMode_IsThread(t *testing.T) {
	gt.Bool(t, model.CaseModeThread.IsThread()).True()
	gt.Bool(t, model.CaseModeChannel.IsThread()).False()
	// Empty normalises to channel mode, so it is not thread mode.
	gt.Bool(t, model.CaseMode("").IsThread()).False()
}
