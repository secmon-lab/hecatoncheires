package usecase_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// mockNotionService is a mock implementation of notion.Service for testing
type mockNotionService struct {
	getDatabaseMetadataFn func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error)
}

func (m *mockNotionService) QueryUpdatedPages(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
	return func(yield func(*notion.Page, error) bool) {}
}

func (m *mockNotionService) GetDatabaseMetadata(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
	if m.getDatabaseMetadataFn != nil {
		return m.getDatabaseMetadataFn(ctx, dbID)
	}
	return &notion.DatabaseMetadata{
		ID:    dbID,
		Title: "Test Database",
		URL:   "https://notion.so/test-db",
	}, nil
}

func TestSourceUseCase_CreateNotionDBSource(t *testing.T) {
	t.Run("creates source with valid database ID", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &mockNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return &notion.DatabaseMetadata{
					ID:    dbID,
					Title: "My Database",
					URL:   "https://notion.so/my-db",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID:  "test-db-id",
			Description: "Test database",
			Enabled:     true,
		}

		source, err := uc.CreateNotionDBSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateNotionDBSource failed: %v", err)
		}

		if source.ID == "" {
			t.Error("expected source ID to be set")
		}
		if source.Name != "My Database" {
			t.Errorf("expected name='My Database', got %s", source.Name)
		}
		if source.SourceType != model.SourceTypeNotionDB {
			t.Errorf("expected sourceType=notion_db, got %s", source.SourceType)
		}
		if source.NotionDBConfig == nil {
			t.Error("expected NotionDBConfig to be set")
		} else {
			if source.NotionDBConfig.DatabaseID != "test-db-id" {
				t.Errorf("expected databaseID='test-db-id', got %s", source.NotionDBConfig.DatabaseID)
			}
			if source.NotionDBConfig.DatabaseTitle != "My Database" {
				t.Errorf("expected databaseTitle='My Database', got %s", source.NotionDBConfig.DatabaseTitle)
			}
		}
	})

	t.Run("uses custom name when provided", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &mockNotionService{}
		uc := usecase.NewSourceUseCase(repo, notionSvc)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			Name:       "Custom Name",
			DatabaseID: "test-db-id",
			Enabled:    true,
		}

		source, err := uc.CreateNotionDBSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateNotionDBSource failed: %v", err)
		}

		if source.Name != "Custom Name" {
			t.Errorf("expected name='Custom Name', got %s", source.Name)
		}
	})

	t.Run("fails with empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID: "",
			Enabled:    true,
		}

		_, err := uc.CreateNotionDBSource(ctx, input)
		if err == nil {
			t.Error("expected error for empty database ID")
		}
	})

	t.Run("fails when Notion API returns error", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &mockNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return nil, errors.New("database not found")
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID: "invalid-db-id",
			Enabled:    true,
		}

		_, err := uc.CreateNotionDBSource(ctx, input)
		if err == nil {
			t.Error("expected error when Notion API fails")
		}
	})

	t.Run("creates source without Notion service", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			Name:       "Manual Source",
			DatabaseID: "test-db-id",
			Enabled:    true,
		}

		source, err := uc.CreateNotionDBSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateNotionDBSource failed: %v", err)
		}

		if source.Name != "Manual Source" {
			t.Errorf("expected name='Manual Source', got %s", source.Name)
		}
	})
}

