package errutil

import (
	"context"
	"errors"
	"net/http"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// Handle logs the error with a message and returns an appropriate user-facing error.
// This function ensures that all errors, especially 5xx errors, are properly logged.
func Handle(ctx context.Context, err error, msg string) {
	if err == nil {
		return
	}

	logger := logging.Default()

	// Extract goerr values for structured logging
	if ge := goerr.Unwrap(err); ge != nil {
		// Log with all context from goerr
		logger.Error(msg,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.StackTrace(),
		)
	} else {
		// Log standard error
		logger.Error(msg, "error", err.Error())
	}
}

// HandleHTTP logs the error and writes an appropriate HTTP error response.
func HandleHTTP(ctx context.Context, w http.ResponseWriter, err error, statusCode int) {
	if err == nil {
		return
	}

	logger := logging.Default()

	// Always log errors, especially 5xx errors
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

	// Write HTTP error response
	http.Error(w, err.Error(), statusCode)
}
