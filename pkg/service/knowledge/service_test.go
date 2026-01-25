package knowledge_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
)

func TestExtract_WithRealGemini(t *testing.T) {
	projectID := os.Getenv("TEST_GEMINI_PROJECT")
	if projectID == "" {
		t.Skip("TEST_GEMINI_PROJECT not set")
	}

	location := os.Getenv("TEST_GEMINI_LOCATION")
	if location == "" {
		t.Skip("TEST_GEMINI_LOCATION not set")
	}

	ctx := context.Background()

	// Create Gemini client
	llmClient, err := gemini.New(ctx, projectID, location)
	if err != nil {
		t.Fatalf("failed to create Gemini client: %v", err)
	}

	// Create knowledge service
	svc, err := knowledge.New(llmClient)
	if err != nil {
		t.Fatalf("failed to create knowledge service: %v", err)
	}

	t.Run("Extract returns related knowledge", func(t *testing.T) {
		risks := []*model.Risk{
			{
				ID:                  1,
				Name:                "Security Vulnerability",
				Description:         "Risks related to security vulnerabilities in software",
				DetectionIndicators: "CVE mentions, security patches, vulnerability reports",
			},
			{
				ID:                  2,
				Name:                "Data Privacy",
				Description:         "Risks related to data privacy and GDPR compliance",
				DetectionIndicators: "Personal data handling, consent management, data breaches",
			},
		}

		sourceData := knowledge.SourceData{
			SourceID:  model.NewSourceID(),
			SourceURL: "https://example.com/security-update",
			SourcedAt: time.Now().UTC(),
			Content: `# Security Update Bulletin

## CVE-2024-1234 - Critical SQL Injection Vulnerability

A critical SQL injection vulnerability has been discovered in our authentication module.
This vulnerability allows unauthenticated attackers to bypass authentication and gain admin access.

### Affected Versions
- v2.0.0 to v2.3.5

### Remediation
Please update to v2.3.6 or later immediately.

### Timeline
- Discovery: 2024-01-10
- Patch released: 2024-01-15
`,
		}

		input := knowledge.Input{
			Risks:      risks,
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		if err != nil {
			t.Fatalf("failed to extract knowledge: %v", err)
		}

		if len(results) == 0 {
			t.Error("expected at least one result for security-related content")
		}

		// The security vulnerability risk should be identified
		foundSecurityRisk := false
		for _, result := range results {
			if result.RiskID == 1 {
				foundSecurityRisk = true
				if result.Title == "" {
					t.Error("expected non-empty title")
				}
				if result.Summary == "" {
					t.Error("expected non-empty summary")
				}
				if len(result.Embedding) != model.EmbeddingDimension {
					t.Errorf("expected embedding dimension %d, got %d", model.EmbeddingDimension, len(result.Embedding))
				}
			}
		}

		if !foundSecurityRisk {
			t.Error("expected security vulnerability risk to be identified")
		}
	})

	t.Run("Extract returns empty for unrelated content", func(t *testing.T) {
		risks := []*model.Risk{
			{
				ID:                  1,
				Name:                "Security Vulnerability",
				Description:         "Risks related to security vulnerabilities",
				DetectionIndicators: "CVE mentions, security patches",
			},
		}

		sourceData := knowledge.SourceData{
			SourceID:  model.NewSourceID(),
			SourceURL: "https://example.com/recipe",
			SourcedAt: time.Now().UTC(),
			Content: `# Grandma's Chocolate Chip Cookies Recipe

## Ingredients
- 2 cups flour
- 1 cup butter
- 1 cup chocolate chips

## Instructions
1. Mix ingredients
2. Bake at 350°F for 12 minutes
`,
		}

		input := knowledge.Input{
			Risks:      risks,
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		if err != nil {
			t.Fatalf("failed to extract knowledge: %v", err)
		}

		if len(results) > 0 {
			t.Errorf("expected no results for unrelated content, got %d", len(results))
		}
	})

	t.Run("Extract with empty risks returns nil", func(t *testing.T) {
		sourceData := knowledge.SourceData{
			SourceID:  model.NewSourceID(),
			SourceURL: "https://example.com/page",
			SourcedAt: time.Now().UTC(),
			Content:   "Some content",
		}

		input := knowledge.Input{
			Risks:      []*model.Risk{},
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if results != nil {
			t.Errorf("expected nil results for empty risks, got %v", results)
		}
	})
}

func TestNew_RequiresLLMClient(t *testing.T) {
	_, err := knowledge.New(nil)
	if err == nil {
		t.Error("expected error when LLM client is nil")
	}
}
