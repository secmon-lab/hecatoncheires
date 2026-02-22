package model

import (
	"time"

	"github.com/google/uuid"
)

// MemoryID is a UUID-based identifier for Memory
type MemoryID string

// NewMemoryID generates a new UUID v4 MemoryID
func NewMemoryID() MemoryID {
	return MemoryID(uuid.New().String())
}

// Memory represents a persistent memory entry associated with a case.
// Memories are used by the assist agent to track facts, observations,
// and follow-up items across multiple execution sessions.
type Memory struct {
	ID        MemoryID
	CaseID    int64
	Claim     string    // The fact or note to remember
	Embedding []float32 // Vector embedding for similarity search (768 dimensions)
	CreatedAt time.Time
}
