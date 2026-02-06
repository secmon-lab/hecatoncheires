package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// client implements Service interface
type client struct {
	llmClient gollem.LLMClient
}

// Option is a functional option for client configuration
type Option func(*client)

// New creates a new Knowledge service with the provided LLM client
func New(llmClient gollem.LLMClient, opts ...Option) (Service, error) {
	if llmClient == nil {
		return nil, goerr.New("LLM client is required")
	}

	c := &client{
		llmClient: llmClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Extract analyzes source data and extracts knowledge related to cases
func (c *client) Extract(ctx context.Context, input Input) ([]Result, error) {
	if len(input.Cases) == 0 {
		return nil, nil
	}

	// Build the prompt for LLM
	prompt := c.buildPrompt(input)

	// Build the response schema
	schema := c.buildResponseSchema()

	// Create session with JSON response type
	session, err := c.llmClient.NewSession(ctx,
		gollem.WithSessionContentType(gollem.ContentTypeJSON),
		gollem.WithSessionResponseSchema(schema),
	)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create LLM session")
	}

	// Generate content
	resp, err := session.GenerateContent(ctx, gollem.Text(prompt))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to generate content from LLM")
	}

	// Parse response
	var llmResp llmResponse
	if err := json.Unmarshal([]byte(resp.Texts[0]), &llmResp); err != nil {
		return nil, goerr.Wrap(err, "failed to parse LLM response", goerr.V("response", resp.Texts[0]))
	}

	// If no related risks, return empty
	if len(llmResp.RelatedCases) == 0 {
		return nil, nil
	}

	// Generate embeddings for each result
	results := make([]Result, 0, len(llmResp.RelatedCases))
	for _, related := range llmResp.RelatedCases {
		// Generate embedding for the summary
		embedding, err := c.generateEmbedding(ctx, related.Summary)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to generate embedding",
				goerr.V("caseID", related.CaseID),
				goerr.V("title", related.Title))
		}

		results = append(results, Result{
			CaseID:    related.CaseID,
			Title:     related.Title,
			Summary:   related.Summary,
			Embedding: embedding,
		})
	}

	return results, nil
}

// buildPrompt creates the prompt for LLM analysis
func (c *client) buildPrompt(input Input) string {
	var sb strings.Builder

	sb.WriteString("Analyze the following source content and determine which cases (if any) are related to it.\n\n")
	sb.WriteString("## Cases to consider:\n\n")

	for _, caseItem := range input.Cases {
		sb.WriteString(fmt.Sprintf("### Case ID: %d\n", caseItem.ID))
		sb.WriteString(fmt.Sprintf("**Title:** %s\n", caseItem.Title))
		if caseItem.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description:** %s\n", caseItem.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Source Content:\n\n")
	sb.WriteString(input.SourceData.Content)
	sb.WriteString("\n\n")

	sb.WriteString("## Instructions:\n\n")
	sb.WriteString("1. Analyze the source content and identify any relevant information for each case.\n")
	sb.WriteString("2. For each related case, provide:\n")
	sb.WriteString("   - case_id: The ID of the related case\n")
	sb.WriteString("   - title: A concise title for the extracted knowledge (in the same language as the source content)\n")
	sb.WriteString("   - summary: A brief summary of how the source content relates to the case (in the same language as the source content)\n")
	sb.WriteString("3. Only include cases that have clear relevance to the source content.\n")
	sb.WriteString("4. If no cases are related, return an empty array.\n")

	return sb.String()
}

// buildResponseSchema creates the JSON schema for structured output
func (c *client) buildResponseSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Title:       "KnowledgeExtractionResponse",
		Description: "Response containing cases related to the source content",
		Type:        gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"related_cases": {
				Type:        gollem.TypeArray,
				Description: "List of cases that are related to the source content",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"case_id": {
							Type:        gollem.TypeInteger,
							Description: "The ID of the related case",
						},
						"title": {
							Type:        gollem.TypeString,
							Description: "A concise title for the extracted knowledge",
						},
						"summary": {
							Type:        gollem.TypeString,
							Description: "A brief summary of how the source content relates to the case",
						},
					},
					Required: []string{"case_id", "title", "summary"},
				},
			},
		},
		Required: []string{"related_cases"},
	}
}

// generateEmbedding generates an embedding vector for the given text
func (c *client) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := c.llmClient.GenerateEmbedding(ctx, model.EmbeddingDimension, []string{text})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to generate embedding")
	}

	if len(embeddings) == 0 {
		return nil, goerr.New("no embedding returned")
	}

	// Convert float64 to float32
	result := make([]float32, len(embeddings[0]))
	for i, v := range embeddings[0] {
		result[i] = float32(v)
	}

	return result, nil
}
