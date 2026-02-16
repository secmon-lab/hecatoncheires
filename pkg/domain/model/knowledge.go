package model

import (
	"time"

	"github.com/google/uuid"
)

// EmbeddingDimension is the dimension of the embedding vector
// Gemini text-embedding-004 uses 768 dimensions
const EmbeddingDimension = 768

// KnowledgeID is a UUID-based identifier for Knowledge
type KnowledgeID string

// NewKnowledgeID generates a new UUID v4 KnowledgeID
func NewKnowledgeID() KnowledgeID {
	return KnowledgeID(uuid.New().String())
}

// Knowledge represents extracted knowledge from source data related to a case
type Knowledge struct {
	ID         KnowledgeID
	CaseID     int64    // Related Case ID (if one source relates to multiple cases, create separate Knowledge for each)
	SourceID   SourceID // Source ID where data was retrieved
	SourceURLs []string // Direct URLs to the source (e.g., Notion pages)
	Title      string   // Title of the extracted insight
	Summary    string   // Summarized content
	Embedding  []float32
	SourcedAt  time.Time // Timestamp of the source data
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
