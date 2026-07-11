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

func runKnowledgeRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	isNotFound := func(err error) bool {
		return errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
	}

	// Construct the repository once and isolate sub-tests with a unique
	// workspace ID each, rather than opening a fresh (Firestore) client per
	// sub-test — the client/TLS setup dominates the cost against real Firestore.
	repo := newRepo(t)

	t.Run("Create and Get round-trips all fields", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		now := time.Now().UTC().Truncate(time.Millisecond)
		embedding := make([]float64, model.EmbeddingDimension)
		for i := range embedding {
			embedding[i] = float64(i) * 0.001
		}
		tagID1 := model.NewTagID()
		tagID2 := model.NewTagID()
		tagID3 := model.NewTagID()

		input := &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: wsID,
			Title:       "Round-trip knowledge",
			Claim:       "## heading\n\n- bullet one\n- bullet two",
			TagIDs:      []model.TagID{tagID1, tagID2, tagID3},
			Embedding:   embedding,
			CreatorID:   "U-CREATOR",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		created, err := repo.Knowledge().Create(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.ID).Equal(input.ID)

		got, err := repo.Knowledge().Get(ctx, wsID, input.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, got.ID).Equal(input.ID)
		gt.Value(t, got.WorkspaceID).Equal(wsID)
		gt.String(t, got.Title).Equal("Round-trip knowledge")
		gt.String(t, got.Claim).Equal("## heading\n\n- bullet one\n- bullet two")
		gt.String(t, got.CreatorID).Equal("U-CREATOR")
		gt.Array(t, got.TagIDs).Length(3).Required()
		gt.Value(t, got.TagIDs[0]).Equal(tagID1)
		gt.Value(t, got.TagIDs[1]).Equal(tagID2)
		gt.Value(t, got.TagIDs[2]).Equal(tagID3)
		gt.Array(t, got.Embedding).Length(model.EmbeddingDimension).Required()
		gt.Value(t, got.Embedding[1]).Equal(0.001)
		gt.Value(t, got.Embedding[10]).Equal(0.010)
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
		gt.Bool(t, got.UpdatedAt.Equal(now)).True()
	})

	t.Run("Get not found", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		_, err := repo.Knowledge().Get(ctx, wsID, model.NewKnowledgeID())
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("List filters by tag AND and sorts by CreatedAt", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		base := time.Now().UTC()

		tagOps := model.NewTagID()
		tagGitHub := model.NewTagID()
		tagSecurity := model.NewTagID()

		mk := func(title string, tagIDs []model.TagID, createdOffset time.Duration) model.KnowledgeID {
			id := model.NewKnowledgeID()
			_, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
				ID:          id,
				WorkspaceID: wsID,
				Title:       title,
				Claim:       "body",
				TagIDs:      tagIDs,
				CreatedAt:   base.Add(createdOffset),
				UpdatedAt:   base.Add(createdOffset),
			})
			gt.NoError(t, err).Required()
			return id
		}

		idA := mk("A", []model.TagID{tagOps, tagGitHub}, 0)
		idB := mk("B", []model.TagID{tagOps}, time.Second)
		mk("C", []model.TagID{tagSecurity}, 2*time.Second)

		// No filter: all three, sorted ascending by CreatedAt (A, B, C).
		all, err := repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(3).Required()
		gt.String(t, all[0].Title).Equal("A")
		gt.String(t, all[1].Title).Equal("B")
		gt.String(t, all[2].Title).Equal("C")

		// Single tag "ops": A and B.
		ops, err := repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{TagIDs: []model.TagID{tagOps}})
		gt.NoError(t, err).Required()
		gt.Array(t, ops).Length(2).Required()
		gt.Value(t, ops[0].ID).Equal(idA)
		gt.Value(t, ops[1].ID).Equal(idB)

		// AND filter ops+github: only A.
		both, err := repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{TagIDs: []model.TagID{tagOps, tagGitHub}})
		gt.NoError(t, err).Required()
		gt.Array(t, both).Length(1).Required()
		gt.Value(t, both[0].ID).Equal(idA)

		// Non-existent tag: empty.
		none, err := repo.Knowledge().List(ctx, wsID, interfaces.KnowledgeListOptions{TagIDs: []model.TagID{model.NewTagID()}})
		gt.NoError(t, err).Required()
		gt.Array(t, none).Length(0)
	})

	t.Run("Update persists changes", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		tagOps := model.NewTagID()
		tagUpdated := model.NewTagID()

		id := model.NewKnowledgeID()
		_, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			ID:          id,
			WorkspaceID: wsID,
			Title:       "before",
			Claim:       "before body",
			TagIDs:      []model.TagID{tagOps},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		gt.NoError(t, err).Required()

		later := now.Add(time.Minute)
		_, err = repo.Knowledge().Update(ctx, wsID, &model.Knowledge{
			ID:          id,
			WorkspaceID: wsID,
			Title:       "after",
			Claim:       "after body",
			TagIDs:      []model.TagID{tagOps, tagUpdated},
			CreatedAt:   now,
			UpdatedAt:   later,
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, wsID, id)
		gt.NoError(t, err).Required()
		gt.String(t, got.Title).Equal("after")
		gt.String(t, got.Claim).Equal("after body")
		gt.Array(t, got.TagIDs).Length(2).Required()
		gt.Value(t, got.TagIDs[0]).Equal(tagOps)
		gt.Value(t, got.TagIDs[1]).Equal(tagUpdated)
		gt.Bool(t, got.UpdatedAt.Equal(later)).True()
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
	})

	t.Run("Update missing returns not found", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		now := time.Now().UTC()

		_, err := repo.Knowledge().Update(ctx, wsID, &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: wsID,
			Title:       "x",
			TagIDs:      []model.TagID{model.NewTagID()},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		gt.Bool(t, isNotFound(err)).True()
	})

	t.Run("Delete removes the entry", func(t *testing.T) {
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		now := time.Now().UTC()

		id := model.NewKnowledgeID()
		_, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			ID:          id,
			WorkspaceID: wsID,
			Title:       "to delete",
			TagIDs:      []model.TagID{model.NewTagID()},
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		gt.NoError(t, err).Required()

		gt.NoError(t, repo.Knowledge().Delete(ctx, wsID, id)).Required()

		_, err = repo.Knowledge().Get(ctx, wsID, id)
		gt.Bool(t, isNotFound(err)).True()

		// Deleting again returns not found.
		err = repo.Knowledge().Delete(ctx, wsID, id)
		gt.Bool(t, isNotFound(err)).True()
	})
}

func TestKnowledgeRepository_Memory(t *testing.T) {
	t.Parallel()
	runKnowledgeRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestKnowledgeRepository_Firestore(t *testing.T) {
	t.Parallel()
	runKnowledgeRepositoryTest(t, newFirestoreRepository)
}
