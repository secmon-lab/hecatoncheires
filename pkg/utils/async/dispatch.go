package async

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// Dispatch executes a handler function asynchronously in a new goroutine
// It creates a background context and handles errors and panics
func Dispatch(ctx context.Context, handler func(ctx context.Context) error) {
	// Create a new background context but preserve logger
	bgCtx := context.Background()
	if logger := logging.From(ctx); logger != nil {
		bgCtx = logging.With(bgCtx, logger)
	}

	go func() {
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
