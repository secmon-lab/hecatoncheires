package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runTagRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	isNotFound := func(err error) bool {
		return errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
	}

	repo := newRepo(t)

	t.Run("Create and Get round-trips all fields", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		now := time.Now().UTC().Truncate(time.Millisecond)
		input := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "ops",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		created, err := repo.Tag().Create(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.ID).Equal(input.ID)

		got, err := repo.Tag().Get(ctx, wsID, input.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, got.ID).Equal(input.ID)
		gt.Value(t, got.WorkspaceID).Equal(wsID)
		gt.String(t, got.Name).Equal("ops")
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
		gt.Bool(t, got.UpdatedAt.Equal(now)).True()
	})

	t.Run("Create tag with empty name", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		now := time.Now().UTC()
		input := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		created, err := repo.Tag().Create(ctx, wsID, input)
		gt.NoError(t, err).Required()

		got, err := repo.Tag().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Name).Equal("")
	})

	t.Run("Get not found", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		_, err := repo.Tag().Get(ctx, wsID, model.NewTagID())
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("List returns tags sorted by CreatedAt ascending", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		base := time.Now().UTC()

		tagA := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "aaa",
			CreatedAt:   base,
			UpdatedAt:   base,
		}
		tagB := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "bbb",
			CreatedAt:   base.Add(time.Second),
			UpdatedAt:   base.Add(time.Second),
		}
		tagC := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "ccc",
			CreatedAt:   base.Add(2 * time.Second),
			UpdatedAt:   base.Add(2 * time.Second),
		}

		_, err := repo.Tag().Create(ctx, wsID, tagA)
		gt.NoError(t, err).Required()
		_, err = repo.Tag().Create(ctx, wsID, tagB)
		gt.NoError(t, err).Required()
		_, err = repo.Tag().Create(ctx, wsID, tagC)
		gt.NoError(t, err).Required()

		items, err := repo.Tag().List(ctx, wsID)
		gt.NoError(t, err).Required()
		gt.Array(t, items).Length(3).Required()
		gt.Value(t, items[0].ID).Equal(tagA.ID)
		gt.Value(t, items[1].ID).Equal(tagB.ID)
		gt.Value(t, items[2].ID).Equal(tagC.ID)
	})

	t.Run("List empty workspace returns empty slice", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		items, err := repo.Tag().List(ctx, wsID)
		gt.NoError(t, err).Required()
		gt.Array(t, items).Length(0)
	})

	t.Run("Update persists name change, ID remains immutable", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		original := &model.Tag{
			ID:          model.NewTagID(),
			WorkspaceID: wsID,
			Name:        "before",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := repo.Tag().Create(ctx, wsID, original)
		gt.NoError(t, err).Required()

		later := now.Add(time.Minute)
		updated, err := repo.Tag().Update(ctx, wsID, &model.Tag{
			ID:          original.ID,
			WorkspaceID: wsID,
			Name:        "after",
			CreatedAt:   now,
			UpdatedAt:   later,
		})
		gt.NoError(t, err).Required()
		gt.Value(t, updated.ID).Equal(original.ID)
		gt.String(t, updated.Name).Equal("after")
		gt.Bool(t, updated.UpdatedAt.Equal(later)).True()
		gt.Bool(t, updated.CreatedAt.Equal(now)).True()

		got, err := repo.Tag().Get(ctx, wsID, original.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Name).Equal("after")
		gt.Value(t, got.ID).Equal(original.ID)
		gt.Bool(t, got.UpdatedAt.Equal(later)).True()
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
	})

	t.Run("Delete then Get returns not found", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		now := time.Now().UTC()

		id := model.NewTagID()
		_, err := repo.Tag().Create(ctx, wsID, &model.Tag{
			ID:          id,
			WorkspaceID: wsID,
			Name:        "to-delete",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		gt.NoError(t, err).Required()

		gt.NoError(t, repo.Tag().Delete(ctx, wsID, id)).Required()

		_, err = repo.Tag().Get(ctx, wsID, id)
		gt.Bool(t, isNotFound(err)).True()

		// Deleting again returns not found.
		err = repo.Tag().Delete(ctx, wsID, id)
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("Get missing returns not found", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		_, err := repo.Tag().Get(ctx, wsID, model.NewTagID())
		gt.Bool(t, isNotFound(err)).True()
	})
}

func TestTagRepository_Memory(t *testing.T) {
	t.Parallel()
	runTagRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestTagRepository_Firestore(t *testing.T) {
	t.Parallel()
	runTagRepositoryTest(t, newFirestoreRepository)
}
