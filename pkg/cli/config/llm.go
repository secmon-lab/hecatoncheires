package config

import (
	"context"
	"log/slog"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/llm/claude"
	"github.com/gollem-dev/gollem/llm/gemini"
	"github.com/gollem-dev/gollem/llm/openai"
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
)

// LLM holds the CLI configuration for the agent's LLM access: which defined
// model is the default, and the credentials each provider needs.
//
// It deliberately does NOT name a provider. Which provider serves a model — and
// what that model costs — is declared once per model in the global config's
// [[llm_model]] sections; naming it here as well would put the same fact in two
// places and make disagreement between them possible. What stays here is the
// credentials, because a secret does not belong in a configuration document.
type LLM struct {
	modelRef        string
	openaiAPIKey    string
	claudeAPIKey    string
	geminiProjectID string
	geminiLocation  string
}

// Flags returns CLI flags for LLM configuration.
func (x *LLM) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name: "llm-model",
			Usage: "Default model: the reference name of an [[llm_model]] entry in the global config " +
				"(its alias, or its model name). Empty disables AI features",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_MODEL"),
			Destination: &x.modelRef,
		},
		&cli.StringFlag{
			Name:        "llm-openai-api-key",
			Usage:       "OpenAI API key (required for a model whose provider is openai)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_OPENAI_API_KEY"),
			Destination: &x.openaiAPIKey,
		},
		&cli.StringFlag{
			Name:        "llm-claude-api-key",
			Usage:       "Anthropic Claude API key (for a claude model reached through Anthropic directly)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_CLAUDE_API_KEY"),
			Destination: &x.claudeAPIKey,
		},
		&cli.StringFlag{
			Name:        "llm-gemini-project-id",
			Usage:       "Google Cloud project ID (Gemini, or Claude via Vertex AI)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_GEMINI_PROJECT_ID"),
			Destination: &x.geminiProjectID,
		},
		&cli.StringFlag{
			Name:        "llm-gemini-location",
			Usage:       "Google Cloud location for Gemini / Claude on Vertex AI (e.g. global, us-central1)",
			Value:       "global",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_GEMINI_LOCATION"),
			Destination: &x.geminiLocation,
		},
	}
}

// IsEnabled reports whether a default model has been named. It is what decides
// whether the AI features are wired at all, the role --llm-provider used to play.
func (x *LLM) IsEnabled() bool { return x.modelRef != "" }

// ModelRef returns the reference name of the default model.
func (x *LLM) ModelRef() string { return x.modelRef }

// LogAttrs returns log attributes for the LLM configuration. Secrets are never
// included. The resolved provider and model are logged by the caller that
// resolves them, since this struct names only the reference.
func (x *LLM) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{slog.String("model_ref", x.modelRef)}
	if x.geminiProjectID != "" {
		attrs = append(attrs,
			slog.String("gcp_project_id", x.geminiProjectID),
			slog.String("gcp_location", x.geminiLocation),
		)
	}
	return attrs
}

// NewClientFor builds the gollem client that serves one defined model.
//
// The credential rules are the provider's, not the model's: a claude model is
// reached either through Anthropic directly (an API key) or through Vertex AI (a
// GCP project), and naming both is a configuration mistake rather than a choice
// this code may make on the operator's behalf.
func (x *LLM) NewClientFor(ctx context.Context, def agentkernel.ModelDef) (gollem.LLMClient, error) {
	if err := def.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid model definition")
	}

	switch def.Provider {
	case agentkernel.ProviderOpenAI:
		if x.openaiAPIKey == "" {
			return nil, goerr.New("--llm-openai-api-key is required for an openai model",
				goerr.V(LLMModelRefKey, def.Ref))
		}
		client, err := openai.New(ctx, x.openaiAPIKey, openai.WithModel(def.Model))
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create OpenAI client",
				goerr.V(LLMModelRefKey, def.Ref))
		}
		return client, nil

	case agentkernel.ProviderClaude:
		hasAPIKey := x.claudeAPIKey != ""
		hasGCP := x.geminiProjectID != ""
		switch {
		case hasAPIKey && hasGCP:
			return nil, goerr.New("--llm-claude-api-key and --llm-gemini-project-id are mutually exclusive for a claude model",
				goerr.V(LLMModelRefKey, def.Ref))
		case hasAPIKey:
			client, err := claude.New(ctx, x.claudeAPIKey, claude.WithModel(def.Model))
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create Claude client",
					goerr.V(LLMModelRefKey, def.Ref))
			}
			return client, nil
		case hasGCP:
			if x.geminiLocation == "" {
				return nil, goerr.New("--llm-gemini-location is required for a claude model on Vertex AI",
					goerr.V(LLMModelRefKey, def.Ref))
			}
			client, err := claude.NewWithVertex(ctx, x.geminiLocation, x.geminiProjectID,
				claude.WithVertexModel(def.Model))
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create Claude (Vertex AI) client",
					goerr.V(LLMModelRefKey, def.Ref))
			}
			return client, nil
		default:
			return nil, goerr.New("a claude model requires either --llm-claude-api-key or --llm-gemini-project-id",
				goerr.V(LLMModelRefKey, def.Ref))
		}

	case agentkernel.ProviderGemini:
		if x.geminiProjectID == "" || x.geminiLocation == "" {
			return nil, goerr.New("a gemini model requires both --llm-gemini-project-id and --llm-gemini-location",
				goerr.V(LLMModelRefKey, def.Ref))
		}
		client, err := gemini.New(ctx, x.geminiProjectID, x.geminiLocation,
			gemini.WithModel(def.Model))
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create Gemini client",
				goerr.V(LLMModelRefKey, def.Ref))
		}
		return client, nil

	default:
		// ModelDef.Validate has already rejected every other value; this arm
		// exists so a provider added there without a client here fails loudly.
		return nil, goerr.New("unsupported model provider",
			goerr.V(LLMModelRefKey, def.Ref), goerr.V("provider", def.Provider))
	}
}
