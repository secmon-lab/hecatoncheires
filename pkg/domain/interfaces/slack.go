package interfaces

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

// SlackRepository defines the interface for Slack message persistence
type SlackRepository interface {
	// PutMessage saves a Slack message (upsert)
	PutMessage(ctx context.Context, msg *slack.Message) error

	// ListMessages retrieves messages from a specific channel within a time range
	// Returns messages in descending order (newest first) with pagination support
	ListMessages(ctx context.Context, channelID string, start, end time.Time, limit int, cursor string) ([]*slack.Message, string, error)

	// PruneMessages deletes messages older than the specified time
	// If channelID is empty, deletes from all channels
	// Returns the number of messages deleted
	PruneMessages(ctx context.Context, channelID string, before time.Time) (int, error)
}
