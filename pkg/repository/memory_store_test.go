package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runMemoryRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	const wsID = "test-ws"

	t.Run("Create creates memory with UUID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		mem := &model.Memory{
			CaseID:    caseID,
			Claim:     "The deployment deadline is March 15",
			Embedding: []float32{0.1, 0.2, 0.3},
		}

		created, err := repo.Memory().Create(ctx, wsID, caseID, mem)
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.CaseID).Equal(caseID)
		gt.Value(t, created.Claim).Equal("The deployment deadline is March 15")
		gt.Array(t, created.Embedding).Length(3)
		gt.Value(t, created.Embedding[0]).Equal(float32(0.1))
		gt.Value(t, created.Embedding[1]).Equal(float32(0.2))
		gt.Value(t, created.Embedding[2]).Equal(float32(0.3))
		gt.Bool(t, created.CreatedAt.IsZero()).False()
	})

	t.Run("Get retrieves existing memory", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		mem := &model.Memory{
			CaseID:    caseID,
			Claim:     "User reported intermittent timeouts",
			Embedding: []float32{0.5, 0.6, 0.7, 0.8},
		}

		created, err := repo.Memory().Create(ctx, wsID, caseID, mem)
		gt.NoError(t, err).Required()

		retrieved, err := repo.Memory().Get(ctx, wsID, caseID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.CaseID).Equal(caseID)
		gt.Value(t, retrieved.Claim).Equal("User reported intermittent timeouts")
		gt.Array(t, retrieved.Embedding).Length(4)
		gt.Bool(t, time.Since(retrieved.CreatedAt) < 3*time.Second).True()
	})

	t.Run("Get returns error for non-existent memory", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		_, err := repo.Memory().Get(ctx, wsID, caseID, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Delete removes existing memory", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		created, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim:     "Temporary observation",
			Embedding: []float32{0.1, 0.2},
		})
		gt.NoError(t, err).Required()

		err = repo.Memory().Delete(ctx, wsID, caseID, created.ID)
		gt.NoError(t, err).Required()

		_, err = repo.Memory().Get(ctx, wsID, caseID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Delete returns error for non-existent memory", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		err := repo.Memory().Delete(ctx, wsID, caseID, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("List returns all memories for a case sorted by CreatedAt desc", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		m1, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim: "First observation",
		})
		gt.NoError(t, err).Required()

		time.Sleep(10 * time.Millisecond)

		m2, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim: "Second observation",
		})
		gt.NoError(t, err).Required()

		// Create memory for a different case
		otherCaseID := caseID + 1
		_, err = repo.Memory().Create(ctx, wsID, otherCaseID, &model.Memory{
			Claim: "Other case memory",
		})
		gt.NoError(t, err).Required()

		memories, err := repo.Memory().List(ctx, wsID, caseID)
		gt.NoError(t, err).Required()

		gt.Array(t, memories).Length(2)
		// Newest first
		gt.Value(t, memories[0].ID).Equal(m2.ID)
		gt.Value(t, memories[0].Claim).Equal("Second observation")
		gt.Value(t, memories[1].ID).Equal(m1.ID)
		gt.Value(t, memories[1].Claim).Equal("First observation")
	})

	t.Run("List returns empty for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		memories, err := repo.Memory().List(ctx, wsID, 999999999)
		gt.NoError(t, err).Required()
		gt.Array(t, memories).Length(0)
	})

	t.Run("FindByEmbedding returns similar memories", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		dim := model.EmbeddingDimension

		// Create embedding close to [1, 0, 0, ..., 0]
		similarEmb := make([]float32, dim)
		similarEmb[0] = 0.9
		similarEmb[1] = 0.1

		_, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim:     "Similar memory",
			Embedding: similarEmb,
		})
		gt.NoError(t, err).Required()

		// Create dissimilar embedding
		dissimilarEmb := make([]float32, dim)
		dissimilarEmb[1] = 0.9
		dissimilarEmb[2] = 0.1

		_, err = repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim:     "Dissimilar memory",
			Embedding: dissimilarEmb,
		})
		gt.NoError(t, err).Required()

		// Create most similar embedding
		mostSimilarEmb := make([]float32, dim)
		mostSimilarEmb[0] = 1.0

		_, err = repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim:     "Most similar memory",
			Embedding: mostSimilarEmb,
		})
		gt.NoError(t, err).Required()

		queryEmb := make([]float32, dim)
		queryEmb[0] = 1.0
		results, err := repo.Memory().FindByEmbedding(ctx, wsID, caseID, queryEmb, 2)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(2)
		gt.Value(t, results[0].Claim).Equal("Most similar memory")
		gt.Value(t, results[1].Claim).Equal("Similar memory")
	})

	t.Run("FindByEmbedding returns empty when no embeddings", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		// Create memory without embedding
		_, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim: "No embedding memory",
		})
		gt.NoError(t, err).Required()

		queryEmb := make([]float32, model.EmbeddingDimension)
		queryEmb[0] = 1.0
		results, err := repo.Memory().FindByEmbedding(ctx, wsID, caseID, queryEmb, 5)
		gt.NoError(t, err).Required()
		gt.Array(t, results).Length(0)
	})

	t.Run("FindByEmbedding respects limit", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		dim := model.EmbeddingDimension

		for i := 0; i < 5; i++ {
			emb := make([]float32, dim)
			emb[0] = float32(i) * 0.1
			emb[1] = 0.5

			_, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
				Claim:     fmt.Sprintf("Memory %d", i),
				Embedding: emb,
			})
			gt.NoError(t, err).Required()
		}

		queryEmb := make([]float32, dim)
		queryEmb[0] = 0.4
		queryEmb[1] = 0.5
		results, err := repo.Memory().FindByEmbedding(ctx, wsID, caseID, queryEmb, 3)
		gt.NoError(t, err).Required()
		gt.Array(t, results).Length(3)
	})

	t.Run("FindByEmbedding returns empty for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		queryEmb := make([]float32, model.EmbeddingDimension)
		queryEmb[0] = 1.0
		results, err := repo.Memory().FindByEmbedding(ctx, wsID, 999999999, queryEmb, 5)
		gt.NoError(t, err).Required()
		gt.Array(t, results).Length(0)
	})

	t.Run("Large embedding vector is preserved", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		embedding := make([]float32, model.EmbeddingDimension)
		for i := range embedding {
			embedding[i] = float32(i) / float32(model.EmbeddingDimension)
		}

		created, err := repo.Memory().Create(ctx, wsID, caseID, &model.Memory{
			Claim:     "Large embedding memory",
			Embedding: embedding,
		})
		gt.NoError(t, err).Required()
		gt.Array(t, created.Embedding).Length(model.EmbeddingDimension)

		retrieved, err := repo.Memory().Get(ctx, wsID, caseID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, retrieved.Embedding).Length(model.EmbeddingDimension)
		gt.Value(t, retrieved.Embedding[0]).Equal(float32(0))
		expectedLast := float32(model.EmbeddingDimension-1) / float32(model.EmbeddingDimension)
		gt.Value(t, retrieved.Embedding[model.EmbeddingDimension-1]).Equal(expectedLast)
	})
}

func newFirestoreMemoryRepository(t *testing.T) interfaces.Repository {
	t.Helper()

	projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("TEST_FIRESTORE_PROJECT_ID not set")
	}

	databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
	if databaseID == "" {
		t.Skip("TEST_FIRESTORE_DATABASE_ID not set")
	}

	ctx := context.Background()
	repo, err := firestore.New(ctx, projectID, databaseID)
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, repo.Close())
	})
	return repo
}

func TestMemoryMemoryRepository(t *testing.T) {
	runMemoryRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreMemoryRepository(t *testing.T) {
	runMemoryRepositoryTest(t, newFirestoreMemoryRepository)
}
