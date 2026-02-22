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

			// Create fireconf client with configuration
			opts := []fireconf.Option{
				fireconf.WithLogger(logger),
				fireconf.WithDryRun(dryRun),
			}

			client, err := fireconf.New(ctx, projectID, databaseID, indexConfig, opts...)
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

				// Import current Firestore state to compare against desired config.
				// DiffConfigs expects current (actual) state as argument, not the desired config.
				currentConfig, err := client.Import(ctx)
				if err != nil {
					return goerr.Wrap(err, "failed to import current Firestore config")
				}

				diff, err := client.DiffConfigs(currentConfig)
				if err != nil {
					return goerr.Wrap(err, "failed to calculate diff")
				}

				if len(diff.Collections) == 0 {
					logger.Info("No changes required")
					return nil
				}

				for _, col := range diff.Collections {
					logger.Info("Collection diff",
						"collection", col.Name,
						"action", col.Action,
						"indexes_to_add", len(col.IndexesToAdd),
						"indexes_to_delete", len(col.IndexesToDelete))
				}
			} else {
				logger.Info("Applying migrations")
				if err := client.Migrate(ctx); err != nil {
					return goerr.Wrap(err, "failed to apply migrations")
				}
				logger.Info("Migrations applied successfully")
			}

			return nil
		},
	}
}

// getIndexConfig returns the Firestore index configuration.
// Knowledges and memories are stored in subcollections
// (workspaces/{workspaceID}/knowledges/, workspaces/{workspaceID}/cases/{caseID}/memories/).
// Firestore vector indexes require COLLECTION scope for FindNearest queries
// on specific subcollections. The collection-group name ensures the index
// applies across all workspace subcollections.
func getIndexConfig() *fireconf.Config {
	return &fireconf.Config{
		Collections: []fireconf.Collection{
			{
				Name: "knowledges",
				Indexes: []fireconf.Index{
					// Vector search index for embedding similarity search.
					// Field name matches the Go struct field Knowledge.Embedding
					// stored as firestore.Vector32.
					{
						QueryScope: fireconf.QueryScopeCollection,
						Fields: []fireconf.IndexField{
							{
								Path: "Embedding",
								Vector: &fireconf.VectorConfig{
									Dimension: model.EmbeddingDimension,
								},
							},
						},
					},
				},
			},
			{
				Name: "memories",
				Indexes: []fireconf.Index{
					// Vector search index for memory embedding similarity search.
					// Field name matches the Go struct field Memory.Embedding
					// stored as firestore.Vector32.
					{
						QueryScope: fireconf.QueryScopeCollection,
						Fields: []fireconf.IndexField{
							{
								Path: "Embedding",
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
