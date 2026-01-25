package usecase_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// mockNotionService is a mock implementation of notion.Service
type mockNotionService struct {
	pages    []*notion.Page
	queryErr error
}

func (m *mockNotionService) QueryUpdatedPages(_ context.Context, _ string, _ time.Time) iter.Seq2[*notion.Page, error] {
	return func(yield func(*notion.Page, error) bool) {
		if m.queryErr != nil {
			yield(nil, m.queryErr)
			return
		}
		for _, page := range m.pages {
			if !yield(page, nil) {
				return
			}
		}
	}
}

func (m *mockNotionService) GetDatabaseMetadata(_ context.Context, _ string) (*notion.DatabaseMetadata, error) {
	return &notion.DatabaseMetadata{
		ID:    "test-db",
		Title: "Test Database",
		URL:   "https://notion.so/test-db",
	}, nil
}

// mockKnowledgeService is a mock implementation of knowledge.Service
type mockKnowledgeService struct {
	results    []knowledge.Result
	extractErr error
}

func (m *mockKnowledgeService) Extract(_ context.Context, input knowledge.Input) ([]knowledge.Result, error) {
	if m.extractErr != nil {
		return nil, m.extractErr
	}
	return m.results, nil
}

func TestCompileUseCase_Execute(t *testing.T) {
	t.Run("Successfully extracts and saves knowledge", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a source
		source, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Test Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID: "test-db-id",
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		// Create a risk
		risk, err := repo.Risk().Create(ctx, &model.Risk{
			Name:        "Security Vulnerability",
			Description: "Risks related to security vulnerabilities",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		// Create mock services
		mockNotion := &mockNotionService{
			pages: []*notion.Page{
				{
					ID:             "page-1",
					URL:            "https://notion.so/page-1",
					LastEditedTime: time.Now().Add(-1 * time.Hour),
					Blocks: notion.Blocks{
						{
							ID:      "block-1",
							Type:    "paragraph",
							Content: map[string]interface{}{"text": "Security update about CVE-2024-1234"},
						},
					},
				},
			},
		}

		mockKnowledge := &mockKnowledgeService{
			results: []knowledge.Result{
				{
					RiskID:    risk.ID,
					Title:     "CVE-2024-1234 Update",
					Summary:   "A critical vulnerability was found",
					Embedding: []float32{0.1, 0.2, 0.3},
				},
			},
		}

		// Create use case
		uc := usecase.NewCompileUseCase(repo, mockNotion, mockKnowledge)

		// Execute
		result, err := uc.Execute(ctx, usecase.CompileInput{
			SourceIDs: []model.SourceID{source.ID},
			Since:     time.Now().Add(-24 * time.Hour),
			Until:     time.Now(),
		})
		if err != nil {
			t.Fatalf("failed to execute compile: %v", err)
		}

		if len(result.Knowledges) != 1 {
			t.Errorf("expected 1 knowledge, got %d", len(result.Knowledges))
		}

		if len(result.Errors) != 0 {
			t.Errorf("expected no errors, got %d", len(result.Errors))
		}

		// Verify knowledge was saved
		knowledges, err := repo.Knowledge().ListByRiskID(ctx, risk.ID)
		if err != nil {
			t.Fatalf("failed to list knowledges: %v", err)
		}

		if len(knowledges) != 1 {
			t.Errorf("expected 1 knowledge in repository, got %d", len(knowledges))
		}

		if knowledges[0].Title != "CVE-2024-1234 Update" {
			t.Errorf("expected title 'CVE-2024-1234 Update', got '%s'", knowledges[0].Title)
		}
	})

	t.Run("Returns empty result when no sources", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		mockNotion := &mockNotionService{}
		mockKnowledge := &mockKnowledgeService{}

		uc := usecase.NewCompileUseCase(repo, mockNotion, mockKnowledge)

		result, err := uc.Execute(ctx, usecase.CompileInput{
			Since: time.Now().Add(-24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Sources) != 0 {
			t.Errorf("expected 0 sources, got %d", len(result.Sources))
		}
		if len(result.Knowledges) != 0 {
			t.Errorf("expected 0 knowledges, got %d", len(result.Knowledges))
		}
	})

	t.Run("Returns empty result when no risks", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a source but no risks
		_, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Test Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID: "test-db-id",
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		mockNotion := &mockNotionService{}
		mockKnowledge := &mockKnowledgeService{}

		uc := usecase.NewCompileUseCase(repo, mockNotion, mockKnowledge)

		result, err := uc.Execute(ctx, usecase.CompileInput{
			Since: time.Now().Add(-24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Sources) != 1 {
			t.Errorf("expected 1 source, got %d", len(result.Sources))
		}
		if len(result.Knowledges) != 0 {
			t.Errorf("expected 0 knowledges, got %d", len(result.Knowledges))
		}
	})

	t.Run("Skips disabled sources", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a disabled source
		_, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Disabled Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    false,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID: "test-db-id",
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		// Create a risk
		_, err = repo.Risk().Create(ctx, &model.Risk{
			Name: "Test Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk: %v", err)
		}

		mockNotion := &mockNotionService{
			pages: []*notion.Page{
				{ID: "page-1", URL: "https://notion.so/page-1"},
			},
		}
		mockKnowledge := &mockKnowledgeService{
			results: []knowledge.Result{
				{RiskID: 1, Title: "Test", Summary: "Test"},
			},
		}

		uc := usecase.NewCompileUseCase(repo, mockNotion, mockKnowledge)

		result, err := uc.Execute(ctx, usecase.CompileInput{
			Since: time.Now().Add(-24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Source is disabled, so no sources should be processed
		if len(result.Sources) != 0 {
			t.Errorf("expected 0 sources (disabled excluded from enabled list), got %d", len(result.Sources))
		}
		if len(result.Knowledges) != 0 {
			t.Errorf("expected 0 knowledges, got %d", len(result.Knowledges))
		}
	})

	t.Run("Returns error when knowledge service is nil", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		mockNotion := &mockNotionService{}

		uc := usecase.NewCompileUseCase(repo, mockNotion, nil)

		_, err := uc.Execute(ctx, usecase.CompileInput{
			Since: time.Now().Add(-24 * time.Hour),
		})
		if err == nil {
			t.Error("expected error when knowledge service is nil")
		}
	})

	t.Run("Processes multiple pages and risks", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a source
		source, err := repo.Source().Create(ctx, &model.Source{
			Name:       "Test Source",
			SourceType: model.SourceTypeNotionDB,
			Enabled:    true,
			NotionDBConfig: &model.NotionDBConfig{
				DatabaseID: "test-db-id",
			},
		})
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}

		// Create risks
		risk1, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Security Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk1: %v", err)
		}

		risk2, err := repo.Risk().Create(ctx, &model.Risk{
			Name: "Compliance Risk",
		})
		if err != nil {
			t.Fatalf("failed to create risk2: %v", err)
		}

		// Create mock services
		mockNotion := &mockNotionService{
			pages: []*notion.Page{
				{
					ID:             "page-1",
					URL:            "https://notion.so/page-1",
					LastEditedTime: time.Now().Add(-1 * time.Hour),
					Blocks:         notion.Blocks{},
				},
				{
					ID:             "page-2",
					URL:            "https://notion.so/page-2",
					LastEditedTime: time.Now().Add(-2 * time.Hour),
					Blocks:         notion.Blocks{},
				},
			},
		}

		callCount := 0
		mockKnowledge := &mockKnowledgeService{}
		// Override to return different results for each call
		originalExtract := mockKnowledge.Extract
		_ = originalExtract // suppress unused warning

		// For simplicity, return both risks for each page
		mockKnowledge.results = []knowledge.Result{
			{RiskID: risk1.ID, Title: "Security Finding", Summary: "Security issue found"},
			{RiskID: risk2.ID, Title: "Compliance Finding", Summary: "Compliance issue found"},
		}

		uc := usecase.NewCompileUseCase(repo, mockNotion, mockKnowledge)

		result, err := uc.Execute(ctx, usecase.CompileInput{
			SourceIDs: []model.SourceID{source.ID},
			Since:     time.Now().Add(-24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("failed to execute compile: %v", err)
		}

		// 2 pages * 2 risks = 4 knowledges
		if len(result.Knowledges) != 4 {
			t.Errorf("expected 4 knowledges, got %d", len(result.Knowledges))
		}

		// Verify both risks have knowledges
		k1, _ := repo.Knowledge().ListByRiskID(ctx, risk1.ID)
		k2, _ := repo.Knowledge().ListByRiskID(ctx, risk2.ID)

		if len(k1) != 2 {
			t.Errorf("expected 2 knowledges for risk1, got %d", len(k1))
		}
		if len(k2) != 2 {
			t.Errorf("expected 2 knowledges for risk2, got %d", len(k2))
		}

		_ = callCount // suppress unused warning
	})
}
