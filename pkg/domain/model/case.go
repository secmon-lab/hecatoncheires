package model

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// Case represents a generic case/project entity
type Case struct {
	ID             int64
	Title          string
	Description    string
	Status         types.CaseStatus
	AssigneeIDs    []string // Slack User IDs
	SlackChannelID string
	FieldValues    map[string]FieldValue // key = FieldID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
