package model

import "time"

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
