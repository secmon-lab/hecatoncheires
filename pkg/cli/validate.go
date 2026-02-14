package cli

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdValidate() *cli.Command {
	var appCfg config.AppConfig
	var firestoreProjectID string
	var firestoreDatabaseID string

	var flags []cli.Flag
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, &cli.StringFlag{
		Name:        "firestore-project-id",
		Usage:       "Firestore Project ID (if specified, DB consistency check is performed)",
		Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_PROJECT_ID"),
		Destination: &firestoreProjectID,
	})
	flags = append(flags, &cli.StringFlag{
		Name:        "firestore-database-id",
		Usage:       "Firestore Database ID",
		Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_DATABASE_ID"),
		Destination: &firestoreDatabaseID,
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

			// Step 2: If Firestore project ID is specified, run DB consistency check
			if firestoreProjectID == "" {
				logger.Info("No Firestore project ID specified, skipping DB consistency check")
				return nil
			}

			repo, err := firestore.New(ctx, firestoreProjectID, firestoreDatabaseID)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize Firestore repository")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logger.Error("failed to close repository", "error", err.Error())
				}
			}()

			logger.Info("Using Firestore repository",
				"project_id", firestoreProjectID,
				"database_id", firestoreDatabaseID,
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
