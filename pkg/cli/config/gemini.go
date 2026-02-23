package config

import (
	"context"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/urfave/cli/v3"
)

// Gemini holds configuration for the Gemini LLM client
type Gemini struct {
	projectID string
	location  string
}

// Flags returns CLI flags for Gemini configuration
func (g *Gemini) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "gemini-project",
			Usage:       "Google Cloud project ID for Gemini API",
			Sources:     cli.EnvVars("HECATONCHEIRES_GEMINI_PROJECT"),
			Destination: &g.projectID,
		},
		&cli.StringFlag{
			Name:        "gemini-location",
			Usage:       "Google Cloud location for Gemini API",
			Value:       "us-central1",
			Sources:     cli.EnvVars("HECATONCHEIRES_GEMINI_LOCATION"),
			Destination: &g.location,
		},
	}
}

// LogAttrs returns log attributes for the Gemini configuration
func (g *Gemini) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("project_id", g.projectID),
		slog.String("location", g.location),
	}
}

// Configure creates a new Gemini LLM client from the configured flags.
// Returns nil if projectID is not configured (AI agent features will be disabled).
func (g *Gemini) Configure(ctx context.Context) (gollem.LLMClient, error) {
	if g.projectID == "" {
		return nil, nil
	}

	client, err := gemini.New(ctx, g.projectID, g.location)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create Gemini client")
	}

	return client, nil
}
