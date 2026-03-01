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
	IsPrivate      bool                  // Private mode flag
	ChannelUserIDs []string              // Slack User IDs of channel members (synced for all cases)
	AccessDenied   bool                  // Runtime-only: set by UseCase when access is restricted (not persisted)
	FieldValues    map[string]FieldValue // key = FieldID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsCaseAccessible checks if a user has access to a case.
// Non-private cases are always accessible.
// Private cases are accessible only if the userID is in ChannelUserIDs.
func IsCaseAccessible(c *Case, userID string) bool {
	if !c.IsPrivate {
		return true
	}
	for _, id := range c.ChannelUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// RestrictCase returns a copy of the case with sensitive fields removed.
// Only ID, Status, IsPrivate, CreatedAt, UpdatedAt are preserved.
// AccessDenied is set to true.
func RestrictCase(c *Case) *Case {
	return &Case{
		ID:           c.ID,
		Status:       c.Status,
		IsPrivate:    c.IsPrivate,
		AccessDenied: true,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}
