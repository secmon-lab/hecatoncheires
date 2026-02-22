package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// AssistLogRepository defines the interface for AssistLog data persistence
type AssistLogRepository interface {
	// Create creates a new assist log entry
	Create(ctx context.Context, workspaceID string, caseID int64, log *model.AssistLog) (*model.AssistLog, error)

	// List retrieves assist log entries for a specific case with pagination.
	// Returns items, totalCount, and error. Items are ordered by CreatedAt descending.
	List(ctx context.Context, workspaceID string, caseID int64, limit, offset int) ([]*model.AssistLog, int, error)
}
