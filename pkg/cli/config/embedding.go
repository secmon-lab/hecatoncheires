package config

import (
	"context"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/urfave/cli/v3"
)

// DefaultEmbeddingModel is the default Gemini embedding model used when
// --embedding-model is not specified. The dimension is still controlled
// per-call via gollem's GenerateEmbedding(ctx, dimension, ...).
const DefaultEmbeddingModel = "gemini-embedding-2"

// Embedding holds CLI configuration for the embedding client. Embedding is
// always backed by Gemini; chat completion has its own LLM configuration
// (config.LLM) that may use a different provider entirely.
type Embedding struct {
	geminiProjectID string
	geminiLocation  string
	model           string
}

// Flags returns CLI flags for Embedding configuration.
func (x *Embedding) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "embedding-gemini-project-id",
			Usage:       "Google Cloud project ID for the Gemini embedding client (required)",
			Sources:     cli.EnvVars("HECATONCHEIRES_EMBEDDING_GEMINI_PROJECT_ID"),
			Destination: &x.geminiProjectID,
		},
		&cli.StringFlag{
			Name:        "embedding-gemini-location",
			Usage:       "Google Cloud location for the Gemini embedding client (e.g. global, us-central1)",
			Value:       "global",
			Sources:     cli.EnvVars("HECATONCHEIRES_EMBEDDING_GEMINI_LOCATION"),
			Destination: &x.geminiLocation,
		},
		&cli.StringFlag{
			Name:        "embedding-model",
			Usage:       "Gemini embedding model name (defaults to gemini-embedding-2)",
			Value:       DefaultEmbeddingModel,
			Sources:     cli.EnvVars("HECATONCHEIRES_EMBEDDING_MODEL"),
			Destination: &x.model,
		},
	}
}

// IsEnabled reports whether the embedding client has the minimum configuration
// required to be constructed. The CLI commands all require embedding to be
// enabled; the helper exists so callers can fail fast with a clear message
// before reaching NewClient.
func (x *Embedding) IsEnabled() bool { return x.geminiProjectID != "" }

// LogAttrs returns log attributes describing the embedding configuration.
// Project ID, location, and model name are surfaced; no secret values exist
// to leak (Application Default Credentials are used by the Gemini SDK).
func (x *Embedding) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("model", x.effectiveModel()),
	}
	if x.geminiProjectID != "" {
		attrs = append(attrs, slog.String("gcp_project_id", x.geminiProjectID))
	}
	if x.geminiLocation != "" {
		attrs = append(attrs, slog.String("gcp_location", x.geminiLocation))
	}
	return attrs
}

// NewClient builds an interfaces.EmbedClient backed by a dedicated Gemini
// client. Returns an error when --embedding-gemini-project-id is missing.
func (x *Embedding) NewClient(ctx context.Context) (interfaces.EmbedClient, error) {
	if x.geminiProjectID == "" {
		return nil, goerr.New("--embedding-gemini-project-id is required")
	}
	if x.geminiLocation == "" {
		return nil, goerr.New("--embedding-gemini-location is required")
	}

	opts := []gemini.Option{
		gemini.WithEmbeddingModel(x.effectiveModel()),
	}
	client, err := gemini.New(ctx, x.geminiProjectID, x.geminiLocation, opts...)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create Gemini embedding client",
			goerr.V("project_id", x.geminiProjectID),
			goerr.V("location", x.geminiLocation),
		)
	}
	return client, nil
}

func (x *Embedding) effectiveModel() string {
	if x.model == "" {
		return DefaultEmbeddingModel
	}
	return x.model
}
