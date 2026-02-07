package cli

import (
	"context"

	"github.com/m-mizutani/fireconf"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdMigrate() *cli.Command {
	var projectID string
	var databaseID string
	var dryRun bool

	return &cli.Command{
		Name:    "migrate",
		Aliases: []string{"m"},
		Usage:   "Migrate Firestore indexes",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "firestore-project-id",
				Usage:       "Firestore Project ID (required)",
				Required:    true,
				Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_PROJECT_ID"),
				Destination: &projectID,
			},
			&cli.StringFlag{
				Name:        "firestore-database-id",
				Usage:       "Firestore Database ID",
				Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_DATABASE_ID"),
				Destination: &databaseID,
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "Preview changes without applying",
				Destination: &dryRun,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.Default()

			logger.Info("Migrate configuration",
				"projectID", projectID,
				"databaseID", databaseID,
				"dryRun", dryRun)

			// Get index configuration
			indexConfig := getIndexConfig()

			// Create fireconf client
			client, err := fireconf.NewClient(ctx, projectID, databaseID)
			if err != nil {
				return goerr.Wrap(err, "failed to create fireconf client")
			}
			defer func() {
				if err := client.Close(); err != nil {
					logger.Error("failed to close fireconf client", "error", err.Error())
				}
			}()

			if dryRun {
				logger.Info("Dry run mode - previewing changes")
				plan, err := client.GetMigrationPlan(ctx, indexConfig)
				if err != nil {
					return goerr.Wrap(err, "failed to create migration plan")
				}

				if len(plan.Steps) == 0 {
					logger.Info("No changes required")
					return nil
				}

				for _, step := range plan.Steps {
					logger.Info("Migration step",
						"collection", step.Collection,
						"operation", step.Operation,
						"description", step.Description,
						"destructive", step.Destructive)
				}
			} else {
				logger.Info("Applying migrations")
				if err := client.Migrate(ctx, indexConfig); err != nil {
					return goerr.Wrap(err, "failed to apply migrations")
				}
				logger.Info("Migrations applied successfully")
			}

			return nil
		},
	}
}

// getIndexConfig returns the Firestore index configuration
func getIndexConfig() *fireconf.Config {
	return &fireconf.Config{
		Collections: []fireconf.Collection{
			{
				Name: "knowledges",
				Indexes: []fireconf.Index{
					// ListByRiskID: risk_id ASC, sourced_at DESC
					{
						Fields: []fireconf.IndexField{
							{Path: "risk_id", Order: fireconf.OrderAscending},
							{Path: "sourced_at", Order: fireconf.OrderDescending},
						},
					},
					// ListBySourceID: source_id ASC, sourced_at DESC
					{
						Fields: []fireconf.IndexField{
							{Path: "source_id", Order: fireconf.OrderAscending},
							{Path: "sourced_at", Order: fireconf.OrderDescending},
						},
					},
					// Vector search index
					{
						Fields: []fireconf.IndexField{
							{
								Path: "embedding",
								Vector: &fireconf.VectorConfig{
									Dimension: model.EmbeddingDimension,
								},
							},
						},
					},
				},
			},
		},
	}
}
