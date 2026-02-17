package usecase_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack"
)

// compileTestNotionService is a mock implementation of notion.Service for compile tests
type compileTestNotionService struct {
	queryUpdatedPagesFn   func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error]
	getDatabaseMetadataFn func(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error)
}

func (m *compileTestNotionService) QueryUpdatedPages(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
	if m.queryUpdatedPagesFn != nil {
		return m.queryUpdatedPagesFn(ctx, dbID, since)
	}
	return func(yield func(*notion.Page, error) bool) {}
}

func (m *compileTestNotionService) GetDatabaseMetadata(ctx context.Context, dbID string) (*notion.DatabaseMetadata, error) {
	if m.getDatabaseMetadataFn != nil {
		return m.getDatabaseMetadataFn(ctx, dbID)
	}
	return &notion.DatabaseMetadata{ID: dbID, Title: "Test DB", URL: "https://notion.so/test"}, nil
}

// compileTestKnowledgeService is a mock implementation of knowledge.Service for compile tests
type compileTestKnowledgeService struct {
	extractFn func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error)
}

func (m *compileTestKnowledgeService) Extract(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
	if m.extractFn != nil {
		return m.extractFn(ctx, input)
	}
	return nil, nil
}

// compileTestSlackService embeds mockSlackService and adds tracking for PostMessage
type compileTestSlackService struct {
	mockSlackService
	postMessageFn    func(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error)
	postMessageCalls []postMessageCall
}

type postMessageCall struct {
	channelID string
	text      string
}

func (m *compileTestSlackService) PostMessage(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error) {
	m.postMessageCalls = append(m.postMessageCalls, postMessageCall{channelID: channelID, text: text})
	if m.postMessageFn != nil {
		return m.postMessageFn(ctx, channelID, blocks, text)
	}
	return "1234567890.123456", nil
}

// setupCompileTest creates test fixtures for compile use case tests
func setupCompileTest(t *testing.T, notionSvc notion.Service, knowledgeSvc knowledge.Service, slackSvc slack.Service) (*usecase.CompileUseCase, *memory.Repository) {
	t.Helper()

	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:     model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		CompilePrompt: "Analyze security relevance",
	})

	uc := usecase.NewCompileUseCase(
		repo,
		registry,
		notionSvc,
		knowledgeSvc,
		slackSvc,
		"https://example.com",
	)

	return uc, repo
}

// createTestSource creates a test Notion DB source in the repository
func createTestSource(t *testing.T, repo interfaces.Repository, wsID string, enabled bool) *model.Source {
	t.Helper()
	ctx := context.Background()

	source := &model.Source{
		Name:       "Test Notion Source",
		SourceType: model.SourceTypeNotionDB,
		Enabled:    enabled,
		NotionDBConfig: &model.NotionDBConfig{
			DatabaseID:    "db-123",
			DatabaseTitle: "Test DB",
		},
	}
	created, err := repo.Source().Create(ctx, wsID, source)
	gt.NoError(t, err).Required()
	return created
}

// createTestCase creates a test case in the repository
func createTestCase(t *testing.T, repo interfaces.Repository, wsID string, title string, status types.CaseStatus, slackChannelID string) *model.Case {
	t.Helper()
	ctx := context.Background()

	c := &model.Case{
		Title:          title,
		Description:    "Test case description",
		Status:         status,
		SlackChannelID: slackChannelID,
	}
	created, err := repo.Case().Create(ctx, wsID, c)
	gt.NoError(t, err).Required()
	return created
}

