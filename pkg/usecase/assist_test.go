package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestAssistUseCase_BuildAssistSystemPrompt(t *testing.T) {
	t.Run("renders template with all sections including DueDate", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := context.Background()

		// Create a case
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Security Incident", "Suspicious login detected", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create actions with and without DueDate
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		dueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Investigate logs", "Check auth logs", []string{"U001"}, "", types.ActionStatusInProgress, &dueDate)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Update firewall", "Block suspicious IP", []string{}, "", types.ActionStatusTodo, nil)
		gt.NoError(t, err).Required()

		// Create workspace entry with assist prompt
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Check deadlines and follow up on pending items.",
		})

		assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		// Verify template renders correctly
		gt.Value(t, strings.Contains(prompt, "Security Incident")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Suspicious login detected")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Investigate logs")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Update firewall")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Due: 2026-03-15")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Check deadlines and follow up on pending items.")).Equal(true)
	})

	t.Run("renders template with no actions or messages", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Empty Case", "No actions yet", []string{}, nil)
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Monitor this case.",
		})

		assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "Empty Case")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Monitor this case.")).Equal(true)
		// Actions section should not appear
		gt.Value(t, strings.Contains(prompt, "## Actions")).Equal(false)
	})

	t.Run("renders template with assist logs and memories", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Test", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create an assist log
		assistLog := &model.AssistLog{
			CaseID:    c.ID,
			Summary:   "Reviewed deadlines and followed up",
			Actions:   "Checked deadlines",
			Reasoning: "Two actions were overdue",
			NextSteps: "Follow up next week",
		}
		_, err = repo.AssistLog().Create(ctx, testWorkspaceID, c.ID, assistLog)
		gt.NoError(t, err).Required()

		// Create a memory
		mem := &model.Memory{
			CaseID:    c.ID,
			Claim:     "User prefers email notifications",
			Embedding: make([]float32, 768),
		}
		_, err = repo.Memory().Create(ctx, testWorkspaceID, c.ID, mem)
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
			AssistPrompt: "Assist this case.",
		})

		assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "Reviewed deadlines and followed up")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Checked deadlines")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Two actions were overdue")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Follow up next week")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "User prefers email notifications")).Equal(true)
	})
}

func TestAssistUseCase_BuildAssistSystemPrompt_Language(t *testing.T) {
	t.Run("includes language instruction when language is set", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil)
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:      model.Workspace{ID: testWorkspaceID, Name: "Test"},
			AssistPrompt:   "Check deadlines.",
			AssistLanguage: "Japanese",
		})

		assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "## Language")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "You MUST respond and write all messages in Japanese.")).Equal(true)
	})

	t.Run("omits language section when language is empty", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Desc", []string{}, nil)
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:    model.Workspace{ID: testWorkspaceID, Name: "Test"},
			AssistPrompt: "Check deadlines.",
		})

		assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
		entry, err := registry.Get(testWorkspaceID)
		gt.NoError(t, err).Required()

		prompt, err := usecase.BuildAssistSystemPrompt(assistUC, ctx, entry, c, usecase.AssistOption{LogCount: 7, MessageCount: 50})
		gt.NoError(t, err).Required()

		gt.Value(t, strings.Contains(prompt, "## Language")).Equal(false)
	})
}

func TestAssistUseCase_RunAssist_SkipsWorkspaceWithoutAssistConfig(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	// Create workspace entry without assist prompt
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
		// AssistPrompt is empty - should be skipped
	})

	assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)
	err := assistUC.RunAssist(ctx, usecase.AssistOption{})
	gt.NoError(t, err)
}

func TestAssistUseCase_RunAssist_DefaultOptions(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	// Empty registry - no workspaces to process
	registry := model.NewWorkspaceRegistry()
	assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)

	err := assistUC.RunAssist(ctx, usecase.AssistOption{})
	gt.NoError(t, err)
}

func TestAssistUseCase_RunAssist_WorkspaceFilter(t *testing.T) {
	repo := memory.New()
	ctx := context.Background()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-1", Name: "Workspace 1"},
	})
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-2", Name: "Workspace 2"},
	})

	assistUC := usecase.NewAssistUseCase(repo, registry, nil, nil)

	// Filter to non-existent workspace should fail
	err := assistUC.RunAssist(ctx, usecase.AssistOption{WorkspaceID: "ws-nonexistent"})
	gt.Value(t, err).NotNil()

	// Filter to existing workspace (without assist prompt) should succeed and skip
	err = assistUC.RunAssist(ctx, usecase.AssistOption{WorkspaceID: "ws-1"})
	gt.NoError(t, err)
}
