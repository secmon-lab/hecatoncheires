package http

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/frontend"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

type Server struct {
	router         *chi.Mux
	enableGraphiQL bool
	authUC         AuthUseCase
}

type Options func(*Server)

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

func New(gqlHandler http.Handler, opts ...Options) *Server {
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
		// Apply GraphQL-specific auth middleware
		if s.authUC != nil {
			r.Use(graphqlAuthMiddleware(s.authUC))
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
			r.Get("/user-info", authUserInfoHandler(s.authUC))
		})
	}

	// Static file serving for SPA (catch-all, must be last)
	staticFS, err := fs.Sub(frontend.StaticFiles, "dist")
	if err != nil {
		// Log error and continue - the server can still serve GraphQL
		logging.Default().Error("failed to create sub FS for frontend static files",
			"error", goerr.Wrap(err, "failed to create sub FS for frontend static files").Error(),
		)
	} else {
		// Check if index.html exists
		if _, err := staticFS.Open("index.html"); err == nil {
			// Serve static files and handle SPA routing with auth
			r.Group(func(r chi.Router) {
				// Apply auth middleware to frontend routes
				if s.authUC != nil {
					r.Use(authMiddleware(s.authUC))
				}
				r.Get("/*", spaHandler(staticFS))
			})
		} else {
			// This is a warning, not a critical error
			logging.Default().Warn("index.html not found in frontend dist", "error", err)
		}
	}

	return s
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
