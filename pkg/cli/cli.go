package cli

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func Run(ctx context.Context, args []string, version string) error {
	var loggerCfg config.Logger
	var closer func()

	app := &cli.Command{
		Name:    "hecatoncheires",
		Usage:   "Hecatoncheires AI native risk management system",
		Version: version,
		Flags:   loggerCfg.Flags(),
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			f, err := loggerCfg.Configure()
			if err != nil {
				return ctx, err
			}
			closer = f

			logging.Default().Info("Starting hecatoncheires", "logger", loggerCfg)
			return ctx, nil
		},
		After: func(ctx context.Context, c *cli.Command) error {
			if closer != nil {
				closer()
			}
			return nil
		},
		Commands: []*cli.Command{
			cmdServe(),
			cmdMigrate(),
		},
	}

	if err := app.Run(ctx, args); err != nil {
		logging.Default().Error("failed to run app", "error", err)
		return err
	}

	return nil
}
