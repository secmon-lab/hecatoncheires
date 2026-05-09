package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// TestStartTurn_HeartbeatRefreshes verifies the heartbeat goroutine actually
// runs while the turn is active: TurnHeartbeatAt must move forward at least
// once during the holding window.
func TestStartTurn_HeartbeatRefreshes(t *testing.T) {
	ctx := context.Background()
	d := &agent.CommonDeps{
		Repo:                memory.New(),
		InstanceID:          "test",
		HeartbeatInterval:   10 * time.Millisecond,
		HeartbeatStaleAfter: 200 * time.Millisecond,
	}

	ssn := &model.Session{
		ID:        "s1",
		ChannelID: "C-HB",
		ThreadTS:  "T-HB",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	h, err := d.StartTurn(ctx, ssn, "trig")
	gt.NoError(t, err).Required()
	gt.Bool(t, h.Acquired).True().Required()
	startHB := h.Session.TurnHeartbeatAt

	// Wait for at least 3 heartbeat ticks.
	time.Sleep(40 * time.Millisecond)

	cur, err := d.Repo.Session().GetByThread(ctx, "C-HB", "T-HB")
	gt.NoError(t, err).Required()
	gt.Value(t, cur).NotNil().Required()
	gt.Bool(t, cur.TurnHeartbeatAt.After(startHB)).True()

	h.Release(ctx)
	async.Wait()
}

// TestStartTurn_HeartbeatCancelsOnOwnerMismatch covers the path where the
// lock owner is replaced (simulating staleness reclaim by another instance).
// The heartbeat goroutine must detect ErrTurnOwnerMismatch on its next tick
// and cancel the original turn's ctx.
func TestStartTurn_HeartbeatCancelsOnOwnerMismatch(t *testing.T) {
	ctx := context.Background()
	d := &agent.CommonDeps{
		Repo:                memory.New(),
		InstanceID:          "test",
		HeartbeatInterval:   5 * time.Millisecond,
		HeartbeatStaleAfter: 50 * time.Millisecond,
	}

	ssn := &model.Session{
		ID:        "s1",
		ChannelID: "C-MM",
		ThreadTS:  "T-MM",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	h, err := d.StartTurn(ctx, ssn, "trig-A")
	gt.NoError(t, err).Required()
	gt.Bool(t, h.Acquired).True().Required()

	// Simulate another instance reclaiming the lock by overwriting the
	// session's TurnOwnerID directly. The next heartbeat tick from the
	// original turn will see the mismatch and cancel its ctx.
	stolen, err := d.Repo.Session().GetByThread(ctx, "C-MM", "T-MM")
	gt.NoError(t, err).Required()
	stolen.TurnOwnerID = "other-instance:trig-other"
	gt.NoError(t, d.Repo.Session().Put(ctx, stolen)).Required()

	select {
	case <-h.Ctx.Done():
		// expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first turn ctx to be cancelled after owner overwrite")
	}

	h.Release(ctx)
	async.Wait()
}
