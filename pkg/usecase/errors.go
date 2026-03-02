package usecase

import "errors"

// Sentinel errors for use case layer
var (
	// Not found errors
	ErrCaseNotFound   = errors.New("case not found")
	ErrActionNotFound = errors.New("action not found")

	// Status errors
	ErrCaseAlreadyClosed = errors.New("case is already closed")
	ErrCaseAlreadyOpen   = errors.New("case is already open")

	// Access control errors
	ErrAccessDenied = errors.New("access denied to private case")

	// Other errors
	ErrDuplicateField = errors.New("duplicate field")
)

// Context keys for error values
const (
	CaseIDKey   = "case_id"
	ActionIDKey = "action_id"
)
