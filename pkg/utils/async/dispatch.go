package async

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

var inflight sync.WaitGroup

// Dispatch executes a handler function asynchronously in a new goroutine.
// It creates a background context (preserving the logger) and handles errors
// and panics. Pending dispatches are tracked by a package-level WaitGroup so
// tests can synchronise on completion via Wait().
func Dispatch(ctx context.Context, handler func(ctx context.Context) error) {
	// Create a new background context but preserve logger
	bgCtx := context.Background()
	if logger := logging.From(ctx); logger != nil {
		bgCtx = logging.With(bgCtx, logger)
	}

	inflight.Add(1)
	go func() {
		defer inflight.Done()
		defer func() {
			if r := recover(); r != nil {
				logger := logging.From(bgCtx)
				logger.Error("panic in async handler", "panic", r)
			}
		}()

		if err := handler(bgCtx); err != nil {
			logger := logging.From(bgCtx)
			logger.Error("async handler failed", "error", goerr.Unwrap(err))
		}
	}()
}

// Wait blocks until all in-flight dispatches launched via Dispatch have
// returned. Intended for use in tests that need to assert on side effects of
// the async tail; production code must not call this.
func Wait() {
	inflight.Wait()
}
