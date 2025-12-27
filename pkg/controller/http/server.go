package http

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/secmon-lab/hecatoncheires/frontend"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

type Server struct {
	router         *chi.Mux
	enableGraphiQL bool
}

type Options func(*Server)

func WithGraphiQL(enabled bool) Options {
	return func(s *Server) {
		s.enableGraphiQL = enabled
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
		r.Post("/", gqlHandler.ServeHTTP)
		r.Get("/", gqlHandler.ServeHTTP) // Support GET for introspection
	})

	// GraphiQL playground
	if s.enableGraphiQL {
		r.Get("/graphiql", playground.Handler("GraphQL playground", "/graphql").ServeHTTP)
	}

	// Static file serving for SPA (catch-all, must be last)
	staticFS, err := fs.Sub(frontend.StaticFiles, "dist")
	if err != nil {
		logging.Default().Error("failed to create sub FS for frontend static files", "error", err)
	} else {
		// Check if index.html exists
		if _, err := staticFS.Open("index.html"); err == nil {
			// Serve static files and handle SPA routing
			r.Get("/*", spaHandler(staticFS))
		} else {
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
