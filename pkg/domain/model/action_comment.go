package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrActionCommentValidation is returned when an ActionComment fails its persistence-boundary invariants.
var ErrActionCommentValidation = goerr.New("action comment validation failed")

// ActionCommentBodyMaxLen caps the Markdown body at 16384 bytes, matching
// AgentAdditionalPromptMaxLen. The cap exists so one comment can never
// approach the Firestore per-document limit; the Slack notification carries
// only a short excerpt, so the cap is unrelated to Slack's block-text limit.
const ActionCommentBodyMaxLen = 16384

// ActionComment is a comment written on an Action from the Web UI. It is
// deliberately NOT a slack.Message: a comment is never posted into the
// Action's Slack thread, only announced there, so it carries no channel /
// team / message-timestamp identity that would have to be fabricated.
type ActionComment struct {
	ID        string // unique within the action
	ActionID  int64  // parent action id
	AuthorID  string // Slack user id of the human author
	Body      string // Markdown source, trimmed by the caller
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsEdited reports whether the body was changed after creation. Derived from
// the timestamps so there is a single source of truth, the same way
// Action.IsArchived and ActionStep.IsDone derive their booleans.
func (c *ActionComment) IsEdited() bool {
	return c != nil && c.UpdatedAt.After(c.CreatedAt)
}

// Validate enforces the invariants required before any persistence write.
// AuthorID is required because a comment is human-authored by definition: a
// system / agent context must not be able to write one, and an empty author
// would render as an unattributable row in the activity feed.
func (c *ActionComment) Validate() error {
	if c == nil {
		return goerr.Wrap(ErrActionCommentValidation, "action comment is nil")
	}
	if c.ID == "" {
		return goerr.Wrap(ErrActionCommentValidation, "action comment ID is required")
	}
	if c.ActionID == 0 {
		return goerr.Wrap(ErrActionCommentValidation, "action comment ActionID is required")
	}
	if c.AuthorID == "" {
		return goerr.Wrap(ErrActionCommentValidation, "action comment AuthorID is required")
	}
	if c.Body == "" {
		return goerr.Wrap(ErrActionCommentValidation, "action comment body is required")
	}
	if len(c.Body) > ActionCommentBodyMaxLen {
		return goerr.Wrap(ErrActionCommentValidation, "action comment body exceeds max length",
			goerr.V("len", len(c.Body)),
			goerr.V("max", ActionCommentBodyMaxLen))
	}
	return nil
}
