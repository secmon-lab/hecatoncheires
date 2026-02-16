package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestActionUseCase_ListOpenCaseActions(t *testing.T) {
	t.Run("returns actions from open cases only", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		// Create an open case and a closed case
		openCase, err := caseUC.CreateCase(ctx, testWorkspaceID, "Open Case", "open", []string{}, nil)
		gt.NoError(t, err).Required()

		closedCase, err := caseUC.CreateCase(ctx, testWorkspaceID, "Closed Case", "closed", []string{}, nil)
		gt.NoError(t, err).Required()
		_, err = caseUC.CloseCase(ctx, testWorkspaceID, closedCase.ID)
		gt.NoError(t, err).Required()

		// Create actions for both cases
		openAction, err := actionUC.CreateAction(ctx, testWorkspaceID, openCase.ID, "Open Action", "desc", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, closedCase.ID, "Closed Action", "desc", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		// List open case actions
		actions, err := actionUC.ListOpenCaseActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()

		gt.A(t, actions).Length(1)
		gt.V(t, actions[0].ID).Equal(openAction.ID)
		gt.V(t, actions[0].Title).Equal("Open Action")
	})

	t.Run("returns empty list when no open cases", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		// Create only a closed case
		closedCase, err := caseUC.CreateCase(ctx, testWorkspaceID, "Closed Case", "closed", []string{}, nil)
		gt.NoError(t, err).Required()
		_, err = caseUC.CloseCase(ctx, testWorkspaceID, closedCase.ID)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, closedCase.ID, "Action", "desc", []string{}, "", types.ActionStatusTodo)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListOpenCaseActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.A(t, actions).Length(0)
	})

	t.Run("returns empty list when no actions exist", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		// Create an open case with no actions
		_, err := caseUC.CreateCase(ctx, testWorkspaceID, "Open Case", "open", []string{}, nil)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListOpenCaseActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.A(t, actions).Length(0)
	})

	t.Run("returns actions from multiple open cases", func(t *testing.T) {
		repo := memory.New()
		caseUC := usecase.NewCaseUseCase(repo, nil, nil, "")
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		// Create two open cases
		case1, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 1", "desc1", []string{}, nil)
		gt.NoError(t, err).Required()

		case2, err := caseUC.CreateCase(ctx, testWorkspaceID, "Case 2", "desc2", []string{}, nil)
		gt.NoError(t, err).Required()

		// Create actions for each case
		_, err = actionUC.CreateAction(ctx, testWorkspaceID, case1.ID, "Action 1A", "desc", []string{}, "", types.ActionStatusBacklog)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, case1.ID, "Action 1B", "desc", []string{}, "", types.ActionStatusInProgress)
		gt.NoError(t, err).Required()

		_, err = actionUC.CreateAction(ctx, testWorkspaceID, case2.ID, "Action 2A", "desc", []string{}, "", types.ActionStatusCompleted)
		gt.NoError(t, err).Required()

		actions, err := actionUC.ListOpenCaseActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.A(t, actions).Length(3)

		// Verify all action titles are present
		titles := make(map[string]bool)
		for _, a := range actions {
			titles[a.Title] = true
		}
		gt.B(t, titles["Action 1A"]).True()
		gt.B(t, titles["Action 1B"]).True()
		gt.B(t, titles["Action 2A"]).True()
	})

	t.Run("returns empty list when no cases exist", func(t *testing.T) {
		repo := memory.New()
		actionUC := usecase.NewActionUseCase(repo, nil, "")
		ctx := context.Background()

		actions, err := actionUC.ListOpenCaseActions(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.A(t, actions).Length(0)
	})
}
