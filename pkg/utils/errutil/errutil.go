// Package errutil owns the project's non-fatal error reporting path.
// Every error that does not propagate further must end up here so it
// lands in both the structured log and (if configured) Sentry.
package errutil

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// TagBenign marks an error as a normal-flow occurrence: something that the
// code MUST handle (return 401, surface a validation message, etc.) but that
// is NOT an anomaly worth alerting on. Errors carrying this tag are logged
// at Info level and are not reported to Sentry by Handle / HandleHTTP.
//
// Apply with goerr.T(errutil.TagBenign) at the call site that knows the
// error is benign. The tag is preserved across goerr.Wrap chains, so it is
// fine to tag deep in a usecase even if the outer layer wraps further.
var TagBenign = goerr.NewTag("benign")

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
//
// Errors tagged with TagBenign are demoted: they are logged at Info level
// and skipped by Sentry. This is the escape hatch for normal-flow
// conditions (e.g. missing auth cookie, expired token) that still need to
// flow through the error return value but should not page anyone.
func Handle(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}

	var ge *goerr.Error
	hasGoerr := errors.As(err, &ge)
	benign := goerr.HasTag(err, TagBenign)

	logger := logging.From(ctx)
	level := slog.LevelError
	if benign {
		level = slog.LevelInfo
	}

	if hasGoerr {
		logger.Log(ctx, level, msg,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.Stacks(),
		)
	} else {
		logger.Log(ctx, level, msg, "error", err.Error())
	}

	if benign {
		return
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
