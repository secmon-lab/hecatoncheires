package model

import "time"

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
	DraftID       CaseDraftID

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

	CreatedAt time.Time
	UpdatedAt time.Time
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
