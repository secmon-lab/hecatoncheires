package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// ErrAssistLogValidation is returned when an AssistLog fails its persistence-boundary invariants.
var ErrAssistLogValidation = goerr.New("assist log validation failed")

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

// Validate enforces the invariants required before any persistence write.
// The repository assigns the storage ID (NewAssistLogID when empty) and sets
// CaseID from its caseID argument, so callers MUST invoke this after that
// assignment; a log with no owning Case (CaseID == 0) fails loudly here
// instead of landing in storage.
func (l *AssistLog) Validate() error {
	if l == nil {
		return goerr.Wrap(ErrAssistLogValidation, "assist log is nil")
	}
	if l.CaseID == 0 {
		return goerr.Wrap(ErrAssistLogValidation, "assist log CaseID is required")
	}
	return nil
}
