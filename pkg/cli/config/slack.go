package config

import (
	"log/slog"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/urfave/cli/v3"
)

type Slack struct {
	clientID      string
	clientSecret  string
	botToken      string
	signingSecret string
}

func (x *Slack) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "slack-client-id",
			Usage:       "Slack OAuth client ID",
			Category:    "Slack",
			Destination: &x.clientID,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_CLIENT_ID"),
		},
		&cli.StringFlag{
			Name:        "slack-client-secret",
			Usage:       "Slack OAuth client secret",
			Category:    "Slack",
			Destination: &x.clientSecret,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_CLIENT_SECRET"),
		},
		&cli.StringFlag{
			Name:        "slack-bot-token",
			Usage:       "Slack Bot User OAuth Token (for fetching user info)",
			Category:    "Slack",
			Destination: &x.botToken,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_BOT_TOKEN"),
		},
		&cli.StringFlag{
			Name:        "slack-signing-secret",
			Usage:       "Slack Signing Secret (for webhook verification)",
			Category:    "Slack",
			Destination: &x.signingSecret,
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_SIGNING_SECRET"),
		},
	}
}

func (x Slack) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("client-id.len", len(x.clientID)),
		slog.Int("client-secret.len", len(x.clientSecret)),
	)
}

// Configure creates an AuthUseCase if Slack is configured, otherwise returns NoAuthnUseCase
func (x *Slack) Configure(repo interfaces.Repository, baseURL string) (usecase.AuthUseCaseInterface, error) {
	// If any Slack config is missing or baseURL is not set, use NoAuthn mode
	if x.clientID == "" || x.clientSecret == "" || baseURL == "" {
		return usecase.NewNoAuthnUseCase(repo), nil
	}

	// Build callback URL from base URL
	callbackURL := baseURL + "/api/auth/callback"

	return usecase.NewAuthUseCase(repo, x.clientID, x.clientSecret, callbackURL, usecase.WithBotToken(x.botToken)), nil
}

// BotToken returns the Slack bot token
func (x *Slack) BotToken() string {
	return x.botToken
}

// IsConfigured checks if Slack configuration is complete
func (x *Slack) IsConfigured() bool {
	return x.clientID != "" && x.clientSecret != ""
}

// IsWebhookConfigured checks if Slack webhook is configured
func (x *Slack) IsWebhookConfigured() bool {
	return x.signingSecret != ""
}

// SigningSecret returns the Slack signing secret
func (x *Slack) SigningSecret() string {
	return x.signingSecret
}
