package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// modelDef builds a priced definition for one provider, which is what
// NewClientFor takes.
func modelDef(provider, model string) agentkernel.ModelDef {
	return agentkernel.ModelDef{
		Ref:      model,
		Provider: provider,
		Model:    model,
		Rate:     pricing.Rate{Input: 1000, Output: 5000},
	}
}

func TestLLM_IsEnabled(t *testing.T) {
	gt.Bool(t, config.NewLLMForTest("", "", "", "", "").IsEnabled()).False()
	gt.Bool(t, config.NewLLMForTest("cheap", "", "", "", "").IsEnabled()).True()
	gt.String(t, config.NewLLMForTest("cheap", "", "", "", "").ModelRef()).Equal("cheap")
}

func TestLLM_NewClientFor_RejectsAnUnpricedDefinition(t *testing.T) {
	cfg := config.NewLLMForTest("cheap", "openai-key", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), agentkernel.ModelDef{
		Ref: "cheap", Provider: agentkernel.ProviderOpenAI, Model: "gpt-4o",
	})

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("priced at nothing")
}

func TestLLM_OpenAI_RequiresAPIKey(t *testing.T) {
	cfg := config.NewLLMForTest("gpt-4o", "", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), modelDef(agentkernel.ProviderOpenAI, "gpt-4o"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("openai-api-key")
}

func TestLLM_Claude_RequiresCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "", "global")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("claude-api-key")
}

func TestLLM_Claude_RejectsBothCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "anthropic-key", "gcp-project", "global")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("mutually exclusive")
}

func TestLLM_Claude_VertexRequiresLocation(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "gcp-project", "")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("location")
}

func TestLLM_Gemini_RequiresProjectAndLocation(t *testing.T) {
	t.Run("missing project", func(t *testing.T) {
		cfg := config.NewLLMForTest("cheap", "", "", "", "global")
		_, err := cfg.NewClientFor(context.Background(),
			modelDef(agentkernel.ProviderGemini, "gemini-3.7-flash"))
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-project-id")
	})

	t.Run("missing location", func(t *testing.T) {
		cfg := config.NewLLMForTest("cheap", "", "", "gcp-project", "")
		_, err := cfg.NewClientFor(context.Background(),
			modelDef(agentkernel.ProviderGemini, "gemini-3.7-flash"))
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-location")
	})
}

func TestLLM_UnsupportedProvider(t *testing.T) {
	cfg := config.NewLLMForTest("bogus", "", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), modelDef("bedrock", "some-model"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("openai, claude or gemini")
}

func TestLLM_LogAttrs_DoesNotLeakSecrets(t *testing.T) {
	cfg := config.NewLLMForTest("gpt-4o", "super-secret-key", "claude-secret", "proj", "global")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		s := a.Value.String()
		gt.String(t, s).NotEqual("super-secret-key")
		gt.String(t, s).NotEqual("claude-secret")
	}

	found := false
	for _, a := range attrs {
		if a.Key == "model_ref" && a.Value.String() == "gpt-4o" {
			found = true
		}
	}
	gt.Bool(t, found).True()
}

func TestLLM_LogAttrs_OmitsGCPWithoutAProject(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "anthropic-key", "", "global")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		gt.String(t, a.Key).NotEqual("gcp_project_id")
		gt.String(t, a.Key).NotEqual("gcp_location")
	}
}

func TestLLM_LogAttrs_IncludesGCPWhenSet(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "proj", "us-east5")
	attrs := cfg.LogAttrs()

	hasProject, hasLocation := false, false
	for _, a := range attrs {
		if a.Key == "gcp_project_id" && a.Value.String() == "proj" {
			hasProject = true
		}
		if a.Key == "gcp_location" && a.Value.String() == "us-east5" {
			hasLocation = true
		}
	}
	gt.Bool(t, hasProject).True()
	gt.Bool(t, hasLocation).True()
}
