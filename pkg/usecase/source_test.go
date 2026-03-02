package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/github"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// sourceTestNotionService is a mock implementation of notion.Service for testing
type sourceTestNotionService struct {
	getDatabaseMetadataFn       func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error)
	getPageMetadataFn           func(ctx context.Context, pageID string) (*notion.PageMetadata, error)
	queryUpdatedPagesFromPageFn func(ctx context.Context, pageID string, since time.Time, recursive bool, maxDepth int) iter.Seq2[*notion.Page, error]
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

func (m *sourceTestNotionService) GetPageMetadata(ctx context.Context, pageID string) (*notion.PageMetadata, error) {
	if m.getPageMetadataFn != nil {
		return m.getPageMetadataFn(ctx, pageID)
	}
	return &notion.PageMetadata{
		ID:    pageID,
		Title: "Test Page",
		URL:   "https://notion.so/test-page",
	}, nil
}

func (m *sourceTestNotionService) QueryUpdatedPagesFromPage(ctx context.Context, pageID string, since time.Time, recursive bool, maxDepth int) iter.Seq2[*notion.Page, error] {
	if m.queryUpdatedPagesFromPageFn != nil {
		return m.queryUpdatedPagesFromPageFn(ctx, pageID, since, recursive, maxDepth)
	}
	return func(yield func(*notion.Page, error) bool) {}
}

// mockSlackService is a mock implementation of slack.Service for testing
type mockSlackService struct {
	listJoinedChannelsFn     func(ctx context.Context) ([]slack.Channel, error)
	getChannelNamesFn        func(ctx context.Context, ids []string) (map[string]string, error)
	getUserInfoFn            func(ctx context.Context, userID string) (*slack.User, error)
	listUsersFn              func(ctx context.Context) ([]*slack.User, error)
	createChannelFn          func(ctx context.Context, caseID int64, caseName string, prefix string) (string, error)
	renameChannelFn          func(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error
	inviteUsersToChannelFn   func(ctx context.Context, channelID string, userIDs []string) error
	addBookmarkFn            func(ctx context.Context, channelID, title, link string) error
	getConversationMembersFn func(ctx context.Context, channelID string) ([]string, error)
	invitedChannelID         string
	invitedUserIDs           []string
	bookmarkChannelID        string
	bookmarkTitle            string
	bookmarkLink             string
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

func (m *mockSlackService) CreateChannel(ctx context.Context, caseID int64, caseName string, prefix string, _ bool) (string, error) {
	if m.createChannelFn != nil {
		return m.createChannelFn(ctx, caseID, caseName, prefix)
	}
	return "C" + caseName, nil
}

func (m *mockSlackService) GetConversationMembers(ctx context.Context, channelID string) ([]string, error) {
	if m.getConversationMembersFn != nil {
		return m.getConversationMembersFn(ctx, channelID)
	}
	return nil, nil
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

func (m *mockSlackService) AddBookmark(ctx context.Context, channelID, title, link string) error {
	m.bookmarkChannelID = channelID
	m.bookmarkTitle = title
	m.bookmarkLink = link
	if m.addBookmarkFn != nil {
		return m.addBookmarkFn(ctx, channelID, title, link)
	}
	return nil
}

func (m *mockSlackService) GetTeamURL(ctx context.Context) (string, error) {
	return "https://test-team.slack.com", nil
}

func (m *mockSlackService) PostMessage(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error) {
	return "1234567890.123456", nil
}

func (m *mockSlackService) UpdateMessage(ctx context.Context, channelID string, timestamp string, blocks []goslack.Block, text string) error {
	return nil
}

func (m *mockSlackService) GetConversationReplies(ctx context.Context, channelID string, threadTS string, limit int) ([]slack.ConversationMessage, error) {
	return nil, nil
}

func (m *mockSlackService) GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slack.ConversationMessage, error) {
	return nil, nil
}

func (m *mockSlackService) PostThreadReply(ctx context.Context, channelID string, threadTS string, text string) (string, error) {
	return "1234567890.999999", nil
}

func (m *mockSlackService) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []goslack.Block, text string) (string, error) {
	return "1234567890.999999", nil
}

func (m *mockSlackService) GetBotUserID(ctx context.Context) (string, error) {
	return "UBOT001", nil
}

func (m *mockSlackService) OpenView(ctx context.Context, triggerID string, view goslack.ModalViewRequest) error {
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
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID:  "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
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
			gt.Value(t, source.NotionDBConfig.DatabaseID).Equal("a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6")
			gt.Value(t, source.NotionDBConfig.DatabaseTitle).Equal("My Database")
		}
	})

	t.Run("uses custom name when provided", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &sourceTestNotionService{}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			Name:       "Custom Name",
			DatabaseID: "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
			Enabled:    true,
		}

		source, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.NoError(t, err).Required()

		gt.Value(t, source.Name).Equal("Custom Name")
	})

	t.Run("fails with empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			DatabaseID: "b1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
			Enabled:    true,
		}

		_, err := uc.CreateNotionDBSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
	})

	t.Run("creates source without Notion service", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionDBSourceInput{
			Name:       "Manual Source",
			DatabaseID: "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		input := usecase.UpdateSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, testWorkspaceID, "")
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		err := uc.DeleteSource(ctx, testWorkspaceID, "non-existent-id")
		gt.Value(t, err).NotNil()
	})
}

