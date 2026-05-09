package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// minimalDeps returns a CommonDeps configured with just enough wiring for
// StartTurn to exercise the lock/heartbeat path. LLM/HistoryRepo/TraceRepo
// are nil because StartTurn does not touch them — modes are responsible for
// their own LLM concerns.
func minimalDeps(t *testing.T) *agent.CommonDeps {
	t.Helper()
	return &agent.CommonDeps{
		Repo:                memory.New(),
		InstanceID:          "test-instance",
		HeartbeatInterval:   20 * time.Millisecond,
		HeartbeatStaleAfter: 100 * time.Millisecond,
	}
}

func newSession(channelID, threadTS string) *model.Session {
	return &model.Session{
		ID:        "session-" + threadTS,
		ChannelID: channelID,
		ThreadTS:  threadTS,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestStartTurn_AcquiresAndReleases(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	ssn := newSession("C1", "T1")
	h, err := d.StartTurn(ctx, ssn, "trig-1")
	gt.NoError(t, err).Required()
	gt.Bool(t, h.Acquired).True().Required()
	gt.Bool(t, h.Idempotent).False()
	gt.Value(t, h.BusyOwner).Nil()
	gt.Value(t, h.OwnerID).Equal("test-instance:trig-1")
	gt.Value(t, h.Session).NotNil().Required()
	gt.Value(t, h.Session.TurnOwnerID).Equal("test-instance:trig-1")
	gt.Value(t, h.Ctx).NotNil()

	h.Release(ctx)

	// After release, a different trigger should acquire cleanly.
	h2, err := d.StartTurn(ctx, ssn, "trig-2")
	gt.NoError(t, err).Required()
	gt.Bool(t, h2.Acquired).True()
	h2.Release(ctx)
}

func TestStartTurn_BusyWhenLocked(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	ssn := newSession("C2", "T2")
	first, err := d.StartTurn(ctx, ssn, "trig-A")
	gt.NoError(t, err).Required()
	gt.Bool(t, first.Acquired).True().Required()
	defer first.Release(ctx)

	second, err := d.StartTurn(ctx, ssn, "trig-B")
	gt.NoError(t, err).Required()
	gt.Bool(t, second.Acquired).False()
	gt.Bool(t, second.Idempotent).False()
	gt.Value(t, second.BusyOwner).NotNil().Required()
	gt.Value(t, second.BusyOwner.TurnOwnerID).Equal(first.OwnerID)
}

func TestStartTurn_IdempotentRetry(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	ssn := newSession("C3", "T3")
	first, err := d.StartTurn(ctx, ssn, "trig-X")
	gt.NoError(t, err).Required()
	gt.Bool(t, first.Acquired).True().Required()
	defer first.Release(ctx)

	dup, err := d.StartTurn(ctx, ssn, "trig-X")
	gt.NoError(t, err).Required()
	gt.Bool(t, dup.Acquired).False()
	gt.Bool(t, dup.Idempotent).True()
	gt.Value(t, dup.BusyOwner).Nil()
}

func TestStartTurn_RejectsInvalidInputs(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	_, err := d.StartTurn(ctx, nil, "trig")
	gt.Error(t, err)

	_, err = d.StartTurn(ctx, &model.Session{ChannelID: ""}, "trig")
	gt.Error(t, err)
}

func TestLoadOrCreateSession_NewWhenMissing(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	got, err := d.LoadOrCreateSession(ctx, agent.LoadOrCreateSessionInput{
		ChannelID:     "C-NEW",
		ThreadTS:      "1700000000.000001",
		WorkspaceID:   "ws",
		CaseID:        42,
		CreatorUserID: "U1",
	})
	gt.NoError(t, err).Required()
	gt.Value(t, got).NotNil().Required()
	gt.String(t, got.ID).NotEqual("")
	gt.Value(t, got.ChannelID).Equal("C-NEW")
	gt.Value(t, got.WorkspaceID).Equal("ws")
	gt.Value(t, got.CaseID).Equal(int64(42))
	gt.Value(t, got.CreatorUserID).Equal("U1")
}

func TestLoadOrCreateSession_ReturnsExisting(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	prev := &model.Session{
		ID:        "existing",
		ChannelID: "C-EX",
		ThreadTS:  "1700000001.000001",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	gt.NoError(t, d.Repo.Session().Put(ctx, prev)).Required()

	got, err := d.LoadOrCreateSession(ctx, agent.LoadOrCreateSessionInput{
		ChannelID: "C-EX",
		ThreadTS:  "1700000001.000001",
	})
	gt.NoError(t, err).Required()
	gt.Value(t, got).NotNil().Required()
	gt.Value(t, got.ID).Equal("existing")
}

func TestLoadOrCreateSession_DetectActionID(t *testing.T) {
	ctx := context.Background()
	d := minimalDeps(t)

	got, err := d.LoadOrCreateSession(ctx, agent.LoadOrCreateSessionInput{
		ChannelID:   "C-ACT",
		ThreadTS:    "1700000002.000001",
		WorkspaceID: "ws",
		DetectActionID: func(_ context.Context, ws, ts string) (int64, error) {
			gt.Value(t, ws).Equal("ws")
			gt.Value(t, ts).Equal("1700000002.000001")
			return 7, nil
		},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, got.ActionID).Equal(int64(7))
}
