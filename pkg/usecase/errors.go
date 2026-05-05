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

	// Action Slack-post state errors
	ErrSlackMessageAlreadyPosted = errors.New("action already has a Slack message")
	ErrCaseHasNoSlackChannel     = errors.New("parent case has no Slack channel")

	// Action archive state errors
	ErrActionAlreadyArchived = errors.New("action is already archived")
	ErrActionNotArchived     = errors.New("action is not archived")

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