func TestCompileUseCase_Compile(t *testing.T) {
	t.Run("normal case with knowledge extraction and slack notification", func(t *testing.T) {
		now := time.Now().UTC()
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: now,
					}
					yield(page, nil)
				}
			},
		}

		var extractedInput knowledge.Input
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				extractedInput = input
				return []knowledge.Result{
					{
						CaseID:    0, // Will be set after case creation
						Title:     "Test Knowledge",
						Summary:   "Summary of knowledge",
						Embedding: []float32{0.1, 0.2, 0.3},
					},
				}, nil
			},
		}

		slackSvc := &compileTestSlackService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, slackSvc)
		ctx := context.Background()

		// Create source and case
		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Security Risk", types.CaseStatusOpen, "C12345")

		// Update the mock to return the correct case ID
		knowledgeSvc.extractFn = func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
			extractedInput = input
			return []knowledge.Result{
				{
					CaseID:    testCase.ID,
					Title:     "Test Knowledge",
					Summary:   "Summary of knowledge",
					Embedding: []float32{0.1, 0.2, 0.3},
				},
			}, nil
		}

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, result.WorkspaceResults).Length(1)
		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.WorkspaceID).Equal(testWorkspaceID)
		gt.Value(t, wsResult.SourcesProcessed).Equal(1)
		gt.Value(t, wsResult.PagesProcessed).Equal(1)
		gt.Value(t, wsResult.KnowledgeCreated).Equal(1)
		gt.Value(t, wsResult.Notifications).Equal(1)
		gt.Value(t, wsResult.Errors).Equal(0)

		// Verify knowledge was saved
		knowledges, err := repo.Knowledge().ListByCaseID(ctx, testWorkspaceID, testCase.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, knowledges).Length(1)
		gt.Value(t, knowledges[0].Title).Equal("Test Knowledge")
		gt.Value(t, knowledges[0].Summary).Equal("Summary of knowledge")
		gt.Array(t, knowledges[0].SourceURLs).Length(1)
		gt.Value(t, knowledges[0].SourceURLs[0]).Equal("https://notion.so/page-1")

		// Verify Slack notification
		gt.Array(t, slackSvc.postMessageCalls).Length(1)
		gt.Value(t, slackSvc.postMessageCalls[0].channelID).Equal("C12345")

		// Verify the prompt was passed through
		gt.Value(t, extractedInput.Prompt).Equal("Analyze security relevance")
	})

	t.Run("no sources found", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, _ := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, result.WorkspaceResults).Length(1)
		gt.Value(t, result.WorkspaceResults[0].SourcesProcessed).Equal(0)
		gt.Value(t, result.WorkspaceResults[0].PagesProcessed).Equal(0)
	})

	t.Run("no OPEN cases found", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		// Create source but only closed case
		createTestSource(t, repo, testWorkspaceID, true)
		createTestCase(t, repo, testWorkspaceID, "Closed Case", types.CaseStatusClosed, "C12345")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, result.WorkspaceResults).Length(1)
		gt.Value(t, result.WorkspaceResults[0].SourcesProcessed).Equal(0)
		gt.Value(t, result.WorkspaceResults[0].PagesProcessed).Equal(0)
	})

	t.Run("skips disabled sources", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, false) // disabled
		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Array(t, result.WorkspaceResults).Length(1)
		gt.Value(t, result.WorkspaceResults[0].SourcesProcessed).Equal(0)
	})

	t.Run("skips Slack source without SlackConfig", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		// Create a Slack source without SlackConfig
		slackSource := &model.Source{
			Name:       "Slack Source",
			SourceType: model.SourceTypeSlack,
			Enabled:    true,
		}
		_, err := repo.Source().Create(ctx, testWorkspaceID, slackSource)
		gt.NoError(t, err).Required()

		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].SourcesProcessed).Equal(0)
	})

	t.Run("workspace filtering", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}

		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-1", Name: "Workspace 1"},
		})
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-2", Name: "Workspace 2"},
		})

		uc := usecase.NewCompileUseCase(repo, registry, notionSvc, knowledgeSvc, nil, "")
		ctx := context.Background()

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since:       time.Now().Add(-24 * time.Hour),
			WorkspaceID: "ws-1",
		})
		gt.NoError(t, err).Required()

		// Only ws-1 should be processed
		gt.Array(t, result.WorkspaceResults).Length(1)
		gt.Value(t, result.WorkspaceResults[0].WorkspaceID).Equal("ws-1")
	})

	t.Run("page fetch error aborts source and increments errors", func(t *testing.T) {
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					yield(nil, errors.New("notion API error"))
				}
			},
		}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].Errors).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].PagesProcessed).Equal(0)
	})

	t.Run("LLM extract error skips page and increments errors", func(t *testing.T) {
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: time.Now().UTC(),
					}
					yield(page, nil)
				}
			},
		}
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return nil, errors.New("LLM extraction failed")
			},
		}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: time.Now().Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].Errors).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].PagesProcessed).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].KnowledgeCreated).Equal(0)
	})

	t.Run("slack notification failure does not block knowledge creation", func(t *testing.T) {
		now := time.Now().UTC()
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: now,
					}
					yield(page, nil)
				}
			},
		}

		slackSvc := &compileTestSlackService{
			postMessageFn: func(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error) {
				return "", errors.New("slack API error")
			},
		}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Test Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1, 0.2},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, slackSvc)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")
		caseID = testCase.ID

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		// Knowledge was still created despite Slack failure
		gt.Value(t, result.WorkspaceResults[0].KnowledgeCreated).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].Notifications).Equal(0)
	})

	t.Run("no slack notification when SlackChannelID is empty", func(t *testing.T) {
		now := time.Now().UTC()
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: now,
					}
					yield(page, nil)
				}
			},
		}

		slackSvc := &compileTestSlackService{}
		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Test Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1, 0.2},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, slackSvc)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "") // No Slack channel
		caseID = testCase.ID

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].KnowledgeCreated).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].Notifications).Equal(0)
		gt.Array(t, slackSvc.postMessageCalls).Length(0)
	})

	t.Run("no slack notification when slack service is nil", func(t *testing.T) {
		now := time.Now().UTC()
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: now,
					}
					yield(page, nil)
				}
			},
		}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Test Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1, 0.2},
					},
				}, nil
			},
		}

		// nil slack service
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")
		caseID = testCase.ID

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].KnowledgeCreated).Equal(1)
		gt.Value(t, result.WorkspaceResults[0].Notifications).Equal(0)
	})

	t.Run("multiple pages and results", func(t *testing.T) {
		now := time.Now().UTC()
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					page1 := &notion.Page{
						ID:             "page-1",
						URL:            "https://notion.so/page-1",
						LastEditedTime: now,
					}
					if !yield(page1, nil) {
						return
					}
					page2 := &notion.Page{
						ID:             "page-2",
						URL:            "https://notion.so/page-2",
						LastEditedTime: now,
					}
					yield(page2, nil)
				}
			},
		}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Knowledge from " + input.SourceData.SourceURLs[0],
						Summary:   "Summary",
						Embedding: []float32{0.1, 0.2},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")
		caseID = testCase.ID

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].PagesProcessed).Equal(2)
		gt.Value(t, result.WorkspaceResults[0].KnowledgeCreated).Equal(2)
	})

	t.Run("statistics accuracy with mixed results", func(t *testing.T) {
		now := time.Now().UTC()
		callCount := 0
		notionSvc := &compileTestNotionService{
			queryUpdatedPagesFn: func(ctx context.Context, dbID string, since time.Time) iter.Seq2[*notion.Page, error] {
				return func(yield func(*notion.Page, error) bool) {
					// Page 1: success
					if !yield(&notion.Page{ID: "page-1", URL: "https://notion.so/page-1", LastEditedTime: now}, nil) {
						return
					}
					// Page 2: success
					if !yield(&notion.Page{ID: "page-2", URL: "https://notion.so/page-2", LastEditedTime: now}, nil) {
						return
					}
					// Page 3: success
					yield(&notion.Page{ID: "page-3", URL: "https://notion.so/page-3", LastEditedTime: now}, nil)
				}
			},
		}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				callCount++
				if callCount == 2 {
					// Second page fails extraction
					return nil, errors.New("extraction failed")
				}
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSource(t, repo, testWorkspaceID, true)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")
		caseID = testCase.ID

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: now.Add(-24 * time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.SourcesProcessed).Equal(1)
		gt.Value(t, wsResult.PagesProcessed).Equal(3)
		gt.Value(t, wsResult.KnowledgeCreated).Equal(2) // Pages 1 and 3 succeeded
		gt.Value(t, wsResult.Errors).Equal(1)           // Page 2 failed
	})
}

