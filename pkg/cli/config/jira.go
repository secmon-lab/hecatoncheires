package config

import (
	"context"
	"log/slog"

	"github.com/gollem-dev/gollem"
	extjira "github.com/gollem-dev/tools/jira"
	"github.com/m-mizutani/goerr/v2"
	jiratool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/jira"
	"github.com/urfave/cli/v3"
)

// Jira holds configuration for the Jira Cloud read-only integration.
type Jira struct {
	baseURL  string
	email    string
	apiToken string
}

// Flags returns CLI flags for Jira configuration.
func (j *Jira) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "jira-base-url",
			Usage:       "Jira Cloud site URL (e.g. https://your-domain.atlassian.net)",
			Sources:     cli.EnvVars("HECATONCHEIRES_JIRA_BASE_URL"),
			Destination: &j.baseURL,
		},
		&cli.StringFlag{
			Name:        "jira-email",
			Usage:       "Jira account email for Basic auth",
			Sources:     cli.EnvVars("HECATONCHEIRES_JIRA_EMAIL"),
			Destination: &j.email,
		},
		&cli.StringFlag{
			Name:        "jira-api-token",
			Usage:       "Jira API token for Basic auth",
			Sources:     cli.EnvVars("HECATONCHEIRES_JIRA_API_TOKEN"),
			Destination: &j.apiToken,
		},
	}
}

// LogAttrs returns log attributes for the Jira configuration (the API token
// is deliberately excluded).
func (j *Jira) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("base_url", j.baseURL),
		slog.String("email", j.email),
	}
}

// IsConfigured returns true if all required Jira flags are set.
func (j *Jira) IsConfigured() bool {
	return j.baseURL != "" && j.email != "" && j.apiToken != ""
}

// Configure builds the Jira agent tools from the configured flags.
// Returns nil, nil if none of the three flags are set (Jira features will be
// disabled). Returns an error if only some are set: a partial configuration
// is a setup mistake, not an intentional opt-out, and silently disabling the
// tools would hide it from the operator.
func (j *Jira) Configure(ctx context.Context) ([]gollem.Tool, error) {
	anySet := j.baseURL != "" || j.email != "" || j.apiToken != ""
	if anySet && !j.IsConfigured() {
		return nil, goerr.New("incomplete Jira configuration: jira-base-url, jira-email, and jira-api-token must all be set")
	}
	if !j.IsConfigured() {
		return nil, nil
	}

	ts, err := extjira.New(j.baseURL, j.email, j.apiToken)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create Jira toolset")
	}

	tools, err := jiratool.New(ctx, ts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to build Jira tools")
	}

	return tools, nil
}
