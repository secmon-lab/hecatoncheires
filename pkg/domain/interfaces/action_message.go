package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

// ActionMessageRepository defines the interface for action-scoped Slack message persistence.
// These are messages posted into the Slack thread of an Action's notification message.
type ActionMessageRepository interface {
	// Put saves a Slack message under a specific action (upsert)
	Put(ctx context.Context, workspaceID string, actionID int64, msg *slack.Message) error

	// List retrieves messages for a specific action with pagination
	// Returns messages in descending order (newest first)
	List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*slack.Message, string, error)
}
