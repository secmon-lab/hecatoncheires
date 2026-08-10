package cli

import (
	"context"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/m-mizutani/goerr/v2"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
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
	var llmCfg config.LLM
	var embCfg config.Embedding
	var agentCfg config.Agent

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
	flags = append(flags, llmCfg.Flags()...)
	flags = append(flags, embCfg.Flags()...)
	flags = append(flags, agentCfg.Flags()...)

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
					errutil.Handle(ctx, goerr.Wrap(err, "failed to close repository"), "failed to close repository")
				}
			}()

			// Initialize LLM client (required)
			if !llmCfg.IsEnabled() {
				return goerr.New("--llm-provider is required for assist")
			}
			llmClient, err := llmCfg.NewClient(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize LLM client")
			}
			logging.Default().Info("LLM client enabled", logAttrsToArgs(llmCfg.LogAttrs())...)

			// Initialize Embedding client (required)
			if !embCfg.IsEnabled() {
				return goerr.New("--embedding-gemini-project-id is required for assist")
			}
			embedClient, err := embCfg.NewClient(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize embedding client")
			}
			logging.Default().Info("Embedding client enabled", logAttrsToArgs(embCfg.LogAttrs())...)

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
				usecase.WithEmbedClient(embedClient),
				usecase.WithSlackService(slackSvc),
			)

			// Build the agent runtime this pass runs on. Registration must
			// complete before the Kernel is built, and the Kernel must be bound
			// before the first Spawn.
			//
			// The Process store is in-process ON PURPOSE, unlike serve's. The
			// assist agent is registered only in this command, so a Process left
			// in a SHARED store would be claimed by a serve worker that cannot
			// resolve the agent and would fail it outright. Assist has never been
			// resumable across a crash — it is one foreground pass — so nothing
			// is lost by keeping its runs where only this pass can see them.
			budgets, bErr := agentCfg.Budgets()
			if bErr != nil {
				return bErr
			}
			agentRegistry := agentkit.NewRegistry()
			if err := uc.Assist.Register(agentRegistry, budgets.Root.Limiter(),
				agentarchive.NewMemoryHistoryStore()); err != nil {
				return goerr.Wrap(err, "failed to register the assist agent")
			}
			agentKernel, err := agentkernel.Build(agentkernel.Deps{
				Repo:    agentprocmemory.New(),
				History: agentarchive.NewMemoryHistoryStore(),
				LLM:     llmClient,
				Trace:   agentarchive.NewMemoryTraceRepository(),
				Budgets: budgets,
				Agents:  agentRegistry,
				// Only the clients this command actually wires. Assist has never
				// had Notion / GitHub / Jira / WebFetch or the User-token Slack
				// clients here, so the palette it resolves is unchanged.
				Tools: agentkernel.ToolDeps{
					Repo:         repo,
					Registry:     registry,
					SlackBot:     slackSvc,
					ActionUC:     usecase.NewActionToolAdapter(uc.Action),
					ActionStepUC: usecase.NewActionStepToolAdapter(uc.ActionStep),
					CaseRefUC:    uc.Case,
				},
			})
			if err != nil {
				return goerr.Wrap(err, "failed to build the agent runtime")
			}
			uc.Assist.Bind(agentKernel)
			logging.Default().Info("Agent runtime configured", logAttrsToArgs(agentCfg.LogAttrs())...)

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
