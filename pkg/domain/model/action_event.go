package model

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

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
