package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/m-mizutani/goerr/v2"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/worker"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// graphqlErrorStatusMiddleware wraps the GraphQL handler to return HTTP 500 when errors occur
func graphqlErrorStatusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the response
		rec := &responseRecorder{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		// Check if response contains GraphQL errors
		var gqlResp struct {
			Errors []interface{} `json:"errors"`
		}
		if err := json.Unmarshal(rec.body.Bytes(), &gqlResp); err == nil && len(gqlResp.Errors) > 0 {
			// GraphQL errors found, set status to 500
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(rec.statusCode)
		}

		// Write the captured body to the original writer
		_, _ = w.Write(rec.body.Bytes())
	})
}

// responseRecorder captures HTTP responses for inspection
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	// Don't write header yet, we'll do it later after inspecting the body
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func cmdServe() *cli.Command {
	var addr string
	var baseURL string
	var enableGraphiQL bool
	var notionToken string
	var noAuthUID string
	var appCfg config.AppConfig
	var repoCfg config.Repository
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
			Name:        "notion-api-token",
			Usage:       "Notion API token for Source integration",
			Sources:     cli.EnvVars("HECATONCHEIRES_NOTION_API_TOKEN"),
			Destination: &notionToken,
		},
		&cli.StringFlag{
			Name:        "no-auth",
			Usage:       "Skip authentication and run as specified Slack user ID (development only). Requires --slack-bot-token. Example: --no-auth=U1234567890",
			Category:    "Authentication",
			Sources:     cli.EnvVars("HECATONCHEIRES_NO_AUTH"),
			Destination: &noAuthUID,
		},
	}

	// Add shared config flags
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, slackCfg.Flags()...)

	return &cli.Command{
		Name:    "serve",
		Aliases: []string{"s"},
		Usage:   "Start HTTP server",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			// Load workspace configurations and build registry
			_, registry, err := appCfg.Configure(c)
			if err != nil {
				return goerr.Wrap(err, "failed to load workspace configurations")
			}

			// Initialize repository based on backend type
			repo, err := repoCfg.Configure(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize repository")
			}
			defer func() {
				if err := repo.Close(); err != nil {
					logging.Default().Error("failed to close repository", "error", err.Error())
				}
			}()

			// Set no-auth UID if provided
			if noAuthUID != "" {
				slackCfg.SetNoAuthUID(noAuthUID)
			}

			// Configure authentication
			authUC, err := slackCfg.Configure(ctx, repo, baseURL)
			if err != nil {
				return goerr.Wrap(err, "failed to configure authentication")
			}

			if slackCfg.IsNoAuthMode() {
				logging.Default().Warn("Running in no-auth mode (development only)", "user_id", noAuthUID)
			} else if slackCfg.IsConfigured() {
				logging.Default().Info("Slack authentication enabled")
			}

			// Initialize use cases with configuration and auth
			ucOpts := []usecase.Option{
				usecase.WithAuth(authUC),
				usecase.WithBaseURL(baseURL),
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
			var slackSvc slack.Service
			if slackCfg.BotToken() != "" {
				svc, err := slack.New(slackCfg.BotToken())
				if err != nil {
					return goerr.Wrap(err, "failed to initialize slack service")
				}
				slackSvc = svc
				ucOpts = append(ucOpts, usecase.WithSlackService(slackSvc))
				logging.Default().Info("Slack service enabled for Source integration")
			} else {
				logging.Default().Info("Slack Bot Token not configured, Slack Source features will be limited")
			}

			uc := usecase.New(repo, registry, ucOpts...)

			// Start Slack user refresh worker if Slack service is available
			// N+1 Prevention Policy: Worker uses DeleteAll → SaveMany (Replace strategy)
			// to avoid individual DB operations in loops
			var slackUserWorker *worker.SlackUserRefreshWorker
			if slackSvc != nil {
				slackUserWorker = worker.NewSlackUserRefreshWorker(repo, slackSvc, 10*time.Minute)
				if err := slackUserWorker.Start(ctx); err != nil {
					return goerr.Wrap(err, "failed to start Slack user refresh worker")
				}
			}

			// Create GraphQL handler with dataloaders
			resolver := gqlctrl.NewResolver(repo, uc)
			srv := handler.NewDefaultServer(
				gqlctrl.NewExecutableSchema(gqlctrl.Config{Resolvers: resolver}),
			)

			// Configure error presenter with stack traces
			srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
				// Convert to GraphQL error first
				gqlErr := graphql.DefaultErrorPresenter(ctx, err)

				// Wrap error with goerr and log with stack trace
				wrappedErr := goerr.Wrap(err, "GraphQL error")
				logging.Default().Error("GraphQL error occurred", "error", wrappedErr)

				return gqlErr
			})

			// Configure panic handler
			srv.SetRecoverFunc(func(ctx context.Context, panicValue interface{}) error {
				// Create error from panic value
				var panicErr error
				switch e := panicValue.(type) {
				case error:
					panicErr = e
				case string:
					panicErr = goerr.New(e)
				default:
					panicErr = goerr.New("panic occurred", goerr.V("panic", panicValue))
				}

				// Wrap and log with stack trace
				wrappedErr := goerr.Wrap(panicErr, "GraphQL panic")
				logging.Default().Error("GraphQL panic occurred", "error", wrappedErr)

				return wrappedErr
			})

			// Wrap with dataloader middleware (request-scoped)
			gqlHandlerBase := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Create new DataLoaders for each request
				loaders := gqlctrl.NewDataLoaders(repo)
				ctx := gqlctrl.WithDataLoaders(r.Context(), loaders)
				srv.ServeHTTP(w, r.WithContext(ctx))
			})

			// Wrap with error status middleware to return HTTP 500 on GraphQL errors
			gqlHandler := graphqlErrorStatusMiddleware(gqlHandlerBase)

			// Create HTTP server options
			httpOpts := []httpctrl.Options{
				httpctrl.WithGraphiQL(enableGraphiQL),
				httpctrl.WithAuth(authUC),
				httpctrl.WithWorkspaceRegistry(registry),
			}

			// Add Slack service if configured
			if slackSvc != nil {
				httpOpts = append(httpOpts, httpctrl.WithSlackService(slackSvc))
			}

			// Add Slack webhook handler if configured
			if slackCfg.IsWebhookConfigured() {
				slackWebhookHandler := httpctrl.NewSlackWebhookHandler(uc.Slack)
				httpOpts = append(httpOpts, httpctrl.WithSlackWebhook(slackWebhookHandler, slackCfg.SigningSecret()))
				logging.Default().Info("Slack webhook handler enabled")

				// Add Slack interaction handler (shares signing secret with webhook)
				slackInteractionHandler := httpctrl.NewSlackInteractionHandler(uc.Action)
				httpOpts = append(httpOpts, httpctrl.WithSlackInteraction(slackInteractionHandler))
				logging.Default().Info("Slack interaction handler enabled")
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

				// Stop Slack user refresh worker first
				if slackUserWorker != nil {
					slackUserWorker.Stop()
				}

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
