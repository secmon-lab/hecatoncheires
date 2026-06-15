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
// Slack posting, conversation fetching, and final reply itself. The runtime
// distinguishes two kinds of progress so the host can render them differently:
// milestones that accumulate vs. ephemeral per-tool activity that overwrites.
type Handler interface {
	// TraceAppend records a milestone line that must stay visible (it is
	// appended to the host's progress history).
	TraceAppend(ctx context.Context, line string)
	// TraceReplace overwrites the single transient activity line in place,
	// so per-tool chatter ("Searching…", "Fetching…") does not accumulate.
	TraceReplace(ctx context.Context, line string)
}

// HandlerFunc is a convenience wrapper for callers that only care about the
// milestone history. The supplied closure receives every milestone via
// TraceAppend; TraceReplace is a no-op (transient activity is discarded).
type HandlerFunc func(ctx context.Context, line string)

// TraceAppend satisfies Handler.
func (f HandlerFunc) TraceAppend(ctx context.Context, line string) {
	if f == nil {
		return
	}
	f(ctx, line)
}

// TraceReplace satisfies Handler. The single-func wrapper has no place to put
// a transient line, so it is dropped.
func (f HandlerFunc) TraceReplace(context.Context, string) {}

// HandlerFuncs is a struct-of-funcs adapter for hosts that render milestones
// and transient activity differently. Nil entries are treated as no-ops.
type HandlerFuncs struct {
	TraceAppendFn  func(ctx context.Context, line string)
	TraceReplaceFn func(ctx context.Context, line string)
}

// TraceAppend satisfies Handler.
func (h HandlerFuncs) TraceAppend(ctx context.Context, line string) {
	if h.TraceAppendFn == nil {
		return
	}
	h.TraceAppendFn(ctx, line)
}

// TraceReplace satisfies Handler.
func (h HandlerFuncs) TraceReplace(ctx context.Context, line string) {
	if h.TraceReplaceFn == nil {
		return
	}
	h.TraceReplaceFn(ctx, line)
}

// Compile-time assertions.
var (
	_ Handler = HandlerFunc(nil)
	_ Handler = HandlerFuncs{}
)
