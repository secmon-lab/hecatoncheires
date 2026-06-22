package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionArchiveScope selects which slice of an action list to return.
type ActionArchiveScope int

const (
	// ActionArchiveScopeActiveOnly returns only non-archived actions.
	// This is the zero value and the default behaviour.
	ActionArchiveScopeActiveOnly ActionArchiveScope = iota
	// ActionArchiveScopeArchivedOnly returns only archived actions.
	ActionArchiveScopeArchivedOnly
	// ActionArchiveScopeAll returns both active and archived actions.
	ActionArchiveScopeAll
)

// Allows reports whether an action with the given archived state passes
// this scope's filter.
func (s ActionArchiveScope) Allows(isArchived bool) bool {
	switch s {
	case ActionArchiveScopeArchivedOnly:
		return isArchived
	case ActionArchiveScopeAll:
		return true
	default: // ActionArchiveScopeActiveOnly
		return !isArchived
	}
}

// ActionListOptions controls how List / GetByCase / GetByCases filter actions.
type ActionListOptions struct {
	// ArchiveScope selects active / archived / both. Defaults to active only.
	ArchiveScope ActionArchiveScope

	// ExcludePrivateCaseActions, when true, drops every action whose parent
	// Case is private — unconditionally, regardless of channel membership.
	// This is stricter than the membership-based access control applied when
	// an auth token is present: it is the policy for entry points (such as
	// the MCP endpoint) where private Cases and their Actions must never be
	// exposed, not even to members. Defaults to false so existing callers
	// keep the membership-based behaviour.
	ExcludePrivateCaseActions bool
}

// ActionRepository defines the interface for Action data access
type ActionRepository interface {
	// Create creates a new action with auto-generated ID
	Create(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)

	// Get retrieves an action by ID. Archived actions are returned as-is so
	// callers can inspect history; UI/agent layers must enforce visibility.
	Get(ctx context.Context, workspaceID string, id int64) (*model.Action, error)

	// GetByIDs retrieves multiple actions by IDs in a single batch.
	// Returns a map keyed by action ID containing only the actions that
	// were found; missing IDs are silently absent from the result map.
	// Archived actions are included for the same reason Get returns
	// them: the GraphQL Action loader is used from sub-resolvers that
	// already need history visibility.
	GetByIDs(ctx context.Context, workspaceID string, ids []int64) (map[int64]*model.Action, error)

	// List retrieves all actions filtered by opts.ArchiveScope.
	List(ctx context.Context, workspaceID string, opts ActionListOptions) ([]*model.Action, error)

	// Update updates an existing action
	Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error)

	// Delete permanently removes an action document. This is INTERNAL ONLY:
	// callers are limited to the Case-deletion cascade in the usecase layer,
	// because the public Action lifecycle no longer exposes deletion. Use
	// ArchiveAction at the usecase layer for user-facing removal.
	Delete(ctx context.Context, workspaceID string, id int64) error

	// GetByCase retrieves all actions associated with a specific case,
	// filtered by opts.ArchiveScope.
	GetByCase(ctx context.Context, workspaceID string, caseID int64, opts ActionListOptions) ([]*model.Action, error)

	// GetByCases retrieves actions for multiple cases (for batch operations).
	// Returns a map of case ID to list of actions, filtered by
	// opts.ArchiveScope.
	GetByCases(ctx context.Context, workspaceID string, caseIDs []int64, opts ActionListOptions) (map[int64][]*model.Action, error)

	// GetBySlackMessageTS retrieves an action by its Slack message timestamp.
	// Returns ErrNotFound if no action matches. Archived actions ARE returned
	// because Slack threads must be resolvable regardless of archive state.
	GetBySlackMessageTS(ctx context.Context, workspaceID string, ts string) (*model.Action, error)
}
