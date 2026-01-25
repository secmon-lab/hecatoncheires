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
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// sourceTestNotionService is a mock implementation of notion.Service for testing
type sourceTestNotionService struct {
	getDatabaseMetadataFn func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error)
}

func (m *sourceTestNotionService) QueryUpdatedPages(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
	return func(yield func(*notion.Page, error) bool) {}
}

func (m *sourceTestNotionService) GetDatabaseMetadata(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
	if m.getDatabaseMetadataFn != nil {
		return m.getDatabaseMetadataFn(ctx, dbID)
	}
	return &notion.DatabaseMetadata{
		ID:    dbID,
		Title: "Test Database",
		URL:   "https://notion.so/test-db",
	}, nil
}

// mockSlackService is a mock implementation of slack.Service for testing
type mockSlackService struct {
	listJoinedChannelsFn func(ctx context.Context) ([]slack.Channel, error)
	getChannelNamesFn    func(ctx context.Context, ids []string) (map[string]string, error)
	getUserInfoFn        func(ctx context.Context, userID string) (*slack.User, error)
	listUsersFn          func(ctx context.Context) ([]*slack.User, error)
	createChannelFn      func(ctx context.Context, riskID int64, riskName string) (string, error)
	renameChannelFn      func(ctx context.Context, channelID string, riskID int64, riskName string) error
}

func (m *mockSlackService) ListJoinedChannels(ctx context.Context) ([]slack.Channel, error) {
	if m.listJoinedChannelsFn != nil {
		return m.listJoinedChannelsFn(ctx)
	}
	return []slack.Channel{
		{ID: "C001", Name: "general"},
		{ID: "C002", Name: "random"},
	}, nil
}

func (m *mockSlackService) GetChannelNames(ctx context.Context, ids []string) (map[string]string, error) {
	if m.getChannelNamesFn != nil {
		return m.getChannelNamesFn(ctx, ids)
	}
	result := make(map[string]string)
	for _, id := range ids {
		result[id] = "channel-" + id
	}
	return result, nil
}

func (m *mockSlackService) GetUserInfo(ctx context.Context, userID string) (*slack.User, error) {
	if m.getUserInfoFn != nil {
		return m.getUserInfoFn(ctx, userID)
	}
	return &slack.User{
		ID:       userID,
		Name:     "testuser",
		RealName: "Test User",
		Email:    "test@example.com",
		ImageURL: "https://example.com/image.png",
	}, nil
}

func (m *mockSlackService) ListUsers(ctx context.Context) ([]*slack.User, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(ctx)
	}
	return []*slack.User{
		{ID: "U001", Name: "user1", RealName: "User One"},
		{ID: "U002", Name: "user2", RealName: "User Two"},
	}, nil
}

func (m *mockSlackService) CreateChannel(ctx context.Context, riskID int64, riskName string) (string, error) {
	if m.createChannelFn != nil {
		return m.createChannelFn(ctx, riskID, riskName)
	}
	return "C" + riskName, nil
}

func (m *mockSlackService) RenameChannel(ctx context.Context, channelID string, riskID int64, riskName string) error {
	if m.renameChannelFn != nil {
		return m.renameChannelFn(ctx, channelID, riskID, riskName)
	}
	return nil
}

func TestSourceUseCase_CreateNotionDBSource(t *testing.T) {
	t.Run("creates source with valid database ID", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &sourceTestNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return &notion.DatabaseMetadata{
					ID:    dbID,
					Title: "My Database",
					URL:   "https://notion.so/my-db",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil)
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
		notionSvc := &sourceTestNotionService{}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		notionSvc := &sourceTestNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return nil, errors.New("database not found")
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, "")
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, "")
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		notionSvc := &sourceTestNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return &notion.DatabaseMetadata{
					ID:    dbID,
					Title: "Valid Database",
					URL:   "https://notion.so/valid-db",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil)
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
		notionSvc := &sourceTestNotionService{
			getDatabaseMetadataFn: func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
				return nil, errors.New("database not found")
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil)
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

