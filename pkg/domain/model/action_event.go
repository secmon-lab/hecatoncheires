package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ErrActionEventValidation is returned when an ActionEvent fails its persistence-boundary invariants.
var ErrActionEventValidation = goerr.New("action event validation failed")

// ActionEvent is one entry in an Action's structural change history. Used
// to render the WebUI activity feed (creation, title / status / assignee
// edits). Messages-from-Slack go through ActionMessageRepository instead.
type ActionEvent struct {
	ID        string                // unique within the action
	ActionID  int64                 // parent action id
	Kind      types.ActionEventKind // CREATED / TITLE_CHANGED / STATUS_CHANGED / ASSIGNEE_CHANGED
	ActorID   string                // Slack user id of the person who triggered the change ("" = system)
	OldValue  string                // old field value rendered as a string (empty for CREATED)
	NewValue  string                // new field value rendered as a string
	CreatedAt time.Time
}

// Validate enforces the invariants required before any persistence write.
// The caller supplies both the entry ID (unique within the action) and the
// parent ActionID, so a history entry with no parent or no ID fails loudly
// here instead of landing in storage.
func (e *ActionEvent) Validate() error {
	if e == nil {
		return goerr.Wrap(ErrActionEventValidation, "action event is nil")
	}
	if e.ID == "" {
		return goerr.Wrap(ErrActionEventValidation, "action event ID is required")
	}
	if e.ActionID == 0 {
		return goerr.Wrap(ErrActionEventValidation, "action event ActionID is required")
	}
	return nil
}
