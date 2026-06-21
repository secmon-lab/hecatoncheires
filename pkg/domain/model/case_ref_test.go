package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestNewCaseRef(t *testing.T) {
	c := &model.Case{
		ID:     99,
		Title:  "test case",
		Status: types.CaseStatusOpen,
	}

	ref := model.NewCaseRef("ws-a", c)

	gt.Value(t, ref.ID).Equal(int64(99))
	gt.Value(t, ref.Title).Equal("test case")
	gt.Value(t, ref.Status).Equal(types.CaseStatusOpen)
	gt.Value(t, ref.WorkspaceID).Equal("ws-a")
}

func TestNewCaseRef_PreservesStatus(t *testing.T) {
	tests := []struct {
		name   string
		status types.CaseStatus
	}{
		{name: "draft", status: types.CaseStatusDraft},
		{name: "open", status: types.CaseStatusOpen},
		{name: "closed", status: types.CaseStatusClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &model.Case{
				ID:     1,
				Title:  "x",
				Status: tt.status,
			}
			ref := model.NewCaseRef("ws-b", c)
			gt.Value(t, ref.Status).Equal(tt.status)
			gt.Value(t, ref.WorkspaceID).Equal("ws-b")
		})
	}
}
