package knowledge_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/m-mizutani/gt"
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
	gt.NoError(t, err).Required()

	// Create knowledge service
	svc, err := knowledge.New(llmClient)
	gt.NoError(t, err).Required()

	t.Run("Extract returns related knowledge", func(t *testing.T) {
		cases := []*model.Case{
			{
				ID:          1,
				Title:       "Security Vulnerability",
				Description: "Cases related to security vulnerabilities in software. Detection indicators: CVE mentions, security patches, vulnerability reports",
			},
			{
				ID:          2,
				Title:       "Data Privacy",
				Description: "Cases related to data privacy and GDPR compliance. Detection indicators: Personal data handling, consent management, data breaches",
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
			Cases:      cases,
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		gt.NoError(t, err).Required()

		gt.Number(t, len(results)).GreaterOrEqual(1)

		// The security vulnerability case should be identified
		foundSecurityCase := false
		for _, result := range results {
			if result.CaseID == 1 {
				foundSecurityCase = true
				gt.String(t, result.Title).NotEqual("")
				gt.String(t, result.Summary).NotEqual("")
				gt.Value(t, len(result.Embedding)).Equal(model.EmbeddingDimension)
			}
		}

		gt.Bool(t, foundSecurityCase).True()
	})

	t.Run("Extract returns empty for unrelated content", func(t *testing.T) {
		cases := []*model.Case{
			{
				ID:          1,
				Title:       "Security Vulnerability",
				Description: "Cases related to security vulnerabilities. Detection indicators: CVE mentions, security patches",
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
			Cases:      cases,
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(0)
	})

	t.Run("Extract with empty cases returns nil", func(t *testing.T) {
		sourceData := knowledge.SourceData{
			SourceID:  model.NewSourceID(),
			SourceURL: "https://example.com/page",
			SourcedAt: time.Now().UTC(),
			Content:   "Some content",
		}

		input := knowledge.Input{
			Cases:      []*model.Case{},
			SourceData: sourceData,
		}

		results, err := svc.Extract(ctx, input)
		gt.NoError(t, err).Required()

		gt.Value(t, results).Nil()
	})
}

func TestNew_RequiresLLMClient(t *testing.T) {
	_, err := knowledge.New(nil)
	gt.Value(t, err).NotNil()
}
