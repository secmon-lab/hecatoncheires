package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runActionRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	const wsID = "test-ws"

	t.Run("Create creates action with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case first
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		action1 := &model.Action{
			CaseID:      c.ID,
			Title:       "Investigate logs",
			Description: "Check server logs for anomalies",
			AssigneeIDs: []string{"U123"},
			Status:      types.ActionStatusTodo,
		}

		created1, err := repo.Action().Create(ctx, wsID, action1)
		gt.NoError(t, err).Required()

		gt.Value(t, created1.ID).NotEqual(int64(0))
		gt.Value(t, created1.CaseID).Equal(c.ID)
		gt.Value(t, created1.Title).Equal(action1.Title)
		gt.Value(t, created1.Status).Equal(action1.Status)
		gt.Bool(t, created1.CreatedAt.IsZero()).False()
	})

	t.Run("GetByCase retrieves actions for a case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Test Case for GetByCase",
		})
		gt.NoError(t, err).Required()

		// Create multiple actions for the case
		for i := 0; i < 3; i++ {
			_, err := repo.Action().Create(ctx, wsID, &model.Action{
				CaseID:      c.ID,
				Title:       "Action " + string(rune('A'+i)),
				Description: "Description " + string(rune('A'+i)),
				Status:      types.ActionStatusTodo,
			})
			gt.NoError(t, err).Required()
		}

		// Retrieve actions for the case
		actions, err := repo.Action().GetByCase(ctx, wsID, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(3)

		// Verify all actions belong to the case
		for _, action := range actions {
			gt.Value(t, action.CaseID).Equal(c.ID)
		}
	})

	t.Run("GetByCase returns empty for case with no actions", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case without actions
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Empty Case",
		})
		gt.NoError(t, err).Required()

		actions, err := repo.Action().GetByCase(ctx, wsID, c.ID)
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(0)
	})

	t.Run("GetByCases retrieves actions for multiple cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create cases
		case1, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Case 1",
		})
		gt.NoError(t, err).Required()

		case2, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Case 2",
		})
		gt.NoError(t, err).Required()

		// Create actions for case1
		for i := 0; i < 2; i++ {
			_, err := repo.Action().Create(ctx, wsID, &model.Action{
				CaseID: case1.ID,
				Title:  "Case1 Action " + string(rune('A'+i)),
				Status: types.ActionStatusTodo,
			})
			gt.NoError(t, err).Required()
		}

		// Create actions for case2
		for i := 0; i < 3; i++ {
			_, err := repo.Action().Create(ctx, wsID, &model.Action{
				CaseID: case2.ID,
				Title:  "Case2 Action " + string(rune('A'+i)),
				Status: types.ActionStatusTodo,
			})
			gt.NoError(t, err).Required()
		}

		// Retrieve actions for both cases
		actionsMap, err := repo.Action().GetByCases(ctx, wsID, []int64{case1.ID, case2.ID})
		gt.NoError(t, err).Required()

		gt.Array(t, actionsMap[case1.ID]).Length(2)
		gt.Array(t, actionsMap[case2.ID]).Length(3)
	})

	t.Run("Update updates existing action", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		created, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID:      c.ID,
			Title:       "Original Title",
			Description: "Original Description",
			Status:      types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		// Update the action
		created.Title = "Updated Title"
		created.Status = types.ActionStatusInProgress

		updated, err := repo.Action().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Status).Equal(types.ActionStatusInProgress)
	})

	t.Run("Delete deletes existing action", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title: "Test Case",
		})
		gt.NoError(t, err).Required()

		created, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID,
			Title:  "To be deleted",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		err = repo.Action().Delete(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()

		// Verify it's deleted
		_, err = repo.Action().Get(ctx, wsID, created.ID)
		gt.Value(t, err).NotNil()
	})
}

func TestActionRepository_Memory(t *testing.T) {
	runActionRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestActionRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runActionRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		return repo
	})
}
