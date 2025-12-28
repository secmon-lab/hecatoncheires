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
func Handle(ctx context.Context, err error, msg string) error {
	if err == nil {
		return nil
	}

	logger := logging.Default()

	// Extract goerr values for structured logging
	var ge *goerr.Error
	if errors.As(err, &ge) {
		// Log with all context from goerr
		logger.Error(msg,
			"error", err.Error(),
			"values", ge.Values(),
			"stack", ge.Stacks(),
		)
	} else {
		// Log standard error
		logger.Error(msg, "error", err.Error())
	}

	// Return the error as-is for GraphQL to handle
	// GraphQL will return it to the client
	return err
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
