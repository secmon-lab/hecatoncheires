package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrActionStepValidation is returned when an ActionStep fails its persistence-boundary invariants.
var ErrActionStepValidation = goerr.New("action step validation failed")

// ActionStep is a small, binary-state work item that lives under an Action.
// Used to track granular progress toward completing an Action. The completion
// state is encoded by DoneAt: nil means ongoing, non-nil means done at the
// given time. There is no separate Done boolean to avoid the two-source-of-
// truth hazard that the Action.ArchivedAt / IsArchived pattern already
// established for archive state.
type ActionStep struct {
	ID        string
	ActionID  int64
	Title     string
	DoneAt    *time.Time
	DoneBy    string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsDone reports whether the step is currently marked done.
func (s *ActionStep) IsDone() bool {
	return s != nil && s.DoneAt != nil
}

// Validate enforces the invariants required before any persistence write.
// The caller supplies both the step ID and the parent ActionID (the latter is
// the repository storage key), so a step with no parent or no ID fails loudly
// here instead of landing in storage.
func (s *ActionStep) Validate() error {
	if s == nil {
		return goerr.Wrap(ErrActionStepValidation, "action step is nil")
	}
	if s.ID == "" {
		return goerr.Wrap(ErrActionStepValidation, "action step ID is required")
	}
	if s.ActionID == 0 {
		return goerr.Wrap(ErrActionStepValidation, "action step ActionID is required")
	}
	return nil
}
