package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ErrActionValidation is returned when an Action fails its persistence-boundary invariants.
var ErrActionValidation = goerr.New("action validation failed")

// Action represents a task/action associated with a case
type Action struct {
	ID             int64
	CaseID         int64 // Required: Action must be associated with a Case (1:n relationship)
	Title          string
	Description    string
	AssigneeID     string // Slack User ID; empty string means unassigned
	SlackMessageTS string // Optional: Slack message ID (timestamp)
	Status         types.ActionStatus
	DueDate        *time.Time // Optional: deadline for the action
	ArchivedAt     *time.Time // nil = active; non-nil = archived at the given time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsArchived reports whether the action is currently archived.
func (a *Action) IsArchived() bool {
	return a != nil && a.ArchivedAt != nil
}

// Validate enforces the invariants required before any persistence write.
// The repository assigns the storage ID; the caller sets CaseID (the 1:n link
// to the owning Case) before the write, so an orphan Action (CaseID == 0)
// fails loudly here instead of landing in storage.
func (a *Action) Validate() error {
	if a == nil {
		return goerr.Wrap(ErrActionValidation, "action is nil")
	}
	if a.CaseID == 0 {
		return goerr.Wrap(ErrActionValidation, "action CaseID is required")
	}
	return nil
}