func TestSourceUseCase_GetSource(t *testing.T) {
	t.Run("gets existing source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, testWorkspaceID, "")
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-existent source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		_, err := uc.GetSource(ctx, testWorkspaceID, "non-existent-id")
		gt.Value(t, err).NotNil()
	})
}

func TestSourceUseCase_ListSources(t *testing.T) {
	t.Run("lists all sources", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "c1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6")
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).True()
		gt.Value(t, result.DatabaseTitle).Equal("Valid Database")
		gt.Value(t, result.DatabaseURL).Equal("https://notion.so/valid-db")
	})

	t.Run("returns invalid for empty database ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "")
		gt.NoError(t, err).Required()

		gt.Bool(t, result.Valid).False()
		gt.String(t, result.ErrorMessage).NotEqual("")
	})

	t.Run("returns invalid when Notion service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "d1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6")
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
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionDB(ctx, "b1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6")
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
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		input := usecase.UpdateSlackSourceInput{
			ID: "",
		}

		_, err := uc.UpdateSlackSource(ctx, testWorkspaceID, input)
		gt.Value(t, err).NotNil()
	})

	t.Run("fails for non-slack source", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
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
		uc := usecase.NewSourceUseCase(repo, nil, slackSvc, nil)
		ctx := context.Background()

		channels, err := uc.ListSlackChannels(ctx)
		gt.NoError(t, err).Required()

		gt.Array(t, channels).Length(3)
		gt.Value(t, channels[0].ID).Equal("C001")
		gt.Value(t, channels[0].Name).Equal("general")
	})

	t.Run("fails when slack service is not configured", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		_, err := uc.ListSlackChannels(ctx)
		gt.Value(t, err).NotNil()
	})
}

func TestCreateNotionPageSource(t *testing.T) {
	const wsID = "test-ws"

	t.Run("creates source with page metadata", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &sourceTestNotionService{
			getPageMetadataFn: func(ctx context.Context, pageID string) (*notion.PageMetadata, error) {
				return &notion.PageMetadata{
					ID:    pageID,
					Title: "My Notion Page",
					URL:   "https://notion.so/my-page",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionPageSourceInput{
			PageID:    "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			Enabled:   true,
			Recursive: true,
			MaxDepth:  2,
		}

		created, err := uc.CreateNotionPageSource(ctx, wsID, input)
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.SourceType).Equal(model.SourceTypeNotionPage)
		gt.Value(t, created.Name).Equal("My Notion Page")
		gt.Value(t, created.Enabled).Equal(true)
		gt.Value(t, created.NotionPageConfig).NotNil().Required()
		gt.Value(t, created.NotionPageConfig.PageTitle).Equal("My Notion Page")
		gt.Value(t, created.NotionPageConfig.PageURL).Equal("https://notion.so/my-page")
		gt.Value(t, created.NotionPageConfig.Recursive).Equal(true)
		gt.Value(t, created.NotionPageConfig.MaxDepth).Equal(2)
	})

	t.Run("uses custom name when provided", func(t *testing.T) {
		repo := memory.New()
		notionSvc := &sourceTestNotionService{}
		uc := usecase.NewSourceUseCase(repo, notionSvc, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionPageSourceInput{
			Name:    "Custom Name",
			PageID:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			Enabled: true,
		}

		created, err := uc.CreateNotionPageSource(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Name).Equal("Custom Name")
	})

	t.Run("returns error for invalid page ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionPageSourceInput{
			PageID:  "not-a-valid-id",
			Enabled: true,
		}

		_, err := uc.CreateNotionPageSource(ctx, wsID, input)
		gt.Value(t, err).NotNil()
		gt.Bool(t, errors.Is(err, model.ErrInvalidNotionID)).True()
	})

	t.Run("creates source without notion service", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)
		ctx := context.Background()

		input := usecase.CreateNotionPageSourceInput{
			Name:    "No Service Page",
			PageID:  "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			Enabled: true,
		}

		created, err := uc.CreateNotionPageSource(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Name).Equal("No Service Page")
		gt.Value(t, created.NotionPageConfig).NotNil().Required()
		gt.Value(t, created.NotionPageConfig.PageTitle).Equal("")
	})
}

