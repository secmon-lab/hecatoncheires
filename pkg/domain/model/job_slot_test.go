package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func validJobSlot(now time.Time) *model.JobSlot {
	return &model.JobSlot{
		Index:       0,
		HolderID:    "holder-1",
		WorkspaceID: "ws",
		CaseID:      42,
		JobID:       "daily",
		AcquiredAt:  now,
		ExpiresAt:   now.Add(30 * time.Second),
	}
}

func TestJobSlot_IsHeld(t *testing.T) {
	now := time.Now().UTC()

	t.Run("live holder is held", func(t *testing.T) {
		gt.Bool(t, validJobSlot(now).IsHeld(now)).True()
	})

	t.Run("expired holder is free", func(t *testing.T) {
		s := validJobSlot(now)
		gt.Bool(t, s.IsHeld(now.Add(31*time.Second))).False()
	})

	t.Run("expiry boundary counts as free", func(t *testing.T) {
		s := validJobSlot(now)
		gt.Bool(t, s.IsHeld(s.ExpiresAt)).False()
	})

	t.Run("record without holder is free", func(t *testing.T) {
		s := validJobSlot(now)
		s.HolderID = ""
		gt.Bool(t, s.IsHeld(now)).False()
	})

	t.Run("nil is free", func(t *testing.T) {
		var s *model.JobSlot
		gt.Bool(t, s.IsHeld(now)).False()
	})
}

func TestJobSlot_Validate(t *testing.T) {
	now := time.Now().UTC()

	t.Run("ok", func(t *testing.T) {
		gt.NoError(t, validJobSlot(now).Validate())
	})

	t.Run("nil", func(t *testing.T) {
		var s *model.JobSlot
		gt.Error(t, s.Validate())
	})

	t.Run("negative index", func(t *testing.T) {
		s := validJobSlot(now)
		s.Index = -1
		gt.Error(t, s.Validate())
	})

	t.Run("empty holder id", func(t *testing.T) {
		s := validJobSlot(now)
		s.HolderID = ""
		gt.Error(t, s.Validate())
	})

	t.Run("empty workspace id", func(t *testing.T) {
		s := validJobSlot(now)
		s.WorkspaceID = ""
		gt.Error(t, s.Validate())
	})

	t.Run("zero case id", func(t *testing.T) {
		s := validJobSlot(now)
		s.CaseID = 0
		gt.Error(t, s.Validate())
	})

	t.Run("empty job id", func(t *testing.T) {
		s := validJobSlot(now)
		s.JobID = ""
		gt.Error(t, s.Validate())
	})

	t.Run("zero acquired_at", func(t *testing.T) {
		s := validJobSlot(now)
		s.AcquiredAt = time.Time{}
		gt.Error(t, s.Validate())
	})

	t.Run("expires_at not after acquired_at", func(t *testing.T) {
		s := validJobSlot(now)
		s.ExpiresAt = s.AcquiredAt
		gt.Error(t, s.Validate())
	})
}
