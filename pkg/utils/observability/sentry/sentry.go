// Package sentry is a thin wrapper around the sentry-go SDK that the rest of
// the codebase uses to report errors. It deliberately exposes only the entry
// points that errutil and the HTTP server need (Init, Flush, Capture,
// HTTPMiddleware) so the SDK is not imported scattershot across packages.
//
// When Init is called with an empty DSN, Sentry stays disabled and every
// other entry point becomes a cheap no-op. Capture short-circuits on the
// Enabled flag, so Sentry-disabled deployments only pay the cost of one
// atomic.Bool load per error.
package sentry

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/m-mizutani/goerr/v2"
)

// Config holds the runtime knobs for Sentry initialization. DSN being empty
// is the supported "Sentry disabled" mode.
type Config struct {
	DSN         string
	Environment string
	Release     string
}

var enabled atomic.Bool

// Init configures the global Sentry hub. When cfg.DSN is empty, it is a
// no-op and Sentry remains disabled. The returned error reports SDK-level
// initialization failures (invalid DSN format, etc.); callers may choose to
// log and continue rather than abort startup.
//
// All errors are reported (SampleRate is fixed at 1.0); we keep the SDK
// surface minimal and let Sentry's project-level rate-limiting handle
// volume control if it ever becomes necessary.
func Init(cfg Config) error {
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

	enabled.Store(true)
	return nil
}

// Enabled reports whether Init succeeded with a non-empty DSN.
func Enabled() bool { return enabled.Load() }

// Flush waits up to timeout for queued events to be delivered. It returns
// true if the buffer was drained in time. When Sentry is disabled, Flush is
// a no-op that returns true immediately.
func Flush(timeout time.Duration) bool {
	if !Enabled() {
		return true
	}
	return sentrygo.Flush(timeout)
}

// Capture sends err to Sentry, attaching every goerr value as a Sentry
// "context" entry so structured fields appear alongside the exception. When
// Sentry is disabled or err is nil, Capture is a no-op.
//
// Capture uses the request-scoped Hub when one is attached to ctx (e.g. via
// HTTPMiddleware); otherwise it falls back to the global Hub.
func Capture(ctx context.Context, err error) {
	if !Enabled() || err == nil {
		return
	}

	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}

	hub.WithScope(func(scope *sentrygo.Scope) {
		var ge *goerr.Error
		if errors.As(err, &ge) {
			values := ge.Values()
			if len(values) > 0 {
				scope.SetContext("goerr_values", sentrygo.Context(values))
			}
		}
		hub.CaptureException(err)
	})
}

// HTTPMiddleware wraps next with the Sentry HTTP middleware so each request
// gets its own Hub on the context. Capture calls inside a request will then
// carry the request URL, method, headers, etc. When Sentry is disabled this
// returns next unchanged.
func HTTPMiddleware(next http.Handler) http.Handler {
	if !Enabled() {
		return next
	}
	return sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle(next)
}
