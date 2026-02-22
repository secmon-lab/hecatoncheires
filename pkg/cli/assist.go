package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdAssist() *cli.Command {
	var slackBotToken string
	var workspaceID string
	var logCount int
	var messageCount int
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var geminiCfg config.Gemini

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "slack-bot-token",
			Usage:       "Slack Bot Token for sending notifications (required)",
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_BOT_TOKEN"),
			Destination: &slackBotToken,
		},
		&cli.StringFlag{
			Name:        "workspace",
			Usage:       "Target workspace ID (if empty, process all workspaces)",
			Sources:     cli.EnvVars("HECATONCHEIRES_ASSIST_WORKSPACE"),
			Destination: &workspaceID,
		},
		&cli.IntFlag{
			Name:        "log-count",
			Usage:       "Number of recent assist logs to include in system prompt",
			Sources:     cli.EnvVars("HECATONCHEIRES_ASSIST_LOG_COUNT"),
			Value:       7,
			Destination: &logCount,
		},
		&cli.IntFlag{
			Name:        "message-count",
			Usage:       "Number of recent Slack messages to include in system prompt",
			Sources:     cli.EnvVars("HECATONCHEIRES_ASSIST_MESSAGE_COUNT"),
			Value:       50,
			Destination: &messageCount,
		},
	}

	// Add shared config flags
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, geminiCfg.Flags()...)

	return &cli.Command{
		Name:    "assist",
		Aliases: []string{"a"},
		Usage:   "Run AI assist agent for all open cases across workspaces",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			// Load workspace configurations and build registry
			_, registry, err := appCfg.Configure(c)
			if err != nil {
				return goerr.Wrap(err, "failed to load workspace configurations")
			}

			// Initialize repository
			repo, err := repoCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize repository")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logging.Default().Error("failed to close repository", "error", err.Error())
				}
			}()

			// Initialize Gemini LLM client (required)
			llmClient, err := geminiCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Gemini client")
			}

			// Initialize Slack service (required)
			if slackBotToken == "" {
				return goerr.New("--slack-bot-token is required for assist")
			}
			slackSvc, err := slack.New(slackBotToken)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Slack service")
			}

			uc := usecase.New(repo, registry,
				usecase.WithLLMClient(llmClient),
				usecase.WithSlackService(slackSvc),
			)

			if uc.Assist == nil {
				return goerr.New("assist use case is not available (LLM client and Slack service are both required)")
			}

			logging.Default().Info("Starting assist",
				"workspace", workspaceID,
				"logCount", logCount,
				"messageCount", messageCount,
			)

			if err := uc.Assist.RunAssist(ctx, usecase.AssistOption{
				WorkspaceID:  workspaceID,
				LogCount:     logCount,
				MessageCount: messageCount,
			}); err != nil {
				return goerr.Wrap(err, "assist failed")
			}

			logging.Default().Info("Assist completed successfully")
			return nil
		},
	}
}
