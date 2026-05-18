package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// ErrCaseNotDraft is returned when an operation requires a case to be in
// DRAFT status but the case is already submitted (OPEN/CLOSED) or otherwise
// not a draft.
var ErrCaseNotDraft = goerr.New("case is not a draft")

// ErrCaseMissingReporter is returned by Case.Validate when a case is
// about to be persisted without a reporter. Every persisted case —
// DRAFT, OPEN, or CLOSED — must name a reporter; cases originate from
// some authenticated user (Web cookie, Slack interactivity callback,
// no-auth dev-mode token) and losing that identity later means the
// Cases / Drafts UI shows an empty Reporter column and Slack invites
// have nobody to add as the channel creator. Repositories MUST call
// Validate() before write so this failure mode never reaches storage.
var ErrCaseMissingReporter = goerr.New("case has no reporter")

// Case represents a generic case/project entity.
//
// Lifecycle (see types.CaseStatus): DRAFT → OPEN → CLOSED. A case in DRAFT is
// an "in-progress" entry saved from the Slack creation modal's Save as Draft
// button; it is visible only to its reporter and triggers none of the
// channel-binding / notification side effects of a real Case until SubmitDraft
// promotes it to OPEN.
type Case struct {
	ID             int64
	Title          string
	Description    string
	Status         types.CaseStatus
	ReporterID     string   // Slack User ID of the case reporter (immutable after creation)
	AssigneeIDs    []string // Slack User IDs
	SlackChannelID string
	IsPrivate      bool                  // Private mode flag
	ChannelUserIDs []string              // Slack User IDs of channel members (synced for all cases)
	AccessDenied   bool                  // Runtime-only: set by UseCase when access is restricted (not persisted)
	FieldValues    map[string]FieldValue // key = FieldID
	RequestKey     string                // UUID for preventing duplicate case creation from Slack modals
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsDraft reports whether this Case is currently in the unsubmitted draft state.
func (c *Case) IsDraft() bool {
	return c != nil && c.Status.IsDraft()
}

// Validate checks the basic invariants every persisted Case must satisfy.
// Repositories MUST call this before every write. For new cases (Create),
// call ValidateNew instead, which additionally enforces ReporterID.
func (c *Case) Validate() error {
	if c == nil {
		return goerr.New("case is nil")
	}
	return nil
}

// ValidateNew checks the invariants that must hold for a newly created Case,
// including ReporterID which is required at creation time. Repositories MUST
// call this instead of Validate for Create operations so an empty ReporterID
// (the canonical failure mode where a Slack handler forgot to inject
// auth.ContextWithToken) is caught at the persistence boundary.
func (c *Case) ValidateNew() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.ReporterID == "" {
		return goerr.Wrap(ErrCaseMissingReporter,
			"case is missing ReporterID",
			goerr.V("title", c.Title),
			goerr.V("status", c.Status),
		)
	}
	return nil
}

// SubmitDraft transitions the case from DRAFT to OPEN in place. Returns an
// error if the case is not in DRAFT (callers must not silently no-op when
// promoting an already-submitted case). Persistence and any post-promotion
// side effects are the caller's responsibility.
func (c *Case) SubmitDraft() error {
	if c == nil {
		return ErrCaseNotDraft
	}
	if !c.Status.IsDraft() {
		return ErrCaseNotDraft
	}
	c.Status = types.CaseStatusOpen
	return nil
}

// IsCaseAccessible checks if a user has access to a case.
// Non-private cases are always accessible.
// Private cases are accessible only if the userID is in ChannelUserIDs.
func IsCaseAccessible(c *Case, userID string) bool {
	if !c.IsPrivate {
		return true
	}
	for _, id := range c.ChannelUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// RestrictCase returns a copy of the case with sensitive fields removed.
// Only ID, Status, IsPrivate, CreatedAt, UpdatedAt are preserved.
// AccessDenied is set to true.
func RestrictCase(c *Case) *Case {
	return &Case{
		ID:           c.ID,
		Status:       c.Status,
		IsPrivate:    c.IsPrivate,
		AccessDenied: true,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}
