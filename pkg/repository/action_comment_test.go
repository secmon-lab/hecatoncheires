package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func newTestActionComment(actionID int64, suffix string, createdAt time.Time) *model.ActionComment {
	return &model.ActionComment{
		ID:        fmt.Sprintf("comment-%d-%s", time.Now().UnixNano(), suffix),
		ActionID:  actionID,
		AuthorID:  "U" + suffix,
		Body:      "body " + suffix,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}

func runActionCommentRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Put and Get round-trip every field", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		created := time.Now().UTC().Truncate(time.Millisecond)
		updated := created.Add(3 * time.Minute)
		want := &model.ActionComment{
			ID:        fmt.Sprintf("comment-%d", time.Now().UnixNano()),
			ActionID:  actionID,
			AuthorID:  "U12345",
			Body:      "the alert matched a known maintenance window",
			CreatedAt: created,
			UpdatedAt: updated,
		}
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, want)).Required()

		got, err := repo.ActionComment().Get(ctx, wsID, actionID, want.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(want.ID)
		gt.Value(t, got.ActionID).Equal(want.ActionID)
		gt.Value(t, got.AuthorID).Equal(want.AuthorID)
		gt.Value(t, got.Body).Equal(want.Body)
		gt.Bool(t, got.CreatedAt.Sub(want.CreatedAt) < time.Second).True()
		gt.Bool(t, want.CreatedAt.Sub(got.CreatedAt) < time.Second).True()
		gt.Bool(t, got.UpdatedAt.Sub(want.UpdatedAt) < time.Second).True()
		gt.Bool(t, want.UpdatedAt.Sub(got.UpdatedAt) < time.Second).True()
		gt.Bool(t, got.IsEdited()).True()
	})

	t.Run("List returns newest first", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		oldest := newTestActionComment(actionID, "oldest", now.Add(-2*time.Minute))
		middle := newTestActionComment(actionID, "middle", now.Add(-1*time.Minute))
		newest := newTestActionComment(actionID, "newest", now)
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, oldest)).Required()
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, middle)).Required()
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, newest)).Required()

		got, cursor, err := repo.ActionComment().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(3).Required()
		gt.Value(t, got[0].ID).Equal(newest.ID)
		gt.Value(t, got[1].ID).Equal(middle.ID)
		gt.Value(t, got[2].ID).Equal(oldest.ID)
		gt.Value(t, cursor).Equal("")
	})

	t.Run("List paginates by cursor", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		// Stored oldest-first so the expected newest-first order is the reverse.
		ids := make([]string, 0, 5)
		for i := range 5 {
			c := newTestActionComment(actionID, fmt.Sprintf("%d", i), now.Add(time.Duration(i)*time.Minute))
			gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, c)).Required()
			ids = append(ids, c.ID)
		}
		wantOrder := []string{ids[4], ids[3], ids[2], ids[1], ids[0]}

		page1, cursor1, err := repo.ActionComment().List(ctx, wsID, actionID, 2, "")
		gt.NoError(t, err).Required()
		gt.Array(t, page1).Length(2).Required()
		gt.Value(t, page1[0].ID).Equal(wantOrder[0])
		gt.Value(t, page1[1].ID).Equal(wantOrder[1])
		gt.Value(t, cursor1).Equal(wantOrder[1])

		page2, cursor2, err := repo.ActionComment().List(ctx, wsID, actionID, 2, cursor1)
		gt.NoError(t, err).Required()
		gt.Array(t, page2).Length(2).Required()
		gt.Value(t, page2[0].ID).Equal(wantOrder[2])
		gt.Value(t, page2[1].ID).Equal(wantOrder[3])
		gt.Value(t, cursor2).Equal(wantOrder[3])

		page3, cursor3, err := repo.ActionComment().List(ctx, wsID, actionID, 2, cursor2)
		gt.NoError(t, err).Required()
		gt.Array(t, page3).Length(1).Required()
		gt.Value(t, page3[0].ID).Equal(wantOrder[4])
		gt.Value(t, cursor3).Equal("")
	})

	t.Run("Put with same ID replaces", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		c := newTestActionComment(actionID, "edit", now)
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, c)).Required()

		c.Body = "rewritten body"
		c.UpdatedAt = now.Add(time.Minute)
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, c)).Required()

		got, cursor, err := repo.ActionComment().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].Body).Equal("rewritten body")
		gt.Bool(t, got[0].IsEdited()).True()
		gt.Value(t, cursor).Equal("")
	})

	t.Run("comments are isolated per action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionA := time.Now().UnixNano()
		actionB := actionA + 1

		now := time.Now().UTC().Truncate(time.Millisecond)
		forA := newTestActionComment(actionA, "a", now)
		forB := newTestActionComment(actionB, "b", now)
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionA, forA)).Required()
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionB, forB)).Required()

		gotA, _, err := repo.ActionComment().List(ctx, wsID, actionA, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, gotA).Length(1).Required()
		gt.Value(t, gotA[0].ID).Equal(forA.ID)

		gotB, _, err := repo.ActionComment().List(ctx, wsID, actionB, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, gotB).Length(1).Required()
		gt.Value(t, gotB[0].ID).Equal(forB.ID)
	})

	t.Run("Get of a missing comment fails", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		_, err := repo.ActionComment().Get(ctx, wsID, actionID, "no-such-comment")
		gt.Error(t, err)
	})

	t.Run("Delete removes the comment and is a no-op when absent", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		c := newTestActionComment(actionID, "gone", now)
		gt.NoError(t, repo.ActionComment().Put(ctx, wsID, actionID, c)).Required()

		gt.NoError(t, repo.ActionComment().Delete(ctx, wsID, actionID, c.ID)).Required()

		_, err := repo.ActionComment().Get(ctx, wsID, actionID, c.ID)
		gt.Error(t, err)

		got, _, err := repo.ActionComment().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)

		gt.NoError(t, repo.ActionComment().Delete(ctx, wsID, actionID, "no-such-comment"))
	})

	t.Run("Put rejects an invalid comment", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		noAuthor := newTestActionComment(actionID, "noauthor", now)
		noAuthor.AuthorID = ""
		gt.Error(t, repo.ActionComment().Put(ctx, wsID, actionID, noAuthor)).Is(model.ErrActionCommentValidation)
	})

	t.Run("Put rejects a mismatched ActionID", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		other := newTestActionComment(actionID+1, "mismatch", now)
		gt.Error(t, repo.ActionComment().Put(ctx, wsID, actionID, other)).Is(model.ErrActionCommentValidation)
	})

	t.Run("List of an unknown action is empty", func(t *testing.T) {
		repo := newRepo(t)

		got, cursor, err := repo.ActionComment().List(ctx, "non-existent-ws", 99999, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
		gt.Value(t, cursor).Equal("")
	})
}

func TestActionCommentRepository_Memory(t *testing.T) {
	t.Parallel()
	runActionCommentRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestActionCommentRepository_Firestore(t *testing.T) {
	t.Parallel()
	runActionCommentRepositoryTest(t, newFirestoreRepository)
}
