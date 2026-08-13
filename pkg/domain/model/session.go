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

// SessionKind discriminates what kind of agent conversation owns a thread.
// The zero value is SessionKindCase so every Session persisted before this
// field existed keeps its original meaning without a migration.
type SessionKind string

const (
	// SessionKindCase is a thread owned by a Case: either a Case-bound thread
	// (CaseID != 0) or a thread whose Case is still being formed by the
	// creation agent (CaseID == 0).
	SessionKindCase SessionKind = ""
	// SessionKindWorkspaceAgent is a thread owned by the workspace agent. An
	// @mention in such a thread never starts a Case.
	SessionKindWorkspaceAgent SessionKind = "workspace_agent"
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

	// Kind discriminates the conversation that owns this thread. It is set when
	// the Session is created and never changed afterwards: the Slack dispatcher
	// reads it to decide whether an @mention in a case-less thread starts a Case
	// (SessionKindCase) or continues the workspace agent
	// (SessionKindWorkspaceAgent).
	Kind SessionKind

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
	// AskedByProcessID is the agent run that asked. The turn the answer starts
	// inherits that run's conversation, so it sees the original request, the
	// investigation behind the question, and the question itself — not just the
	// answer text. Without it the answering turn begins from an empty history and
	// has to rediscover everything the asking turn already established.
	//
	// Empty for a question recorded before this was tracked; such an answer simply
	// starts a fresh conversation, which is the pre-existing behaviour.
	AskedByProcessID string
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
