package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// KnowledgeRepository defines the interface for Knowledge data persistence
type KnowledgeRepository interface {
	// Create creates a new knowledge entry
	Create(ctx context.Context, knowledge *model.Knowledge) (*model.Knowledge, error)

	// Get retrieves a knowledge entry by ID
	Get(ctx context.Context, id model.KnowledgeID) (*model.Knowledge, error)

	// ListByRiskID retrieves all knowledge entries for a specific risk
	ListByRiskID(ctx context.Context, riskID int64) ([]*model.Knowledge, error)

	// ListBySourceID retrieves all knowledge entries from a specific source
	ListBySourceID(ctx context.Context, sourceID model.SourceID) ([]*model.Knowledge, error)

	// ListWithPagination retrieves knowledge entries with pagination
	// Returns knowledges, total count, and error
	ListWithPagination(ctx context.Context, limit, offset int) ([]*model.Knowledge, int, error)

	// Delete deletes a knowledge entry by ID
	Delete(ctx context.Context, id model.KnowledgeID) error
}
