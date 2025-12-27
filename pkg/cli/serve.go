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
	"github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	httpctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/http"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

func cmdServe() *cli.Command {
	var addr string
	var enableGraphiQL bool

	return &cli.Command{
		Name:    "serve",
		Aliases: []string{"s"},
		Usage:   "Start HTTP server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "addr",
				Usage:       "HTTP server address",
				Value:       ":8080",
				Sources:     cli.EnvVars("HECATONCHEIRES_ADDR"),
				Destination: &addr,
			},
			&cli.BoolFlag{
				Name:        "graphiql",
				Usage:       "Enable GraphiQL playground",
				Value:       true,
				Sources:     cli.EnvVars("HECATONCHEIRES_GRAPHIQL"),
				Destination: &enableGraphiQL,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			// Initialize repository (using memory for now)
			repo := memory.New()

			// Initialize use cases
			uc := usecase.New(repo)

			// Create GraphQL handler
			resolver := graphql.NewResolver(repo, uc)
			gqlHandler := handler.NewDefaultServer(
				graphql.NewExecutableSchema(graphql.Config{Resolvers: resolver}),
			)

			// Create HTTP server
			httpHandler := httpctrl.New(gqlHandler, httpctrl.WithGraphiQL(enableGraphiQL))
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
