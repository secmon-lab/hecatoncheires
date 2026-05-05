package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionListOptions controls how List / GetByCase / GetByCases filter actions.
type ActionListOptions struct {
	// IncludeArchived determines whether archived actions are included.
	// Default zero value (false) means archived actions are filtered out.
	IncludeArchived bool
}

// ActionRepository defines the interface for Action data access
type ActionRepository interface {
	// Create creates a new action with auto-generated ID
	Create(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)

	// Get retrieves an action by ID. Archived actions are returned as-is so
	// callers can inspect history; UI/agent layers must enforce visibility.
	Get(ctx context.Context, workspaceID string, id int64) (*model.Action, error)

	// List retrieves all actions. Archived actions are excluded by default
	// unless opts.IncludeArchived is true.
	List(ctx context.Context, workspaceID string, opts ActionListOptions) ([]*model.Action, error)

	// Update updates an existing action
	Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)

	// Delete permanently removes an action document. This is INTERNAL ONLY:
	// callers are limited to the Case-deletion cascade in the usecase layer,
	// because the public Action lifecycle no longer exposes deletion. Use
	// ArchiveAction at the usecase layer for user-facing removal.
	Delete(ctx context.Context, workspaceID string, id int64) error

	// GetByCase retrieves all actions associated with a specific case.
	// Archived actions are excluded by default unless opts.IncludeArchived
	// is true.
	GetByCase(ctx context.Context, workspaceID string, caseID int64, opts ActionListOptions) ([]*model.Action, error)

	// GetByCases retrieves actions for multiple cases (for batch operations).
	// Returns a map of case ID to list of actions. Archived actions are
	// excluded by default unless opts.IncludeArchived is true.
	GetByCases(ctx context.Context, workspaceID string, caseIDs []int64, opts ActionListOptions) (map[int64][]*model.Action, error)

	// GetBySlackMessageTS retrieves an action by its Slack message timestamp.
	// Returns ErrNotFound if no action matches. Archived actions ARE returned
	// because Slack threads must be resolvable regardless of archive state.
	GetBySlackMessageTS(ctx context.Context, workspaceID string, ts string) (*model.Action, error)
}
