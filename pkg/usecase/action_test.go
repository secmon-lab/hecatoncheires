package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestActionUseCase_CreateAction(t *testing.T) {
	t.Run("create action with valid case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		// Create case first
		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create action
		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Action Description", []string{"U001"}, "msg-123", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		gt.Number(t, created.ID).NotEqual(0)
		gt.Value(t, created.CaseID).Equal(c.ID)
		gt.Value(t, created.Title).Equal("Test Action")
		gt.Value(t, created.SlackMessageTS).Equal("msg-123")
		gt.Value(t, created.Status).Equal(types.ActionStatusTodo)
	})

	t.Run("create action without title fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "", "Description", []string{}, "", types.ActionStatusTodo)
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		_, err := actionUC.CreateAction(ctx, testWorkspaceID, 999, "Test Action", "Description", []string{}, "", types.ActionStatusTodo)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("create action with invalid status fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", "invalid-status")
		gt.Value(t, err).NotNil()
	})

	t.Run("create action with default status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", "")
		gt.NoError(t, err).Required()

		gt.Value(t, created.Status).Equal(types.ActionStatusTodo)
	})
}

func TestActionUseCase_UpdateAction(t *testing.T) {
	t.Run("update action title and status", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Original Title", "Original Description", []string{"U001"}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		newTitle := "Updated Title"
		newStatus := types.ActionStatusInProgress
		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, nil, &newTitle, nil, nil, nil, &newStatus)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
		gt.Value(t, updated.CaseID).Equal(c.ID)
	})

	t.Run("update action caseID", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil)
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		updated, err := actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, &c2.ID, nil, nil, nil, nil, nil)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.CaseID).Equal(c2.ID)
	})

	t.Run("update action with non-existent case fails", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		newCaseID := int64(999)
		_, err = actionUC.UpdateAction(ctx, testWorkspaceID, created.ID, &newCaseID, nil, nil, nil, nil, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrCaseNotFound)
	})

	t.Run("update non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		_, err := actionUC.UpdateAction(ctx, testWorkspaceID, 999, nil, nil, nil, nil, nil, nil)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_DeleteAction(t *testing.T) {
	t.Run("delete action", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		gt.NoError(t, actionUC.DeleteAction(ctx, testWorkspaceID, created.ID)).Required()

		_, err = actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("delete non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		err := actionUC.DeleteAction(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_GetAction(t *testing.T) {
	t.Run("get existing action", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		created, err := actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Test Action", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		retrieved, err := actionUC.GetAction(ctx, testWorkspaceID, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
	})

	t.Run("get non-existent action fails", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		_, err := actionUC.GetAction(ctx, testWorkspaceID, 999)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(usecase.ErrActionNotFound)
	})
}

func TestActionUseCase_ListActions(t *testing.T) {
	t.Run("list actions", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c, err := caseUC.CreateCase(ctx, testWorkspaceID, "Test Case", "Description", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 1", "Description 1", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c.ID, "Action 2", "Description 2", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(2)
	})
}

func TestActionUseCase_GetActionsByCase(t *testing.T) {
	t.Run("get actions by case", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil)
		actionUC := usecase.NewActionUseCase(repo)
		ctx := context.Background()

		c1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "Description 1", []string{}, nil)
		gt.NoError(t, err).Required()

		c2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "Description 2", []string{}, nil)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-1", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c1.ID, "Action 1-2", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, c2.ID, "Action 2-1", "Description", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		actions, err := actionUC.GetActionsByCase(ctx, testWorkspaceID, c1.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(2)

		for _, action := range actions {
			gt.Value(t, action.CaseID).Equal(c1.ID)
		}
	})
}
