package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestLLM_Disabled(t *testing.T) {
	cfg := config.NewLLMForTest("", "", "", "", "", "")
	gt.Bool(t, cfg.IsEnabled()).False()

	client, err := cfg.NewClient(context.Background())
	gt.NoError(t, err)
	gt.Value(t, client).Nil()
}

func TestLLM_OpenAI_RequiresAPIKey(t *testing.T) {
	cfg := config.NewLLMForTest("openai", "", "", "", "", "")
	gt.Bool(t, cfg.IsEnabled()).True()

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("openai-api-key")
}

func TestLLM_Claude_RequiresCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude", "", "", "", "", "")

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("claude")
}

func TestLLM_Claude_RejectsBothCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude", "", "", "anthropic-key", "gcp-project", "global")

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("mutually exclusive")
}

func TestLLM_Claude_VertexRequiresLocation(t *testing.T) {
	cfg := config.NewLLMForTest("claude", "", "", "", "gcp-project", "")

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("location")
}

func TestLLM_Gemini_RequiresProjectAndLocation(t *testing.T) {
	t.Run("missing project", func(t *testing.T) {
		cfg := config.NewLLMForTest("gemini", "", "", "", "", "global")
		_, err := cfg.NewClient(context.Background())
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-project-id")
	})

	t.Run("missing location", func(t *testing.T) {
		cfg := config.NewLLMForTest("gemini", "", "", "", "gcp-project", "")
		_, err := cfg.NewClient(context.Background())
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-location")
	})
}

func TestLLM_UnsupportedProvider(t *testing.T) {
	cfg := config.NewLLMForTest("bogus", "", "", "", "", "")

	_, err := cfg.NewClient(context.Background())
	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("unsupported")
}

func TestLLM_LogAttrs_DoesNotLeakSecrets(t *testing.T) {
	cfg := config.NewLLMForTest("openai", "gpt-4o", "super-secret-key", "claude-secret", "proj", "global")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		s := a.Value.String()
		gt.String(t, s).NotEqual("super-secret-key")
		gt.String(t, s).NotEqual("claude-secret")
	}

	// Sanity: provider is logged.
	found := false
	for _, a := range attrs {
		if a.Key == "provider" && a.Value.String() == "openai" {
			found = true
		}
	}
	gt.Bool(t, found).True()
}

func TestLLM_IsEnabled(t *testing.T) {
	gt.Bool(t, config.NewLLMForTest("", "", "", "", "", "").IsEnabled()).False()
	gt.Bool(t, config.NewLLMForTest("openai", "", "", "", "", "").IsEnabled()).True()
}
