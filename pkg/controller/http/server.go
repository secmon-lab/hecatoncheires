package http

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/frontend"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

type Server struct {
	router              *chi.Mux
	enableGraphiQL      bool
	authUC              AuthUseCase
	slackService        slack.Service
	slackWebhookHandler *SlackWebhookHandler
	slackSigningSecret  string
	workspaceRegistry   *model.WorkspaceRegistry
}

type Options func(*Server)

func WithWorkspaceRegistry(registry *model.WorkspaceRegistry) Options {
	return func(s *Server) {
		s.workspaceRegistry = registry
	}
}

func WithGraphiQL(enabled bool) Options {
	return func(s *Server) {
		s.enableGraphiQL = enabled
	}
}

func WithAuth(authUC AuthUseCase) Options {
	return func(s *Server) {
		s.authUC = authUC
	}
}

func WithSlackWebhook(handler *SlackWebhookHandler, signingSecret string) Options {
	return func(s *Server) {
		s.slackWebhookHandler = handler
		s.slackSigningSecret = signingSecret
	}
}

func WithSlackService(svc slack.Service) Options {
	return func(s *Server) {
		s.slackService = svc
	}
}

func New(gqlHandler http.Handler, opts ...Options) (*Server, error) {
	r := chi.NewRouter()

	s := &Server{
		router:         r,
		enableGraphiQL: false,
	}
	for _, opt := range opts {
		opt(s)
	}

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(accessLogger)
	r.Use(middleware.Recoverer)

	// GraphQL endpoint (must be registered before catch-all route)
	r.Route("/graphql", func(r chi.Router) {
		// Apply auth middleware
		if s.authUC != nil {
			r.Use(authMiddleware(s.authUC))
		}
		r.Post("/", gqlHandler.ServeHTTP)
		r.Get("/", gqlHandler.ServeHTTP) // Support GET for introspection
	})

	// GraphiQL playground
	if s.enableGraphiQL {
		r.Get("/graphiql", playground.Handler("GraphQL playground", "/graphql").ServeHTTP)
	}

	// Auth endpoints (if auth is configured)
	if s.authUC != nil {
		r.Route("/api/auth", func(r chi.Router) {
			r.Get("/login", authLoginHandler(s.authUC))
			r.Get("/callback", authCallbackHandler(s.authUC))
			r.Post("/logout", authLogoutHandler(s.authUC))
			r.Get("/me", authMeHandler(s.authUC))
			r.Get("/user-info", authUserInfoHandler(s.slackService))
		})
	}

	// Workspace list endpoint
	if s.workspaceRegistry != nil {
		r.Get("/api/workspaces", workspacesHandler(s.workspaceRegistry))
	}

	// Slack webhook endpoint (if configured) - No auth required, uses signature verification
	if s.slackWebhookHandler != nil {
		r.Route("/hooks/slack", func(r chi.Router) {
			// Apply Slack signature verification middleware to all /hooks/slack/* routes
			r.Use(SlackSignatureMiddleware(s.slackSigningSecret))

			// Event webhook endpoint
			r.Post("/event", s.slackWebhookHandler.ServeHTTP)
		})
	}

	// Static file serving for SPA (catch-all, must be last)
	staticFS, err := fs.Sub(frontend.StaticFiles, "dist")
	if err != nil {
		return nil, goerr.Wrap(err, "failed to bind dist dir for static")
	}

	r.Get("/*", spaHandler(staticFS))

	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// accessLogger is a middleware that logs HTTP requests
func accessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			logging.Default().Info("access",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"remote", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		}()

		next.ServeHTTP(ww, r)
	})
}

// workspacesHandler returns a handler that serves the workspace list as JSON
func workspacesHandler(registry *model.WorkspaceRegistry) http.HandlerFunc {
	type workspaceResponse struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type response struct {
		Workspaces []workspaceResponse `json:"workspaces"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		workspaces := registry.Workspaces()
		resp := response{
			Workspaces: make([]workspaceResponse, len(workspaces)),
		}
		for i, ws := range workspaces {
			resp.Workspaces[i] = workspaceResponse{
				ID:   ws.ID,
				Name: ws.Name,
			}
		}

		data, err := json.Marshal(resp)
		if err != nil {
			errutil.HandleHTTP(r.Context(), w, goerr.Wrap(err, "failed to marshal workspaces response"), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck // header already committed
	}
}

// spaHandler handles SPA routing by serving static files and falling back to index.html
func spaHandler(staticFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(staticFS))

	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/")

		// If the path is empty, serve index.html
		if urlPath == "" {
			urlPath = "index.html"
		}

		// Try to open the file to check if it exists
		if file, err := staticFS.Open(urlPath); err != nil {
			// File not found, serve index.html for SPA routing
			if indexFile, err := staticFS.Open("index.html"); err == nil {
				defer safe.Close(r.Context(), indexFile)
				w.Header().Set("Content-Type", "text/html")
				safe.Copy(r.Context(), w, indexFile)
				return
			}

			// If index.html is also not found, return 404
			http.NotFound(w, r)
			return
		} else {
			// File exists, close it and let fileServer handle it
			safe.Close(r.Context(), file)
		}

		// Serve the requested file using the file server
		fileServer.ServeHTTP(w, r)
	}
}
