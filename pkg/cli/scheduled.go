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

// cmdScheduled is the `hecatoncheires scheduled` subcommand: a one-shot
// sweep over every workspace's scheduled Jobs. The same logic backs
// `POST /hooks/scheduled`. Wire to Cloud Scheduler (or any cron) — the
// command exits when the sweep + in-flight async dispatches finish.
func cmdScheduled() *cli.Command {
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var llmCfg config.LLM

	var flags []cli.Flag
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, llmCfg.Flags()...)

	return &cli.Command{
		Name:  "scheduled",
		Usage: "Run a single sweep over scheduled Agent Jobs and dispatch due ones",
		Flags: flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.From(ctx)

			deps, err := buildScheduledRuntime(ctx, &appCfg, &repoCfg, &llmCfg, c)
			if err != nil {
				return goerr.Wrap(err, "failed to build scheduled runtime")
			}
			defer func() {
				if err := deps.repo.Close(); err != nil {
					errutil.Handle(ctx, goerr.Wrap(err, "close repo"), "close repo")
				}
			}()

			if err := deps.scanner.Scan(ctx); err != nil {
				return goerr.Wrap(err, "scheduled scan failed")
			}

			// Wait for every async dispatch the publisher launched so the
			// process does not exit before the Job goroutines finish.
			async.Wait()

			logger.Info("scheduled sweep complete")
			return nil
		},
	}
}
