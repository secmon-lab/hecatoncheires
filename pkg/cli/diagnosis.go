package cli

import (
	"context"
	"log/slog"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/diagnosis"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// cmdDiagnosis is the umbrella subcommand for one-shot data inspection /
// repair jobs. The command itself does no work; each sub-subcommand wires the
// minimum dependencies it needs and delegates to a usecase under
// pkg/usecase/diagnosis. Repair logic must NOT live in this CLI layer.
func cmdDiagnosis() *cli.Command {
	return &cli.Command{
		Name:  "diagnosis",
		Usage: "Run one-shot data inspection / repair jobs",
		Commands: []*cli.Command{
			cmdFixUnsentAction(),
		},
	}
}

// cmdFixUnsentAction wires repo + registry + Slack service into the
// diagnosis usecase and runs FixUnsentActions. The CLI flag surface mirrors
// the subset of `serve` flags that the repair actually depends on
// (repository, workspace registry, Slack, base URL). LLM / GitHub / Notion
// configuration is intentionally omitted: this job only re-posts existing
// Action data, so spinning up unrelated services would just be noise.
func cmdFixUnsentAction() *cli.Command {
	var baseURL string
	var defaultLangStr string
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var slackCfg config.Slack
	var sentryCfg config.Sentry

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "base-url",
			Usage:       "Base URL for the application (e.g., https://your-domain.com)",
			Sources:     cli.EnvVars("HECATONCHEIRES_BASE_URL"),
			Destination: &baseURL,
		},
		&cli.StringFlag{
			Name:        "default-lang",
			Usage:       "Default language for Slack messages (en, ja)",
			Value:       "en",
			Sources:     cli.EnvVars("HECATONCHEIRES_DEFAULT_LANG"),
			Destination: &defaultLangStr,
		},
	}
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, slackCfg.Flags()...)
	flags = append(flags, sentryCfg.Flags()...)

	return &cli.Command{
		Name:  "fix-unsent-action",
		Usage: "Re-post Slack messages for actions whose initial post never reached Slack",
		Flags: flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			logger := logging.From(ctx)

			sentryCfg.Configure(ctx)

			defaultLang, err := i18n.ParseLang(defaultLangStr)
			if err != nil {
				return goerr.Wrap(err, "invalid default-lang value")
			}
			i18n.Init(defaultLang)

			_, registry, err := appCfg.Configure(c)
			if err != nil {
				return goerr.Wrap(err, "failed to load workspace configurations")
			}

			repo, err := repoCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize repository")
			}
			defer func() {
				if closeErr := repo.Close(); closeErr != nil {
					logger.Error("failed to close repository", slog.String("error", closeErr.Error()))
				}
			}()

			botToken := slackCfg.BotToken()
			if botToken == "" {
				// The whole point of this job is to post Slack messages, so
				// running it without a bot token is almost certainly a
				// configuration mistake — fail fast rather than silently
				// produce a sweep of zero fixes.
				return goerr.New("slack bot token is required for fix-unsent-action")
			}
			slackSvc, err := slack.New(botToken)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize slack service")
			}

			actionUC := usecase.NewActionUseCase(repo, registry, slackSvc, baseURL)
			diagUC := diagnosis.New(repo, registry, actionUC)

			report, err := diagUC.FixUnsentActions(ctx)
			if err != nil {
				return goerr.Wrap(err, "fix-unsent-action sweep failed")
			}

			logger.Info("fix-unsent-action complete",
				slog.Int("total", report.Total),
				slog.Int("fixed", report.Fixed),
				slog.Int("skipped", report.Skipped),
				slog.Int("failed", report.Failed),
			)
			return nil
		},
	}
}
