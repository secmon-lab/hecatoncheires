package knowledge

import (
	"context"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Service defines the interface for knowledge extraction from source data
type Service interface {
	// Extract analyzes source data and extracts knowledge related to risks
	// It performs risk relevance assessment, summary extraction, and embedding generation
	Extract(ctx context.Context, input Input) ([]Result, error)
}

// Input represents the input for knowledge extraction
type Input struct {
	Risks      []*model.Risk
	SourceData SourceData
}

// SourceData represents data from a source to be analyzed
type SourceData struct {
	SourceID  model.SourceID
	SourceURL string
	SourcedAt time.Time
	Content   string // Markdown formatted text
	// Future: ImageData for multimodal support
}

// Result represents extracted knowledge for a specific risk
type Result struct {
	RiskID    int64
	Title     string
	Summary   string
	Embedding []float32
}

// llmResponse is the structured output from the LLM
type llmResponse struct {
	// RelatedRisks contains only risks that are related to the source content
	RelatedRisks []relatedRisk `json:"related_risks"`
}

// relatedRisk represents a risk that is related to the source content
type relatedRisk struct {
	RiskID  int64  `json:"risk_id"`
	Title   string `json:"title"`   // Title of the extracted insight
	Summary string `json:"summary"` // Summary including relevance to the risk
}
