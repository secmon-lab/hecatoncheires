// Package casebound contains the agent runtime for case-bound channels —
// Slack channels that already have an associated Case. The agent runs the
// gollem ReAct loop with the full action mutator tool set, replying as a
// thread message. Slack-side concerns (posting, fetching conversation,
// trace UI) live in the host orchestrator at pkg/usecase/agent.go; this
// package is Slack-independent.
package casebound

import "context"

// ConversationMessage is the Slack-independent shape passed to RunTurn for
// the conversation snapshot. The host (pkg/usecase) converts its
// pkg/service/slack.ConversationMessage into this type so casebound never
// imports the Slack service.
type ConversationMessage struct {
	UserID    string
	UserName  string
	Text      string
	Timestamp string
}

// Handler is the host-side interface casebound calls during a turn for the
// progressive trace UI. It is intentionally minimal: the host handles all
// Slack posting, conversation fetching, and final reply itself; the only
// thing the casebound runtime needs from the host while gollem is running
// is a "post this short progress line" hook.
type Handler interface {
	// Trace appends a single line to the host's progress display (typically
	// a Slack thread message that gets updated in place).
	Trace(ctx context.Context, line string)
}

// HandlerFunc is a convenience wrapper that lets callers supply just the
// Trace function as a closure instead of defining a struct.
type HandlerFunc func(ctx context.Context, line string)

// Trace satisfies Handler.
func (f HandlerFunc) Trace(ctx context.Context, line string) {
	if f == nil {
		return
	}
	f(ctx, line)
}
