package model

import "time"

// Case represents a generic case/project entity
type Case struct {
	ID             int64
	Title          string
	Description    string
	AssigneeIDs    []string // Slack User IDs
	SlackChannelID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
