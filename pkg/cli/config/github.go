package config

import (
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/service/github"
	"github.com/urfave/cli/v3"
)

// GitHub holds configuration for the GitHub App integration
type GitHub struct {
	appID          int
	installationID int
	privateKey     string
}

// Flags returns CLI flags for GitHub App configuration
func (g *GitHub) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "github-app-id",
			Usage:       "GitHub App ID",
			Sources:     cli.EnvVars("HECATONCHEIRES_GITHUB_APP_ID"),
			Destination: &g.appID,
		},
		&cli.IntFlag{
			Name:        "github-app-installation-id",
			Usage:       "GitHub App Installation ID",
			Sources:     cli.EnvVars("HECATONCHEIRES_GITHUB_APP_INSTALLATION_ID"),
			Destination: &g.installationID,
		},
		&cli.StringFlag{
			Name:        "github-app-private-key",
			Usage:       "GitHub App Private Key (PEM string or file path)",
			Sources:     cli.EnvVars("HECATONCHEIRES_GITHUB_APP_PRIVATE_KEY"),
			Destination: &g.privateKey,
		},
	}
}

// LogAttrs returns log attributes for the GitHub configuration (secrets hidden)
func (g *GitHub) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Int("app_id", g.appID),
		slog.Int("installation_id", g.installationID),
	}
}

// IsConfigured returns true if all required GitHub App flags are set
func (g *GitHub) IsConfigured() bool {
	return g.appID != 0 && g.installationID != 0 && g.privateKey != ""
}

// Configure creates a new GitHub Service from the configured flags.
// Returns nil if not all flags are configured (GitHub features will be disabled).
func (g *GitHub) Configure() (github.Service, error) {
	if !g.IsConfigured() {
		return nil, nil
	}

	svc, err := github.New(int64(g.appID), int64(g.installationID), g.privateKey)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create GitHub service")
	}

	return svc, nil
}
