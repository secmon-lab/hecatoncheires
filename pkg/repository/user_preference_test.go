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

func runUserPreferenceRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	isNotFound := func(err error) bool {
		return errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
	}

	repo := newRepo(t)

	t.Run("Set and Get round-trips all fields", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		input := &model.UserPreference{
			UserID:               userID,
			FavoriteWorkspaceIDs: []string{"ws-a", "ws-b", "ws-c"},
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		gt.NoError(t, repo.UserPreference().Set(ctx, input)).Required()

		got, err := repo.UserPreference().Get(ctx, userID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.UserID).Equal(userID)
		gt.Array(t, got.FavoriteWorkspaceIDs).Length(3).Required()
		gt.Value(t, got.FavoriteWorkspaceIDs[0]).Equal("ws-a")
		gt.Value(t, got.FavoriteWorkspaceIDs[1]).Equal("ws-b")
		gt.Value(t, got.FavoriteWorkspaceIDs[2]).Equal("ws-c")
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
		gt.Bool(t, got.UpdatedAt.Equal(now)).True()
	})

	t.Run("Get not found", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())

		_, err := repo.UserPreference().Get(ctx, userID)
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("Set overwrites the whole list", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.UserPreference().Set(ctx, &model.UserPreference{
			UserID:               userID,
			FavoriteWorkspaceIDs: []string{"ws-a", "ws-b"},
			CreatedAt:            now,
			UpdatedAt:            now,
		})).Required()

		later := now.Add(time.Minute)
		gt.NoError(t, repo.UserPreference().Set(ctx, &model.UserPreference{
			UserID:               userID,
			FavoriteWorkspaceIDs: []string{"ws-z"},
			CreatedAt:            now,
			UpdatedAt:            later,
		})).Required()

		got, err := repo.UserPreference().Get(ctx, userID)
		gt.NoError(t, err).Required()
		gt.Array(t, got.FavoriteWorkspaceIDs).Length(1).Required()
		gt.Value(t, got.FavoriteWorkspaceIDs[0]).Equal("ws-z")
		gt.Bool(t, got.UpdatedAt.Equal(later)).True()
	})

	t.Run("Set with empty favorites", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.UserPreference().Set(ctx, &model.UserPreference{
			UserID:               userID,
			FavoriteWorkspaceIDs: []string{},
			CreatedAt:            now,
			UpdatedAt:            now,
		})).Required()

		got, err := repo.UserPreference().Get(ctx, userID)
		gt.NoError(t, err).Required()
		gt.Array(t, got.FavoriteWorkspaceIDs).Length(0)
	})

	t.Run("Set rejects empty user ID", func(t *testing.T) {
		ctx := context.Background()
		err := repo.UserPreference().Set(ctx, &model.UserPreference{UserID: ""})
		gt.Error(t, err).Is(model.ErrUserPreferenceValidation)
	})
}

func TestUserPreferenceRepository_Memory(t *testing.T) {
	t.Parallel()
	runUserPreferenceRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestUserPreferenceRepository_Firestore(t *testing.T) {
	t.Parallel()
	runUserPreferenceRepositoryTest(t, newFirestoreRepository)
}
