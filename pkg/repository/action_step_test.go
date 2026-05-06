package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	repofirestore "github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runActionStepRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository, notFoundErr error) {
	t.Helper()
	ctx := context.Background()

	t.Run("Put / Get / List in created order", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		s1 := &model.ActionStep{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Title:     "Step one",
			CreatedBy: "U001",
			CreatedAt: now.Add(-3 * time.Second),
			UpdatedAt: now.Add(-3 * time.Second),
		}
		s2 := &model.ActionStep{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Title:     "Step two",
			CreatedBy: "U002",
			CreatedAt: now.Add(-2 * time.Second),
			UpdatedAt: now.Add(-2 * time.Second),
		}
		s3 := &model.ActionStep{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Title:     "Step three (done)",
			DoneAt:    timePtr(now.Add(-1 * time.Second)),
			DoneBy:    "U003",
			CreatedBy: "U001",
			CreatedAt: now.Add(-1 * time.Second),
			UpdatedAt: now.Add(-1 * time.Second),
		}

		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, s1)).Required()
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, s2)).Required()
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, s3)).Required()

		got, err := repo.ActionStep().Get(ctx, wsID, actionID, s3.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(s3.ID)
		gt.Value(t, got.Title).Equal("Step three (done)")
		gt.Value(t, got.DoneBy).Equal("U003")
		gt.Value(t, got.CreatedBy).Equal("U001")
		gt.Value(t, got.DoneAt).NotNil()

		listed, err := repo.ActionStep().List(ctx, wsID, actionID)
		gt.NoError(t, err).Required()
		gt.Array(t, listed).Length(3).Required()
		gt.Value(t, listed[0].ID).Equal(s1.ID)
		gt.Value(t, listed[1].ID).Equal(s2.ID)
		gt.Value(t, listed[2].ID).Equal(s3.ID)
	})

	t.Run("Put replaces existing step", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()
		stepID := uuid.NewString()

		now := time.Now().UTC().Truncate(time.Millisecond)
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, &model.ActionStep{
			ID:        stepID,
			ActionID:  actionID,
			Title:     "first title",
			CreatedAt: now,
			UpdatedAt: now,
		})).Required()

		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, &model.ActionStep{
			ID:        stepID,
			ActionID:  actionID,
			Title:     "second title",
			DoneAt:    timePtr(now.Add(time.Second)),
			DoneBy:    "U042",
			CreatedAt: now,
			UpdatedAt: now.Add(time.Second),
		})).Required()

		got, err := repo.ActionStep().Get(ctx, wsID, actionID, stepID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Title).Equal("second title")
		gt.Value(t, got.DoneBy).Equal("U042")
		gt.Value(t, got.DoneAt).NotNil()

		listed, err := repo.ActionStep().List(ctx, wsID, actionID)
		gt.NoError(t, err).Required()
		gt.Array(t, listed).Length(1)
	})

	t.Run("Get returns not-found for missing step", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		_, err := repo.ActionStep().Get(ctx, wsID, actionID, "missing-id")
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, notFoundErr)).True()
	})

	t.Run("List returns empty for action with no steps", func(t *testing.T) {
		repo := newRepo(t)
		listed, err := repo.ActionStep().List(ctx, "no-ws", 99999)
		gt.NoError(t, err).Required()
		gt.Array(t, listed).Length(0)
	})

	t.Run("Delete removes only the targeted step", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		s1 := &model.ActionStep{ID: uuid.NewString(), ActionID: actionID, Title: "keep", CreatedAt: now, UpdatedAt: now}
		s2 := &model.ActionStep{ID: uuid.NewString(), ActionID: actionID, Title: "drop", CreatedAt: now.Add(time.Millisecond), UpdatedAt: now.Add(time.Millisecond)}
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, s1)).Required()
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, s2)).Required()

		gt.NoError(t, repo.ActionStep().Delete(ctx, wsID, actionID, s2.ID)).Required()

		listed, err := repo.ActionStep().List(ctx, wsID, actionID)
		gt.NoError(t, err).Required()
		gt.Array(t, listed).Length(1).Required()
		gt.Value(t, listed[0].ID).Equal(s1.ID)
		gt.Value(t, listed[0].Title).Equal("keep")
	})

	t.Run("Delete is no-op for missing step", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		gt.NoError(t, repo.ActionStep().Delete(ctx, wsID, actionID, "missing-id"))
	})

	t.Run("steps are scoped per action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionA := time.Now().UnixNano()
		actionB := actionA + 1
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, &model.ActionStep{
			ID: uuid.NewString(), ActionID: actionA, Title: "for A",
			CreatedAt: now, UpdatedAt: now,
		})).Required()
		gt.NoError(t, repo.ActionStep().Put(ctx, wsID, &model.ActionStep{
			ID: uuid.NewString(), ActionID: actionB, Title: "for B",
			CreatedAt: now, UpdatedAt: now,
		})).Required()

		listA, err := repo.ActionStep().List(ctx, wsID, actionA)
		gt.NoError(t, err).Required()
		gt.Array(t, listA).Length(1).Required()
		gt.Value(t, listA[0].Title).Equal("for A")

		listB, err := repo.ActionStep().List(ctx, wsID, actionB)
		gt.NoError(t, err).Required()
		gt.Array(t, listB).Length(1).Required()
		gt.Value(t, listB[0].Title).Equal("for B")
	})

	t.Run("rejects nil step and empty ID", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		gt.Error(t, repo.ActionStep().Put(ctx, wsID, nil))
		gt.Error(t, repo.ActionStep().Put(ctx, wsID, &model.ActionStep{
			ID: "", ActionID: actionID, Title: "x",
		}))
	})
}

func timePtr(t time.Time) *time.Time { return &t }

func TestActionStepRepository_Memory(t *testing.T) {
	runActionStepRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	}, memory.ErrNotFound)
}

func TestActionStepRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runActionStepRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := repofirestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})
		return repo
	}, repofirestore.ErrNotFound)
}
