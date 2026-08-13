// Package threadcase hosts the thread-mode case agents: the one that
// materialises a new Case from the conversation that triggered it, and the one
// that answers a mention on an existing Case. Both are plan-and-execute agents
// running on the agentkit runtime.
//
// A turn is a durable process: StartTurn spawns it and returns, and its decision
// is applied by the completion handler through the Host port. Slack SDK imports
// are forbidden here; the usecase layer owns the Slack service and i18n.
package threadcase

import (
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// TurnRequest collects the inputs resolved by the host before handing control
// to the threadcase runtime.
type TurnRequest struct {
	Session   *model.Session
	Workspace *model.WorkspaceEntry
	Case      *model.Case

	ChannelID string
	ThreadTS  string
	// UIChannelID / UIThreadTS locate the thread the requester is watching, for the
	// one flow where that is not ChannelID / ThreadTS: a case raised by a reaction
	// lives in the monitored channel while the reactor watches the thread they
	// reacted in. Progress, questions and failure notices go there. Empty means the
	// two are the same thread.
	UIChannelID string
	UIThreadTS  string
	MentionTS   string
	// MentionText is the raw text of the mention that triggered this turn.
	MentionText string
	// MentionUserID / MentionUserName identify its author. The ID is what makes
	// a self-referential request ("assign me") actionable: case__assign takes
	// Slack user IDs and no tool resolves a display name into one. It is also the
	// run's access actor.
	MentionUserID   string
	MentionUserName string

	SystemMessages []ConversationMessage
	DeltaMessages  []ConversationMessage

	// TriggerTS is the Slack TS of the event that started this turn. It is the
	// idempotency key, which is what makes a re-delivered Slack event resolve to
	// the run it already started instead of starting a second one.
	TriggerTS string

	// InheritFrom continues a finished run's conversation in this one. It is how an
	// answered question resumes: the answering turn is a NEW run — its own budget,
	// its own record — but it must see the request, the investigation and the
	// question that produced it. Empty starts a fresh conversation.
	InheritFrom string

	// Mode selects the turn purpose (materialize on creation vs mention).
	Mode Mode

	// CreateInstruction is an optional extra instruction appended to the
	// ModeCreate planner system prompt (under a "# Trigger context" heading).
	// It lets a host inject trigger-specific guidance the generic prompt cannot
	// know — e.g. that the case was raised by a reaction on one message and the
	// surrounding conversation must be read. Empty and ignored for other modes.
	CreateInstruction string
}

// Status discriminates what StartTurn did.
type Status int

const (
	// StatusStarted means the turn was spawned; its decision is applied by the
	// run's own completion handler, so the caller has nothing to apply or post.
	StatusStarted Status = iota
	// StatusBusy means another turn holds this thread; BusyOwner names its session.
	StatusBusy
	// StatusIdempotent means the trigger duplicates a turn already started; drop
	// it silently.
	StatusIdempotent
)

// Result is the outcome of StartTurn.
type Result struct {
	Status Status
	// BusyOwner is the session whose turn holds the thread, set only on
	// StatusBusy.
	BusyOwner *model.Session
}

func validateRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	// ModeCreate runs before any case exists, so Case is intentionally nil
	// there; every other mode operates on an existing case.
	if req.Case == nil && req.Mode != ModeCreate {
		return goerr.New("Case is required")
	}
	if req.Workspace == nil {
		return goerr.New("Workspace is required")
	}
	if req.TriggerTS == "" {
		return goerr.New("TriggerTS is required")
	}
	return nil
}
