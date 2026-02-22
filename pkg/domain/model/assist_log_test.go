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
