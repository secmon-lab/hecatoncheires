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

func runAssigneeRankingRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	isNotFound := func(err error) bool {
		return errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
	}

	repo := newRepo(t)

	t.Run("Set and Get round-trips all fields", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		computedAt := time.Now().UTC().Truncate(time.Millisecond)

		input := &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{"U-first", "U-second", "U-third"},
			ComputedAt:  computedAt,
		}
		gt.NoError(t, repo.AssigneeRanking().Set(ctx, input)).Required()

		got, err := repo.AssigneeRanking().Get(ctx, workspaceID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.WorkspaceID).Equal(workspaceID)
		gt.Array(t, got.UserIDs).Length(3).Required()
		// Order is the whole point of this document — assert element by element.
		gt.Value(t, got.UserIDs[0]).Equal("U-first")
		gt.Value(t, got.UserIDs[1]).Equal("U-second")
		gt.Value(t, got.UserIDs[2]).Equal("U-third")
		gt.Bool(t, got.ComputedAt.Equal(computedAt)).True()
	})

	t.Run("Get returns not found for an unknown workspace", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		_, err := repo.AssigneeRanking().Get(ctx, workspaceID)
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("Set replaces the whole ranking", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		first := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{"U-a", "U-b", "U-c"},
			ComputedAt:  first,
		})).Required()

		// Two instances may recompute concurrently; the later write must win
		// outright rather than merging into the previous list.
		second := first.Add(time.Hour)
		gt.NoError(t, repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{"U-z"},
			ComputedAt:  second,
		})).Required()

		got, err := repo.AssigneeRanking().Get(ctx, workspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got.UserIDs).Length(1).Required()
		gt.Value(t, got.UserIDs[0]).Equal("U-z")
		gt.Bool(t, got.ComputedAt.Equal(second)).True()
	})

	t.Run("Set stores an empty ranking", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		computedAt := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{},
			ComputedAt:  computedAt,
		})).Required()

		got, err := repo.AssigneeRanking().Get(ctx, workspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got.UserIDs).Length(0)
		// ComputedAt must survive an empty ranking, or the caller would keep
		// recomputing a workspace that genuinely has no assignees.
		gt.Bool(t, got.ComputedAt.Equal(computedAt)).True()
	})

	t.Run("Set rejects an empty workspace ID", func(t *testing.T) {
		ctx := context.Background()
		err := repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{UserIDs: []string{"U-a"}})
		gt.Error(t, err).Is(model.ErrAssigneeRankingValidation)
	})

	t.Run("Set rejects an empty user ID element", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		err := repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{"U-a", ""},
			ComputedAt:  time.Now(),
		})
		gt.Error(t, err).Is(model.ErrAssigneeRankingValidation)
	})

	t.Run("mutating the input after Set does not alter stored state", func(t *testing.T) {
		ctx := context.Background()
		workspaceID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		input := &model.AssigneeRanking{
			WorkspaceID: workspaceID,
			UserIDs:     []string{"U-a", "U-b"},
			ComputedAt:  time.Now().UTC().Truncate(time.Millisecond),
		}
		gt.NoError(t, repo.AssigneeRanking().Set(ctx, input)).Required()
		input.UserIDs[0] = "U-tampered"

		got, err := repo.AssigneeRanking().Get(ctx, workspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got.UserIDs).Length(2).Required()
		gt.Value(t, got.UserIDs[0]).Equal("U-a")
	})
}

func TestAssigneeRankingRepository_Memory(t *testing.T) {
	t.Parallel()
	runAssigneeRankingRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestAssigneeRankingRepository_Firestore(t *testing.T) {
	t.Parallel()
	runAssigneeRankingRepositoryTest(t, newFirestoreRepository)
}
