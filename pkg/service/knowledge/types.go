package knowledge

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Service defines the interface for knowledge extraction from source data
type Service interface {
	// Extract analyzes source data and extracts knowledge related to cases
	// It performs case relevance assessment, summary extraction, and embedding generation
	Extract(ctx context.Context, input Input) ([]Result, error)
}

// Input represents the input for knowledge extraction
type Input struct {
	Cases      []*model.Case
	SourceData SourceData
	Prompt     string // Custom prompt for LLM analysis (optional, uses default if empty)
}

// SourceData represents data from a source to be analyzed
type SourceData struct {
	SourceID   model.SourceID
	SourceURLs []string
	SourcedAt  time.Time
	Content    string // Markdown formatted text
	// Future: ImageData for multimodal support
}

// Result represents extracted knowledge for a specific case
type Result struct {
	CaseID    int64
	Title     string
	Summary   string
	Embedding []float32
}

// llmResponse is the structured output from the LLM
type llmResponse struct {
	// RelatedCases contains only cases that are related to the source content
	RelatedCases []relatedCase `json:"related_cases"`
}

// relatedCase represents a case that is related to the source content
type relatedCase struct {
	CaseID  int64  `json:"case_id"`
	Title   string `json:"title"`   // Title of the extracted insight
	Summary string `json:"summary"` // Summary including relevance to the case
}
