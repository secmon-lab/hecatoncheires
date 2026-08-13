package repository_test

import (
	"context"
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

	// The cursor is what the next turn's delta scan starts after, and the call that
	// advances it races the turn it just started — whose completion handler writes
	// the same row. So it must move one field, monotonically, and never resurrect a
	// Session that is gone.
	t.Run("AdvanceLastMention moves the cursor forward only", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("cursor")
		now := time.Now().UTC()
		ssn := &model.Session{
			ID:            uuid.Must(uuid.NewV7()).String(),
			ChannelID:     ch,
			ThreadTS:      ts,
			LastMentionTS: "1700000000.000200",
			LastAction:    model.SessionEndedWithQuestion,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		gt.NoError(t, repo.Session().Put(ctx, ssn)).Required()

		// Forward: applied.
		gt.NoError(t, repo.Session().AdvanceLastMention(ctx, ch, ts, "1700000000.000300")).Required()
		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.String(t, got.LastMentionTS).Equal("1700000000.000300")
		// Everything else survives: a full write from the spawning side would have
		// dropped the pending outcome the completion handler recorded.
		gt.Value(t, got.LastAction).Equal(model.SessionEndedWithQuestion)
		gt.Value(t, got.ID).Equal(ssn.ID)

		// Backward and equal: ignored, so two racing triggers leave the later cursor
		// standing whichever write lands second.
		gt.NoError(t, repo.Session().AdvanceLastMention(ctx, ch, ts, "1700000000.000100")).Required()
		gt.NoError(t, repo.Session().AdvanceLastMention(ctx, ch, ts, "1700000000.000300")).Required()
		got, err = repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.String(t, got.LastMentionTS).Equal("1700000000.000300")
	})

	t.Run("AdvanceLastMention on a missing session is a no-op", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("cursor-missing")
		gt.NoError(t, repo.Session().AdvanceLastMention(ctx, ch, ts, "1700000000.000100")).Required()
		got, err := repo.Session().GetByThread(ctx, ch, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("AdvanceLastMention rejects missing keys", func(t *testing.T) {
		repo := newRepo(t)
		ch, ts := makeKey("cursor-invalid")
		gt.Error(t, repo.Session().AdvanceLastMention(ctx, "", ts, "1700000000.000100"))
		gt.Error(t, repo.Session().AdvanceLastMention(ctx, ch, "", "1700000000.000100"))
		gt.Error(t, repo.Session().AdvanceLastMention(ctx, ch, ts, ""))
	})

	t.Run("rejects missing required fields on Put", func(t *testing.T) {
		repo := newRepo(t)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{})).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, nil)).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{ID: "s", ThreadTS: "1.1"})).Is(model.ErrSessionValidation)
		gt.Error(t, repo.Session().Put(ctx, &model.Session{ID: "s", ChannelID: "C"})).Is(model.ErrSessionValidation)
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