func TestValidateNotionPage(t *testing.T) {
	t.Run("returns valid result for correct page ID", func(t *testing.T) {
		notionSvc := &sourceTestNotionService{
			getPageMetadataFn: func(ctx context.Context, pageID string) (*notion.PageMetadata, error) {
				return &notion.PageMetadata{
					ID:    pageID,
					Title: "Validated Page",
					URL:   "https://notion.so/validated",
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(memory.New(), notionSvc, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionPage(ctx, "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(true)
		gt.Value(t, result.PageTitle).Equal("Validated Page")
		gt.Value(t, result.PageURL).Equal("https://notion.so/validated")
		gt.Value(t, result.ErrorMessage).Equal("")
	})

	t.Run("returns invalid result for bad page ID format", func(t *testing.T) {
		uc := usecase.NewSourceUseCase(memory.New(), nil, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionPage(ctx, "not-a-valid-id")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(false)
		gt.String(t, result.ErrorMessage).NotEqual("")
	})

	t.Run("returns invalid when notion service is nil", func(t *testing.T) {
		uc := usecase.NewSourceUseCase(memory.New(), nil, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionPage(ctx, "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(false)
		gt.String(t, result.ErrorMessage).NotEqual("")
	})

	t.Run("returns invalid when page fetch fails", func(t *testing.T) {
		notionSvc := &sourceTestNotionService{
			getPageMetadataFn: func(ctx context.Context, pageID string) (*notion.PageMetadata, error) {
				return nil, errors.New("page not found")
			},
		}
		uc := usecase.NewSourceUseCase(memory.New(), notionSvc, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateNotionPage(ctx, "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(false)
		gt.String(t, result.ErrorMessage).NotEqual("")
	})
}

func TestCreateGitHubSource(t *testing.T) {
	t.Parallel()

	t.Run("creates GitHub source successfully", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		// Use a minimal mock GitHub service
		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(repo, nil, nil, githubSvc)

		input := usecase.CreateGitHubSourceInput{
			Name:         "My GitHub Source",
			Description:  "Test repositories",
			Repositories: []string{"secmon-lab/hecatoncheires", "https://github.com/secmon-lab/other-repo"},
			Enabled:      true,
		}

		created, err := uc.CreateGitHubSource(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Name).Equal("My GitHub Source")
		gt.Value(t, created.SourceType).Equal(model.SourceTypeGitHub)
		gt.Value(t, created.Enabled).Equal(true)
		gt.Value(t, created.GitHubConfig).NotNil()
		gt.A(t, created.GitHubConfig.Repositories).Length(2).Required()
		gt.Value(t, created.GitHubConfig.Repositories[0].Owner).Equal("secmon-lab")
		gt.Value(t, created.GitHubConfig.Repositories[0].Repo).Equal("hecatoncheires")
		gt.Value(t, created.GitHubConfig.Repositories[1].Owner).Equal("secmon-lab")
		gt.Value(t, created.GitHubConfig.Repositories[1].Repo).Equal("other-repo")
	})

	t.Run("uses default name when empty", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(repo, nil, nil, githubSvc)

		input := usecase.CreateGitHubSourceInput{
			Repositories: []string{"owner/repo"},
			Enabled:      true,
		}

		created, err := uc.CreateGitHubSource(ctx, wsID, input)
		gt.NoError(t, err).Required()
		gt.Value(t, created.Name).Equal("GitHub Source")
	})

	t.Run("returns error when GitHub service not configured", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)

		input := usecase.CreateGitHubSourceInput{
			Repositories: []string{"owner/repo"},
			Enabled:      true,
		}

		_, err := uc.CreateGitHubSource(ctx, wsID, input)
		gt.Error(t, err)
		gt.True(t, errors.Is(err, usecase.ErrGitHubNotConfigured))
	})

	t.Run("returns error for invalid repository format", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(repo, nil, nil, githubSvc)

		input := usecase.CreateGitHubSourceInput{
			Repositories: []string{"not-valid"},
			Enabled:      true,
		}

		_, err := uc.CreateGitHubSource(ctx, wsID, input)
		gt.Error(t, err)
	})

	t.Run("returns error when no repositories provided", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(repo, nil, nil, githubSvc)

		input := usecase.CreateGitHubSourceInput{
			Repositories: []string{},
			Enabled:      true,
		}

		_, err := uc.CreateGitHubSource(ctx, wsID, input)
		gt.Error(t, err)
	})
}

func TestUpdateGitHubSource(t *testing.T) {
	t.Parallel()

	t.Run("updates GitHub source fields", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(repo, nil, nil, githubSvc)

		// Create first
		created, err := uc.CreateGitHubSource(ctx, wsID, usecase.CreateGitHubSourceInput{
			Name:         "Original",
			Repositories: []string{"owner/repo"},
			Enabled:      true,
		})
		gt.NoError(t, err).Required()

		// Update
		newName := "Updated"
		updated, err := uc.UpdateGitHubSource(ctx, wsID, usecase.UpdateGitHubSourceInput{
			ID:           created.ID,
			Name:         &newName,
			Repositories: []string{"owner/repo", "owner/repo2"},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Name).Equal("Updated")
		gt.A(t, updated.GitHubConfig.Repositories).Length(2)
	})

	t.Run("returns error when GitHub service not configured", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		uc := usecase.NewSourceUseCase(repo, nil, nil, nil)

		_, err := uc.UpdateGitHubSource(ctx, wsID, usecase.UpdateGitHubSourceInput{
			ID: "some-id",
		})
		gt.Error(t, err)
		gt.True(t, errors.Is(err, usecase.ErrGitHubNotConfigured))
	})
}

func TestValidateGitHubRepo(t *testing.T) {
	t.Parallel()

	t.Run("returns invalid when GitHub service not configured", func(t *testing.T) {
		uc := usecase.NewSourceUseCase(memory.New(), nil, nil, nil)
		ctx := context.Background()

		result, err := uc.ValidateGitHubRepo(ctx, "owner/repo")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(false)
		gt.String(t, result.ErrorMessage).Contains("not configured")
	})

	t.Run("returns invalid for bad repo format", func(t *testing.T) {
		githubSvc := &sourceTestGitHubService{}
		uc := usecase.NewSourceUseCase(memory.New(), nil, nil, githubSvc)
		ctx := context.Background()

		result, err := uc.ValidateGitHubRepo(ctx, "not-valid")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(false)
		gt.String(t, result.ErrorMessage).Contains("invalid")
	})

	t.Run("returns valid result from GitHub service", func(t *testing.T) {
		githubSvc := &sourceTestGitHubService{
			validateFn: func(ctx context.Context, owner, repo string) (*github.RepositoryValidation, error) {
				return &github.RepositoryValidation{
					Valid:                true,
					Owner:                owner,
					Repo:                 repo,
					FullName:             owner + "/" + repo,
					Description:          "Test repo",
					IsPrivate:            false,
					PullRequestCount:     10,
					IssueCount:           5,
					CanFetchPullRequests: true,
					CanFetchIssues:       true,
				}, nil
			},
		}
		uc := usecase.NewSourceUseCase(memory.New(), nil, nil, githubSvc)
		ctx := context.Background()

		result, err := uc.ValidateGitHubRepo(ctx, "secmon-lab/hecatoncheires")
		gt.NoError(t, err).Required()
		gt.Value(t, result.Valid).Equal(true)
		gt.Value(t, result.Owner).Equal("secmon-lab")
		gt.Value(t, result.Repo).Equal("hecatoncheires")
		gt.Value(t, result.PullRequestCount).Equal(10)
		gt.Value(t, result.IssueCount).Equal(5)
	})
}

// sourceTestGitHubService is a minimal mock for GitHub service in source tests
type sourceTestGitHubService struct {
	validateFn func(ctx context.Context, owner, repo string) (*github.RepositoryValidation, error)
}

func (s *sourceTestGitHubService) FetchRecentPullRequests(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*github.PullRequest, error] {
	return func(yield func(*github.PullRequest, error) bool) {}
}

func (s *sourceTestGitHubService) FetchRecentIssues(ctx context.Context, owner, repo string, since time.Time) iter.Seq2[*github.Issue, error] {
	return func(yield func(*github.Issue, error) bool) {}
}

func (s *sourceTestGitHubService) FetchUpdatedIssueComments(ctx context.Context, owner, repo string, since time.Time, excludeNumbers map[int]struct{}) iter.Seq2[*github.IssueWithComments, error] {
	return func(yield func(*github.IssueWithComments, error) bool) {}
}

func (s *sourceTestGitHubService) ValidateRepository(ctx context.Context, owner, repo string) (*github.RepositoryValidation, error) {
	if s.validateFn != nil {
		return s.validateFn(ctx, owner, repo)
	}
	return &github.RepositoryValidation{Valid: true, Owner: owner, Repo: repo, FullName: owner + "/" + repo}, nil
}
