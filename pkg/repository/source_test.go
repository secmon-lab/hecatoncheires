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
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.Name).Equal(source.Name)
		gt.Value(t, created.SourceType).Equal(source.SourceType)
		gt.Value(t, created.Description).Equal(source.Description)
		gt.Value(t, created.Enabled).Equal(source.Enabled)
		gt.Value(t, created.NotionDBConfig).NotNil()
		gt.Value(t, created.NotionDBConfig.DatabaseID).Equal(source.NotionDBConfig.DatabaseID)
		gt.Value(t, created.NotionDBConfig.DatabaseTitle).Equal(source.NotionDBConfig.DatabaseTitle)
		gt.Value(t, created.NotionDBConfig.DatabaseURL).Equal(source.NotionDBConfig.DatabaseURL)
		gt.Bool(t, created.CreatedAt.IsZero()).False()
		gt.Bool(t, created.UpdatedAt.IsZero()).False()
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
		gt.NoError(t, err).Required()

		gt.Value(t, created.ID).Equal(customID)
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
		gt.NoError(t, err).Required()

		retrieved, err := repo.Source().Get(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Name).Equal(created.Name)
		gt.Value(t, retrieved.SourceType).Equal(created.SourceType)
		gt.Value(t, retrieved.Description).Equal(created.Description)
		gt.Value(t, retrieved.Enabled).Equal(created.Enabled)
		gt.Value(t, retrieved.NotionDBConfig).NotNil()
		gt.Value(t, retrieved.NotionDBConfig.DatabaseID).Equal(created.NotionDBConfig.DatabaseID)
		gt.Bool(t, time.Since(retrieved.CreatedAt) <= 3*time.Second).True()
		gt.Bool(t, time.Since(retrieved.UpdatedAt) <= 3*time.Second).True()
	})

	t.Run("Get returns error for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Source().Get(ctx, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("List returns all sources", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		sources, err := repo.Source().List(ctx)
		gt.NoError(t, err).Required()
		gt.Array(t, sources).Length(0)

		source1, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Source 1",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		})
		gt.NoError(t, err).Required()

		source2, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Source 2",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    false,
		})
		gt.NoError(t, err).Required()

		sources, err = repo.Source().List(ctx)
		gt.NoError(t, err).Required()
		gt.Array(t, sources).Length(2)

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
		gt.Bool(t, foundSource1).True()
		gt.Bool(t, foundSource2).True()
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
		gt.NoError(t, err).Required()

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
		gt.NoError(t, err).Required()

		gt.Value(t, updated.ID).Equal(created.ID)
		gt.Value(t, updated.Name).Equal("Updated Name")
		gt.Value(t, updated.Description).Equal("Updated Description")
		gt.Value(t, updated.Enabled).Equal(false)
		gt.Value(t, updated.NotionDBConfig).NotNil()
		gt.Value(t, updated.NotionDBConfig.DatabaseID).Equal("updated-db-id")
		gt.Bool(t, time.Since(updated.CreatedAt) <= time.Since(created.CreatedAt)+time.Second).True()
		gt.Bool(t, updated.UpdatedAt.After(created.UpdatedAt)).True()

		retrieved, err := repo.Source().Get(ctx, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, retrieved.Name).Equal("Updated Name")
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
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Delete removes existing source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Source().Create(ctx, &model.Source{
			Name:       "To Be Deleted",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		})
		gt.NoError(t, err).Required()

		err = repo.Source().Delete(ctx, created.ID)
		gt.NoError(t, err).Required()

		_, err = repo.Source().Get(ctx, created.ID)
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
	})

	t.Run("Delete returns error for non-existent source", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		err := repo.Source().Delete(ctx, "non-existent-id")
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)).True()
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
		gt.NoError(t, err).Required()

		gt.Value(t, created.NotionDBConfig).Nil()

		retrieved, err := repo.Source().Get(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.NotionDBConfig).Nil()
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
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.Name).Equal(source.Name)
		gt.Value(t, created.SourceType).Equal(model.SourceTypeSlack)
		gt.Value(t, created.SlackConfig).NotNil().Required()
		gt.Array(t, created.SlackConfig.Channels).Length(2)
		gt.Value(t, created.SlackConfig.Channels[0].ID).Equal("C01234567")
		gt.Value(t, created.SlackConfig.Channels[0].Name).Equal("general")
		gt.Value(t, created.SlackConfig.Channels[1].ID).Equal("C89012345")
		gt.Value(t, created.SlackConfig.Channels[1].Name).Equal("random")
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
		gt.NoError(t, err).Required()

		retrieved, err := repo.Source().Get(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.SlackConfig).NotNil().Required()
		gt.Array(t, retrieved.SlackConfig.Channels).Length(1)
		gt.Value(t, retrieved.SlackConfig.Channels[0].ID).Equal("C11111111")
		gt.Value(t, retrieved.SlackConfig.Channels[0].Name).Equal("test-channel")
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
		gt.NoError(t, err).Required()

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
		gt.NoError(t, err).Required()

		gt.Value(t, updated.SlackConfig).NotNil().Required()
		gt.Array(t, updated.SlackConfig.Channels).Length(2)
		gt.Value(t, updated.SlackConfig.Channels[0].ID).Equal("C00000002")
		gt.Value(t, updated.SlackConfig.Channels[1].ID).Equal("C00000003")
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
		gt.NoError(t, err).Required()

		gt.Value(t, created.SlackConfig).NotNil().Required()
		gt.Array(t, created.SlackConfig.Channels).Length(0)

		retrieved, err := repo.Source().Get(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.SlackConfig).NotNil().Required()
		gt.Array(t, retrieved.SlackConfig.Channels).Length(0)
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
	repo, err := firestore.New(ctx, projectID, firestore.WithCollectionPrefix(prefix))
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, repo.Close())
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
