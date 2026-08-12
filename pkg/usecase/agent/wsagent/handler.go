// Package wsagent hosts the workspace-channel agent: the plan-and-execute agent
// that runs when the bot is mentioned in a channel-mode workspace's configured
// workspace channel ([slack] workspace_channel). Unlike casebound / threadcase
// (pinned to one Case), this host is workspace-scoped and operates across every
// Case the mentioning user can access, via the cross-case casemulti tool set.
//
// A turn is a durable process: StartTurn spawns it and returns, and the answer is
// posted from the completion handler through the Host port. Slack SDK imports are
// forbidden here; the usecase layer owns the Slack service and i18n.
package wsagent

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// TurnRequest carries everything one workspace-agent turn needs. The host
// resolves nothing itself: the usecase layer supplies the already-loaded
// session / workspace and the mentioning user's identity.
type TurnRequest struct {
	// Session is the (channel, thread) session; CaseID is 0 (workspace-scoped,
	// not bound to any single case). It must already exist: its ID is the turn's
	// subject, so the row has to be claimed before a turn can be spawned.
	Session *model.Session
	// Workspace is the channel-mode workspace whose workspace channel was
	// mentioned.
	Workspace *model.WorkspaceEntry
	// ActorID is the mentioning Slack user id — the access actor. It is recorded
	// on the run so every casemulti read/write enforces this user's private-case
	// membership.
	ActorID string
	// MentionText is the user's message (with the bot mention stripped by the
	// caller). It is the planner's first user message.
	MentionText string
	// TriggerTS is the Slack ts of the triggering event. It is the idempotency
	// key, which is what makes a re-delivered Slack event resolve to the run it
	// already started instead of starting a second one.
	TriggerTS string
}

// Status is the outcome of starting a workspace-agent turn.
type Status int

const (
	// StatusStarted: the turn was spawned; its answer is posted by the run's own
	// completion handler, so the caller has nothing to post.
	StatusStarted Status = iota
	// StatusBusy: another turn holds this thread (Result.BusyOwner names it).
	StatusBusy
	// StatusIdempotent: a duplicate Slack delivery for a turn already started;
	// drop it silently.
	StatusIdempotent
)

// Result is what StartTurn returns to the usecase layer.
type Result struct {
	Status Status
	// BusyOwner describes the run holding the thread, set only on StatusBusy.
	BusyOwner string
}