func TestSourceUseCase_UpdateSource(t *testing.T) {
	t.Run("updates source fields", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		source := &model.Source{
			Name:       "Original",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		newName := "Updated"
		newDesc := "Updated description"
		newEnabled := false
		input := usecase.UpdateSourceInput{
			ID:          created.ID,
			Name:        &newName,
			Description: &newDesc,
			Enabled:     &newEnabled,
		}

		updated, err := uc.UpdateSource(ctx, input)
		if err != nil {
			t.Fatalf("UpdateSource failed: %v", err)
		}

		if updated.Name != "Updated" {
			t.Errorf("expected name='Updated', got %s", updated.Name)
		}
		if updated.Description != "Updated description" {
			t.Errorf("expected description='Updated description', got %s", updated.Description)
		}
		if updated.Enabled != false {
			t.Errorf("expected enabled=false, got %v", updated.Enabled)
		}
	})

	t.Run("partial update only changes specified fields", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		source := &model.Source{
			Name:        "Original",
			Description: "Original Description",
			SourceType:  model.SourceTypeNotionDB,
			Enabled:     true,
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		newName := "New Name"
		input := usecase.UpdateSourceInput{
			ID:   created.ID,
			Name: &newName,
		}

		updated, err := uc.UpdateSource(ctx, input)
		if err != nil {
			t.Fatalf("UpdateSource failed: %v", err)
		}

		if updated.Name != "New Name" {
			t.Errorf("expected name='New Name', got %s", updated.Name)
		}
		if updated.Description != "Original Description" {
			t.Errorf("expected description='Original Description', got %s", updated.Description)
		}
		if updated.Enabled != true {
			t.Errorf("expected enabled=true, got %v", updated.Enabled)
		}
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		input := usecase.UpdateSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSource(ctx, input)
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		newName := "New Name"
		input := usecase.UpdateSourceInput{
			ID:   "non-existent-id",
			Name: &newName,
		}

		_, err := uc.UpdateSource(ctx, input)
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestSourceUseCase_DeleteSource(t *testing.T) {
	t.Run("deletes existing source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		source := &model.Source{
			Name:       "To Delete",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		err = uc.DeleteSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("DeleteSource failed: %v", err)
		}

		_, err = repo.Source().Get(ctx, created.ID)
		if err == nil {
			t.Error("expected error when getting deleted source")
		}
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, "")
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestSourceUseCase_GetSource(t *testing.T) {
	t.Run("gets existing source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		source := &model.Source{
			Name:       "Test Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		retrieved, err := uc.GetSource(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetSource failed: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("expected ID=%s, got %s", created.ID, retrieved.ID)
		}
		if retrieved.Name != "Test Source" {
			t.Errorf("expected name='Test Source', got %s", retrieved.Name)
		}
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, "")
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, "non-existent-id")
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}

func TestSourceUseCase_ListSources(t *testing.T) {
	t.Run("lists all sources", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		sources, err := uc.ListSources(ctx)
		if err != nil {
			t.Fatalf("ListSources failed: %v", err)
		}
		if len(sources) != 0 {
			t.Errorf("expected 0 sources, got %d", len(sources))
		}

		source1 := &model.Source{Name: "Source 1", SourceType: model.SourceTypeNotionDB, Enabled: true}
		source2 := &model.Source{Name: "Source 2", SourceType: model.SourceTypeNotionDB, Enabled: false}
		_, _ = repo.Source().Create(ctx, source1)
		_, _ = repo.Source().Create(ctx, source2)

		sources, err = uc.ListSources(ctx)
		if err != nil {
			t.Fatalf("ListSources failed: %v", err)
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}
	})
}

func TestSourceUseCase_ValidateNotionDB(t *testing.T) {
	t.Run("validates existing database", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &mockNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return &notion.DatabaseMetadata{
					ID:    dbID,
					Title: "Valid Database",
					URL:   "https://notion.so/valid-db",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "valid-db-id")
		if err != nil {
			t.Fatalf("ValidateNotionDB failed: %v", err)
		}

		if !result.Valid {
			t.Error("expected Valid=true")
		}
		if result.DatabaseTitle != "Valid Database" {
			t.Errorf("expected databaseTitle='Valid Database', got %s", result.DatabaseTitle)
		}
		if result.DatabaseURL != "https://notion.so/valid-db" {
			t.Errorf("expected databaseURL='https://notion.so/valid-db', got %s", result.DatabaseURL)
		}
	})

	t.Run("returns invalid for empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "")
		if err != nil {
			t.Fatalf("ValidateNotionDB failed: %v", err)
		}

		if result.Valid {
			t.Error("expected Valid=false")
		}
		if result.ErrorMessage == "" {
			t.Error("expected ErrorMessage to be set")
		}
	})

	t.Run("returns invalid when Notion service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "some-db-id")
		if err != nil {
			t.Fatalf("ValidateNotionDB failed: %v", err)
		}

		if result.Valid {
			t.Error("expected Valid=false")
		}
		if result.ErrorMessage == "" {
			t.Error("expected ErrorMessage to be set")
		}
	})

	t.Run("returns invalid when database not found", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &mockNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return nil, errors.New("database not found")
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "invalid-db-id")
		if err != nil {
			t.Fatalf("ValidateNotionDB failed: %v", err)
		}

		if result.Valid {
			t.Error("expected Valid=false")
		}
		if result.ErrorMessage == "" {
			t.Error("expected ErrorMessage to be set")
		}
	})
}
