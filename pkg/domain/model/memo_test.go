package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestNewMemoID(t *testing.T) {
	a := model.NewMemoID()
	b := model.NewMemoID()
	gt.Value(t, a).NotEqual(model.MemoID(""))
	gt.Value(t, a).NotEqual(b)
	gt.String(t, a.String()).Equal(string(a))
}

func TestMemoValidate(t *testing.T) {
	now := time.Now()
	valid := func() *model.Memo {
		return &model.Memo{
			ID:          model.NewMemoID(),
			WorkspaceID: "ws-1",
			CaseID:      42,
			Title:       "memo title",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	t.Run("valid", func(t *testing.T) {
		gt.NoError(t, valid().Validate())
	})

	t.Run("nil receiver", func(t *testing.T) {
		var m *model.Memo
		gt.Error(t, m.Validate()).Is(model.ErrMemoValidation)
	})

	cases := []struct {
		name   string
		mutate func(m *model.Memo)
	}{
		{"empty ID", func(m *model.Memo) { m.ID = "" }},
		{"empty workspace", func(m *model.Memo) { m.WorkspaceID = "" }},
		{"zero case ID", func(m *model.Memo) { m.CaseID = 0 }},
		{"negative case ID", func(m *model.Memo) { m.CaseID = -1 }},
		{"empty title", func(m *model.Memo) { m.Title = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := valid()
			tc.mutate(m)
			gt.Error(t, m.Validate()).Is(model.ErrMemoValidation)
		})
	}
}

func TestMemoIsArchived(t *testing.T) {
	var nilMemo *model.Memo
	gt.Bool(t, nilMemo.IsArchived()).False()

	active := &model.Memo{}
	gt.Bool(t, active.IsArchived()).False()

	at := time.Now()
	archived := &model.Memo{ArchivedAt: &at}
	gt.Bool(t, archived.IsArchived()).True()
}