// createTestSlackSource creates a test Slack source in the repository
func createTestSlackSource(t *testing.T, repo interfaces.Repository, wsID string, enabled bool, channels []model.SlackChannel) *model.Source {
	t.Helper()
	ctx := context.Background()

	source := &model.Source{
		Name:       "Test Slack Source",
		SourceType: model.SourceTypeSlack,
		Enabled:    enabled,
		SlackConfig: &model.SlackConfig{
			Channels: channels,
		},
	}
	created, err := repo.Source().Create(ctx, wsID, source)
	gt.NoError(t, err).Required()
	return created
}

// seedSlackMessages inserts messages into the repository for testing
func seedSlackMessages(t *testing.T, repo interfaces.Repository, messages []*slackmodel.Message) {
	t.Helper()
	ctx := context.Background()
	for _, msg := range messages {
		err := repo.Slack().PutMessage(ctx, msg)
		gt.NoError(t, err).Required()
	}
}

func TestCompileUseCase_Slack(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("normal case with Slack messages and knowledge extraction", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		slackSvc := &compileTestSlackService{}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Slack Knowledge",
						Summary:   "Summary from Slack",
						Embedding: []float32{0.1, 0.2, 0.3},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, slackSvc)
		ctx := context.Background()

		channels := []model.SlackChannel{{ID: "C001", Name: "general"}}
		createTestSlackSource(t, repo, testWorkspaceID, true, channels)
		testCase := createTestCase(t, repo, testWorkspaceID, "Security Risk", types.CaseStatusOpen, "C12345")
		caseID = testCase.ID

		// Seed messages
		seedSlackMessages(t, repo, []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C001", "", "T001", "U001", "alice", "Hello world", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312860.000200", "C001", "1705312800.000100", "T001", "U002", "bob", "Hi alice!", "", baseTime.Add(time.Minute), nil),
		})

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.SourcesProcessed).Equal(1)
		gt.Value(t, wsResult.PagesProcessed).Equal(1)
		gt.Value(t, wsResult.KnowledgeCreated).Equal(1)
		gt.Value(t, wsResult.Notifications).Equal(1)
		gt.Value(t, wsResult.Errors).Equal(0)

		// Verify knowledge was saved
		knowledges, err := repo.Knowledge().ListByCaseID(ctx, testWorkspaceID, testCase.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, knowledges).Length(1)
		gt.Value(t, knowledges[0].Title).Equal("Slack Knowledge")
		gt.Value(t, knowledges[0].Summary).Equal("Summary from Slack")

		// Verify Slack notification
		gt.Array(t, slackSvc.postMessageCalls).Length(1)
		gt.Value(t, slackSvc.postMessageCalls[0].channelID).Equal("C12345")
	})

	t.Run("skips channel with no messages", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				t.Fatal("Extract should not be called when there are no messages")
				return nil, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		channels := []model.SlackChannel{{ID: "C-empty", Name: "empty-channel"}}
		createTestSlackSource(t, repo, testWorkspaceID, true, channels)
		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.SourcesProcessed).Equal(1)
		gt.Value(t, wsResult.PagesProcessed).Equal(0)
		gt.Value(t, wsResult.KnowledgeCreated).Equal(0)
		gt.Value(t, wsResult.Errors).Equal(0)
	})

	t.Run("multiple channels processing", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}

		var extractCalls int
		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				extractCalls++
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Knowledge " + input.SourceData.Content[:20],
						Summary:   "Summary",
						Embedding: []float32{0.1},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		channels := []model.SlackChannel{
			{ID: "C100", Name: "channel-a"},
			{ID: "C200", Name: "channel-b"},
		}
		createTestSlackSource(t, repo, testWorkspaceID, true, channels)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")
		caseID = testCase.ID

		// Seed messages for both channels
		seedSlackMessages(t, repo, []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C100", "", "T001", "U001", "alice", "Msg in channel-a", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312800.000200", "C200", "", "T001", "U002", "bob", "Msg in channel-b", "", baseTime, nil),
		})

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.PagesProcessed).Equal(2) // 2 channels
		gt.Value(t, wsResult.KnowledgeCreated).Equal(2)
		gt.Value(t, extractCalls).Equal(2)
	})

	t.Run("LLM extract error continues to next channel", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}

		var extractCalls int
		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				extractCalls++
				if extractCalls == 1 {
					return nil, errors.New("LLM extraction failed")
				}
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		channels := []model.SlackChannel{
			{ID: "C300", Name: "ch-fail"},
			{ID: "C400", Name: "ch-ok"},
		}
		createTestSlackSource(t, repo, testWorkspaceID, true, channels)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")
		caseID = testCase.ID

		seedSlackMessages(t, repo, []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C300", "", "T001", "U001", "alice", "Msg in ch-fail", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312800.000200", "C400", "", "T001", "U002", "bob", "Msg in ch-ok", "", baseTime, nil),
		})

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.PagesProcessed).Equal(2)
		gt.Value(t, wsResult.KnowledgeCreated).Equal(1) // Only second channel succeeded
		gt.Value(t, wsResult.Errors).Equal(1)
	})

	t.Run("skips Slack source with empty channels", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		knowledgeSvc := &compileTestKnowledgeService{}
		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, nil)
		ctx := context.Background()

		createTestSlackSource(t, repo, testWorkspaceID, true, []model.SlackChannel{})
		createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "")

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		gt.Value(t, result.WorkspaceResults[0].SourcesProcessed).Equal(0)
	})

	t.Run("Slack notification failure does not block knowledge creation", func(t *testing.T) {
		notionSvc := &compileTestNotionService{}
		slackSvc := &compileTestSlackService{
			postMessageFn: func(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error) {
				return "", errors.New("slack API error")
			},
		}

		var caseID int64
		knowledgeSvc := &compileTestKnowledgeService{
			extractFn: func(ctx context.Context, input knowledge.Input) ([]knowledge.Result, error) {
				return []knowledge.Result{
					{
						CaseID:    caseID,
						Title:     "Slack Knowledge",
						Summary:   "Summary",
						Embedding: []float32{0.1},
					},
				}, nil
			},
		}

		uc, repo := setupCompileTest(t, notionSvc, knowledgeSvc, slackSvc)
		ctx := context.Background()

		channels := []model.SlackChannel{{ID: "C500", Name: "notify-fail"}}
		createTestSlackSource(t, repo, testWorkspaceID, true, channels)
		testCase := createTestCase(t, repo, testWorkspaceID, "Open Case", types.CaseStatusOpen, "C12345")
		caseID = testCase.ID

		seedSlackMessages(t, repo, []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C500", "", "T001", "U001", "alice", "Hello", "", baseTime, nil),
		})

		result, err := uc.Compile(ctx, usecase.CompileOption{
			Since: baseTime.Add(-time.Hour),
		})
		gt.NoError(t, err).Required()

		wsResult := result.WorkspaceResults[0]
		gt.Value(t, wsResult.KnowledgeCreated).Equal(1)
		gt.Value(t, wsResult.Notifications).Equal(0)
	})
}

