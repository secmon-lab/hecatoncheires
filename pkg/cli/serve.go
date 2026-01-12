package cli

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdServe() *cli.Command {
	var addr string
	var baseURL string
	var enableGraphiQL bool
	var projectID string
	var databaseID string
	var configPath string
	var notionToken string
	var slackCfg config.Slack

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:        "addr",
			Usage:       "HTTP server address",
			Value:       ":8080",
			Sources:     cli.EnvVars("HECATONCHEIRES_ADDR"),
			Destination: &addr,
		},
		&cli.StringFlag{
			Name:        "base-url",
			Usage:       "Base URL for the application (e.g., https://your-domain.com)",
			Sources:     cli.EnvVars("HECATONCHEIRES_BASE_URL"),
			Destination: &baseURL,
		},
		&cli.BoolFlag{
			Name:        "graphiql",
			Usage:       "Enable GraphiQL playground",
			Value:       true,
			Sources:     cli.EnvVars("HECATONCHEIRES_GRAPHIQL"),
			Destination: &enableGraphiQL,
		},
		&cli.StringFlag{
			Name:        "config",
			Usage:       "Path to configuration file (TOML)",
			Value:       "./config.toml",
			Sources:     cli.EnvVars("HECATONCHEIRES_CONFIG"),
			Destination: &configPath,
		},
		&cli.StringFlag{
			Name:        "firestore-project-id",
			Usage:       "Firestore Project ID (required)",
			Required:    true,
			Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_PROJECT_ID", "GCP_PROJECT_ID"),
			Destination: &projectID,
		},
		&cli.StringFlag{
			Name:        "firestore-database-id",
			Usage:       "Firestore Database ID",
			Value:       "(default)",
			Sources:     cli.EnvVars("HECATONCHEIRES_FIRESTORE_DATABASE_ID"),
			Destination: &databaseID,
		},
		&cli.StringFlag{
			Name:        "notion-api-token",
			Usage:       "Notion API token for Source integration",
			Sources:     cli.EnvVars("HECATONCHEIRES_NOTION_API_TOKEN"),
			Destination: &notionToken,
		},
	}

	// Add Slack flags
	flags = append(flags, slackCfg.Flags()...)

	return &cli.Command{
		Name:    "serve",
		Aliases: []string{"s"},
		Usage:   "Start HTTP server",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			// Load application configuration
			var appConfig *config.AppConfig
			if _, err := os.Stat(configPath); err == nil {
				appConfig, err = config.LoadAppConfiguration(configPath)
				if err != nil {
					return goerr.Wrap(err, "failed to load configuration file", goerr.V("path", configPath))
				}
				logging.Default().Info("Configuration loaded", "path", configPath)
			} else if !os.IsNotExist(err) {
				return goerr.Wrap(err, "failed to check configuration file", goerr.V("path", configPath))
			} else {
				logging.Default().Warn("Configuration file not found, using empty configuration", "path", configPath)
				appConfig = &config.AppConfig{}
			}

			// Initialize Firestore repository
			repo, err := firestore.New(ctx, projectID, databaseID)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize firestore repository")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logging.Default().Error("failed to close firestore repository", "error", err.Error())
				}
			}()

			// Configure authentication
			authUC, err := slackCfg.Configure(repo, baseURL)
			if err != nil {
				return goerr.Wrap(err, "failed to configure authentication")
			}

			if slackCfg.IsConfigured() {
				logging.Default().Info("Slack authentication enabled")
			} else {
				logging.Default().Info("No authentication configured, running in anonymous mode")
			}

			// Initialize use cases with configuration and auth
			riskConfig := appConfig.ToDomainRiskConfig()
			ucOpts := []usecase.Option{
				usecase.WithRiskConfig(riskConfig),
				usecase.WithAuth(authUC),
			}

			// Initialize Notion service if token is provided
			if notionToken != "" {
				notionSvc, err := notion.New(notionToken)
				if err != nil {
					return goerr.Wrap(err, "failed to initialize notion service")
				}
				ucOpts = append(ucOpts, usecase.WithNotion(notionSvc))
				logging.Default().Info("Notion service enabled")
			} else {
				logging.Default().Info("Notion API token not configured, Source features will be limited")
			}

			// Initialize Slack service for Source integration if bot token is provided
			if slackCfg.BotToken() != "" {
				slackSvc, err := slack.New(slackCfg.BotToken())
				if err != nil {
					return goerr.Wrap(err, "failed to initialize slack service")
				}
				ucOpts = append(ucOpts, usecase.WithSlackService(slackSvc))
				logging.Default().Info("Slack service enabled for Source integration")
			} else {
				logging.Default().Info("Slack Bot Token not configured, Slack Source features will be limited")
			}

			uc := usecase.New(repo, ucOpts...)

			// Create GraphQL handler with dataloaders
			resolver := graphql.NewResolver(repo, uc)
			srv := handler.NewDefaultServer(
				graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}),
			)

			// Wrap with dataloader middleware
			loaders := graphql.NewDataLoaders(repo)
			gqlHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := graphql.WithDataLoaders(r.Context(), loaders)
				srv.ServeHTTP(w, r.WithContext(ctx))
			})

			// Create HTTP server options
			httpOpts := []httpctrl.Options{
				httpctrl.WithGraphiQL(enableGraphiQL),
				httpctrl.WithAuth(authUC),
			}

			// Add Slack webhook handler if configured
			if slackCfg.IsWebhookConfigured() {
				slackWebhookHandler := httpctrl.NewSlackWebhookHandler(uc.Slack)
				httpOpts = append(httpOpts, httpctrl.WithSlackWebhook(slackWebhookHandler, slackCfg.SigningSecret()))
				logging.Default().Info("Slack webhook handler enabled")
			}

			// Create HTTP server
			httpHandler, err := httpctrl.New(gqlHandler, httpOpts...)
			if err != nil {
				return goerr.Wrap(err, "failed to create http server")
			}
			server := &http.Server{
				Addr:              addr,
				Handler:           httpHandler,
				ReadHeaderTimeout: 30 * time.Second,
			}

			// Setup signal handling for graceful shutdown
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			// Start server in goroutine
			errCh := make(chan error, 1)
			go func() {
				logging.Default().Info("Starting HTTP server", "addr", addr, "graphiql", enableGraphiQL)
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- goerr.Wrap(err, "failed to start server")
				}
			}()

			// Wait for shutdown signal or server error
			select {
			case err := <-errCh:
				return err
			case sig := <-sigCh:
				logging.Default().Info("Received shutdown signal", "signal", sig)

				// Create shutdown context with timeout
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				// Attempt graceful shutdown
				if err := server.Shutdown(shutdownCtx); err != nil {
					return goerr.Wrap(err, "failed to shutdown server gracefully")
				}

				logging.Default().Info("Server shutdown completed")
				return nil
			}
		},
	}
}
