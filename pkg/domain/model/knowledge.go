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

// Knowledge represents extracted knowledge from source data related to a risk
type Knowledge struct {
	ID        KnowledgeID
	RiskID    int64    // Related Risk ID (if one source relates to multiple risks, create separate Knowledge for each)
	SourceID  SourceID // Source ID where data was retrieved
	SourceURL string   // Direct URL to the source (e.g., Notion page)
	Title     string   // Title of the extracted insight
	Summary   string   // Summarized content
	Embedding []float32
	SourcedAt time.Time // Timestamp of the source data
	CreatedAt time.Time
	UpdatedAt time.Time
}
