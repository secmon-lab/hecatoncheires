package model

import (
	"time"

	"github.com/google/uuid"
)

// AssistLogID is a UUID-based identifier for AssistLog
type AssistLogID string

// NewAssistLogID generates a new UUID v4 AssistLogID
func NewAssistLogID() AssistLogID {
	return AssistLogID(uuid.New().String())
}

// AssistLog represents an execution log entry from the assist agent.
// After each assist session, the agent's actions are summarized and
// stored as a log for context in subsequent runs.
type AssistLog struct {
	ID        AssistLogID
	CaseID    int64
	Summary   string // One-line summary of this session
	Actions   string // What was done in this session (may be empty if nothing was done)
	Reasoning string // Rationale behind decisions made
	NextSteps string // Items to address in future sessions
	CreatedAt time.Time
}
