package errutil

import (
	"context"
	"errors"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	obssentry "github.com/secmon-lab/hecatoncheires/pkg/utils/observability/sentry"
)

// Handle records a non-fatal or already-propagated error to both the
// structured log and (if configured) Sentry. The logger is taken from ctx
// so request-scoped fields propagate; goerr values become structured log
// attributes and Sentry contexts.
func Handle(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}

	logger := logging.From(ctx)

	var ge *goerr.Error
	if errors.As(err, &ge) {
		logger.Error(msg,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.Stacks(),
		)
	} else {
		logger.Error(msg, "error", err.Error())
	}

	obssentry.Capture(ctx, err)
}

// HandleHTTP logs the error like Handle and additionally writes an HTTP
// error response with the given status code. The logger is taken from ctx.
func HandleHTTP(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	if err == nil {
		return
	}

	logger := logging.From(ctx)

	var ge *goerr.Error
	if errors.As(err, &ge) {
		logger.Error("HTTP error",
			"status", statusCode,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.Stacks(),
		)
	} else {
		logger.Error("HTTP error",
			"status", statusCode,
			"error", err.Error(),
		)
	}

	obssentry.Capture(ctx, err)
	http.Error(w, err.Error(), statusCode)
}
