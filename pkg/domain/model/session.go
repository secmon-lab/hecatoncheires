package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrSessionValidation is returned when a Session fails its persistence-boundary invariants.
var ErrSessionValidation = goerr.New("session validation failed")

// SessionEndReason captures the terminal plan action that ended the most
// recent turn for a Session. The dispatcher uses it to decide whether a
// thread reply (without an @mention) should resume the agent.
type SessionEndReason string

const (
	SessionEndedNone               SessionEndReason = ""
	SessionEndedWithMessage        SessionEndReason = "post_message"
	SessionEndedWithQuestion       SessionEndReason = "post_question"
	SessionEndedWithMaterialize    SessionEndReason = "materialize"
	SessionEndedWithCaseBoundReply SessionEndReason = "case_bound"
)

// SessionTurnState captures whether a turn is currently running on a Session.
// It is the CAS key used by the Firestore-backed turn lock and is updated
// by AcquireTurnLock / Heartbeat / ReleaseTurnLock atomically.
type SessionTurnState string

const (
	SessionTurnIdle        SessionTurnState = ""
	SessionTurnRunning     SessionTurnState = "running"
	SessionTurnInterrupted SessionTurnState = "interrupted"
)

// Session represents an ongoing agent conversation bound to a Slack thread.
// It unifies what was previously split between AgentSession (case-bound) and
// per-mention draft state (open mode). One Session per (channelID, threadTS).
//
// Lookup keys are (ChannelID, ThreadTS). Case binding is detected via
// CaseID != 0; open mode (draft creation) is the zero-CaseID case.
type Session struct {
	ID            string
	ChannelID     string
	ThreadTS      string
	LastMentionTS string
	LastAction    SessionEndReason

	// Case binding — zero values when the thread is not in a case-bound channel.
	WorkspaceID string
	CaseID      int64
	ActionID    int64

	// Open-mode metadata — zero values when case-bound.
	CreatorUserID string
	ProposalID    CaseProposalID

	// Reaction-origin metadata — set only for a cross-channel reaction-triggered
	// case creation, where the case root lives in the monitored channel but the
	// flagged message lives elsewhere. Persisted so a later resume turn (after a
	// question) can still link the exact source message. Zero for every other
	// creation path.
	ReactionSourceChannelID string
	ReactionSourceMessageTS string

	// Turn lock fields. Maintained by SessionRepository.AcquireTurnLock /
	// Heartbeat / ReleaseTurnLock. Heartbeat staleness (TurnHeartbeatAt vs now)
	// is the activity signal; TurnStartedAt is recorded for traces / UX only.
	TurnState       SessionTurnState
	TurnOwnerID     string
	TurnStartedAt   time.Time
	TurnHeartbeatAt time.Time
	TurnTriggerTS   string

	// Reserved for future interrupt support. Never read or written by Phase A
	// code paths; populated by RequestInterrupt in a later spec.
	InterruptRequestedAt time.Time
	InterruptByTriggerTS string

	// PendingQuestion mirrors the planner's most recent question payload when
	// LastAction == SessionEndedWithQuestion. It is the single source of
	// truth for the Slack-side question form: rendering it, parsing the
	// submission state back into typed answers, and rebuilding the read-only
	// "answered" view after the user clicks Submit. Cleared on the next
	// terminal action.
	PendingQuestion *PendingQuestion

	CreatedAt time.Time
	UpdatedAt time.Time
}

// PendingQuestion is the persisted snapshot of a question turn while we wait
// for the user's submission. It is set when the planner emits a `question`
// terminal action and consumed when the Submit button fires.
type PendingQuestion struct {
	// PostedChannelID / PostedMessageTS locate the Slack message hosting the
	// question form so the submit handler can update it in place into the
	// read-only "answered" view.
	PostedChannelID string
	PostedMessageTS string
	// Reason is the planner's single-rationale text shared across all items.
	Reason string
	// Items mirrors proposal.QuestionPayload.Items at the time the question was
	// posted. Stored here so the submit handler can label each answer back
	// against the original question text and option list, even after the
	// planner advances and the Slack message blocks have been rebuilt.
	Items []PendingQuestionItem
}

// PendingQuestionItem is a single question's persisted snapshot.
type PendingQuestionItem struct {
	ID      string
	Text    string
	Type    string // "select" | "multi_select"
	Options []string
}

// IsCaseBound reports whether this Session belongs to a case-bound thread.
func (s *Session) IsCaseBound() bool {
	return s != nil && s.CaseID != 0
}

// ResumeOnReply reports whether a thread reply (without @mention) should
// kick off a new turn. Currently only true when the previous turn ended on
// a post_question (open mode); case-bound resume is @mention-only.
func (s *Session) ResumeOnReply() bool {
	return s != nil && s.LastAction == SessionEndedWithQuestion
}

// Validate enforces the invariants required before any persistence write.
// A Session is located by (ChannelID, ThreadTS) and carries a caller-supplied
// ID, so a record missing any of these three fails loudly here instead of
// landing in storage under an incomplete key.
func (s *Session) Validate() error {
	if s == nil {
		return goerr.Wrap(ErrSessionValidation, "session is nil")
	}
	if s.ID == "" {
		return goerr.Wrap(ErrSessionValidation, "session ID is required")
	}
	if s.ChannelID == "" {
		return goerr.Wrap(ErrSessionValidation, "session ChannelID is required")
	}
	if s.ThreadTS == "" {
		return goerr.Wrap(ErrSessionValidation, "session ThreadTS is required")
	}
	return nil
}
