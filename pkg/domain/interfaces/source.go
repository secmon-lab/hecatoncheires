package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// SourceRepository defines the interface for Source data persistence
type SourceRepository interface {
	// Create creates a new source
	Create(ctx context.Context, workspaceID string, source *model.Source) (*model.Source, error)

	// Get retrieves a source by ID
	Get(ctx context.Context, workspaceID string, id model.SourceID) (*model.Source, error)

	// List retrieves all sources
	List(ctx context.Context, workspaceID string) ([]*model.Source, error)

	// Update updates an existing source
	Update(ctx context.Context, workspaceID string, source *model.Source) (*model.Source, error)

	// Delete deletes a source by ID
	Delete(ctx context.Context, workspaceID string, id model.SourceID) error
}
