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
)

// HomeMessageLLM is a dedicated, optional LLM configuration for the home
// dashboard's greeting message. It mirrors config.LLM's provider dispatch but
// with its own flags so the greeting can target a cheaper/faster model than the
// main chat LLM. When left unconfigured (IsEnabled() == false), the caller
// falls back to the shared chat LLM client.
type HomeMessageLLM struct {
	provider        string
	model           string
	openaiAPIKey    string
	claudeAPIKey    string
	geminiProjectID string
	geminiLocation  string
}

// Flags returns CLI flags for the home-message LLM configuration.
func (x *HomeMessageLLM) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "home-message-llm-provider",
			Usage:       "LLM provider for the home greeting: openai, claude, or gemini (empty falls back to --llm-provider)",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_PROVIDER"),
			Destination: &x.provider,
		},
		&cli.StringFlag{
			Name:        "home-message-llm-model",
			Usage:       "Model name for the home greeting LLM (provider default if empty)",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_MODEL"),
			Destination: &x.model,
		},
		&cli.StringFlag{
			Name:        "home-message-llm-openai-api-key",
			Usage:       "OpenAI API key (required when --home-message-llm-provider=openai)",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_OPENAI_API_KEY"),
			Destination: &x.openaiAPIKey,
		},
		&cli.StringFlag{
			Name:        "home-message-llm-claude-api-key",
			Usage:       "Anthropic Claude API key (used when --home-message-llm-provider=claude with direct Anthropic access)",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_CLAUDE_API_KEY"),
			Destination: &x.claudeAPIKey,
		},
		&cli.StringFlag{
			Name:        "home-message-llm-gemini-project-id",
			Usage:       "Google Cloud project ID for the home greeting LLM (Gemini, or Claude via Vertex AI)",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_GEMINI_PROJECT_ID"),
			Destination: &x.geminiProjectID,
		},
		&cli.StringFlag{
			Name:        "home-message-llm-gemini-location",
			Usage:       "Google Cloud location for the home greeting LLM (e.g. global, us-central1)",
			Value:       "global",
			Sources:     cli.EnvVars("HECATONCHEIRES_HOME_MESSAGE_LLM_GEMINI_LOCATION"),
			Destination: &x.geminiLocation,
		},
	}
}

// IsEnabled reports whether a dedicated home-message LLM provider is configured.
// When false, the caller uses the shared chat LLM client instead.
func (x *HomeMessageLLM) IsEnabled() bool { return x.provider != "" }

// LogAttrs returns log attributes for the home-message LLM configuration.
// Secrets are never included.
func (x *HomeMessageLLM) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("provider", x.provider),
	}
	if x.model != "" {
		attrs = append(attrs, slog.String("model", x.model))
	}
	if x.geminiProjectID != "" {
		attrs = append(attrs, slog.String("gcp_project_id", x.geminiProjectID))
	}
	return attrs
}

// NewClient builds a gollem.LLMClient for the configured provider. It must only
// be called when IsEnabled() is true.
func (x *HomeMessageLLM) NewClient(ctx context.Context) (gollem.LLMClient, error) {
	switch x.provider {
	case "openai":
		if x.openaiAPIKey == "" {
			return nil, goerr.New("--home-message-llm-openai-api-key is required when --home-message-llm-provider=openai")
		}
		var opts []openai.Option
		if x.model != "" {
			opts = append(opts, openai.WithModel(x.model))
		}
		client, err := openai.New(ctx, x.openaiAPIKey, opts...)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create home-message OpenAI client")
		}
		return client, nil

	case "claude":
		hasAPIKey := x.claudeAPIKey != ""
		hasGCP := x.geminiProjectID != ""
		if hasAPIKey && hasGCP {
			return nil, goerr.New("--home-message-llm-claude-api-key and --home-message-llm-gemini-project-id are mutually exclusive when --home-message-llm-provider=claude")
		}
		switch {
		case hasAPIKey:
			var opts []claude.Option
			if x.model != "" {
				opts = append(opts, claude.WithModel(x.model))
			}
			client, err := claude.New(ctx, x.claudeAPIKey, opts...)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create home-message Claude client")
			}
			return client, nil
		case hasGCP:
			if x.geminiLocation == "" {
				return nil, goerr.New("--home-message-llm-gemini-location is required when --home-message-llm-provider=claude with --home-message-llm-gemini-project-id")
			}
			var opts []claude.VertexOption
			if x.model != "" {
				opts = append(opts, claude.WithVertexModel(x.model))
			}
			client, err := claude.NewWithVertex(ctx, x.geminiLocation, x.geminiProjectID, opts...)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to create home-message Claude (Vertex AI) client")
			}
			return client, nil
		default:
			return nil, goerr.New("--home-message-llm-provider=claude requires either --home-message-llm-claude-api-key or --home-message-llm-gemini-project-id")
		}

	case "gemini":
		if x.geminiProjectID == "" || x.geminiLocation == "" {
			return nil, goerr.New("--home-message-llm-provider=gemini requires both --home-message-llm-gemini-project-id and --home-message-llm-gemini-location")
		}
		var opts []gemini.Option
		if x.model != "" {
			opts = append(opts, gemini.WithModel(x.model))
		}
		client, err := gemini.New(ctx, x.geminiProjectID, x.geminiLocation, opts...)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to create home-message Gemini client")
		}
		return client, nil

	default:
		return nil, goerr.New("unsupported --home-message-llm-provider value", goerr.V("provider", x.provider))
	}
}
