package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// cmdTick is the `hecatoncheires tick` subcommand: a one-shot sweep over
// every workspace's scheduled Jobs. The same logic backs `POST /hooks/tick`.
// Wire to Cloud Scheduler (or any cron) — the command exits when the sweep
// and every run it dispatched have finished.
//
// The runs execute on the same agent runtime `serve` uses, so a scheduled run gets
// the same step / token budget and the same one-transition-at-a-time checkpointing.
// The sweep drives the worker itself rather than leaving the runs for a `serve`
// instance: a scheduled sweep must not depend on another process being up to
// execute what it dispatched.
func cmdTick() *cli.Command {
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var llmCfg config.LLM
	var embCfg config.Embedding
	var jobCfg config.JobConcurrency
	var agentCfg config.Agent
	var storageCfg config.Storage
	// The integrations below are configured here for the same reason the LLM is:
	// a sweep EXECUTES the runs it dispatches, so the Job agent's tools are built
	// from THIS process's clients. Leaving one out does not simply disable a
	// capability — the Job palette still advertises its toolset id to the planner,
	// which then hands the model a tool that resolves to nothing. See
	// docs/operations.md § Running scheduled Jobs.
	//
	// GitHub is absent on purpose: a sweep runs only Job agents, and the Job tool
	// palette withholds the github toolset from an unattended run.
	var slackCfg config.Slack
	var slackToolCfg config.SlackTool
	var jiraCfg config.Jira
	var webfetchCfg config.WebFetch
	var notionToken string
	var baseURL string

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "notion-api-token",
			Usage:       "Notion API token for Source integration and the notion__* agent tools",
			Sources:     cli.EnvVars("HECATONCHEIRES_NOTION_API_TOKEN"),
			Destination: &notionToken,
		},
		&cli.StringFlag{
			Name:        "base-url",
			Usage:       "Base URL for the application (e.g., https://your-domain.com). Slack messages a dispatched run posts link back to it",
			Sources:     cli.EnvVars("HECATONCHEIRES_BASE_URL"),
			Destination: &baseURL,
		},
	}
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, llmCfg.Flags()...)
	flags = append(flags, embCfg.Flags()...)
	flags = append(flags, jobCfg.Flags()...)
	flags = append(flags, agentCfg.Flags()...)
	flags = append(flags, storageCfg.Flags()...)
	// RuntimeFlags, not Flags: a sweep serves no endpoints, so the OAuth client
	// pair and the webhook signing secret would be credentials it can never use.
	flags = append(flags, slackCfg.RuntimeFlags()...)
	flags = append(flags, slackToolCfg.Flags()...)
	flags = append(flags, jiraCfg.Flags()...)
	flags = append(flags, webfetchCfg.Flags()...)

	return &cli.Command{
		Name:  "tick",
		Usage: "Run a single sweep over scheduled Agent Jobs and dispatch due ones",
		Flags: flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.From(ctx)

			if err := jobCfg.Validate(); err != nil {
				return goerr.Wrap(err, "invalid job concurrency configuration")
			}

			deps, err := buildTickRuntime(ctx, &appCfg, &repoCfg, &llmCfg, &embCfg, &jobCfg,
				&agentCfg, &storageCfg, tickIntegrationConfigs{
					Slack:       &slackCfg,
					SlackTool:   &slackToolCfg,
					Jira:        &jiraCfg,
					WebFetch:    &webfetchCfg,
					NotionToken: notionToken,
					BaseURL:     baseURL,
				}, c)
			if err != nil {
				return goerr.Wrap(err, "failed to build tick runtime")
			}
			// Same line serve emits: the effective concurrency must be visible
			// from whichever process actually dispatched the runs.
			logging.Default().Info("Agent Job runtime configured", logAttrsToArgs(jobCfg.LogAttrs())...)
			defer deps.cleanup()
			defer func() {
				if err := deps.repo.Close(); err != nil {
					errutil.Handle(ctx, goerr.Wrap(err, "close repo"), "close repo")
				}
			}()

			if err := deps.scanner.Scan(ctx); err != nil {
				return goerr.Wrap(err, "tick sweep failed")
			}

			// Wait for every async dispatch the publisher launched, so every run this
			// sweep starts has been recorded before the drain below decides what to
			// wait for.
			async.Wait()

			// Then execute them. Run returns as soon as a run is recorded, so without
			// this the command would exit having dispatched work nobody runs.
			if err := deps.durable.Drain(ctx); err != nil {
				return goerr.Wrap(err, "drain the runs this sweep dispatched")
			}

			logger.Info("tick sweep complete")
			return nil
		},
	}
}
