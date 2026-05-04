// Package errutil owns the project's non-fatal error reporting path.
// Every error that does not propagate further must end up here so it
// lands in both the structured log and (if configured) Sentry.
package errutil

import (
	"context"
	"errors"
	"net/http"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

type SentryConfig struct {
	DSN         string
	Environment string
	Release     string
}

func InitSentry(cfg SentryConfig) error {
	if cfg.DSN == "" {
		return nil
	}
	if err := sentrygo.Init(sentrygo.ClientOptions{
		Dsn:         cfg.DSN,
		Environment: cfg.Environment,
		Release:     cfg.Release,
		SampleRate:  1.0,
	}); err != nil {
		return goerr.Wrap(err, "failed to initialize sentry")
	}
	return nil
}

func FlushSentry(timeout time.Duration) bool {
	return sentrygo.Flush(timeout)
}

func SentryHTTPMiddleware(next http.Handler) http.Handler {
	return sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle(next)
}

// Handle records a non-fatal error to the structured log and Sentry.
// The logger is taken from ctx so request-scoped fields propagate; goerr
// values are attached as both log attributes and Sentry context.
func Handle(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}

	var ge *goerr.Error
	if errors.As(err, &ge) {
		logging.From(ctx).Error(msg,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.Stacks(),
		)
	} else {
		logging.From(ctx).Error(msg, "error", err.Error())
	}

	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		if ge != nil && len(ge.Values()) > 0 {
			scope.SetContext("goerr_values", sentrygo.Context(ge.Values()))
		}
		hub.CaptureException(err)
	})
}

// HandleHTTP is Handle plus an HTTP error response.
func HandleHTTP(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	if err == nil {
		return
	}
	Handle(ctx, err, "HTTP error")
	http.Error(w, err.Error(), statusCode)
}
