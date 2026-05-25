package agent

import "time"

// BusyInfo describes the live-owner state at the moment another caller
// failed to acquire the turn lock. It is passed through to host-side
// handlers (proposal.Handler.PostBusy / casebound busy notification) so the
// user-facing message can mention how long the in-flight turn has been
// running, etc.
type BusyInfo struct {
	// StartedAt is the live owner's TurnStartedAt timestamp.
	StartedAt time.Time
	// OwnerID is the live owner's TurnOwnerID, useful for debug logs.
	OwnerID string
}
