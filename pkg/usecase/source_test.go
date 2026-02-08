package usecase_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
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
	listJoinedChannelsFn   func(ctx context.Context) ([]slack.Channel, error)
	getChannelNamesFn      func(ctx context.Context, ids []string) (map[string]string, error)
	getUserInfoFn          func(ctx context.Context, userID string) (*slack.User, error)
	listUsersFn            func(ctx context.Context) ([]*slack.User, error)
	createChannelFn        func(ctx context.Context, caseID int64, caseName string, prefix string) (string, error)
	renameChannelFn        func(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error
	inviteUsersToChannelFn func(ctx context.Context, channelID string, userIDs []string) error
	invitedChannelID       string
	invitedUserIDs         []string
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

func (m *mockSlackService) CreateChannel(ctx context.Context, caseID int64, caseName string, prefix string) (string, error) {
	if m.createChannelFn != nil {
		return m.createChannelFn(ctx, caseID, caseName, prefix)
	}
	return "C" + caseName, nil
}

func (m *mockSlackService) RenameChannel(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error {
	if m.renameChannelFn != nil {
		return m.renameChannelFn(ctx, channelID, caseID, caseName, prefix)
	}
	return nil
}

func (m *mockSlackService) InviteUsersToChannel(ctx context.Context, channelID string, userIDs []string) error {
	m.invitedChannelID = channelID
	m.invitedUserIDs = userIDs
	if m.inviteUsersToChannelFn != nil {
		return m.inviteUsersToChannelFn(ctx, channelID, userIDs)
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

		source, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.ID).NotEqual(model.SourceID(""))
		gt.Value(t, source.Name).Equal("My Database")
		gt.Value(t, source.SourceType).Equal(model.SourceTypeNotionDB)
		gt.Value(t, source.NotionDBConfig).NotNil()
		if source.NotionDBConfig != nil {
			gt.Value(t, source.NotionDBConfig.DatabaseID).Equal("test-db-id")
			gt.Value(t, source.NotionDBConfig.DatabaseTitle).Equal("My Database")
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

		source, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.Name).Equal("Custom Name")
	})

	t.Run("fails with empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID: "",
			Enabled:    true,
		}

		_, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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

		_, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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

		source, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.Name).Equal("Manual Source")
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		newName := "Updated"
		newDesc := "Updated description"
		newEnabled := false
		input := usecase.UpdateSourceInput{
			ID:          created.ID,
			Name:        &newName,
			Description: &newDesc,
			Enabled:     &newEnabled,
		}

		updated, err := uc.UpdateSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Name).Equal("Updated")
		gt.Value(t, updated.Description).Equal("Updated description")
		gt.Value(t, updated.Enabled).Equal(false)
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		newName := "New Name"
		input := usecase.UpdateSourceInput{
			ID:   created.ID,
			Name: &newName,
		}

		updated, err := uc.UpdateSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Name).Equal("New Name")
		gt.Value(t, updated.Description).Equal("Original Description")
		gt.Value(t, updated.Enabled).Equal(true)
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.UpdateSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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

		_, err := uc.UpdateSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		gt.NoError(t, uc.DeleteSource(ctx, testWorkspaceID, created.ID)).Required()

		_, err = repo.Source().Get(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, testWorkspaceID, "")
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, testWorkspaceID, "non-existent-id")
		gt.Value(t, err).NotNil()
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		retrieved, err := uc.GetSource(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Name).Equal("Test Source")
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, testWorkspaceID, "")
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, testWorkspaceID, "non-existent-id")
		gt.Value(t, err).NotNil()
	})
}

func TestSourceUseCase_ListSources(t *testing.T) {
	t.Run("lists all sources", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		sources, err := uc.ListSources(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, sources).Length(0)

		source1 := &model.Source{Name: "Source 1", SourceType: model.SourceTypeNotionDB, Enabled: true}
		source2 := &model.Source{Name: "Source 2", SourceType: model.SourceTypeNotionDB, Enabled: false}
		_, _ = repo.Source().Create(ctx, testWorkspaceID, source1)
		_, _ = repo.Source().Create(ctx, testWorkspaceID, source2)

		sources, err = uc.ListSources(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, sources).Length(2)
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
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).True()
		gt.Value(t, result.DatabaseTitle).Equal("Valid Database")
		gt.Value(t, result.DatabaseURL).Equal("https://notion.so/valid-db")
	})

	t.Run("returns invalid for empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "")
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).False()
		gt.String(t, result.ErrorMessage).NotEqual("")
	})

	t.Run("returns invalid when Notion service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "some-db-id")
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).False()
		gt.String(t, result.ErrorMessage).NotEqual("")
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
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).False()
		gt.String(t, result.ErrorMessage).NotEqual("")
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

		source, err := uc.CreateSlackSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.ID).NotEqual(model.SourceID(""))
		gt.Value(t, source.Name).Equal("My Slack Source")
		gt.Value(t, source.SourceType).Equal(model.SourceTypeSlack)
		gt.Value(t, source.SlackConfig).NotNil().Required()
		gt.Array(t, source.SlackConfig.Channels).Length(2)
		gt.Value(t, source.SlackConfig.Channels[0].ID).Equal("C001")
		gt.Value(t, source.SlackConfig.Channels[0].Name).Equal("general")
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

		source, err := uc.CreateSlackSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.Name).Equal("Slack Source")
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

		_, err := uc.CreateSlackSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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

		source, err := uc.CreateSlackSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.SlackConfig.Channels[0].Name).Equal("C001")
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		newName := "Updated"
		newDesc := "Updated description"
		newEnabled := false
		input := usecase.UpdateSlackSourceInput{
			ID:          created.ID,
			Name:        &newName,
			Description: &newDesc,
			Enabled:     &newEnabled,
		}

		updated, err := uc.UpdateSlackSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Name).Equal("Updated")
		gt.Value(t, updated.Description).Equal("Updated description")
		gt.Value(t, updated.Enabled).Equal(false)
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		input := usecase.UpdateSlackSourceInput{
			ID:         created.ID,
			ChannelIDs: []string{"C003"},
		}

		updated, err := uc.UpdateSlackSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Array(t, updated.SlackConfig.Channels).Length(1)
		gt.Value(t, updated.SlackConfig.Channels[0].ID).Equal("C003")
	})

	t.Run("fails with empty ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		input := usecase.UpdateSlackSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSlackSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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
		created, err := repo.Source().Create(ctx, testWorkspaceID, source)
		gt.NoError(t, err).Required()

		newName := "New Name"
		input := usecase.UpdateSlackSourceInput{
			ID:   created.ID,
			Name: &newName,
		}

		_, err = uc.UpdateSlackSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
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
		gt.NoError(t, err).Required()

		gt.Array(t, channels).Length(3)
		gt.Value(t, channels[0].ID).Equal("C001")
		gt.Value(t, channels[0].Name).Equal("general")
	})

	t.Run("fails when slack service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil)
		ctx := context.Background()

		_, err := uc.ListSlackChannels(ctx)
		gt.Value(t, err).NotNil()
	})
}
