package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runKnowledgeRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create creates knowledge with UUID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		sourcedAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

		knowledge := &model.Knowledge{
			RiskID:    123,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/12345",
			Title:     "Security patch update",
			Summary:   "A new security patch was released for CVE-2024-1234",
			Embedding: []float32{0.1, 0.2, 0.3},
			SourcedAt: sourcedAt,
		}

		created, err := repo.Knowledge().Create(ctx, knowledge)
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		if created.ID == "" {
			t.Error("expected non-empty ID")
		}
		if created.RiskID != knowledge.RiskID {
			t.Errorf("expected RiskID=%d, got %d", knowledge.RiskID, created.RiskID)
		}
		if created.SourceID != knowledge.SourceID {
			t.Errorf("expected SourceID=%s, got %s", knowledge.SourceID, created.SourceID)
		}
		if created.SourceURL != knowledge.SourceURL {
			t.Errorf("expected SourceURL=%s, got %s", knowledge.SourceURL, created.SourceURL)
		}
		if created.Title != knowledge.Title {
			t.Errorf("expected Title=%s, got %s", knowledge.Title, created.Title)
		}
		if created.Summary != knowledge.Summary {
			t.Errorf("expected Summary=%s, got %s", knowledge.Summary, created.Summary)
		}
		if len(created.Embedding) != 3 {
			t.Errorf("expected Embedding length=3, got %d", len(created.Embedding))
		}
		if created.Embedding[0] != 0.1 || created.Embedding[1] != 0.2 || created.Embedding[2] != 0.3 {
			t.Errorf("expected Embedding=[0.1, 0.2, 0.3], got %v", created.Embedding)
		}
		if created.CreatedAt.IsZero() {
			t.Error("expected non-zero CreatedAt")
		}
		if created.UpdatedAt.IsZero() {
			t.Error("expected non-zero UpdatedAt")
		}
	})

	t.Run("Create with provided ID preserves it", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		customID := model.KnowledgeID(fmt.Sprintf("custom-id-%d", time.Now().UnixNano()))
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		knowledge := &model.Knowledge{
			ID:        customID,
			RiskID:    456,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/custom",
			Title:     "Custom ID Knowledge",
			Summary:   "Knowledge with custom ID",
		}

		created, err := repo.Knowledge().Create(ctx, knowledge)
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		if created.ID != customID {
			t.Errorf("expected ID=%s, got %s", customID, created.ID)
		}
	})

	t.Run("Get retrieves existing knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		sourcedAt := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)

		knowledge := &model.Knowledge{
			RiskID:    789,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/get-test",
			Title:     "Test Knowledge",
			Summary:   "For testing Get",
			Embedding: []float32{0.5, 0.6, 0.7, 0.8},
			SourcedAt: sourcedAt,
		}

		created, err := repo.Knowledge().Create(ctx, knowledge)
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		retrieved, err := repo.Knowledge().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get knowledge: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%s, got %s", created.ID, retrieved.ID)
		}
		if retrieved.RiskID != created.RiskID {
			t.Errorf("expected RiskID=%d, got %d", created.RiskID, retrieved.RiskID)
		}
		if retrieved.SourceID != created.SourceID {
			t.Errorf("expected SourceID=%s, got %s", created.SourceID, retrieved.SourceID)
		}
		if retrieved.SourceURL != created.SourceURL {
			t.Errorf("expected SourceURL=%s, got %s", created.SourceURL, retrieved.SourceURL)
		}
		if retrieved.Title != created.Title {
			t.Errorf("expected Title=%s, got %s", created.Title, retrieved.Title)
		}
		if retrieved.Summary != created.Summary {
			t.Errorf("expected Summary=%s, got %s", created.Summary, retrieved.Summary)
		}
		if len(retrieved.Embedding) != 4 {
			t.Errorf("expected Embedding length=4, got %d", len(retrieved.Embedding))
		}
		if time.Since(retrieved.CreatedAt) > time.Second {
			t.Errorf("CreatedAt time diff too large: %v", time.Since(retrieved.CreatedAt))
		}
	})

	t.Run("Get returns error for non-existent knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Knowledge().Get(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent knowledge")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ListByRiskID returns knowledge for specific risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		riskID := time.Now().UnixNano()
		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		// Create knowledge entries for the same risk
		k1, err := repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/1",
			Title:     "Knowledge 1",
			Summary:   "First knowledge",
			SourcedAt: time.Now().Add(-1 * time.Hour).UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge 1: %v", err)
		}

		k2, err := repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/2",
			Title:     "Knowledge 2",
			Summary:   "Second knowledge",
			SourcedAt: time.Now().Add(-2 * time.Hour).UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge 2: %v", err)
		}

		// Create knowledge for a different risk
		_, err = repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID + 1,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/3",
			Title:     "Knowledge 3",
			Summary:   "Different risk knowledge",
			SourcedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge 3: %v", err)
		}

		knowledges, err := repo.Knowledge().ListByRiskID(ctx, riskID)
		if err != nil {
			t.Fatalf("failed to list by risk ID: %v", err)
		}

		if len(knowledges) != 2 {
			t.Errorf("expected 2 knowledges, got %d", len(knowledges))
		}

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
		if !foundK1 {
			t.Error("knowledge 1 not found in list")
		}
		if !foundK2 {
			t.Error("knowledge 2 not found in list")
		}
	})

	t.Run("ListByRiskID returns empty for non-existent risk", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		knowledges, err := repo.Knowledge().ListByRiskID(ctx, 999999999)
		if err != nil {
			t.Fatalf("failed to list by risk ID: %v", err)
		}

		if len(knowledges) != 0 {
			t.Errorf("expected 0 knowledges, got %d", len(knowledges))
		}
	})

	t.Run("ListBySourceID returns knowledge for specific source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))
		otherSourceID := model.SourceID(fmt.Sprintf("other-source-%d", time.Now().UnixNano()))
		riskID := time.Now().UnixNano()

		// Create knowledge entries for the same source
		k1, err := repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/a",
			Title:     "Knowledge A",
			Summary:   "First knowledge for source",
			SourcedAt: time.Now().Add(-1 * time.Hour).UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge A: %v", err)
		}

		k2, err := repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID + 1,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/b",
			Title:     "Knowledge B",
			Summary:   "Second knowledge for source",
			SourcedAt: time.Now().Add(-2 * time.Hour).UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge B: %v", err)
		}

		// Create knowledge for a different source
		_, err = repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    riskID,
			SourceID:  otherSourceID,
			SourceURL: "https://www.notion.so/page/c",
			Title:     "Knowledge C",
			Summary:   "Different source knowledge",
			SourcedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("failed to create knowledge C: %v", err)
		}

		knowledges, err := repo.Knowledge().ListBySourceID(ctx, sourceID)
		if err != nil {
			t.Fatalf("failed to list by source ID: %v", err)
		}

		if len(knowledges) != 2 {
			t.Errorf("expected 2 knowledges, got %d", len(knowledges))
		}

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
		if !foundK1 {
			t.Error("knowledge A not found in list")
		}
		if !foundK2 {
			t.Error("knowledge B not found in list")
		}
	})

	t.Run("ListBySourceID returns empty for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		knowledges, err := repo.Knowledge().ListBySourceID(ctx, "non-existent-source")
		if err != nil {
			t.Fatalf("failed to list by source ID: %v", err)
		}

		if len(knowledges) != 0 {
			t.Errorf("expected 0 knowledges, got %d", len(knowledges))
		}
	})

	t.Run("Delete removes existing knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		created, err := repo.Knowledge().Create(ctx, &model.Knowledge{
			RiskID:    111,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/delete",
			Title:     "To Be Deleted",
			Summary:   "This will be deleted",
		})
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		err = repo.Knowledge().Delete(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to delete knowledge: %v", err)
		}

		_, err = repo.Knowledge().Get(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted knowledge")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete returns error for non-existent knowledge", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Knowledge().Delete(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent knowledge")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Knowledge without Embedding works", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sourceID := model.SourceID(fmt.Sprintf("source-%d", time.Now().UnixNano()))

		knowledge := &model.Knowledge{
			RiskID:    222,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/no-embedding",
			Title:     "No Embedding",
			Summary:   "Knowledge without embedding",
			Embedding: nil,
		}

		created, err := repo.Knowledge().Create(ctx, knowledge)
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		if len(created.Embedding) != 0 {
			t.Errorf("expected nil or empty Embedding, got %v", created.Embedding)
		}

		retrieved, err := repo.Knowledge().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get knowledge: %v", err)
		}

		if len(retrieved.Embedding) != 0 {
			t.Errorf("expected nil or empty Embedding after retrieval, got %v", retrieved.Embedding)
		}
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
			RiskID:    333,
			SourceID:  sourceID,
			SourceURL: "https://www.notion.so/page/large-embedding",
			Title:     "Large Embedding",
			Summary:   "Knowledge with 768-dimension embedding",
			Embedding: embedding,
		}

		created, err := repo.Knowledge().Create(ctx, knowledge)
		if err != nil {
			t.Fatalf("failed to create knowledge: %v", err)
		}

		if len(created.Embedding) != model.EmbeddingDimension {
			t.Errorf("expected Embedding length=%d, got %d", model.EmbeddingDimension, len(created.Embedding))
		}

		retrieved, err := repo.Knowledge().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get knowledge: %v", err)
		}

		if len(retrieved.Embedding) != model.EmbeddingDimension {
			t.Errorf("expected Embedding length=%d after retrieval, got %d", model.EmbeddingDimension, len(retrieved.Embedding))
		}

		// Verify first and last values
		if retrieved.Embedding[0] != 0 {
			t.Errorf("expected first embedding value=0, got %f", retrieved.Embedding[0])
		}
		expectedLast := float32(model.EmbeddingDimension-1) / float32(model.EmbeddingDimension)
		if retrieved.Embedding[model.EmbeddingDimension-1] != expectedLast {
			t.Errorf("expected last embedding value=%f, got %f", expectedLast, retrieved.Embedding[model.EmbeddingDimension-1])
		}
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
	// Use standard collection names (no prefix) to utilize existing Firestore indexes
	// Test data isolation is achieved through random IDs in test data
	repo, err := firestore.New(ctx, projectID, databaseID)
	if err != nil {
		t.Fatalf("failed to create firestore repository: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("failed to close firestore repository: %v", err)
		}
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
