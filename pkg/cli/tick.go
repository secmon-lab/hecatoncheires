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
// + in-flight async dispatches finish.
func cmdTick() *cli.Command {
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var llmCfg config.LLM

	var flags []cli.Flag
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, llmCfg.Flags()...)

	return &cli.Command{
		Name:  "tick",
		Usage: "Run a single sweep over scheduled Agent Jobs and dispatch due ones",
		Flags: flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.From(ctx)

			deps, err := buildTickRuntime(ctx, &appCfg, &repoCfg, &llmCfg, c)
			if err != nil {
				return goerr.Wrap(err, "failed to build tick runtime")
			}
			defer func() {
				if err := deps.repo.Close(); err != nil {
					errutil.Handle(ctx, goerr.Wrap(err, "close repo"), "close repo")
				}
			}()

			if err := deps.scanner.Scan(ctx); err != nil {
				return goerr.Wrap(err, "tick sweep failed")
			}

			// Wait for every async dispatch the publisher launched so the
			// process does not exit before the Job goroutines finish.
			async.Wait()

			logger.Info("tick sweep complete")
			return nil
		},
	}
}
