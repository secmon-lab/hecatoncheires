package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/bqexport"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"github.com/urfave/cli/v3"
)

// cmdExport builds the `export` subcommand: read the current data of every
// workspace listed under [export] in the global config and full-refresh it into
// BigQuery, one dataset per workspace and one table per entity.
func cmdExport() *cli.Command {
	var (
		repoCfg config.Repository
		appCfg  config.AppConfig
	)
	flags := append(repoCfg.Flags(), appCfg.Flags()...)

	return &cli.Command{
		Name:  "export",
		Usage: "Export the current data of every configured workspace to BigQuery",
		Flags: flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.From(ctx)

			repo, err := repoCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to configure repository")
			}
			defer safe.Close(ctx, repo)

			_, registry, err := appCfg.Configure(c)
			if err != nil {
				return goerr.Wrap(err, "failed to load workspace configuration")
			}

			exportCfg, err := appCfg.ConfigureExport(c, registry)
			if err != nil {
				return goerr.Wrap(err, "failed to load export configuration")
			}
			if exportCfg == nil {
				return cli.Exit("no [export] section found in --global-config; nothing to export", 1)
			}

			targets, err := buildExportTargets(exportCfg, registry)
			if err != nil {
				return err
			}

			sink, err := bqexport.New(ctx, exportCfg.BigQuery.Project, exportCfg.BigQuery.Location)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize BigQuery sink")
			}
			defer safe.Close(ctx, sink)

			logger.Info("starting export",
				"project", exportCfg.BigQuery.Project,
				"workspaces", len(targets))

			exporter := export.New(repo, sink)
			if err := exporter.Run(ctx, targets); err != nil {
				return goerr.Wrap(err, "export completed with errors")
			}

			logger.Info("export finished", "workspaces", len(targets))
			return nil
		},
	}
}

// buildExportTargets resolves each export workspace mapping to a Target. The
// mapping was already validated (workspace existence + dataset format) by
// ConfigureExport, so a Get failure here is unexpected but still surfaced.
func buildExportTargets(cfg *config.ExportSection, registry *model.WorkspaceRegistry) ([]export.Target, error) {
	targets := make([]export.Target, 0, len(cfg.BigQuery.Workspaces))
	for _, m := range cfg.BigQuery.Workspaces {
		entry, err := registry.Get(m.ID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to resolve export workspace",
				goerr.V("workspace_id", m.ID))
		}
		targets = append(targets, export.Target{
			Entry:          entry,
			Namespace:      m.Dataset,
			ExcludePrivate: !cfg.IncludePrivateFor(m),
		})
	}
	return targets, nil
}
