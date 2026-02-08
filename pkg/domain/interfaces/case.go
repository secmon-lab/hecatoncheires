package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// CaseRepository defines the interface for Case data access
type CaseRepository interface {
	// Create creates a new case with auto-generated ID
	Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// Get retrieves a case by ID
	Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error)

	// List retrieves all cases
	List(ctx context.Context, workspaceID string) ([]*model.Case, error)

	// Update updates an existing case
	Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error)

	// Delete deletes a case by ID
	Delete(ctx context.Context, workspaceID string, id int64) error
}
