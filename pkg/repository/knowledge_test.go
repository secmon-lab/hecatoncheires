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

func runKnowledgeRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	const wsID = "test-ws"

	t.Run("Create creates knowledge with UUID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		sourcedAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

		knowledge := &model.Knowledge{
			CaseID:     123,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/12345"},
			Title:      "Security patch update",
			Summary:    "A new security patch was released for CVE-2024-1234",
			Embedding:  []float32{0.1, 0.2, 0.3},
			SourcedAt:  sourcedAt,
		}

		created, err := repo.Knowledge().Create(ctx, wsID, knowledge)
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.CaseID).Equal(knowledge.CaseID)
		gt.Value(t, created.SourceID).Equal(knowledge.SourceID)
		gt.Array(t, created.SourceURLs).Length(1)
		gt.Value(t, created.Title).Equal(knowledge.Title)
		gt.Value(t, created.Summary).Equal(knowledge.Summary)
		gt.Array(t, created.Embedding).Length(3)
		gt.Value(t, created.Embedding[0]).Equal(float32(0.1))
		gt.Value(t, created.Embedding[1]).Equal(float32(0.2))
		gt.Value(t, created.Embedding[2]).Equal(float32(0.3))
		gt.Bool(t, created.CreatedAt.IsZero()).False()
		gt.Bool(t, created.UpdatedAt.IsZero()).False()
	})

	t.Run("Create with provided ID preserves it", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		customID := model.KnowledgeID(fmt.Sprintf("custom-id-%d", time.Now().UnixNano()))
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		knowledge := &model.Knowledge{
			ID:         customID,
			CaseID:     456,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/custom"},
			Title:      "Custom ID Knowledge",
			Summary:    "Knowledge with custom ID",
		}

		created, err := repo.Knowledge().Create(ctx, wsID, knowledge)
		gt.NoError(t, err).Required()

		gt.Value(t, created.ID).Equal(customID)
	})

	t.Run("Get retrieves existing knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		sourcedAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

		knowledge := &model.Knowledge{
			CaseID:     789,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/get-test"},
			Title:      "Test Knowledge",
			Summary:    "For testing Get",
			Embedding:  []float32{0.5, 0.6, 0.7, 0.8},
			SourcedAt:  sourcedAt,
		}

		created, err := repo.Knowledge().Create(ctx, wsID, knowledge)
		gt.NoError(t, err).Required()

		retrieved, err := repo.Knowledge().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.CaseID).Equal(created.CaseID)
		gt.Value(t, retrieved.SourceID).Equal(created.SourceID)
		gt.Array(t, retrieved.SourceURLs).Length(1)
		gt.Value(t, retrieved.Title).Equal(created.Title)
		gt.Value(t, retrieved.Summary).Equal(created.Summary)
		gt.Array(t, retrieved.Embedding).Length(4)
		gt.Bool(t, time.Since(retrieved.CreatedAt) <= 3*time.Second).True()
	})

	t.Run("Get returns error for non-existent knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Knowledge().Get(ctx, wsID, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("ListByCaseID returns knowledge for specific risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		riskID := time.Now().UnixNano()
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Create knowledge entries for the same risk
		k1, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/1"},
			Title:      "Knowledge 1",
			Summary:    "First knowledge",
			SourcedAt:  time.Now().Add(-1 * time.Hour).UTC(),
		})
		gt.NoError(t, err).Required()

		k2, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/2"},
			Title:      "Knowledge 2",
			Summary:    "Second knowledge",
			SourcedAt:  time.Now().Add(-2 * time.Hour).UTC(),
		})
		gt.NoError(t, err).Required()

		// Create knowledge for a different risk
		_, err = repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID + 1,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/3"},
			Title:      "Knowledge 3",
			Summary:    "Different risk knowledge",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		knowledges, err := repo.Knowledge().ListByCaseID(ctx, wsID, riskID)
		gt.NoError(t, err).Required()

		gt.Array(t, knowledges).Length(2)

		foundK1 := false
		foundK2 := false
		for _, k := range knowledges {
			if k.ID == k1.ID {
				foundK1 = true
			}
			if k.ID == k2.ID {
				foundK2 = true
			}
		}
		gt.Bool(t, foundK1).True()
		gt.Bool(t, foundK2).True()
	})

	t.Run("ListByCaseID returns empty for non-existent risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		knowledges, err := repo.Knowledge().ListByCaseID(ctx, wsID, 999999999)
		gt.NoError(t, err).Required()

		gt.Array(t, knowledges).Length(0)
	})

	t.Run("ListByCaseIDs returns knowledge for multiple risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		now := time.Now().UnixNano()
		riskID1 := now
		riskID2 := now + 1
		riskID3 := now + 2
		sourceID := model.SourceID(fmt.Sprintf("source-%d", now))

		// Create knowledge entries for risk1
		k1, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID1,
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/1"},
			Title:      "Knowledge 1 for Risk 1",
			Summary:    "First knowledge for risk 1",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		k2, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID1,
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/2"},
			Title:      "Knowledge 2 for Risk 1",
			Summary:    "Second knowledge for risk 1",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Create knowledge entries for risk2
		k3, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID2,
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/3"},
			Title:      "Knowledge 1 for Risk 2",
			Summary:    "First knowledge for risk 2",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Create knowledge for risk3 (not requested)
		_, err = repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID3,
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/4"},
			Title:      "Knowledge for Risk 3",
			Summary:    "Not requested",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Request knowledges for risk1 and risk2
		result, err := repo.Knowledge().ListByCaseIDs(ctx, wsID, []int64{riskID1, riskID2})
		gt.NoError(t, err).Required()

		gt.Value(t, len(result)).Equal(2)

		risk1Knowledges, ok := result[riskID1]
		gt.Bool(t, ok).True().Required()
		gt.Array(t, risk1Knowledges).Length(2)

		risk2Knowledges, ok := result[riskID2]
		gt.Bool(t, ok).True().Required()
		gt.Array(t, risk2Knowledges).Length(1)

		// Verify specific knowledge IDs
		foundK1, foundK2, foundK3 := false, false, false
		for _, k := range risk1Knowledges {
			if k.ID == k1.ID {
				foundK1 = true
			}
			if k.ID == k2.ID {
				foundK2 = true
			}
		}
		for _, k := range risk2Knowledges {
			if k.ID == k3.ID {
				foundK3 = true
			}
		}

		gt.Bool(t, foundK1).True()
		gt.Bool(t, foundK2).True()
		gt.Bool(t, foundK3).True()
	})

	t.Run("ListByCaseIDs returns empty map for empty input", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		result, err := repo.Knowledge().ListByCaseIDs(ctx, wsID, []int64{})
		gt.NoError(t, err).Required()

		gt.Value(t, len(result)).Equal(0)
	})

	t.Run("ListByCaseIDs returns empty slices for non-existent risks", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		now := time.Now().UnixNano()
		nonExistentID1 := now + 9999
		nonExistentID2 := now + 99999

		result, err := repo.Knowledge().ListByCaseIDs(ctx, wsID, []int64{nonExistentID1, nonExistentID2})
		gt.NoError(t, err).Required()

		gt.Value(t, len(result)).Equal(2)

		if knowledges, ok := result[nonExistentID1]; !ok {
			gt.Bool(t, ok).True()
		} else {
			gt.Array(t, knowledges).Length(0)
		}

		if knowledges, ok := result[nonExistentID2]; !ok {
			gt.Bool(t, ok).True()
		} else {
			gt.Array(t, knowledges).Length(0)
		}
	})

	t.Run("ListBySourceID returns knowledge for specific source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		otherSourceID := model.SourceID(fmt.Sprintf("other-source-%d", time.Now().UnixNano()))
		riskID := time.Now().UnixNano()

		// Create knowledge entries for the same source
		k1, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/a"},
			Title:      "Knowledge A",
			Summary:    "First knowledge for source",
			SourcedAt:  time.Now().Add(-1 * time.Hour).UTC(),
		})
		gt.NoError(t, err).Required()

		k2, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID + 1,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/b"},
			Title:      "Knowledge B",
			Summary:    "Second knowledge for source",
			SourcedAt:  time.Now().Add(-2 * time.Hour).UTC(),
		})
		gt.NoError(t, err).Required()

		// Create knowledge for a different source
		_, err = repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     riskID,
			SourceID:   otherSourceID,
			SourceURLs: []string{"https://www.notion.so/page/c"},
			Title:      "Knowledge C",
			Summary:    "Different source knowledge",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		knowledges, err := repo.Knowledge().ListBySourceID(ctx, wsID, sourceID)
		gt.NoError(t, err).Required()

		gt.Array(t, knowledges).Length(2)

		foundK1 := false
		foundK2 := false
		for _, k := range knowledges {
			if k.ID == k1.ID {
				foundK1 = true
			}
			if k.ID == k2.ID {
				foundK2 = true
			}
		}
		gt.Bool(t, foundK1).True()
		gt.Bool(t, foundK2).True()
	})

	t.Run("ListBySourceID returns empty for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		knowledges, err := repo.Knowledge().ListBySourceID(ctx, wsID, "non-existent-source")
		gt.NoError(t, err).Required()

		gt.Array(t, knowledges).Length(0)
	})

	t.Run("Delete removes existing knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		created, err := repo.Knowledge().Create(ctx, wsID, &model.Knowledge{
			CaseID:     111,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/delete"},
			Title:      "To Be Deleted",
			Summary:    "This will be deleted",
		})
		gt.NoError(t, err).Required()

		err = repo.Knowledge().Delete(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		_, err = repo.Knowledge().Get(ctx, wsID, created.ID)
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Delete returns error for non-existent knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Knowledge().Delete(ctx, wsID, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Knowledge without Embedding works", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		knowledge := &model.Knowledge{
			CaseID:     222,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/no-embedding"},
			Title:      "No Embedding",
			Summary:    "Knowledge without embedding",
			Embedding:  nil,
		}

		created, err := repo.Knowledge().Create(ctx, wsID, knowledge)
		gt.NoError(t, err).Required()

		gt.Value(t, len(created.Embedding)).Equal(0)

		retrieved, err := repo.Knowledge().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, len(retrieved.Embedding)).Equal(0)
	})

	t.Run("FindByEmbedding returns similar knowledge entries", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Use unique wsID to isolate from other subtests' embedding data
		embWsID := fmt.Sprintf("test-ws-emb-similar-%d", time.Now().UnixNano())
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Use EmbeddingDimension (768) to match the Firestore vector index
		dim := model.EmbeddingDimension

		// Create embedding close to [1, 0, 0, ..., 0]
		similarEmb := make([]float32, dim)
		similarEmb[0] = 0.9
		similarEmb[1] = 0.1

		_, err := repo.Knowledge().Create(ctx, embWsID, &model.Knowledge{
			CaseID:     time.Now().UnixNano(),
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/similar"},
			Title:      "Similar Knowledge",
			Summary:    "This is similar",
			Embedding:  similarEmb,
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Create embedding close to [0, 1, 0, ..., 0] (dissimilar)
		dissimilarEmb := make([]float32, dim)
		dissimilarEmb[1] = 0.9
		dissimilarEmb[2] = 0.1

		_, err = repo.Knowledge().Create(ctx, embWsID, &model.Knowledge{
			CaseID:     time.Now().UnixNano(),
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/dissimilar"},
			Title:      "Dissimilar Knowledge",
			Summary:    "This is dissimilar",
			Embedding:  dissimilarEmb,
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Create embedding very close to [1, 0, 0, ..., 0]
		mostSimilarEmb := make([]float32, dim)
		mostSimilarEmb[0] = 1.0

		_, err = repo.Knowledge().Create(ctx, embWsID, &model.Knowledge{
			CaseID:     time.Now().UnixNano(),
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/most-similar"},
			Title:      "Most Similar Knowledge",
			Summary:    "This is the most similar",
			Embedding:  mostSimilarEmb,
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Search for [1, 0, 0, ..., 0] with limit 2
		queryEmb := make([]float32, dim)
		queryEmb[0] = 1.0
		results, err := repo.Knowledge().FindByEmbedding(ctx, embWsID, queryEmb, 2)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(2)
		// Most similar should be first
		gt.Value(t, results[0].Title).Equal("Most Similar Knowledge")
		gt.Value(t, results[1].Title).Equal("Similar Knowledge")
	})

	t.Run("FindByEmbedding returns empty for no embeddings", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Use unique wsID to isolate from other subtests' embedding data
		embWsID := fmt.Sprintf("test-ws-emb-empty-%d", time.Now().UnixNano())
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Create knowledge without embedding
		_, err := repo.Knowledge().Create(ctx, embWsID, &model.Knowledge{
			CaseID:     time.Now().UnixNano(),
			SourceID:   sourceID,
			SourceURLs: []string{"https://example.com/no-embed"},
			Title:      "No Embedding",
			Summary:    "No embedding here",
			SourcedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		queryEmb := make([]float32, model.EmbeddingDimension)
		queryEmb[0] = 1.0
		results, err := repo.Knowledge().FindByEmbedding(ctx, embWsID, queryEmb, 5)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(0)
	})

	t.Run("FindByEmbedding respects limit", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Use unique wsID to isolate from other subtests' embedding data
		embWsID := fmt.Sprintf("test-ws-emb-limit-%d", time.Now().UnixNano())
		dim := model.EmbeddingDimension
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Create 5 knowledge entries with embeddings
		for i := 0; i < 5; i++ {
			emb := make([]float32, dim)
			emb[0] = float32(i) * 0.1
			emb[1] = 0.5
			emb[2] = 0.5

			_, err := repo.Knowledge().Create(ctx, embWsID, &model.Knowledge{
				CaseID:     time.Now().UnixNano() + int64(i),
				SourceID:   sourceID,
				SourceURLs: []string{fmt.Sprintf("https://example.com/embed-%d", i)},
				Title:      fmt.Sprintf("Knowledge %d", i),
				Summary:    fmt.Sprintf("Knowledge entry %d", i),
				Embedding:  emb,
				SourcedAt:  time.Now().UTC(),
			})
			gt.NoError(t, err).Required()
		}

		queryEmb := make([]float32, dim)
		queryEmb[0] = 0.4
		queryEmb[1] = 0.5
		queryEmb[2] = 0.5
		results, err := repo.Knowledge().FindByEmbedding(ctx, embWsID, queryEmb, 3)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(3)
	})

	t.Run("FindByEmbedding returns empty for empty workspace", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		queryEmb := make([]float32, model.EmbeddingDimension)
		queryEmb[0] = 1.0
		results, err := repo.Knowledge().FindByEmbedding(ctx, "non-existent-ws", queryEmb, 5)
		gt.NoError(t, err).Required()

		gt.Array(t, results).Length(0)
	})

	t.Run("Large Embedding vector is preserved", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Create a 768-dimension embedding (Gemini text-embedding-004 dimension)
		embedding := make([]float32, model.EmbeddingDimension)
		for i := range embedding {
			embedding[i] = float32(i) / float32(model.EmbeddingDimension)
		}

		knowledge := &model.Knowledge{
			CaseID:     333,
			SourceID:   sourceID,
			SourceURLs: []string{"https://www.notion.so/page/large-embedding"},
			Title:      "Large Embedding",
			Summary:    "Knowledge with 768-dimension embedding",
			Embedding:  embedding,
		}

		created, err := repo.Knowledge().Create(ctx, wsID, knowledge)
		gt.NoError(t, err).Required()

		gt.Array(t, created.Embedding).Length(model.EmbeddingDimension)

		retrieved, err := repo.Knowledge().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, retrieved.Embedding).Length(model.EmbeddingDimension)

		// Verify first and last values
		gt.Value(t, retrieved.Embedding[0]).Equal(float32(0))
		expectedLast := float32(model.EmbeddingDimension-1) / float32(model.EmbeddingDimension)
		gt.Value(t, retrieved.Embedding[model.EmbeddingDimension-1]).Equal(expectedLast)
	})
}

func newFirestoreKnowledgeRepository(t *testing.T) interfaces.Repository {
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

func TestMemoryKnowledgeRepository(t *testing.T) {
	runKnowledgeRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreKnowledgeRepository(t *testing.T) {
	runKnowledgeRepositoryTest(t, newFirestoreKnowledgeRepository)
}
