package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runAssistLogRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	const wsID = "test-ws"

	t.Run("Create creates assist log with UUID", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()
		log := &model.AssistLog{
			CaseID:    caseID,
			Summary:   "Checked deadlines and sent reminders",
			Actions:   "Checked action deadlines and sent reminders",
			Reasoning: "Action A-123 has a deadline in 2 days",
			NextSteps: "Follow up on action A-123 after the deadline",
		}

		created, err := repo.AssistLog().Create(ctx, wsID, caseID, log)
		gt.NoError(t, err).Required()

		gt.String(t, string(created.ID)).NotEqual("")
		gt.Value(t, created.CaseID).Equal(caseID)
		gt.Value(t, created.Summary).Equal("Checked deadlines and sent reminders")
		gt.Value(t, created.Actions).Equal("Checked action deadlines and sent reminders")
		gt.Value(t, created.Reasoning).Equal("Action A-123 has a deadline in 2 days")
		gt.Value(t, created.NextSteps).Equal("Follow up on action A-123 after the deadline")
		gt.Bool(t, created.CreatedAt.IsZero()).False()
	})

	t.Run("List returns logs sorted by CreatedAt descending", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		l1, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
			Summary:   "First run summary",
			Actions:   "First run actions",
			Reasoning: "First run reasoning",
			NextSteps: "First run next steps",
		})
		gt.NoError(t, err).Required()

		time.Sleep(10 * time.Millisecond)

		l2, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
			Summary:   "Second run summary",
			Actions:   "Second run actions",
			Reasoning: "Second run reasoning",
			NextSteps: "Second run next steps",
		})
		gt.NoError(t, err).Required()

		time.Sleep(10 * time.Millisecond)

		l3, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
			Summary:   "Third run summary",
			Actions:   "Third run actions",
			Reasoning: "Third run reasoning",
			NextSteps: "Third run next steps",
		})
		gt.NoError(t, err).Required()

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, caseID, 10, 0)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(3)
		gt.Array(t, items).Length(3)

		// Newest first
		gt.Value(t, items[0].ID).Equal(l3.ID)
		gt.Value(t, items[0].Actions).Equal("Third run actions")
		gt.Value(t, items[1].ID).Equal(l2.ID)
		gt.Value(t, items[1].Actions).Equal("Second run actions")
		gt.Value(t, items[2].ID).Equal(l1.ID)
		gt.Value(t, items[2].Actions).Equal("First run actions")
	})

	t.Run("List respects limit", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		for i := 0; i < 5; i++ {
			_, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
				Actions:   fmt.Sprintf("Run %d actions", i),
				Reasoning: fmt.Sprintf("Run %d reasoning", i),
				NextSteps: fmt.Sprintf("Run %d next steps", i),
			})
			gt.NoError(t, err).Required()
			time.Sleep(5 * time.Millisecond)
		}

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, caseID, 2, 0)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(5)
		gt.Array(t, items).Length(2)
	})

	t.Run("List respects offset", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		for i := 0; i < 5; i++ {
			_, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
				Actions:   fmt.Sprintf("Run %d actions", i),
				Reasoning: fmt.Sprintf("Run %d reasoning", i),
				NextSteps: fmt.Sprintf("Run %d next steps", i),
			})
			gt.NoError(t, err).Required()
			time.Sleep(5 * time.Millisecond)
		}

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, caseID, 2, 2)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(5)
		gt.Array(t, items).Length(2)
		// Items at index 2,3 (0-indexed from sorted desc order)
		gt.Value(t, items[0].Actions).Equal("Run 2 actions")
		gt.Value(t, items[1].Actions).Equal("Run 1 actions")
	})

	t.Run("List with offset beyond total returns empty", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID := time.Now().UnixNano()

		_, err := repo.AssistLog().Create(ctx, wsID, caseID, &model.AssistLog{
			Actions:   "Some actions",
			Reasoning: "Some reasoning",
			NextSteps: "Some next steps",
		})
		gt.NoError(t, err).Required()

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, caseID, 10, 100)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(1)
		gt.Array(t, items).Length(0)
	})

	t.Run("List returns empty for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, 999999999, 10, 0)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(0)
		gt.Array(t, items).Length(0)
	})

	t.Run("List isolates cases", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		caseID1 := time.Now().UnixNano()
		caseID2 := caseID1 + 1

		_, err := repo.AssistLog().Create(ctx, wsID, caseID1, &model.AssistLog{
			Actions:   "Case 1 actions",
			Reasoning: "Case 1 reasoning",
			NextSteps: "Case 1 next steps",
		})
		gt.NoError(t, err).Required()

		_, err = repo.AssistLog().Create(ctx, wsID, caseID2, &model.AssistLog{
			Actions:   "Case 2 actions",
			Reasoning: "Case 2 reasoning",
			NextSteps: "Case 2 next steps",
		})
		gt.NoError(t, err).Required()

		items, totalCount, err := repo.AssistLog().List(ctx, wsID, caseID1, 10, 0)
		gt.NoError(t, err).Required()
		gt.Value(t, totalCount).Equal(1)
		gt.Array(t, items).Length(1)
		gt.Value(t, items[0].Actions).Equal("Case 1 actions")
	})
}

func newFirestoreAssistLogRepository(t *testing.T) interfaces.Repository {
	t.Helper()

	projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("TEST_FIRESTORE_PROJECT_ID not set")
	}

	databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
	if databaseID == "" {
		t.Skip("TEST_FIRESTORE_DATABASE_ID not set")
	}

	ctx := context.Background()
	repo, err := firestore.New(ctx, projectID, databaseID)
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, repo.Close())
	})
	return repo
}

func TestMemoryAssistLogRepository(t *testing.T) {
	runAssistLogRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreAssistLogRepository(t *testing.T) {
	runAssistLogRepositoryTest(t, newFirestoreAssistLogRepository)
}
