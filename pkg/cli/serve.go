package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/m-mizutani/goerr/v2"
	"github.com/vektah/gqlparser/v2/gqlerror"

	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	gqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/worker"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// logAttrsToArgs converts slog.Attr slice to slog.Logger compatible args
func logAttrsToArgs(attrs []slog.Attr) []any {
	args := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		args = append(args, a.Key, a.Value.Any())
	}
	return args
}

// graphqlErrorStatusMiddleware maps GraphQL error responses to an appropriate
// HTTP status. The ErrorPresenter tags client-faulted errors (validation,
// not-found, access-denied) with extensions.code; this middleware reads those
// codes and returns 4xx for them. Genuine server faults stay 5xx.
func graphqlErrorStatusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &responseRecorder{
			ResponseWriter: w,
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		var gqlResp struct {
			Errors []gqlErrorEnvelope `json:"errors"`
		}

		if err := json.Unmarshal(rec.body.Bytes(), &gqlResp); err == nil && len(gqlResp.Errors) > 0 {
			w.WriteHeader(httpStatusForGraphQLErrors(gqlResp.Errors))
		} else {
			w.WriteHeader(rec.statusCode)
		}

		_, _ = w.Write(rec.body.Bytes())
	})
}

// gqlErrorEnvelope is the shape we care about in a GraphQL error response —
// just enough to read extensions.code for HTTP status mapping.
type gqlErrorEnvelope struct {
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

// httpStatusForGraphQLErrors picks the worst (most-server-faulty) HTTP status
// across all errors. If every error is a tagged client error we return its
// 4xx; otherwise we fall back to 500.
func httpStatusForGraphQLErrors(errs []gqlErrorEnvelope) int {
	worst := 0
	for _, e := range errs {
		s := statusForExtensionCode(e.Extensions.Code)
		if s == 0 {
			return http.StatusInternalServerError
		}
		if s > worst {
			worst = s
		}
	}
	if worst == 0 {
		return http.StatusInternalServerError
	}
	return worst
}

// classifyError maps a domain/usecase error to a GraphQL extensions.code.
// Returning "" leaves the error untagged, which the HTTP middleware treats
// as a server fault (500).
func classifyError(err error) string {
	switch {
	case errors.Is(err, model.ErrInvalidFieldType),
		errors.Is(err, model.ErrInvalidOptionID),
		errors.Is(err, model.ErrMissingRequired),
		errors.Is(err, model.ErrInvalidNotionID),
		errors.Is(err, model.ErrInvalidGitHubRepo):
		return "BAD_USER_INPUT"
	case errors.Is(err, usecase.ErrCaseNotFound),
		errors.Is(err, usecase.ErrActionNotFound),
		errors.Is(err, model.ErrWorkspaceNotFound):
		return "NOT_FOUND"
	case errors.Is(err, usecase.ErrAccessDenied):
		return "FORBIDDEN"
	case errors.Is(err, usecase.ErrCaseAlreadyClosed),
		errors.Is(err, usecase.ErrCaseAlreadyOpen),
		errors.Is(err, usecase.ErrDuplicateField):
		return "CONFLICT"
	}
	return ""
}

func isClientError(err error) bool {
	return classifyError(err) != ""
}

func statusForExtensionCode(code string) int {
	switch code {
	case "BAD_USER_INPUT":
		return http.StatusBadRequest
	case "NOT_FOUND":
		return http.StatusNotFound
	case "FORBIDDEN":
		return http.StatusForbidden
	case "CONFLICT":
		return http.StatusConflict
	case "UNAUTHENTICATED":
		return http.StatusUnauthorized
	default:
		return 0
	}
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
	var defaultLangStr string
	var appCfg config.AppConfig
	var repoCfg config.Repository
	var slackCfg config.Slack
	var llmCfg config.LLM
	var githubCfg config.GitHub
	var storageCfg config.Storage

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
		&cli.StringFlag{
			Name:        "default-lang",
			Usage:       "Default language for UI and Slack messages (en, ja)",
			Value:       "en",
			Sources:     cli.EnvVars("HECATONCHEIRES_DEFAULT_LANG"),
			Destination: &defaultLangStr,
		},
	}

	// Add shared config flags
	flags = append(flags, appCfg.Flags()...)
	flags = append(flags, repoCfg.Flags()...)
	flags = append(flags, slackCfg.Flags()...)
	flags = append(flags, llmCfg.Flags()...)
	flags = append(flags, githubCfg.Flags()...)
	flags = append(flags, storageCfg.Flags()...)

	return &cli.Command{
		Name:    "serve",
		Aliases: []string{"s"},
		Usage:   "Start HTTP server",
		Flags:   flags,
		Action: func(ctx context.Context, c *cli.Command) error {
			// Load workspace configurations and build registry
			workspaceConfigs, registry, err := appCfg.Configure(c)
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
				logging.Default().Info("Slack authentication enabled", logAttrsToArgs(slackCfg.LogAttrs())...)
			}

			// Parse and initialize translator
			defaultLang, err := i18n.ParseLang(defaultLangStr)
			if err != nil {
				return goerr.Wrap(err, "invalid default-lang value")
			}
			i18n.Init(defaultLang)
			logging.Default().Info("i18n initialized", "default_lang", string(defaultLang))

			// Initialize use cases with configuration and auth
			ucOpts := []usecase.Option{
				usecase.WithAuth(authUC),
				usecase.WithBaseURL(baseURL),
			}

			// Initialize Notion services if token is provided. Two clients are
			// constructed off the same token: pkg/service/notion drives
			// Source/Compile, and pkg/agent/tool/notion drives the agent's
			// notion__search / notion__get_page tools.
			if notionToken != "" {
				notionSvc, err := notion.New(notionToken)
				if err != nil {
					return goerr.Wrap(err, "failed to initialize notion service")
				}
				ucOpts = append(ucOpts, usecase.WithNotion(notionSvc))

				notionToolClient, err := notiontool.NewClient(notionToken)
				if err != nil {
					return goerr.Wrap(err, "failed to initialize notion tool client")
				}
				ucOpts = append(ucOpts, usecase.WithNotionToolClient(notionToolClient))

				logging.Default().Info("Notion service enabled")
			} else {
				logging.Default().Info("Notion API token not configured, Source features and Notion agent tools disabled")
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

				// Initialize Slack Admin / Search services if User OAuth Token is provided.
				// The same User OAuth Token backs both:
				//   - admin.conversations.* (cross-workspace channel connect)
				//   - search.messages       (agent message search; needs search:read scope)
				if slackCfg.UserOAuthToken() != "" {
					adminSvc, err := slack.NewAdminClient(slackCfg.UserOAuthToken())
					if err != nil {
						return goerr.Wrap(err, "failed to initialize Slack admin service")
					}
					ucOpts = append(ucOpts, usecase.WithSlackAdminService(adminSvc))
					logging.Default().Info("Slack admin service enabled for cross-workspace channel connect")

					searchSvc, err := slacktool.NewSearchClient(slackCfg.UserOAuthToken())
					if err != nil {
						return goerr.Wrap(err, "failed to initialize Slack search service")
					}
					ucOpts = append(ucOpts, usecase.WithSlackSearchService(searchSvc))
					logging.Default().Info("Slack search service enabled for agent (requires search:read scope)")
				}

				// Detect org-level app and validate workspace team IDs
				if err := slackCfg.DetectOrgLevel(ctx); err != nil {
					return goerr.Wrap(err, "failed to detect Slack app level")
				}
				if slackCfg.IsOrgLevel() {
					logging.Default().Info("Detected org-level Slack app",
						"enterprise_id", slackCfg.EnterpriseID(),
						"team_id", slackCfg.AuthTeamID(),
					)
				} else {
					logging.Default().Info("Detected workspace-level Slack app",
						"team_id", slackCfg.AuthTeamID(),
					)
				}
				if err := slackCfg.ValidateWorkspaceTeamIDs(workspaceConfigs); err != nil {
					return goerr.Wrap(err, "workspace slack.team_id validation failed")
				}
			} else {
				logging.Default().Info("Slack Bot Token not configured, Slack Source features will be limited")
			}

			// Initialize LLM client. Required for Slack-based features
			// (agent / assist / mention-draft) — usecase.New enforces that
			// strictly when slackService is configured. When LLM isn't set
			// up (e.g. e2e tests without API keys) we run in a degraded
			// mode that still serves the GraphQL API + frontend.
			llmClient, err := llmCfg.NewClient(ctx)
			if err != nil {
				return goerr.Wrap(err, "failed to initialize LLM client")
			}
			if llmClient == nil {
				logging.Default().Warn("LLM provider not configured; Slack-driven AI features will be unavailable")
			} else {
				ucOpts = append(ucOpts, usecase.WithLLMClient(llmClient))
				logging.Default().Info("LLM client enabled", logAttrsToArgs(llmCfg.LogAttrs())...)
			}

			// Initialize GitHub service if configured
			githubSvc, err := githubCfg.Configure()
			if err != nil {
				return goerr.Wrap(err, "failed to initialize GitHub service")
			}
			if githubSvc != nil {
				ucOpts = append(ucOpts, usecase.WithGitHubService(githubSvc))
				logging.Default().Info("GitHub service enabled", logAttrsToArgs(githubCfg.LogAttrs())...)
			} else {
				logging.Default().Info("GitHub App not configured, GitHub Source features will be disabled")
			}

			// Configure agent session archive (Cloud Storage) when Slack is wired.
			// Slack-driven AI flows (mention agent) require History + Trace
			// persistence; the bucket flag is mandatory in that case.
			var storageCleanup func()
			if slackSvc != nil {
				historyRepo, traceRepo, cleanup, err := storageCfg.Configure(ctx)
				if err != nil {
					return goerr.Wrap(err, "failed to configure agent storage")
				}
				storageCleanup = cleanup
				ucOpts = append(ucOpts, usecase.WithHistoryRepository(historyRepo))
				ucOpts = append(ucOpts, usecase.WithTraceRepository(traceRepo))
				logging.Default().Info("Agent session archive enabled", logAttrsToArgs(storageCfg.LogAttrs())...)
			}
			defer func() {
				if storageCleanup != nil {
					storageCleanup()
				}
			}()

			uc := usecase.New(repo, registry, ucOpts...)

			// Start Slack user refresh worker if Slack service is available
			// N+1 Prevention Policy: Worker uses DeleteAll → SaveMany (Replace strategy)
			// to avoid individual DB operations in loops
			var slackUserWorker *worker.SlackUserRefreshWorker
			if slackSvc != nil {
				slackUserWorker = worker.NewSlackUserRefreshWorker(repo, slackSvc, 10*time.Minute, slackCfg.IsOrgLevel())
				if err := slackUserWorker.Start(ctx); err != nil {
					return goerr.Wrap(err, "failed to start Slack user refresh worker")
				}
			}

			// Create GraphQL handler with dataloaders
			resolver := gqlctrl.NewResolver(repo, uc)
			srv := handler.NewDefaultServer(
				gqlctrl.NewExecutableSchema(gqlctrl.Config{Resolvers: resolver}),
			)

			// Configure error presenter with stack traces and client/server
			// classification (extensions.code is read by graphqlErrorStatusMiddleware
			// to map errors to the right HTTP status).
			srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
				gqlErr := graphql.DefaultErrorPresenter(ctx, err)
				if gqlErr.Extensions == nil {
					gqlErr.Extensions = map[string]any{}
				}
				if code := classifyError(err); code != "" {
					gqlErr.Extensions["code"] = code
				}

				// Log full stack trace for diagnostics. Client-faulted errors
				// are logged at warn (they're expected), server faults at error.
				wrappedErr := goerr.Wrap(err, "GraphQL error")
				if isClientError(err) {
					logging.Default().Warn("GraphQL request rejected", "error", wrappedErr)
				} else {
					logging.Default().Error("GraphQL error occurred", "error", wrappedErr)
				}

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
				slackInteractionHandler := httpctrl.NewSlackInteractionHandler(uc.Action, uc.Agent)

				// Add slash command handler and configure interaction handler for view submissions
				slackCommandHandler := httpctrl.NewSlackCommandHandler(uc.Slack)
				slackInteractionHandler.WithSlackCommand(uc.Slack, uc.Case)
				slackInteractionHandler.WithMentionDraft(uc.MentionDraft)
				httpOpts = append(httpOpts, httpctrl.WithSlackInteraction(slackInteractionHandler))
				httpOpts = append(httpOpts, httpctrl.WithSlackCommand(slackCommandHandler))
				logging.Default().Info("Slack interaction handler enabled")
				logging.Default().Info("Slack slash command handler enabled")
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
