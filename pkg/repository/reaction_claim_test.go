package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runReactionClaimRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("first claim wins, second is deduped", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ch := fmt.Sprintf("C%d", time.Now().UnixNano())
		ts := fmt.Sprintf("%d.000100", time.Now().UnixNano())

		claimed, err := repo.ReactionClaim().Claim(ctx, wsID, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, claimed).True()

		claimed2, err := repo.ReactionClaim().Claim(ctx, wsID, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, claimed2).False()
	})

	t.Run("different source messages claim independently", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ch := fmt.Sprintf("C%d", time.Now().UnixNano())
		tsA := fmt.Sprintf("%d.000100", time.Now().UnixNano())
		tsB := fmt.Sprintf("%d.000200", time.Now().UnixNano())

		a, err := repo.ReactionClaim().Claim(ctx, wsID, ch, tsA)
		gt.NoError(t, err).Required()
		gt.Bool(t, a).True()

		b, err := repo.ReactionClaim().Claim(ctx, wsID, ch, tsB)
		gt.NoError(t, err).Required()
		gt.Bool(t, b).True()
	})

	t.Run("same message in different workspaces claim independently", func(t *testing.T) {
		repo := newRepo(t)
		ch := fmt.Sprintf("C%d", time.Now().UnixNano())
		ts := fmt.Sprintf("%d.000100", time.Now().UnixNano())
		ws1 := fmt.Sprintf("ws1-%d", time.Now().UnixNano())
		ws2 := fmt.Sprintf("ws2-%d", time.Now().UnixNano())

		a, err := repo.ReactionClaim().Claim(ctx, ws1, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, a).True()

		b, err := repo.ReactionClaim().Claim(ctx, ws2, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, b).True()
	})

	t.Run("release allows a re-claim", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ch := fmt.Sprintf("C%d", time.Now().UnixNano())
		ts := fmt.Sprintf("%d.000100", time.Now().UnixNano())

		claimed, err := repo.ReactionClaim().Claim(ctx, wsID, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, claimed).True()

		gt.NoError(t, repo.ReactionClaim().Release(ctx, wsID, ch, ts)).Required()

		reclaimed, err := repo.ReactionClaim().Claim(ctx, wsID, ch, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, reclaimed).True()
	})

	t.Run("release of a missing claim is not an error", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ch := fmt.Sprintf("C%d", time.Now().UnixNano())
		ts := fmt.Sprintf("%d.000100", time.Now().UnixNano())
		gt.NoError(t, repo.ReactionClaim().Release(ctx, wsID, ch, ts))
	})

	t.Run("empty identity is rejected", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.ReactionClaim().Claim(ctx, "", "C1", "1.0")
		gt.Error(t, err)
		_, err = repo.ReactionClaim().Claim(ctx, "ws", "", "1.0")
		gt.Error(t, err)
		_, err = repo.ReactionClaim().Claim(ctx, "ws", "C1", "")
		gt.Error(t, err)
	})
}

func TestReactionClaimRepository_Memory(t *testing.T) {
	t.Parallel()
	runReactionClaimRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestReactionClaimRepository_Firestore(t *testing.T) {
	t.Parallel()
	runReactionClaimRepositoryTest(t, newFirestoreRepository)
}
