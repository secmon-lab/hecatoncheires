package model

import (
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// Workspace represents a workspace's identity
type Workspace struct {
	ID          string
	Name        string
	Description string // Human-readable description (e.g. for AI workspace estimation, UI tooltips)
	// Emoji is an optional display glyph for the workspace badge. Mutually
	// exclusive with Color (enforced at config load). Empty when unset.
	Emoji string
	// Color is an optional #RRGGBB hex used as the workspace badge background.
	// Mutually exclusive with Emoji (enforced at config load). Empty when unset.
	Color string
}

// ErrWorkspaceNotFound is returned when a workspace is not found in the registry
var ErrWorkspaceNotFound = goerr.New("workspace not found")

// WorkspaceEntry holds workspace identity and its field schema
type WorkspaceEntry struct {
	Workspace            Workspace
	FieldSchema          *config.FieldSchema
	ActionStatusSet      *ActionStatusSet
	SlackChannelPrefix   string
	SlackTeamID          string // Slack Team ID for org-level app support (empty for WS-level apps)
	SlackInviteUsers     []string
	SlackInviteGroups    []string
	SlackWelcomeMessages []string // Go text/template strings posted to the case channel after creation
	CompilePrompt        string
	AssistPrompt         string
	AssistLanguage       string
	Jobs                 []*Job // Event-driven agent jobs loaded from workspace TOML

	// CaseMode selects channel-per-case (default) or thread-per-case binding.
	CaseMode CaseMode
	// SlackMonitorChannelID is the channel watched for thread-mode case
	// creation. Required (and only meaningful) when CaseMode is thread.
	SlackMonitorChannelID string
	// CaseStatusSet is the configurable workflow status set that attaches to
	// Cases in thread mode (the Kanban columns). Non-nil only for thread-mode
	// workspaces; reuses the generic ActionStatusSet value type.
	CaseStatusSet *ActionStatusSet
}

// IsThreadMode reports whether this workspace uses thread-per-case binding.
func (e *WorkspaceEntry) IsThreadMode() bool {
	return e != nil && e.CaseMode.IsThread()
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

// FindByMonitorChannel returns the thread-mode workspace entry whose monitored
// channel matches channelID. It only considers thread-mode workspaces; the
// boolean is false when no thread-mode workspace watches the channel.
func (r *WorkspaceRegistry) FindByMonitorChannel(channelID string) (*WorkspaceEntry, bool) {
	if r == nil || channelID == "" {
		return nil, false
	}
	for _, id := range r.order {
		entry := r.entries[id]
		if entry.IsThreadMode() && entry.SlackMonitorChannelID == channelID {
			return entry, true
		}
	}
	return nil, false
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
