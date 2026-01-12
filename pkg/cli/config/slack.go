package config

import (
	"context"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/slack-go/slack"
	"github.com/urfave/cli/v3"
)

// SlackUserInfo holds user information retrieved from Slack API
type SlackUserInfo struct {
	ID    string
	Email string
	Name  string
}

type Slack struct {
	clientID      string
	clientSecret  string
	botToken      string
	signingSecret string
	noAuthUID     string
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

// SetNoAuthUID sets the no-auth user ID
func (x *Slack) SetNoAuthUID(uid string) {
	x.noAuthUID = uid
}

// NoAuthUID returns the no-auth user ID
func (x *Slack) NoAuthUID() string {
	return x.noAuthUID
}

// Configure creates an AuthUseCase if Slack is configured, otherwise returns NoAuthnUseCase
func (x *Slack) Configure(ctx context.Context, repo interfaces.Repository, baseURL string) (usecase.AuthUseCaseInterface, error) {
	// If no-auth mode is enabled, validate and use the specified user
	if x.noAuthUID != "" {
		// no-auth requires bot token for user validation
		if x.botToken == "" {
			return nil, goerr.New("--no-auth requires --slack-bot-token for user validation")
		}

		// Warn if OAuth credentials are also configured (no-auth takes precedence)
		if x.clientID != "" || x.clientSecret != "" {
			slog.Warn("--no-auth is set, ignoring --slack-client-id/--slack-client-secret")
		}

		// Validate user exists in Slack
		userInfo, err := x.GetSlackUserInfo(ctx, x.noAuthUID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to validate Slack user", goerr.V("uid", x.noAuthUID))
		}

		return usecase.NewNoAuthnUseCase(repo, userInfo.ID, userInfo.Email, userInfo.Name), nil
	}

	// If any Slack config is missing or baseURL is not set, return error
	// (no more fallback to anonymous mode)
	if x.clientID == "" || x.clientSecret == "" || baseURL == "" {
		return nil, goerr.New("Slack OAuth configuration is required: set --slack-client-id, --slack-client-secret, and --base-url, or use --no-auth with --slack-bot-token")
	}

	// Build callback URL from base URL
	callbackURL := baseURL + "/api/auth/callback"

	return usecase.NewAuthUseCase(repo, x.clientID, x.clientSecret, callbackURL, usecase.WithBotToken(x.botToken)), nil
}

// GetSlackUserInfo retrieves user information from Slack API
func (x *Slack) GetSlackUserInfo(ctx context.Context, userID string) (*SlackUserInfo, error) {
	if x.botToken == "" {
		return nil, goerr.New("bot token is required to fetch user info")
	}

	api := slack.New(x.botToken)
	user, err := api.GetUserInfoContext(ctx, userID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get user info from Slack", goerr.V("user_id", userID))
	}

	return &SlackUserInfo{
		ID:    user.ID,
		Email: user.Profile.Email,
		Name:  user.RealName,
	}, nil
}

// BotToken returns the Slack bot token
func (x *Slack) BotToken() string {
	return x.botToken
}

// IsConfigured checks if Slack configuration is complete
func (x *Slack) IsConfigured() bool {
	return x.clientID != "" && x.clientSecret != ""
}

// IsNoAuthMode returns true if no-auth mode is enabled
func (x *Slack) IsNoAuthMode() bool {
	return x.noAuthUID != ""
}

// IsWebhookConfigured checks if Slack webhook is configured
func (x *Slack) IsWebhookConfigured() bool {
	return x.signingSecret != ""
}

// SigningSecret returns the Slack signing secret
func (x *Slack) SigningSecret() string {
	return x.signingSecret
}
