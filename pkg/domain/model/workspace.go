package model

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// Workspace represents a workspace's identity
type Workspace struct {
	ID   string
	Name string
}

// ErrWorkspaceNotFound is returned when a workspace is not found in the registry
var ErrWorkspaceNotFound = goerr.New("workspace not found")

// WorkspaceEntry holds workspace identity and its field schema
type WorkspaceEntry struct {
	Workspace          Workspace
	FieldSchema        *config.FieldSchema
	SlackChannelPrefix string
}

// WorkspaceRegistry holds workspace configurations.
// It does not hold Repository or UseCase instances (settings only).
type WorkspaceRegistry struct {
	entries map[string]*WorkspaceEntry
	order   []string // preserves registration order
}

// NewWorkspaceRegistry creates a new empty WorkspaceRegistry
func NewWorkspaceRegistry() *WorkspaceRegistry {
	return &WorkspaceRegistry{
		entries: make(map[string]*WorkspaceEntry),
	}
}

// Register adds a workspace entry to the registry
func (r *WorkspaceRegistry) Register(entry *WorkspaceEntry) {
	if _, exists := r.entries[entry.Workspace.ID]; !exists {
		r.order = append(r.order, entry.Workspace.ID)
	}
	r.entries[entry.Workspace.ID] = entry
}

// Get retrieves a workspace entry by ID
func (r *WorkspaceRegistry) Get(workspaceID string) (*WorkspaceEntry, error) {
	entry, ok := r.entries[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrWorkspaceNotFound, "workspace not found",
			goerr.V("workspace_id", workspaceID))
	}
	return entry, nil
}

// List returns all registered workspace entries in registration order
func (r *WorkspaceRegistry) List() []*WorkspaceEntry {
	result := make([]*WorkspaceEntry, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.entries[id])
	}
	return result
}

// Workspaces returns all registered workspaces in registration order
func (r *WorkspaceRegistry) Workspaces() []Workspace {
	result := make([]Workspace, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, r.entries[id].Workspace)
	}
	return result
}
