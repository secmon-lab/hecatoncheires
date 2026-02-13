package interfaces

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

// CaseMessageRepository defines the interface for case-scoped Slack message persistence
type CaseMessageRepository interface {
	// Put saves a Slack message under a specific case (upsert)
	Put(ctx context.Context, workspaceID string, caseID int64, msg *slack.Message) error

	// List retrieves messages for a specific case with pagination
	// Returns messages in descending order (newest first)
	List(ctx context.Context, workspaceID string, caseID int64, limit int, cursor string) ([]*slack.Message, string, error)

	// Prune deletes messages older than the specified time for a specific case
	// Returns the number of messages deleted
	Prune(ctx context.Context, workspaceID string, caseID int64, before time.Time) (int, error)
}
