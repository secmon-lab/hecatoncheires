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
	// ErrCaseThreadModeUseStatus is returned by CloseCase / ReopenCase when the
	// targeted case is thread-mode (bound to a Slack thread). Thread-mode cases
	// change lifecycle by moving the configurable board status via
	// UpdateCaseStatus, which keeps BoardStatus and the lifecycle Status in sync;
	// closing / reopening one directly would set Status while leaving BoardStatus
	// on a mismatched column, desyncing the two. The boundary rejects it so a
	// mis-wired caller fails loudly instead of producing the inconsistent state.
	ErrCaseThreadModeUseStatus = errors.New("thread-mode case lifecycle must change via board status")
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

	// ErrCasePrivateThreadModeUnsupported is returned by the case create flows
	// when isPrivate=true is requested for a thread-mode workspace. A private
	// case's only effect is a dedicated private Slack channel, which channel
	// mode creates and thread mode has no equivalent for (thread-mode cases
	// reuse the monitored channel and never carry IsPrivate). The frontend
	// already hides the private toggle in thread mode; the invariant is enforced
	// here at the usecase boundary so every entry point (GraphQL, Slack modal,
	// agent tool, import) is covered, not just the web form.
	ErrCasePrivateThreadModeUnsupported = errors.New("private case is not supported in thread mode")

	// ErrCaseThreadModeNoActions is returned by ActionUseCase write paths when
	// the parent (or reparent target) Case is thread-mode. Thread-mode cases
	// track progress through the configurable board status (Kanban) and have no
	// Actions — the configurable status attaches to the Case itself there, while
	// in channel mode it attaches to Actions (see model.Case.BoardStatus). The
	// invariant is enforced at the usecase boundary so every entry point
	// (GraphQL, Slack, agent tools, eval) is covered, not just the agent tool
	// wiring that withholds the action tools for thread-mode workspaces.
	ErrCaseThreadModeNoActions = errors.New("thread-mode case cannot have actions")

	// ErrCaseThreadModeSlackRequired is returned by CreateCase when a thread-mode
	// workspace cannot bind a new Case to a Slack thread because the Slack service
	// is not wired or the workspace has no monitored channel configured. It is a
	// deployment / configuration fault (not user input), so the GraphQL layer
	// leaves it unclassified and maps it to a 500.
	ErrCaseThreadModeSlackRequired = errors.New("thread-mode case creation requires Slack")

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
