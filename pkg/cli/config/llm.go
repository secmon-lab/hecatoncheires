package config

import (
	"context"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/llm/claude"
	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/m-mizutani/gollem/llm/openai"
	"github.com/urfave/cli/v3"
)

// LLM holds CLI configuration for LLM clients backed by gollem.
// It supports OpenAI, Anthropic Claude (direct API or Vertex AI), and Google Gemini.
type LLM struct {
	provider        string
	model           string
	openaiAPIKey    string
	claudeAPIKey    string
	geminiProjectID string
	geminiLocation  string
}

// Flags returns CLI flags for LLM configuration.
func (x *LLM) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "llm-provider",
			Usage:       "LLM provider: openai, claude, or gemini (empty disables AI features)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_PROVIDER"),
			Destination: &x.provider,
		},
		&cli.StringFlag{
			Name:        "llm-model",
			Usage:       "LLM model name (provider default if empty)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_MODEL"),
			Destination: &x.model,
		},
		&cli.StringFlag{
			Name:        "llm-openai-api-key",
			Usage:       "OpenAI API key (required when --llm-provider=openai)",
			Sources:     cli.EnvVars("HECATONCHEIRES_LLM_OPENAI_API_KEY"),
			Destination: &x.openaiAPIKey,
		},
		&cli.StringFlag{
			Name:        "llm-claude-api-key",
			Usage:       "Anthropic Claude API key (used when --llm-provider=claude with direct Anthropic access)",
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

// IsEnabled reports whether an LLM provider has been configured.
func (x *LLM) IsEnabled() bool { return x.provider != "" }

// LogAttrs returns log attributes for the LLM configuration. Secrets are never included.
// Provider-specific attributes are emitted only when relevant to the active provider.
func (x *LLM) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("provider", x.provider),
	}
	if x.model != "" {
		attrs = append(attrs, slog.String("model", x.model))
	}

	switch x.provider {
	case "claude":
		// Only Vertex AI mode uses GCP project/location.
		if x.geminiProjectID != "" {
			attrs = append(attrs,
				slog.String("gcp_project_id", x.geminiProjectID),
				slog.String("gcp_location", x.geminiLocation),
			)
		}
	case "gemini":
		if x.geminiProjectID != "" {
			attrs = append(attrs, slog.String("gcp_project_id", x.geminiProjectID))
		}
		if x.geminiLocation != "" {
			attrs = append(attrs, slog.String("gcp_location", x.geminiLocation))
		}
	}
	return attrs
}

// NewClient builds a gollem.LLMClient for the configured provider. Returns
// (nil, nil) when the LLM feature is disabled (no provider configured).
func (x *LLM) NewClient(ctx context.Context) (gollem.LLMClient, error) {
	if !x.IsEnabled() {
		return nil, nil
	}

	switch x.provider {
	case "openai":
		if x.openaiAPIKey == "" {
			return nil, goerr.New("--llm-openai-api-key is required when --llm-provider=openai")
		}
		var opts []openai.Option
		if x.model != "" {
			opts = append(opts, openai.WithModel(x.model))
		}
		client, err := openai.New(ctx, x.openaiAPIKey, opts...)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create OpenAI client")
		}
		return client, nil

	case "claude":
		hasAPIKey := x.claudeAPIKey != ""
		hasGCP := x.geminiProjectID != ""
		if hasAPIKey && hasGCP {
			return nil, goerr.New("--llm-claude-api-key and --llm-gemini-project-id are mutually exclusive when --llm-provider=claude")
		}
		switch {
		case hasAPIKey:
			var opts []claude.Option
			if x.model != "" {
				opts = append(opts, claude.WithModel(x.model))
			}
			client, err := claude.New(ctx, x.claudeAPIKey, opts...)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create Claude client")
			}
			return client, nil
		case hasGCP:
			if x.geminiLocation == "" {
				return nil, goerr.New("--llm-gemini-location is required when --llm-provider=claude with --llm-gemini-project-id")
			}
			var opts []claude.VertexOption
			if x.model != "" {
				opts = append(opts, claude.WithVertexModel(x.model))
			}
			client, err := claude.NewWithVertex(ctx, x.geminiLocation, x.geminiProjectID, opts...)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create Claude (Vertex AI) client")
			}
			return client, nil
		default:
			return nil, goerr.New("--llm-provider=claude requires either --llm-claude-api-key or --llm-gemini-project-id")
		}

	case "gemini":
		if x.geminiProjectID == "" || x.geminiLocation == "" {
			return nil, goerr.New("--llm-provider=gemini requires both --llm-gemini-project-id and --llm-gemini-location")
		}
		var opts []gemini.Option
		if x.model != "" {
			opts = append(opts, gemini.WithModel(x.model))
		}
		client, err := gemini.New(ctx, x.geminiProjectID, x.geminiLocation, opts...)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create Gemini client")
		}
		return client, nil

	default:
		return nil, goerr.New("unsupported --llm-provider value", goerr.V("provider", x.provider))
	}
}
