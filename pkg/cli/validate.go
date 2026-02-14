package cli

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdValidate() *cli.Command {
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var checkDB bool

	var flags []cli.Flag
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, &cli.BoolFlag{
		Name:        "check-db",
		Usage:       "Perform database consistency check",
		Sources:     cli.EnvVars("HECATONCHEIRES_CHECK_DB"),
		Destination: &checkDB,
	})

	return &cli.Command{
		Name:    "validate",
		Aliases: []string{"v"},
		Usage:   "Validate configuration files and optionally check DB consistency",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.Default()

			// Step 1: Load and validate configuration files
			workspaceConfigs, registry, err := appCfg.Configure(c)
			if err != nil {
				return goerr.Wrap(err, "configuration validation failed")
			}

			logger.Info("Configuration validation passed",
				"workspace_count", len(workspaceConfigs),
			)
			for _, wc := range workspaceConfigs {
				logger.Info("Workspace validated",
					"id", wc.ID,
					"name", wc.Name,
					"field_count", len(wc.FieldSchema.Fields),
				)
			}

			// Step 2: If --check-db is specified, run DB consistency check
			if !checkDB {
				logger.Info("`--check-db` not specified, skipping DB consistency check")
				return nil
			}

			repo, err := repoCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize repository for DB check")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logger.Error("failed to close repository", "error", err.Error())
				}
			}()

			logger.Info("Using repository for DB check",
				"backend", repoCfg.Backend(),
			)

			// Run DB consistency check
			uc := usecase.New(repo, registry)
			validationResult, err := uc.ValidateDB(ctx)
			if err != nil {
				return goerr.Wrap(err, "DB consistency check failed")
			}

			if validationResult.HasIssues() {
				for _, issue := range validationResult.Issues {
					logger.Warn("DB consistency issue found",
						"workspace_id", issue.WorkspaceID,
						"case_id", issue.CaseID,
						"field_id", issue.FieldID,
						"message", issue.Message,
						"expected", issue.Expected,
						"actual", issue.Actual,
					)
				}

				return fmt.Errorf("DB consistency check found %d issue(s)", len(validationResult.Issues))
			}

			logger.Info("DB consistency check passed")
			return nil
		},
	}
}
