package cli

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdCompile() *cli.Command {
	var notionToken string
	var slackBotToken string
	var since string
	var workspaceID string
	var baseURL string
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var geminiCfg config.Gemini
	var githubCfg config.GitHub

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "notion-api-token",
			Usage:       "Notion API token for Source integration",
			Sources:     cli.EnvVars("HECATONCHEIRES_NOTION_API_TOKEN"),
			Destination: &notionToken,
		},
		&cli.StringFlag{
			Name:        "since",
			Usage:       "Process pages updated since this time (RFC3339 format, default: 24h ago)",
			Sources:     cli.EnvVars("HECATONCHEIRES_COMPILE_SINCE"),
			Destination: &since,
		},
		&cli.StringFlag{
			Name:        "workspace",
			Usage:       "Target workspace ID (if empty, process all workspaces)",
			Sources:     cli.EnvVars("HECATONCHEIRES_COMPILE_WORKSPACE"),
			Destination: &workspaceID,
		},
		&cli.StringFlag{
			Name:        "base-url",
			Usage:       "Base URL for the application (e.g., https://your-domain.com)",
			Sources:     cli.EnvVars("HECATONCHEIRES_BASE_URL"),
			Destination: &baseURL,
		},
		&cli.StringFlag{
			Name:        "slack-bot-token",
			Usage:       "Slack Bot Token for sending notifications (optional)",
			Sources:     cli.EnvVars("HECATONCHEIRES_SLACK_BOT_TOKEN"),
			Destination: &slackBotToken,
		},
	}

	// Add shared config flags
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, geminiCfg.Flags()...)
	flags = append(flags, githubCfg.Flags()...)

	return &cli.Command{
		Name:    "compile",
		Aliases: []string{"c"},
		Usage:   "Extract knowledge from external sources using LLM",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			// Parse since time
			sinceTime := time.Now().Add(-24 * time.Hour)
			if since != "" {
				parsed, err := time.Parse(time.RFC3339, since)
				if err != nil {
					return goerr.Wrap(err, "failed to parse --since flag, expected RFC3339 format")
				}
				sinceTime = parsed
			}

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

			// Initialize Notion service
			if notionToken == "" {
				return goerr.New("--notion-api-token is required for compile")
			}
			notionSvc, err := notion.New(notionToken)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Notion service")
			}

			// Initialize Gemini LLM client
			llmClient, err := geminiCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Gemini client")
			}
			if llmClient != nil {
				logging.Default().Info("Gemini LLM client enabled", logAttrsToArgs(geminiCfg.LogAttrs())...)
			}

			// Initialize knowledge service
			knowledgeSvc, err := knowledge.New(llmClient)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize knowledge service")
			}

			// Build use case options
			ucOpts := []usecase.Option{
				usecase.WithNotion(notionSvc),
				usecase.WithKnowledgeService(knowledgeSvc),
				usecase.WithBaseURL(baseURL),
			}

			// Initialize Slack service if configured
			if slackBotToken != "" {
				slackSvc, err := slack.New(slackBotToken)
				if err != nil {
					return goerr.Wrap(err, "failed to initialize Slack service")
				}
				ucOpts = append(ucOpts, usecase.WithSlackService(slackSvc))
				logging.Default().Info("Slack notifications enabled")
			} else {
				logging.Default().Info("Slack Bot Token not configured, notifications will be skipped")
			}

			// Initialize GitHub service if configured
			githubSvc, err := githubCfg.Configure()
			if err != nil {
				return goerr.Wrap(err, "failed to initialize GitHub service")
			}
			if githubSvc != nil {
				ucOpts = append(ucOpts, usecase.WithGitHubService(githubSvc))
				logging.Default().Info("GitHub service enabled", logAttrsToArgs(githubCfg.LogAttrs())...)
			} else {
				logging.Default().Info("GitHub App not configured, GitHub Source features will be disabled")
			}

			uc := usecase.New(repo, registry, ucOpts...)

			// Run compile
			logging.Default().Info("Starting compile",
				"since", sinceTime.Format(time.RFC3339),
				"workspace", workspaceID,
			)

			result, err := uc.Compile.Compile(ctx, usecase.CompileOption{
				Since:       sinceTime,
				WorkspaceID: workspaceID,
			})
			if err != nil {
				return goerr.Wrap(err, "compile failed")
			}

			// Log results
			for _, ws := range result.WorkspaceResults {
				logging.Default().Info("Compile result",
					"workspaceID", ws.WorkspaceID,
					"sourcesProcessed", ws.SourcesProcessed,
					"pagesProcessed", ws.PagesProcessed,
					"knowledgeCreated", ws.KnowledgeCreated,
					"notifications", ws.Notifications,
					"errors", ws.Errors,
				)
			}

			logging.Default().Info("Compile completed successfully")
			return nil
		},
	}
}
