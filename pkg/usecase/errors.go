package usecase

import "github.com/m-mizutani/goerr/v2"

// Sentinel errors for use case layer
var (
	// Not found errors
	ErrCaseNotFound   = goerr.New("case not found")
	ErrActionNotFound = goerr.New("action not found")

	// Other errors
	ErrDuplicateField = goerr.New("duplicate field")
)

// Context keys for error values
const (
	CaseIDKey   = "case_id"
	ActionIDKey = "action_id"
)
