package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runSessionRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	makeKey := func(suffix string) (channelID, threadTS string) {
		channelID = fmt.Sprintf("C%d", time.Now().UnixNano())
		threadTS = fmt.Sprintf("%d.%s", time.Now().UnixNano(), suffix)
		return
	}

	makeSeed := func(channelID, threadTS string) func() *model.Session {
		return func() *model.Session {
			return &model.Session{
				ID:        uuid.Must(uuid.NewV7()).String(),
				ChannelID: channelID,
				ThreadTS:  threadTS,
			}
		}
	}

	t.Run("GetByThread returns nil for missing session", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("missing")
		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("Put then GetByThread round trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("rt")
		now := time.Now().UTC().Truncate(time.Millisecond)

		s := &model.Session{
			ID:                      uuid.Must(uuid.NewV7()).String(),
			ChannelID:               ch,
			ThreadTS:                ts,
			LastMentionTS:           ts,
			LastAction:              model.SessionEndedWithQuestion,
			Kind:                    model.SessionKindWorkspaceAgent,
			WorkspaceID:             "ws-1",
			CaseID:                  42,
			ActionID:                7,
			CreatorUserID:           "U1",
			ProposalID:              model.CaseProposalID("draft-1"),
			ReactionSourceChannelID: "C-SRC",
			ReactionSourceMessageTS: "1700000000.000100",
			CreatedAt:               now,
			UpdatedAt:               now,
		}
		gt.NoError(t, repo.Session().Put(ctx, s)).Required()

		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()

		gt.Value(t, got.ID).Equal(s.ID)
		gt.Value(t, got.ChannelID).Equal(ch)
		gt.Value(t, got.ThreadTS).Equal(ts)
		gt.Value(t, got.LastMentionTS).Equal(ts)
		gt.Value(t, got.LastAction).Equal(model.SessionEndedWithQuestion)
		gt.Value(t, got.Kind).Equal(model.SessionKindWorkspaceAgent)
		gt.Value(t, got.WorkspaceID).Equal("ws-1")
		gt.Value(t, got.CaseID).Equal(int64(42))
		gt.Value(t, got.ActionID).Equal(int64(7))
		gt.Value(t, got.CreatorUserID).Equal("U1")
		gt.Value(t, got.ProposalID).Equal(model.CaseProposalID("draft-1"))
		gt.Value(t, got.ReactionSourceChannelID).Equal("C-SRC")
		gt.Value(t, got.ReactionSourceMessageTS).Equal("1700000000.000100")
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
		gt.Bool(t, got.UpdatedAt.Equal(now)).True()
	})

	// Kind is the discriminator the Slack dispatcher reads to keep a
	// workspace-agent thread out of the case-creation path. Sessions written
	// before the field existed carry no Kind at all; they must read back as the
	// zero value (SessionKindCase) so the old routing still applies to them.
	t.Run("Kind defaults to SessionKindCase when unset", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("kind-default")
		s := &model.Session{
			ID:        uuid.Must(uuid.NewV7()).String(),
			ChannelID: ch,
			ThreadTS:  ts,
		}
		gt.NoError(t, repo.Session().Put(ctx, s)).Required()

		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.Kind).Equal(model.SessionKindCase)
	})

	// Claim is what a host takes before its setup work so a concurrent event
	// already observes who owns the thread. It must create on first call and
	// return the stored record untouched on every later one.
	t.Run("Claim creates once and never overwrites", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("claim")

		first, err := repo.Session().Claim(ctx, ch, ts, func() *model.Session {
			return &model.Session{
				ID:          "claim-first",
				ChannelID:   ch,
				ThreadTS:    ts,
				WorkspaceID: "ws-claim",
				Kind:        model.SessionKindWorkspaceAgent,
			}
		})
		gt.NoError(t, err).Required()
		gt.Value(t, first).NotNil().Required()
		gt.Value(t, first.ID).Equal("claim-first")
		gt.Value(t, first.Kind).Equal(model.SessionKindWorkspaceAgent)
		gt.Bool(t, first.CreatedAt.IsZero()).False()
		gt.Bool(t, first.UpdatedAt.IsZero()).False()

		// A second claim with a different seed must lose: the first decision
		// about what owns this thread stands.
		second, err := repo.Session().Claim(ctx, ch, ts, func() *model.Session {
			return &model.Session{
				ID:          "claim-second",
				ChannelID:   ch,
				ThreadTS:    ts,
				WorkspaceID: "ws-claim",
				Kind:        model.SessionKindCase,
			}
		})
		gt.NoError(t, err).Required()
		gt.Value(t, second).NotNil().Required()
		gt.Value(t, second.ID).Equal("claim-first")
		gt.Value(t, second.Kind).Equal(model.SessionKindWorkspaceAgent)

		stored, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, stored).NotNil().Required()
		gt.Value(t, stored.ID).Equal("claim-first")
		gt.Value(t, stored.Kind).Equal(model.SessionKindWorkspaceAgent)
	})

	// Concurrent claims are the whole point: exactly one seed may win, and every
	// caller must be told the same winner.
	t.Run("Claim is atomic under concurrency", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("claim-race")

		const racers = 8
		var wg sync.WaitGroup
		results := make([]*model.Session, racers)
		errs := make([]error, racers)
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				id := fmt.Sprintf("claim-%d", i)
				kind := model.SessionKindCase
				if i%2 == 0 {
					kind = model.SessionKindWorkspaceAgent
				}
				results[i], errs[i] = repo.Session().Claim(ctx, ch, ts, func() *model.Session {
					return &model.Session{
						ID:          id,
						ChannelID:   ch,
						ThreadTS:    ts,
						WorkspaceID: "ws-claim",
						Kind:        kind,
					}
				})
			}(i)
		}
		wg.Wait()

		stored, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, stored).NotNil().Required()

		for i := 0; i < racers; i++ {
			gt.NoError(t, errs[i]).Required()
			gt.Value(t, results[i]).NotNil().Required()
			gt.Value(t, results[i].ID).Equal(stored.ID)
			gt.Value(t, results[i].Kind).Equal(stored.Kind)
		}
	})

	t.Run("Claim rejects missing keys and a nil seed", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("claim-invalid")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().Claim(ctx, "", ts, seed)
		gt.Error(t, err)
		_, err = repo.Session().Claim(ctx, ch, "", seed)
		gt.Error(t, err)
		_, err = repo.Session().Claim(ctx, ch, ts, nil)
		gt.Error(t, err)
	})

	// AcquireTurnLock persists the seeded Session on first acquisition, so the
	// Kind set by the host must survive that path too — not just Put.
	t.Run("AcquireTurnLock persists the seeded Kind", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("kind-seed")
		res, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trigger-1", "owner-1", time.Hour, func() *model.Session {
			return &model.Session{
				ID:        uuid.Must(uuid.NewV7()).String(),
				ChannelID: ch,
				ThreadTS:  ts,
				Kind:      model.SessionKindWorkspaceAgent,
			}
		})
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Acquired).True().Required()
		gt.Value(t, res.Session.Kind).Equal(model.SessionKindWorkspaceAgent)

		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.Kind).Equal(model.SessionKindWorkspaceAgent)
	})

	t.Run("rejects missing required fields on Put", func(t *testing.T) {
		repo := newRepo(t)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{})).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, nil)).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{ID: "s", ThreadTS: "1.1"})).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{ID: "s", ChannelID: "C"})).Is(model.ErrSessionValidation)
	})

	t.Run("AcquireTurnLock creates new session on first call", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("acq-new")
		seed := makeSeed(ch, ts)

		res, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Acquired).True()
		gt.Bool(t, res.Reclaimed).False()
		gt.Bool(t, res.IdempotentRetry).False()
		gt.Value(t, res.Session).NotNil().Required()
		gt.Value(t, res.Session.TurnState).Equal(model.SessionTurnRunning)
		gt.Value(t, res.Session.TurnOwnerID).Equal("owner-1")
		gt.Value(t, res.Session.TurnTriggerTS).Equal("trig-1")
		gt.Bool(t, res.Session.TurnHeartbeatAt.IsZero()).False()
		gt.Bool(t, res.Session.TurnStartedAt.IsZero()).False()

		stored, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, stored).NotNil().Required()
		gt.Value(t, stored.TurnOwnerID).Equal("owner-1")
	})

	t.Run("AcquireTurnLock returns busy when fresh owner exists", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("busy")
		seed := makeSeed(ch, ts)

		first, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, first.Acquired).True().Required()

		second, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-2", "owner-2", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, second.Acquired).False()
		gt.Bool(t, second.IdempotentRetry).False()
		gt.Bool(t, second.Reclaimed).False()
		gt.Value(t, second.Session).NotNil().Required()
		gt.Value(t, second.Session.TurnOwnerID).Equal("owner-1")
	})

	t.Run("AcquireTurnLock returns IdempotentRetry on duplicate trigger", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("idem")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-dup", "owner-A", time.Minute, seed)
		gt.NoError(t, err).Required()

		// Same trigger from a different ownerID should still flag as idempotent retry.
		res, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-dup", "owner-B", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Acquired).False()
		gt.Bool(t, res.IdempotentRetry).True()
		gt.Value(t, res.Session).NotNil().Required()
		gt.Value(t, res.Session.TurnOwnerID).Equal("owner-A")
	})

	t.Run("AcquireTurnLock reclaims after staleAfter elapses", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("stale")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Millisecond, seed)
		gt.NoError(t, err).Required()

		// Wait long enough to exceed staleAfter.
		time.Sleep(20 * time.Millisecond)

		res, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-2", "owner-2", time.Millisecond, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Acquired).True()
		gt.Bool(t, res.Reclaimed).True()
		gt.Value(t, res.Session.TurnOwnerID).Equal("owner-2")
		gt.Value(t, res.Session.TurnTriggerTS).Equal("trig-2")
	})

	t.Run("Heartbeat refreshes TurnHeartbeatAt for live owner", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("hb")
		seed := makeSeed(ch, ts)

		first, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()
		startHB := first.Session.TurnHeartbeatAt

		time.Sleep(5 * time.Millisecond)

		got, err := repo.Session().Heartbeat(ctx, ch, ts, "owner-1")
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Bool(t, got.TurnHeartbeatAt.After(startHB)).True()
		gt.Value(t, got.TurnOwnerID).Equal("owner-1")
	})

	t.Run("Heartbeat returns ErrTurnOwnerMismatch on owner mismatch", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("hb-mismatch")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()

		got, err := repo.Session().Heartbeat(ctx, ch, ts, "owner-other")
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, interfaces.ErrTurnOwnerMismatch)).True()
		gt.Value(t, got).Nil()
	})

	t.Run("Heartbeat returns ErrTurnOwnerMismatch when session does not exist", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("hb-missing")
		_, err := repo.Session().Heartbeat(ctx, ch, ts, "owner-1")
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, interfaces.ErrTurnOwnerMismatch)).True()
	})

	t.Run("ReleaseTurnLock idle's the session for the live owner", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("release")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()

		gt.NoError(t, repo.Session().ReleaseTurnLock(ctx, ch, ts, "owner-1")).Required()

		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.TurnState).Equal(model.SessionTurnIdle)
		gt.Value(t, got.TurnOwnerID).Equal("")
		gt.Value(t, got.TurnTriggerTS).Equal("")
	})

	t.Run("ReleaseTurnLock is a no-op for mismatched owner", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("release-mismatch")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()

		gt.NoError(t, repo.Session().ReleaseTurnLock(ctx, ch, ts, "owner-other")).Required()

		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got.TurnState).Equal(model.SessionTurnRunning)
		gt.Value(t, got.TurnOwnerID).Equal("owner-1")
	})

	t.Run("ReleaseTurnLock on missing session is a no-op", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("release-missing")
		gt.NoError(t, repo.Session().ReleaseTurnLock(ctx, ch, ts, "owner-1")).Required()
	})

	t.Run("Acquire after Release succeeds without staleness", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("re-acq")
		seed := makeSeed(ch, ts)

		_, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-1", "owner-1", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.NoError(t, repo.Session().ReleaseTurnLock(ctx, ch, ts, "owner-1")).Required()

		res, err := repo.Session().AcquireTurnLock(ctx, ch, ts, "trig-2", "owner-2", time.Minute, seed)
		gt.NoError(t, err).Required()
		gt.Bool(t, res.Acquired).True()
		gt.Bool(t, res.Reclaimed).False()
		gt.Value(t, res.Session.TurnOwnerID).Equal("owner-2")
	})

	t.Run("parallel AcquireTurnLock yields exactly one Acquired", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("parallel")
		seed := makeSeed(ch, ts)

		const N = 8
		var wg sync.WaitGroup
		var mu sync.Mutex
		acquiredCount := 0
		busyCount := 0
		errs := []error{}

		for i := range N {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				res, err := repo.Session().AcquireTurnLock(ctx, ch, ts,
					fmt.Sprintf("trig-%d", i),
					fmt.Sprintf("owner-%d", i),
					time.Minute, seed)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				if res.Acquired {
					acquiredCount++
				} else {
					busyCount++
				}
			}(i)
		}
		wg.Wait()

		gt.Array(t, errs).Length(0)
		gt.Number(t, acquiredCount).Equal(1)
		gt.Number(t, busyCount).Equal(N - 1)
	})
}

func TestSessionRepository_Memory(t *testing.T) {
	t.Parallel()
	runSessionRepositoryTest(t, func(_ *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestSessionRepository_Firestore(t *testing.T) {
	t.Parallel()
	runSessionRepositoryTest(t, newFirestoreRepository)
}