func TestBuildThreadedMarkdown(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	ch := model.SlackChannel{ID: "C001", Name: "general"}

	t.Run("root messages only", func(t *testing.T) {
		messages := []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C001", "", "T001", "U001", "alice", "First message", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312860.000200", "C001", "", "T001", "U002", "bob", "Second message", "", baseTime.Add(time.Minute), nil),
		}

		md := usecase.BuildThreadedMarkdown(messages, ch)

		gt.Value(t, strings.Contains(md, "# Slack Channel: #general (C001)")).Equal(true)
		gt.Value(t, strings.Contains(md, "## Message by alice at "+baseTime.Format(time.DateTime))).Equal(true)
		gt.Value(t, strings.Contains(md, "First message")).Equal(true)
		gt.Value(t, strings.Contains(md, "## Message by bob at "+baseTime.Add(time.Minute).Format(time.DateTime))).Equal(true)
		gt.Value(t, strings.Contains(md, "Second message")).Equal(true)
		gt.Value(t, strings.Contains(md, "---")).Equal(true)
	})

	t.Run("root message with replies", func(t *testing.T) {
		messages := []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312800.000100", "C001", "", "T001", "U001", "alice", "Root message", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312860.000200", "C001", "1705312800.000100", "T001", "U002", "bob", "Reply 1", "", baseTime.Add(time.Minute), nil),
			slackmodel.NewMessageFromData("1705312920.000300", "C001", "1705312800.000100", "T001", "U003", "charlie", "Reply 2", "", baseTime.Add(2*time.Minute), nil),
		}

		md := usecase.BuildThreadedMarkdown(messages, ch)

		gt.Value(t, strings.Contains(md, "## Message by alice")).Equal(true)
		gt.Value(t, strings.Contains(md, "Root message")).Equal(true)
		gt.Value(t, strings.Contains(md, "### Reply by bob")).Equal(true)
		gt.Value(t, strings.Contains(md, "Reply 1")).Equal(true)
		gt.Value(t, strings.Contains(md, "### Reply by charlie")).Equal(true)
		gt.Value(t, strings.Contains(md, "Reply 2")).Equal(true)
		// Should NOT contain separator between root and its replies
		gt.Value(t, strings.Count(md, "---")).Equal(0)
	})

	t.Run("orphan reply treated as standalone", func(t *testing.T) {
		messages := []*slackmodel.Message{
			slackmodel.NewMessageFromData("1705312860.000200", "C001", "1705312700.000000", "T001", "U002", "bob", "Orphan reply", "", baseTime.Add(time.Minute), nil),
		}

		md := usecase.BuildThreadedMarkdown(messages, ch)

		// Orphan reply should appear as a standalone message (## not ###)
		gt.Value(t, strings.Contains(md, "## Message by bob")).Equal(true)
		gt.Value(t, strings.Contains(md, "Orphan reply")).Equal(true)
	})

	t.Run("empty messages", func(t *testing.T) {
		md := usecase.BuildThreadedMarkdown([]*slackmodel.Message{}, ch)

		gt.Value(t, strings.Contains(md, "# Slack Channel: #general (C001)")).Equal(true)
		// No messages, just the header
		gt.Value(t, strings.Contains(md, "## Message")).Equal(false)
	})

	t.Run("messages sorted by CreatedAt ascending", func(t *testing.T) {
		messages := []*slackmodel.Message{
			// Insert in reverse order
			slackmodel.NewMessageFromData("1705312920.000300", "C001", "", "T001", "U003", "charlie", "Third", "", baseTime.Add(2*time.Minute), nil),
			slackmodel.NewMessageFromData("1705312800.000100", "C001", "", "T001", "U001", "alice", "First", "", baseTime, nil),
			slackmodel.NewMessageFromData("1705312860.000200", "C001", "", "T001", "U002", "bob", "Second", "", baseTime.Add(time.Minute), nil),
		}

		md := usecase.BuildThreadedMarkdown(messages, ch)

		aliceIdx := strings.Index(md, "alice")
		bobIdx := strings.Index(md, "bob")
		charlieIdx := strings.Index(md, "charlie")
		gt.Value(t, aliceIdx < bobIdx).Equal(true)
		gt.Value(t, bobIdx < charlieIdx).Equal(true)
	})
}

