package model

import "github.com/m-mizutani/goerr/v2"

// WorkspaceGroup is a deployment-wide, organizational grouping of workspaces.
// It is an orthogonal concept: membership never changes a workspace's own
// behavior, and a deployment may define zero groups. It is loaded from the
// global config (see pkg/cli/config) and held read-only in memory, exactly
// like WorkspaceEntry.
type WorkspaceGroup struct {
	ID          string
	Name        string
	Description string
	// MemberIDs are workspace IDs in declared order. May be empty. Existence of
	// each ID in the workspace set is verified at config load, not here.
	MemberIDs []string
}

// ErrWorkspaceGroupNotFound is returned when a group is not found in the registry.
var ErrWorkspaceGroupNotFound = goerr.New("workspace group not found")

// ErrInvalidWorkspaceGroup is returned when a group violates its core invariant
// (an empty ID). Richer validation (ID pattern, cross-file duplicate IDs,
// member existence) lives in the config layer; this is the last-line guard so a
// code path that builds a group with no identity fails loudly before Register.
var ErrInvalidWorkspaceGroup = goerr.New("invalid workspace group")

// Validate enforces the group's core identity invariant: a non-empty ID.
func (g *WorkspaceGroup) Validate() error {
	if g.ID == "" {
		return goerr.Wrap(ErrInvalidWorkspaceGroup, "workspace group ID is required")
	}
	return nil
}

// WorkspaceGroupRegistry holds workspace group configurations. Like
// WorkspaceRegistry it carries settings only (no Repository / UseCase).
type WorkspaceGroupRegistry struct {
	entries map[string]*WorkspaceGroup
	order   []string // preserves registration order
}

// NewWorkspaceGroupRegistry creates a new empty WorkspaceGroupRegistry.
func NewWorkspaceGroupRegistry() *WorkspaceGroupRegistry {
	return &WorkspaceGroupRegistry{
		entries: make(map[string]*WorkspaceGroup),
	}
}

// Register adds a group to the registry. A repeated ID overwrites the existing
// entry while preserving its position in the registration order.
func (r *WorkspaceGroupRegistry) Register(g *WorkspaceGroup) {
	if _, exists := r.entries[g.ID]; !exists {
		r.order = append(r.order, g.ID)
	}
	r.entries[g.ID] = g
}

// Get retrieves a group by ID.
func (r *WorkspaceGroupRegistry) Get(id string) (*WorkspaceGroup, error) {
	g, ok := r.entries[id]
	if !ok {
		return nil, goerr.Wrap(ErrWorkspaceGroupNotFound, "workspace group not found",
			goerr.V("workspace_group_id", id))
	}
	return g, nil
}

// List returns all registered groups in registration order. It never returns
// nil, so callers (and the GraphQL non-null list contract) can range freely.
func (r *WorkspaceGroupRegistry) List() []*WorkspaceGroup {
	result := make([]*WorkspaceGroup, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.entries[id])
	}
	return result
}
