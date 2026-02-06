package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionRepository defines the interface for Action data access
type ActionRepository interface {
	// Create creates a new action with auto-generated ID
	Create(ctx context.Context, action *model.Action) (*model.Action, error)

	// Get retrieves an action by ID
	Get(ctx context.Context, id int64) (*model.Action, error)

	// List retrieves all actions
	List(ctx context.Context) ([]*model.Action, error)

	// Update updates an existing action
	Update(ctx context.Context, action *model.Action) (*model.Action, error)

	// Delete deletes an action by ID
	Delete(ctx context.Context, id int64) error

	// GetByCase retrieves all actions associated with a specific case
	GetByCase(ctx context.Context, caseID int64) ([]*model.Action, error)

	// GetByCases retrieves actions for multiple cases (for batch operations)
	// Returns a map of case ID to list of actions
	GetByCases(ctx context.Context, caseIDs []int64) (map[int64][]*model.Action, error)
}
