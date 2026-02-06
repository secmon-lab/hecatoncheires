package model

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Action represents a task/action associated with a case
type Action struct {
	ID             int64
	CaseID         int64 // Required: Action must be associated with a Case (1:n relationship)
	Title          string
	Description    string
	AssigneeIDs    []string // Slack User IDs
	SlackMessageTS string   // Optional: Slack message ID (timestamp)
	Status         types.ActionStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
