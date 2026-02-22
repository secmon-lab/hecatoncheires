package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// MemoryRepository defines the interface for Memory data persistence
type MemoryRepository interface {
	// Create creates a new memory entry
	Create(ctx context.Context, workspaceID string, caseID int64, memory *model.Memory) (*model.Memory, error)

	// Get retrieves a memory entry by ID
	Get(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) (*model.Memory, error)

	// Delete deletes a memory entry by ID
	Delete(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) error

	// List retrieves all memory entries for a specific case
	List(ctx context.Context, workspaceID string, caseID int64) ([]*model.Memory, error)

	// FindByEmbedding performs vector similarity search using cosine distance.
	// Returns up to limit Memory entries most similar to the given embedding.
	FindByEmbedding(ctx context.Context, workspaceID string, caseID int64, embedding []float32, limit int) ([]*model.Memory, error)
}
