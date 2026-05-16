package usecase

import "errors"

// Sentinel errors for use case layer
var (
	// Not found errors
	ErrCaseNotFound       = errors.New("case not found")
	ErrActionNotFound     = errors.New("action not found")
	ErrActionStepNotFound = errors.New("action step not found")

	// Status errors
	ErrCaseAlreadyClosed = errors.New("case is already closed")
	ErrCaseAlreadyOpen   = errors.New("case is already open")
	// ErrCaseIsDraft is returned when a status-transition operation (close /
	// reopen) is invoked on a case that is still in DRAFT. Drafts only leave
	// DRAFT via SubmitDraft (→ OPEN) or DiscardDraft (delete).
	ErrCaseIsDraft = errors.New("case is in draft state")
	// ErrCaseNotDraft is returned by draft-specific operations (Submit /
	// Discard) when the targeted case is not in DRAFT.
	ErrCaseNotDraft = errors.New("case is not a draft")

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
	CaseIDKey       = "case_id"
	ActionIDKey     = "action_id"
	ActionStepIDKey = "action_step_id"
)
