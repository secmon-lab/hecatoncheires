// Package wsagent hosts the workspace-channel agent: a plan-and-execute turn
// (planexec.Runner) that runs when the bot is mentioned in a channel-mode
// workspace's configured workspace channel ([slack] workspace_channel). Unlike
// casebound / threadcase (pinned to one Case), this host is workspace-scoped and
// operates across every Case the mentioning user can access, via the cross-case
// casemulti tool set. Slack SDK imports are forbidden here; the host progresses
// via the Handler interface and returns the reply text in Result — the usecase
// layer performs the Slack posting, exactly like casebound / threadcase.
package wsagent

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// Handler is the host-side surface for one workspace-agent turn. It carries
// only progress-trace callbacks; the final reply is returned in Result and
// posted by the usecase layer (which owns the Slack service and i18n).
type Handler interface {
	// TraceAppend records a milestone line that must stay visible (planner
	// rounds, task results) in the thread-side trace block.
	TraceAppend(ctx context.Context, line string)
	// TraceReplace overwrites the single transient activity line in place, so
	// per-tool chatter ("Searching…", "Fetching…") does not accumulate.
	TraceReplace(ctx context.Context, line string)
}

// HandlerFuncs is a struct-of-funcs adapter for tests and minimal hosts.
// Missing entries are no-ops.
type HandlerFuncs struct {
	TraceAppendFn  func(ctx context.Context, line string)
	TraceReplaceFn func(ctx context.Context, line string)
}

func (h HandlerFuncs) TraceAppend(ctx context.Context, line string) {
	if h.TraceAppendFn == nil {
		return
	}
	h.TraceAppendFn(ctx, line)
}

func (h HandlerFuncs) TraceReplace(ctx context.Context, line string) {
	if h.TraceReplaceFn == nil {
		return
	}
	h.TraceReplaceFn(ctx, line)
}

var _ Handler = HandlerFuncs{}

// TurnRequest carries everything one workspace-agent turn needs. The host
// resolves nothing itself: the usecase layer supplies the already-loaded
// session / workspace and the mentioning user's identity.
type TurnRequest struct {
	// Session is the (channel, thread) session; CaseID is 0 (workspace-scoped,
	// not bound to any single case).
	Session *model.Session
	// Workspace is the channel-mode workspace whose workspace channel was
	// mentioned.
	Workspace *model.WorkspaceEntry
	// ActorID is the mentioning Slack user id — the access actor. The host
	// injects it as the ctx auth token so every casemulti read/write enforces
	// this user's private-case membership.
	ActorID string
	// MentionText is the user's message (with the bot mention stripped by the
	// caller). Used as the planner UserInput.
	MentionText string
	// TriggerTS is the Slack ts of the triggering event, used for turn-lock
	// idempotency.
	TriggerTS string
	// Handler receives progress-trace callbacks. May be nil (treated as no-op).
	Handler Handler
}

// Status is the outcome of a workspace-agent turn.
type Status int

const (
	// StatusCompleted: the turn produced a reply (Result.ReplyText).
	StatusCompleted Status = iota
	// StatusBusy: another turn holds the per-thread lock (Result.BusyOwner set).
	StatusBusy
	// StatusIdempotent: a duplicate Slack delivery for the live turn; drop it.
	StatusIdempotent
	// StatusFallback: the planner exhausted its budget or hit an internal error
	// without producing a reply; the caller should post a fallback message.
	StatusFallback
)

// Result is what RunTurn returns to the usecase layer.
type Result struct {
	Status Status
	// ReplyText is the agent's reply, set only on StatusCompleted. The usecase
	// layer posts it to the workspace-channel thread.
	ReplyText string
	// BusyOwner is the Slack user id of the turn currently holding the lock,
	// set only on StatusBusy.
	BusyOwner string
}
