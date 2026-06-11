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
	// ErrMissingRequiredOnSubmit is returned by SubmitDraft when the draft
	// is missing one or more required custom fields. The wrapping goerr
	// carries the field IDs and human-friendly names (see MissingFieldIDsKey /
	// MissingFieldNamesKey) so the frontend can point the user at exactly
	// which inputs to fill.
	ErrMissingRequiredOnSubmit = errors.New("draft is missing required fields")
	// ErrDraftTitleRequired is returned by SubmitDraft when the draft was
	// saved with an empty title. Slack channel naming requires a non-empty
	// title, so this is enforced at promote time even though Save-as-Draft
	// accepted the empty value.
	ErrDraftTitleRequired = errors.New("draft title is required before submit")
	// ErrFieldValidationFailed wraps a field-level validation failure from
	// the workspace's field schema (option lookup, type coercion, etc.).
	// Used so the GraphQL layer can surface "fix the field" as a specific
	// code without grepping wrapped error messages.
	ErrFieldValidationFailed = errors.New("field validation failed")
	// ErrActivationFailed wraps a post-promotion activation failure (Slack
	// channel creation, channel invites, welcome message). The draft is
	// rolled back to DRAFT before this error returns, so callers / frontends
	// should advise "retry submit".
	ErrActivationFailed = errors.New("case activation failed")

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

	// ErrUnknownUser is returned by a case write when an assignee id or a
	// user / multi-user field value references a user that does not exist in
	// the SlackUser store. It guards against an agent (or API client)
	// persisting a hallucinated / mistyped user id. Slack sync delay is
	// treated as non-existence per project policy.
	ErrUnknownUser = errors.New("unknown user")

	// ErrInvalidArgument is returned by usecase methods when the caller
	// provides input that violates a domain invariant (unknown ID, list
	// element that does not belong to the workspace, etc.). Distinct from
	// ErrAccessDenied / ErrNotFound so the GraphQL layer can map it to
	// BAD_USER_INPUT rather than INTERNAL.
	ErrInvalidArgument = errors.New("invalid argument")
)

// Context keys for error values
const (
	CaseIDKey       = "case_id"
	ActionIDKey     = "action_id"
	ActionStepIDKey = "action_step_id"
	// MissingFieldIDsKey / MissingFieldNamesKey are populated on
	// ErrMissingRequiredOnSubmit so the GraphQL error mapper can expose the
	// list to the frontend (which renders them as the offending inputs).
	MissingFieldIDsKey   = "missing_field_ids"
	MissingFieldNamesKey = "missing_field_names"
	// CurrentStatusKey is populated on status-transition errors so the
	// frontend can tell the user the current state of the case (e.g. "this
	// draft has already been opened — refresh to see the change").
	CurrentStatusKey = "current_status"
)
