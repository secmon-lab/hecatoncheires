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
	AssigneeIDs    []string         `json:"assigneeIDs"`
	Assignees      []*SlackUser     `json:"assignees"`
	SlackChannelID *string          `json:"slackChannelID,omitempty"`
	Fields         []*FieldValue    `json:"fields"`
	Actions        []*Action        `json:"actions"`
	Knowledges     []*Knowledge     `json:"knowledges"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// Action is a custom GraphQL model with WorkspaceID for argument-based propagation.
type Action struct {
	ID             int                `json:"id"`
	WorkspaceID    string             `json:"-"`
	CaseID         int                `json:"caseID"`
	Case           *Case              `json:"case,omitempty"`
	Title          string             `json:"title"`
	Description    string             `json:"description"`
	AssigneeIDs    []string           `json:"assigneeIDs"`
	Assignees      []*SlackUser       `json:"assignees"`
	SlackMessageTs *string            `json:"slackMessageTS,omitempty"`
	Status         types.ActionStatus `json:"status"`
	DueDate        *time.Time         `json:"dueDate,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

// Knowledge is a custom GraphQL model with WorkspaceID for argument-based propagation.
type Knowledge struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"-"`
	CaseID      int       `json:"caseID"`
	Case        *Case     `json:"case,omitempty"`
	SourceID    string    `json:"sourceID"`
	SourceURLs  []string  `json:"sourceURLs"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	SourcedAt   time.Time `json:"sourcedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