func TestSourceUseCase_CreateSlackSource(t *testing.T) {
	t.Run("creates source with valid channel IDs", func(t *testing.T) {
		repo := memory.New()
		slackSvc := &mockSlackService{
			getChannelNamesFn: func(ctx context.Context, ids []string) (map[string]string, error) {
				return map[string]string{
					"C001": "general",
					"C002": "random",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc)
		ctx := context.Background()

		input := usecase.CreateSlackSourceInput{
			Name:        "My Slack Source",
			Description: "Test slack source",
			ChannelIDs:  []string{"C001", "C002"},
			Enabled:     true,
		}

		source, err := uc.CreateSlackSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateSlackSource failed: %v", err)
		}

		if source.ID == "" {
			t.Error("expected source ID to be set")
		}
		if source.Name != "My Slack Source" {
			t.Errorf("expected name='My Slack Source', got %s", source.Name)
		}
		if source.SourceType != model.SourceTypeSlack {
			t.Errorf("expected sourceType=slack, got %s", source.SourceType)
		}
		if source.SlackConfig == nil {
			t.Fatal("expected SlackConfig to be set")
		}
		if len(source.SlackConfig.Channels) != 2 {
			t.Errorf("expected 2 channels, got %d", len(source.SlackConfig.Channels))
		}
		if source.SlackConfig.Channels[0].ID != "C001" {
			t.Errorf("expected channel ID='C001', got %s", source.SlackConfig.Channels[0].ID)
		}
		if source.SlackConfig.Channels[0].Name != "general" {
			t.Errorf("expected channel name='general', got %s", source.SlackConfig.Channels[0].Name)
		}
	})

	t.Run("uses default name when not provided", func(t *testing.T) {
		repo := memory.New()
		slackSvc := &mockSlackService{}
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc)
		ctx := context.Background()

		input := usecase.CreateSlackSourceInput{
			ChannelIDs: []string{"C001"},
			Enabled:    true,
		}

		source, err := uc.CreateSlackSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateSlackSource failed: %v", err)
		}

		if source.Name != "Slack Source" {
			t.Errorf("expected name='Slack Source', got %s", source.Name)
		}
	})

	t.Run("fails with empty channel IDs", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.CreateSlackSourceInput{
			Name:       "Empty Channels",
			ChannelIDs: []string{},
			Enabled:    true,
		}

		_, err := uc.CreateSlackSource(ctx, input)
		if err == nil {
			t.Error("expected error for empty channel IDs")
		}
	})

	t.Run("creates source without Slack service", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.CreateSlackSourceInput{
			Name:       "Manual Source",
			ChannelIDs: []string{"C001", "C002"},
			Enabled:    true,
		}

		source, err := uc.CreateSlackSource(ctx, input)
		if err != nil {
			t.Fatalf("CreateSlackSource failed: %v", err)
		}

		if source.SlackConfig.Channels[0].Name != "C001" {
			t.Errorf("expected channel name='C001' (fallback to ID), got %s", source.SlackConfig.Channels[0].Name)
		}
	})
}

func TestSourceUseCase_UpdateSlackSource(t *testing.T) {
	t.Run("updates slack source fields", func(t *testing.T) {
		repo := memory.New()
		slackSvc := &mockSlackService{}
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc)
		ctx := context.Background()

		source := &model.Source{
			Name:       "Original",
			SourceType: model.SourceTypeSlack,
			Enabled:    true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{{ID: "C001", Name: "general"}},
			},
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		newName := "Updated"
		newDesc := "Updated description"
		newEnabled := false
		input := usecase.UpdateSlackSourceInput{
			ID:          created.ID,
			Name:        &newName,
			Description: &newDesc,
			Enabled:     &newEnabled,
		}

		updated, err := uc.UpdateSlackSource(ctx, input)
		if err != nil {
			t.Fatalf("UpdateSlackSource failed: %v", err)
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

	t.Run("updates channels", func(t *testing.T) {
		repo := memory.New()
		slackSvc := &mockSlackService{
			getChannelNamesFn: func(ctx context.Context, ids []string) (map[string]string, error) {
				return map[string]string{
					"C003": "new-channel",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc)
		ctx := context.Background()

		source := &model.Source{
			Name:       "Original",
			SourceType: model.SourceTypeSlack,
			Enabled:    true,
			SlackConfig: &model.SlackConfig{
				Channels: []model.SlackChannel{{ID: "C001", Name: "general"}},
			},
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		input := usecase.UpdateSlackSourceInput{
			ID:         created.ID,
			ChannelIDs: []string{"C003"},
		}

		updated, err := uc.UpdateSlackSource(ctx, input)
		if err != nil {
			t.Fatalf("UpdateSlackSource failed: %v", err)
		}

		if len(updated.SlackConfig.Channels) != 1 {
			t.Errorf("expected 1 channel, got %d", len(updated.SlackConfig.Channels))
		}
		if updated.SlackConfig.Channels[0].ID != "C003" {
			t.Errorf("expected channel ID='C003', got %s", updated.SlackConfig.Channels[0].ID)
		}
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.UpdateSlackSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSlackSource(ctx, input)
		if err == nil {
			t.Error("expected error for empty ID")
		}
	})

	t.Run("fails for non-slack source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		source := &model.Source{
			Name:       "Notion Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
		}
		created, err := repo.Source().Create(ctx, source)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		newName := "New Name"
		input := usecase.UpdateSlackSourceInput{
			ID:   created.ID,
			Name: &newName,
		}

		_, err = uc.UpdateSlackSource(ctx, input)
		if err == nil {
			t.Error("expected error for non-slack source")
		}
	})
}

func TestSourceUseCase_ListSlackChannels(t *testing.T) {
	t.Run("lists channels from slack service", func(t *testing.T) {
		repo := memory.New()
		slackSvc := &mockSlackService{
			listJoinedChannelsFn: func(ctx context.Context) ([]slack.Channel, error) {
				return []slack.Channel{
					{ID: "C001", Name: "general"},
					{ID: "C002", Name: "random"},
					{ID: "C003", Name: "dev"},
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc)
		ctx := context.Background()

		channels, err := uc.ListSlackChannels(ctx)
		if err != nil {
			t.Fatalf("ListSlackChannels failed: %v", err)
		}

		if len(channels) != 3 {
			t.Errorf("expected 3 channels, got %d", len(channels))
		}
		if channels[0].ID != "C001" {
			t.Errorf("expected channel ID='C001', got %s", channels[0].ID)
		}
		if channels[0].Name != "general" {
			t.Errorf("expected channel name='general', got %s", channels[0].Name)
		}
	})

	t.Run("fails when slack service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.ListSlackChannels(ctx)
		if err == nil {
			t.Error("expected error when slack service is not configured")
		}
	})
}
