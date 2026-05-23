package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestSystemActorID(t *testing.T) {
	gt.String(t, model.SystemActorID).Equal("@system")
}

func TestIsSystemActor(t *testing.T) {
	gt.Bool(t, model.IsSystemActor(model.SystemActorID)).True()
	gt.Bool(t, model.IsSystemActor("@system")).True()
	gt.Bool(t, model.IsSystemActor("U12345")).False()
	gt.Bool(t, model.IsSystemActor("W67890")).False()
	gt.Bool(t, model.IsSystemActor("")).False()
}
