package graphql

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Case is a custom GraphQL model with WorkspaceID for argument-based propagation.
// WorkspaceID is not exposed in the GraphQL schema; it is used internally
// to pass workspace context to nested field resolvers (actions, knowledges, assignees).
type Case struct {
	ID             int              `json:"id"`
	WorkspaceID    string           `json:"-"`
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	Status         types.CaseStatus `json:"status"`
	IsPrivate      bool             `json:"isPrivate"`
	AccessDenied   bool             `json:"accessDenied"`
	ChannelUserIDs []string         `json:"-"` // Internal: used by channelUsers resolver
	ReporterID     *string          `json:"reporterID,omitempty"`
	AssigneeIDs    []string         `json:"assigneeIDs"`
	Assignees      []*SlackUser     `json:"assignees"`
	SlackChannelID *string          `json:"slackChannelID,omitempty"`
	SlackThreadTS  *string          `json:"slackThreadTS,omitempty"`
	IsThreadBound  bool             `json:"isThreadBound"`
	BoardStatus    *string          `json:"boardStatus,omitempty"`
	Fields         []*FieldValue    `json:"fields"`
	Actions        []*Action        `json:"actions"`
	// AgentAdditionalPrompt is the Markdown text appended to the agent
	// system prompt for this Case. Always present (empty string when
	// unset) so the schema's String! contract is honoured.
	AgentAdditionalPrompt string `json:"agentAdditionalPrompt"`
	// AgentSourceIDs is the internal-only allowlist used by the
	// agentSources resolver to hydrate the public [Source!]! field.
	// Hidden from JSON so it never leaks through the GraphQL surface.
	AgentSourceIDs []string  `json:"-"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Action is a custom GraphQL model with WorkspaceID for argument-based propagation.
type Action struct {
	ID             int        `json:"id"`
	WorkspaceID    string     `json:"-"`
	CaseID         int        `json:"caseID"`
	Case           *Case      `json:"case,omitempty"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	AssigneeID     *string    `json:"assigneeID,omitempty"`
	Assignee       *SlackUser `json:"assignee,omitempty"`
	SlackMessageTs *string    `json:"slackMessageTS,omitempty"`
	// Status is the per-workspace status id (no longer a typed enum). The
	// allowed value set is defined in TOML via [[action.status]] and exposed
	// to clients through FieldConfiguration.actionConfig.
	Status     string     `json:"status"`
	DueDate    *time.Time `json:"dueDate,omitempty"`
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Memo is a custom GraphQL model with WorkspaceID for argument-based propagation.
// WorkspaceID is not exposed in the GraphQL schema; it is used internally
// to pass workspace context to nested field resolvers (case, fields).
type Memo struct {
	ID          string        `json:"id"`
	WorkspaceID string        `json:"-"`
	CaseID      int           `json:"caseID"`
	Case        *Case         `json:"case,omitempty"`
	Title       string        `json:"title"`
	Fields      []*FieldValue `json:"fields"`
	ArchivedAt  *time.Time    `json:"archivedAt,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}
