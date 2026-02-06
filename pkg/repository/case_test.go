package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runCaseRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create creates case with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		case1 := &model.Case{
			Title:       "SQL Injection Risk",
			Description: "Database vulnerable to SQL injection",
			AssigneeIDs: []string{"U123", "U456"},
		}

		created1, err := repo.Case().Create(ctx, case1)
		gt.NoError(t, err).Required()

		gt.Value(t, created1.ID).NotEqual(int64(0))
		gt.Value(t, created1.Title).Equal(case1.Title)
		gt.Value(t, created1.Description).Equal(case1.Description)
		gt.Array(t, created1.AssigneeIDs).Length(len(case1.AssigneeIDs))
		gt.Bool(t, created1.CreatedAt.IsZero()).False()
		gt.Bool(t, created1.UpdatedAt.IsZero()).False()

		// Create second case to test auto-increment
		case2 := &model.Case{
			Title:       "XSS Risk",
			Description: "Cross-site scripting vulnerability",
		}

		created2, err := repo.Case().Create(ctx, case2)
		gt.NoError(t, err).Required()

		gt.Value(t, created2.ID).NotEqual(created1.ID)
	})

	t.Run("Get retrieves existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, &model.Case{
			Title:       "CSRF Risk",
			Description: "Cross-site request forgery",
			AssigneeIDs: []string{"U789"},
		})
		gt.NoError(t, err).Required()

		retrieved, err := repo.Case().Get(ctx, created.ID)
		gt.NoError(t, err).Required()

		gt.Value(t, retrieved.ID).Equal(created.ID)
		gt.Value(t, retrieved.Title).Equal(created.Title)
		gt.Value(t, retrieved.Description).Equal(created.Description)
		gt.Array(t, retrieved.AssigneeIDs).Length(len(created.AssigneeIDs))
		gt.Bool(t, retrieved.CreatedAt.Equal(created.CreatedAt)).True()
		gt.Bool(t, retrieved.UpdatedAt.Equal(created.UpdatedAt)).True()
	})

	t.Run("Get returns error for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		_, err := repo.Case().Get(ctx, time.Now().UnixNano())
		gt.Value(t, err).NotNil()
	})

	t.Run("Update updates existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, &model.Case{
			Title:       "Original Title",
			Description: "Original Description",
		})
		gt.NoError(t, err).Required()

		// Update the case
		created.Title = "Updated Title"
		created.Description = "Updated Description"
		created.AssigneeIDs = []string{"U111", "U222"}

		updated, err := repo.Case().Update(ctx, created)
		gt.NoError(t, err).Required()

		gt.Value(t, updated.ID).Equal(created.ID)
		gt.Value(t, updated.Title).Equal("Updated Title")
		gt.Value(t, updated.Description).Equal("Updated Description")
		gt.Array(t, updated.AssigneeIDs).Length(2)
		gt.Bool(t, updated.UpdatedAt.Before(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt)).False()
	})

	t.Run("Delete deletes existing case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		created, err := repo.Case().Create(ctx, &model.Case{
			Title: "To be deleted",
		})
		gt.NoError(t, err).Required()

		err = repo.Case().Delete(ctx, created.ID)
		gt.NoError(t, err).Required()

		// Verify it's deleted
		_, err = repo.Case().Get(ctx, created.ID)
		gt.Value(t, err).NotNil()
	})

	t.Run("List retrieves all cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		// Create multiple cases
		for i := 0; i < 3; i++ {
			_, err := repo.Case().Create(ctx, &model.Case{
				Title:       "Case " + string(rune('A'+i)),
				Description: "Description " + string(rune('A'+i)),
			})
			gt.NoError(t, err).Required()
		}

		cases, err := repo.Case().List(ctx)
		gt.NoError(t, err).Required()

		gt.Number(t, len(cases)).GreaterOrEqual(3)
	})
}

func TestCaseRepository_Memory(t *testing.T) {
	runCaseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestCaseRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runCaseRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID)
		gt.NoError(t, err).Required()
		return repo
	})
}
