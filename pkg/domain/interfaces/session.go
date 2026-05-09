package interfaces

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ErrTurnOwnerMismatch is returned by Heartbeat / ReleaseTurnLock when the
// caller's ownerID does not match the current turn owner. The caller should
// treat it as a signal that its session was reclaimed (heartbeat staleness)
// or interrupted, and stop the in-flight turn.
var ErrTurnOwnerMismatch = goerr.New("turn lock owner mismatch")

// AcquireResult is the outcome of SessionRepository.AcquireTurnLock.
//
// Exactly one of Acquired / IdempotentRetry should be true (or neither, when
// the lock is held by a different live owner — busy). Reclaimed implies
// Acquired (the lock was taken from a stale prior owner).
type AcquireResult struct {
	// Acquired is true when the caller now holds the turn lock.
	Acquired bool
	// Reclaimed is true when Acquired came from displacing a stale owner
	// (TurnHeartbeatAt older than staleAfter). For trace logging only.
	Reclaimed bool
	// IdempotentRetry is true when the caller's triggerTS is non-empty and
	// matches the existing turn's TurnTriggerTS — typically a duplicate
	// Slack event. Acquired is false in this case; the caller should drop
	// the trigger. Synthetic triggers (ws-switch etc.) pass triggerTS="" so
	// they never match and always proceed (or get Busy).
	IdempotentRetry bool
	// Session is the Session as observed after the operation. When Acquired
	// is true, this reflects the running turn (with TurnOwnerID set to the
	// caller). When Acquired is false (busy), this reflects the live owner's
	// state so the caller can format a busy notification.
	Session *model.Session
}

// SessionRepository persists Session metadata and provides atomic turn-lock
// primitives shared across modes (draft / casebound / future triage).
//
// The lookup key is (ChannelID, ThreadTS). All lock operations are
// per-Session and serialised at the storage layer (Firestore RunTransaction
// or in-memory mutex). See spec §5.3 for full semantics.
type SessionRepository interface {
	// GetByThread returns the Session for (channelID, threadTS), or
	// (nil, nil) when no Session exists yet.
	GetByThread(ctx context.Context, channelID, threadTS string) (*model.Session, error)

	// Put writes the Session. It does not touch the turn-lock fields
	// beyond what the caller has set; locks are normally maintained via
	// AcquireTurnLock / Heartbeat / ReleaseTurnLock instead.
	Put(ctx context.Context, s *model.Session) error

	// AcquireTurnLock atomically transitions the Session from idle (or
	// stale-running) into running owned by ownerID and returns the result.
	//
	// If no Session exists yet, newSessionFn() is invoked to construct an
	// initial Session (with zero turn-lock fields) which is then claimed
	// in the same transaction.
	//
	// staleAfter controls when an existing running lock can be reclaimed
	// based on TurnHeartbeatAt age. Pass the configured heartbeat staleness
	// window (typically 30s).
	AcquireTurnLock(
		ctx context.Context,
		channelID, threadTS, triggerTS, ownerID string,
		staleAfter time.Duration,
		newSessionFn func() *model.Session,
	) (AcquireResult, error)

	// Heartbeat refreshes TurnHeartbeatAt to now if and only if the
	// Session's TurnOwnerID equals ownerID. Returns the latest Session
	// snapshot on success, or ErrTurnOwnerMismatch when the lock has
	// been taken by another owner (stale reclaim, interrupt, or release).
	Heartbeat(ctx context.Context, channelID, threadTS, ownerID string) (*model.Session, error)

	// ReleaseTurnLock atomically transitions the Session from running back
	// to idle, but only if TurnOwnerID matches ownerID. Mismatched releases
	// are silently ignored (no error) so the Phase A defer pattern is safe
	// to call even after a stale reclaim.
	ReleaseTurnLock(ctx context.Context, channelID, threadTS, ownerID string) error
}
