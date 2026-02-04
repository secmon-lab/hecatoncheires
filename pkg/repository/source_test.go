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

func runSourceRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create creates source with UUID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:        "My Notion DB",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Test notion database",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "abc123",
				DatabaseTitle: "Test DB",
				DatabaseURL:   "https://notion.so/abc123",
			},
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		if created.ID == "" {
			t.Error("expected non-empty ID")
		}
		if created.Name != source.Name {
			t.Errorf("expected name=%s, got %s", source.Name, created.Name)
		}
		if created.SourceType != source.SourceType {
			t.Errorf("expected sourceType=%s, got %s", source.SourceType, created.SourceType)
		}
		if created.Description != source.Description {
			t.Errorf("expected description=%s, got %s", source.Description, created.Description)
		}
		if created.Enabled != source.Enabled {
			t.Errorf("expected enabled=%v, got %v", source.Enabled, created.Enabled)
		}
		if created.NotionDBConfig == nil {
			t.Error("expected NotionDBConfig to be set")
		} else {
			if created.NotionDBConfig.DatabaseID != source.NotionDBConfig.DatabaseID {
				t.Errorf("expected databaseID=%s, got %s", source.NotionDBConfig.DatabaseID, created.NotionDBConfig.DatabaseID)
			}
			if created.NotionDBConfig.DatabaseTitle != source.NotionDBConfig.DatabaseTitle {
				t.Errorf("expected databaseTitle=%s, got %s", source.NotionDBConfig.DatabaseTitle, created.NotionDBConfig.DatabaseTitle)
			}
			if created.NotionDBConfig.DatabaseURL != source.NotionDBConfig.DatabaseURL {
				t.Errorf("expected databaseURL=%s, got %s", source.NotionDBConfig.DatabaseURL, created.NotionDBConfig.DatabaseURL)
			}
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

		customID := model.SourceID(fmt.Sprintf("custom-id-%d", time.Now().UnixNano()))
		source := &model.Source{
			ID:          customID,
			Name:        "Custom ID Source",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Source with custom ID",
			Enabled:     true,
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		if created.ID != customID {
			t.Errorf("expected ID=%s, got %s", customID, created.ID)
		}
	})

	t.Run("Get retrieves existing source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:        "Test Source",
			SourceType:  model.SourceTypeNotionDB,
			Description: "For testing Get",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "test123",
				DatabaseTitle: "Test Title",
				DatabaseURL:   "https://notion.so/test123",
			},
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		retrieved, err := repo.Source().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get source: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%s, got %s", created.ID, retrieved.ID)
		}
		if retrieved.Name != created.Name {
			t.Errorf("expected name=%s, got %s", created.Name, retrieved.Name)
		}
		if retrieved.SourceType != created.SourceType {
			t.Errorf("expected sourceType=%s, got %s", created.SourceType, retrieved.SourceType)
		}
		if retrieved.Description != created.Description {
			t.Errorf("expected description=%s, got %s", created.Description, retrieved.Description)
		}
		if retrieved.Enabled != created.Enabled {
			t.Errorf("expected enabled=%v, got %v", created.Enabled, retrieved.Enabled)
		}
		if retrieved.NotionDBConfig == nil {
			t.Error("expected NotionDBConfig to be set")
		} else {
			if retrieved.NotionDBConfig.DatabaseID != created.NotionDBConfig.DatabaseID {
				t.Errorf("expected databaseID=%s, got %s", created.NotionDBConfig.DatabaseID, retrieved.NotionDBConfig.DatabaseID)
			}
		}
		if time.Since(retrieved.CreatedAt) > 3*time.Second {
			t.Errorf("CreatedAt time diff too large: %v", time.Since(retrieved.CreatedAt))
		}
		if time.Since(retrieved.UpdatedAt) > 3*time.Second {
			t.Errorf("UpdatedAt time diff too large: %v", time.Since(retrieved.UpdatedAt))
		}
	})

	t.Run("Get returns error for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Source().Get(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent source")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List returns all sources", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sources, err := repo.Source().List(ctx)
		if err != nil {
			t.Fatalf("failed to list sources: %v", err)
		}
		if len(sources) != 0 {
			t.Errorf("expected 0 sources, got %d", len(sources))
		}

		source1, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Source 1",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("failed to create source1: %v", err)
		}

		source2, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Source 2",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    false,
		})
		if err != nil {
			t.Fatalf("failed to create source2: %v", err)
		}

		sources, err = repo.Source().List(ctx)
		if err != nil {
			t.Fatalf("failed to list sources: %v", err)
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}

		foundSource1 := false
		foundSource2 := false
		for _, s := range sources {
			if s.ID == source1.ID && s.Name == source1.Name && s.Enabled == source1.Enabled {
				foundSource1 = true
			}
			if s.ID == source2.ID && s.Name == source2.Name && s.Enabled == source2.Enabled {
				foundSource2 = true
			}
		}
		if !foundSource1 {
			t.Error("source1 not found in list")
		}
		if !foundSource2 {
			t.Error("source2 not found in list")
		}
	})

	t.Run("Update modifies existing source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Source().Create(ctx, &model.Source{
			Name:        "Original Name",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Original Description",
			Enabled:     true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "original-db-id",
				DatabaseTitle: "Original Title",
				DatabaseURL:   "https://notion.so/original",
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Source().Update(ctx, &model.Source{
			ID:          created.ID,
			Name:        "Updated Name",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Updated Description",
			Enabled:     false,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID:    "updated-db-id",
				DatabaseTitle: "Updated Title",
				DatabaseURL:   "https://notion.so/updated",
			},
		})
		if err != nil {
			t.Fatalf("failed to update source: %v", err)
		}

		if updated.ID != created.ID {
			t.Errorf("ID should not change, got %s", updated.ID)
		}
		if updated.Name != "Updated Name" {
			t.Errorf("expected name='Updated Name', got %s", updated.Name)
		}
		if updated.Description != "Updated Description" {
			t.Errorf("expected description='Updated Description', got %s", updated.Description)
		}
		if updated.Enabled != false {
			t.Errorf("expected enabled=false, got %v", updated.Enabled)
		}
		if updated.NotionDBConfig == nil {
			t.Error("expected NotionDBConfig to be set")
		} else {
			if updated.NotionDBConfig.DatabaseID != "updated-db-id" {
				t.Errorf("expected databaseID='updated-db-id', got %s", updated.NotionDBConfig.DatabaseID)
			}
		}
		if time.Since(updated.CreatedAt) > time.Since(created.CreatedAt)+time.Second {
			t.Errorf("CreatedAt should not change significantly")
		}
		if !updated.UpdatedAt.After(created.UpdatedAt) {
			t.Errorf("UpdatedAt should be after original, got %v", updated.UpdatedAt)
		}

		retrieved, err := repo.Source().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get updated source: %v", err)
		}
		if retrieved.Name != "Updated Name" {
			t.Errorf("expected name='Updated Name' after retrieval, got %s", retrieved.Name)
		}
	})

	t.Run("Update returns error for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Source().Update(ctx, &model.Source{
			ID:          "non-existent-id",
			Name:        "Non-existent",
			SourceType:  model.SourceTypeNotionDB,
			Description: "Should fail",
			Enabled:     true,
		})
		if err == nil {
			t.Error("expected error for non-existent source")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete removes existing source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Source().Create(ctx, &model.Source{
			Name:       "To Be Deleted",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		err = repo.Source().Delete(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to delete source: %v", err)
		}

		_, err = repo.Source().Get(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted source")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete returns error for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Source().Delete(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent source")
		}
		if !errors.Is(err, memory.ErrNotFound) && !errors.Is(err, firestore.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Source without NotionDBConfig works", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:           "Source Without Config",
			SourceType:     model.SourceTypeNotionDB,
			Description:    "No config attached",
			Enabled:        true,
			NotionDBConfig: nil,
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		if created.NotionDBConfig != nil {
			t.Error("expected NotionDBConfig to be nil")
		}

		retrieved, err := repo.Source().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get source: %v", err)
		}

		if retrieved.NotionDBConfig != nil {
			t.Error("expected NotionDBConfig to be nil after retrieval")
		}
	})

	t.Run("Create Slack source with channels", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:        "My Slack Source",
			SourceType:  model.SourceTypeSlack,
			Description: "Test slack source",
			Enabled:     true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{
					{ID: "C01234567", Name: "general"},
					{ID: "C89012345", Name: "random"},
				},
			},
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		if created.ID == "" {
			t.Error("expected non-empty ID")
		}
		if created.Name != source.Name {
			t.Errorf("expected name=%s, got %s", source.Name, created.Name)
		}
		if created.SourceType != model.SourceTypeSlack {
			t.Errorf("expected sourceType=%s, got %s", model.SourceTypeSlack, created.SourceType)
		}
		if created.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set")
		}
		if len(created.SlackConfig.Channels) != 2 {
			t.Errorf("expected 2 channels, got %d", len(created.SlackConfig.Channels))
		}
		if created.SlackConfig.Channels[0].ID != "C01234567" {
			t.Errorf("expected channel ID=C01234567, got %s", created.SlackConfig.Channels[0].ID)
		}
		if created.SlackConfig.Channels[0].Name != "general" {
			t.Errorf("expected channel name=general, got %s", created.SlackConfig.Channels[0].Name)
		}
		if created.SlackConfig.Channels[1].ID != "C89012345" {
			t.Errorf("expected channel ID=C89012345, got %s", created.SlackConfig.Channels[1].ID)
		}
		if created.SlackConfig.Channels[1].Name != "random" {
			t.Errorf("expected channel name=random, got %s", created.SlackConfig.Channels[1].Name)
		}
	})

	t.Run("Get retrieves Slack source with channels", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:        "Slack Source for Get",
			SourceType:  model.SourceTypeSlack,
			Description: "For testing Get",
			Enabled:     true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{
					{ID: "C11111111", Name: "test-channel"},
				},
			},
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		retrieved, err := repo.Source().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get source: %v", err)
		}

		if retrieved.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set")
		}
		if len(retrieved.SlackConfig.Channels) != 1 {
			t.Errorf("expected 1 channel, got %d", len(retrieved.SlackConfig.Channels))
		}
		if retrieved.SlackConfig.Channels[0].ID != "C11111111" {
			t.Errorf("expected channel ID=C11111111, got %s", retrieved.SlackConfig.Channels[0].ID)
		}
		if retrieved.SlackConfig.Channels[0].Name != "test-channel" {
			t.Errorf("expected channel name=test-channel, got %s", retrieved.SlackConfig.Channels[0].Name)
		}
	})

	t.Run("Update Slack source channels", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Source().Create(ctx, &model.Source{
			Name:        "Slack Source for Update",
			SourceType:  model.SourceTypeSlack,
			Description: "Original",
			Enabled:     true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{
					{ID: "C00000001", Name: "original-channel"},
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		updated, err := repo.Source().Update(ctx, &model.Source{
			ID:          created.ID,
			Name:        "Updated Slack Source",
			SourceType:  model.SourceTypeSlack,
			Description: "Updated",
			Enabled:     false,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{
					{ID: "C00000002", Name: "updated-channel-1"},
					{ID: "C00000003", Name: "updated-channel-2"},
				},
			},
		})
		if err != nil {
			t.Fatalf("failed to update source: %v", err)
		}

		if updated.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set")
		}
		if len(updated.SlackConfig.Channels) != 2 {
			t.Errorf("expected 2 channels, got %d", len(updated.SlackConfig.Channels))
		}
		if updated.SlackConfig.Channels[0].ID != "C00000002" {
			t.Errorf("expected channel ID=C00000002, got %s", updated.SlackConfig.Channels[0].ID)
		}
		if updated.SlackConfig.Channels[1].ID != "C00000003" {
			t.Errorf("expected channel ID=C00000003, got %s", updated.SlackConfig.Channels[1].ID)
		}
	})

	t.Run("Slack source without channels works", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		source := &model.Source{
			Name:        "Slack Source Without Channels",
			SourceType:  model.SourceTypeSlack,
			Description: "No channels",
			Enabled:     true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{},
			},
		}

		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		if created.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set")
		}
		if len(created.SlackConfig.Channels) != 0 {
			t.Errorf("expected 0 channels, got %d", len(created.SlackConfig.Channels))
		}

		retrieved, err := repo.Source().Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get source: %v", err)
		}

		if retrieved.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set after retrieval")
		}
		if len(retrieved.SlackConfig.Channels) != 0 {
			t.Errorf("expected 0 channels after retrieval, got %d", len(retrieved.SlackConfig.Channels))
		}
	})
}

func newFirestoreSourceRepository(t *testing.T) interfaces.Repository {
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
	prefix := fmt.Sprintf("test_%d", time.Now().UnixNano())
	repo, err := firestore.New(ctx, projectID, databaseID, firestore.WithCollectionPrefix(prefix))
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

func TestMemorySourceRepository(t *testing.T) {
	runSourceRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreSourceRepository(t *testing.T) {
	runSourceRepositoryTest(t, newFirestoreSourceRepository)
}
