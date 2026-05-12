package usecase

import (
	"slices"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// broadcastableActionEvents enumerates the ActionEventKind values whose
// Slack thread notification should also surface in the parent channel via
// reply_broadcast. Adding or removing an entry here changes the broadcast
// behaviour for every caller (postActionChangeNotification, notifyStepEvent,
// ...) at once, so this map is the single source of truth.
var broadcastableActionEvents = map[types.ActionEventKind]struct{}{
	types.ActionEventStatusChanged:    {},
	types.ActionEventAssigneeChanged:  {},
	types.ActionEventStepAdded:        {},
	types.ActionEventStepRemoved:      {},
	types.ActionEventStepDone:         {},
	types.ActionEventStepReopened:     {},
	types.ActionEventStepTitleChanged: {},
}

// shouldBroadcastActionEvent reports whether the notification for the given
// ActionEventKind should be broadcast to the parent channel in addition to
// the thread.
func shouldBroadcastActionEvent(kind types.ActionEventKind) bool {
	_, ok := broadcastableActionEvents[kind]
	return ok
}

// shouldBroadcastAnyActionEvent reports whether any of the given kinds is in
// the broadcast set. Used when a single thread reply summarises multiple
// concurrent diffs (e.g. title + status + assignee in one UpdateAction call)
// and the call site needs to decide on broadcasting for the merged reply.
func shouldBroadcastAnyActionEvent(kinds ...types.ActionEventKind) bool {
	return slices.ContainsFunc(kinds, shouldBroadcastActionEvent)
}
