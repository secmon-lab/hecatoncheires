package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// KnowledgeRepository defines the interface for Knowledge data persistence
type KnowledgeRepository interface {
	// Create creates a new knowledge entry
	Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error)

	// Get retrieves a knowledge entry by ID
	Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error)

	// ListByCaseID retrieves all knowledge entries for a specific case
	ListByCaseID(ctx context.Context, workspaceID string, caseID int64) ([]*model.Knowledge, error)

	// ListByCaseIDs retrieves all knowledge entries for multiple cases
	// Implementation uses parallel individual queries to avoid requiring new Firestore indexes
	ListByCaseIDs(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Knowledge, error)

	// ListBySourceID retrieves all knowledge entries from a specific source
	ListBySourceID(ctx context.Context, workspaceID string, sourceID model.SourceID) ([]*model.Knowledge, error)

	// ListWithPagination retrieves knowledge entries with pagination
	// Returns knowledges, total count, and error
	ListWithPagination(ctx context.Context, workspaceID string, limit, offset int) ([]*model.Knowledge, int, error)

	// Delete deletes a knowledge entry by ID
	Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error

	// FindByEmbedding performs vector similarity search using cosine distance.
	// Returns up to limit Knowledge entries most similar to the given embedding.
	FindByEmbedding(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error)
}
