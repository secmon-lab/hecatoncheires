package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// startHeartbeat spawns a goroutine that refreshes the Session's
// TurnHeartbeatAt every interval. The loop also reads the returned Session
// snapshot so future Phase B code can detect interrupt requests on the same
// tick without an extra round trip.
//
// Owner-mismatch errors from Heartbeat (i.e. another instance reclaimed the
// lock via staleness, or the lock was released externally) are treated as
// fatal: cancelTurn() is invoked so the agent body unwinds via ctx.Done().
//
// Transient errors (network blips, Firestore 5xx) are non-fatal: errutil.Handle
// records them and the loop continues. The turn driver is the one in charge
// of detecting persistent ownership issues; this goroutine only escalates the
// definitive ErrTurnOwnerMismatch case.
//
// The returned stop function signals the goroutine to exit (via a private
// channel — async.Dispatch deliberately does not propagate ctx cancellation,
// so we cannot rely on parent cancellation alone) and waits for it to finish.
// Callers wrap the call in defer; LIFO order in the turn driver ensures
// stop() runs before ReleaseTurnLock so the goroutine is guaranteed to be
// quiet by the time we release.
func (d *CommonDeps) startHeartbeat(
	parent context.Context,
	ssn *model.Session,
	ownerID string,
	cancelTurn context.CancelFunc,
) (stop func()) {
	interval := d.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	var done sync.WaitGroup
	done.Add(1)

	async.Dispatch(parent, func(ctx context.Context) error {
		defer done.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return nil
			case <-ticker.C:
				_, err := d.Repo.Session().Heartbeat(ctx, ssn.ChannelID, ssn.ThreadTS, ownerID)
				if err != nil {
					if errors.Is(err, interfaces.ErrTurnOwnerMismatch) {
						cancelTurn()
						return nil
					}
					errutil.Handle(ctx, err, "session heartbeat failed")
					continue
				}
				// Phase B hook (not used in Phase A): inspect the heartbeat
				// snapshot's TurnState and call cancelTurn() when
				// SessionTurnInterrupted.
			}
		}
	})

	return func() {
		stopOnce.Do(func() { close(stopCh) })
		done.Wait()
	}
}

// Default values for the per-CommonDeps heartbeat config when callers leave
// HeartbeatInterval / HeartbeatStaleAfter zero. These keep tests terse but
// production wiring should set them explicitly via CLI flags.
const (
	DefaultHeartbeatInterval   = 10 * time.Second
	DefaultHeartbeatStaleAfter = 30 * time.Second
)