func TestBuildSlackSourceURLs(t *testing.T) {
	baseTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("generates URLs for root messages only", func(t *testing.T) {
		messages := []*slackmodel.Message{
			slackmodel.NewMessageFromData("1234567890.123456", "C001", "", "T001", "U001", "alice", "Root", "", baseTime, nil),
			slackmodel.NewMessageFromData("1234567891.123457", "C001", "1234567890.123456", "T001", "U002", "bob", "Reply", "", baseTime.Add(time.Minute), nil),
			slackmodel.NewMessageFromData("1234567892.123458", "C001", "", "T001", "U003", "charlie", "Another root", "", baseTime.Add(2*time.Minute), nil),
		}

		urls := usecase.BuildSlackSourceURLs(messages)

		gt.Array(t, urls).Length(2) // Only root messages
		gt.Value(t, urls[0]).Equal("https://app.slack.com/client/T001/C001/p1234567890123456")
		gt.Value(t, urls[1]).Equal("https://app.slack.com/client/T001/C001/p1234567892123458")
	})

	t.Run("returns nil when no teamID", func(t *testing.T) {
		messages := []*slackmodel.Message{
			slackmodel.NewMessageFromData("1234567890.123456", "C001", "", "", "U001", "alice", "Root", "", baseTime, nil),
		}

		urls := usecase.BuildSlackSourceURLs(messages)
		gt.Value(t, urls == nil).Equal(true)
	})

	t.Run("empty messages returns nil", func(t *testing.T) {
		urls := usecase.BuildSlackSourceURLs([]*slackmodel.Message{})
		gt.Value(t, urls == nil).Equal(true)
	})
}
