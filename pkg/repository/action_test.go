package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runActionRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create/Update reject actions with missing CaseID", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		_, err := repo.Action().Create(ctx, wsID, &model.Action{
			Title:  "orphan",
			Status: types.ActionStatusTodo,
		})
		gt.Error(t, err).Is(model.ErrActionValidation)

		_, err = repo.Action().Update(ctx, wsID, &model.Action{
			ID:     1,
			Title:  "orphan update",
			Status: types.ActionStatusTodo,
		})
		gt.Error(t, err).Is(model.ErrActionValidation)
	})

	t.Run("Create creates action with auto-increment ID", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create a case first
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Test Case",
		})
		gt.NoError(t, err).Required()

		action1 := &model.Action{
			CaseID:      c.ID,
			Title:       "Investigate logs",
			Description: "Check server logs for anomalies",
			AssigneeID:  "U123",
			Status:      types.ActionStatusTodo,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		created1, err := repo.Action().Create(ctx, wsID, action1)
		gt.NoError(t, err).Required()

		gt.Value(t, created1.ID).NotEqual(int64(0))
		gt.Value(t, created1.CaseID).Equal(c.ID)
		gt.Value(t, created1.Title).Equal(action1.Title)
		gt.Value(t, created1.Status).Equal(action1.Status)
		gt.Bool(t, created1.CreatedAt.IsZero()).False()
	})

	t.Run("GetByIDs returns actions for matching IDs", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Case for GetByIDs",
		})
		gt.NoError(t, err).Required()

		a1, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID:      c.ID,
			Title:       "First",
			Description: "First action",
			AssigneeID:  "U111",
			Status:      types.ActionStatusTodo,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		a2, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID:      c.ID,
			Title:       "Second",
			Description: "Second action",
			AssigneeID:  "U222",
			Status:      types.ActionStatusInProgress,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Mix in a non-existent ID — must be silently absent, not an error.
		missingID := a2.ID + 999_999
		got, err := repo.Action().GetByIDs(ctx, wsID, []int64{a1.ID, missingID, a2.ID})
		gt.NoError(t, err).Required()
		gt.Map(t, got).HasKey(a1.ID)
		gt.Map(t, got).HasKey(a2.ID)
		gt.Number(t, len(got)).Equal(2)

		gotA1 := got[a1.ID]
		gt.Value(t, gotA1.Title).Equal(a1.Title)
		gt.Value(t, gotA1.Description).Equal(a1.Description)
		gt.Value(t, gotA1.AssigneeID).Equal(a1.AssigneeID)
		gt.Value(t, gotA1.Status).Equal(a1.Status)
		gt.Value(t, gotA1.CaseID).Equal(c.ID)

		gotA2 := got[a2.ID]
		gt.Value(t, gotA2.Title).Equal(a2.Title)
		gt.Value(t, gotA2.Status).Equal(a2.Status)

		_, ok := got[missingID]
		gt.Bool(t, ok).False()
	})

	t.Run("GetByIDs returns empty map for empty ID slice", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		got, err := repo.Action().GetByIDs(ctx, wsID, []int64{})
		gt.NoError(t, err).Required()
		gt.Number(t, len(got)).Equal(0)
	})

	t.Run("GetByCase retrieves actions for a case", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Test Case for GetByCase",
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
		actions, err := repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(3)

		// Verify all actions belong to the case
		for _, action := range actions {
			gt.Value(t, action.CaseID).Equal(c.ID)
		}
	})

	t.Run("GetByCase returns empty for case with no actions", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create a case without actions
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Empty Case",
		})
		gt.NoError(t, err).Required()

		actions, err := repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()

		gt.Array(t, actions).Length(0)
	})

	t.Run("GetByCases retrieves actions for multiple cases", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create cases
		case1, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Case 1",
		})
		gt.NoError(t, err).Required()

		case2, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Case 2",
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
		actionsMap, err := repo.Action().GetByCases(ctx, wsID, []int64{case1.ID, case2.ID}, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()

		gt.Array(t, actionsMap[case1.ID]).Length(2)
		gt.Array(t, actionsMap[case2.ID]).Length(3)
	})

	t.Run("Update updates existing action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Test Case",
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
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		// Create a case
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Test Case",
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

	t.Run("GetBySlackMessageTS returns matching action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Test Case",
		})
		gt.NoError(t, err).Required()

		ts := "1700000000.000123"
		created, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID:         c.ID,
			Title:          "Has slack message",
			Status:         types.ActionStatusTodo,
			SlackMessageTS: ts,
		})
		gt.NoError(t, err).Required()

		got, err := repo.Action().GetBySlackMessageTS(ctx, wsID, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.SlackMessageTS).Equal(ts)
		gt.Value(t, got.Title).Equal("Has slack message")
	})

	t.Run("GetBySlackMessageTS returns ErrNotFound when no match", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		_, err := repo.Action().GetBySlackMessageTS(ctx, wsID, "9999999999.999999")
		gt.Value(t, err).NotNil()
	})

	t.Run("GetBySlackMessageTS returns ErrNotFound for empty ts", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		_, err := repo.Action().GetBySlackMessageTS(ctx, wsID, "")
		gt.Value(t, err).NotNil()
	})

	t.Run("List excludes archived actions by default", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Archive list filter case",
		})
		gt.NoError(t, err).Required()

		active, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID,
			Title:  "active",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		archived, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID,
			Title:  "archived",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		archived.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, archived)
		gt.NoError(t, err).Required()

		// Default: archived actions excluded
		got, err := repo.Action().List(ctx, wsID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0].ID).Equal(active.ID)

		// ArchiveScopeAll returns both
		gotAll, err := repo.Action().List(ctx, wsID, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeAll})
		gt.NoError(t, err).Required()
		gt.Array(t, gotAll).Length(2)

		// ArchiveScopeArchivedOnly returns archived
		gotArchived, err := repo.Action().List(ctx, wsID, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeArchivedOnly})
		gt.NoError(t, err).Required()
		gt.Array(t, gotArchived).Length(1).Required()
		gt.Value(t, gotArchived[0].ID).Equal(archived.ID)
	})

	t.Run("GetByCase honours ArchiveScope option", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-TEST-DEFAULT",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			Title:      "Archive case filter",
		})
		gt.NoError(t, err).Required()

		active, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID,
			Title:  "active",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		archived, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID,
			Title:  "archived",
			Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		archived.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, archived)
		gt.NoError(t, err).Required()

		gotActive, err := repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, gotActive).Length(1).Required()
		gt.Value(t, gotActive[0].ID).Equal(active.ID)

		gotAll, err := repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeAll})
		gt.NoError(t, err).Required()
		gt.Array(t, gotAll).Length(2)

		gotArchived, err := repo.Action().GetByCase(ctx, wsID, c.ID, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeArchivedOnly})
		gt.NoError(t, err).Required()
		gt.Array(t, gotArchived).Length(1).Required()
		gt.Value(t, gotArchived[0].ID).Equal(archived.ID)
	})

	t.Run("GetByCases honours ArchiveScope option", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c1, err := repo.Case().Create(ctx, wsID, &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "Case A", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
		gt.NoError(t, err).Required()
		c2, err := repo.Case().Create(ctx, wsID, &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "Case B", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
		gt.NoError(t, err).Required()

		// One active and one archived in c1; one archived in c2.
		_, err = repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c1.ID, Title: "c1-active", Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		c1archived, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c1.ID, Title: "c1-archived", Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()
		now := time.Now().UTC()
		c1archived.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, c1archived)
		gt.NoError(t, err).Required()

		c2archived, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c2.ID, Title: "c2-archived", Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()
		c2archived.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, c2archived)
		gt.NoError(t, err).Required()

		// Default
		gotDefault, err := repo.Action().GetByCases(ctx, wsID, []int64{c1.ID, c2.ID}, interfaces.ActionListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, gotDefault[c1.ID]).Length(1)
		gt.Array(t, gotDefault[c2.ID]).Length(0)

		// ArchiveScopeAll
		gotAll, err := repo.Action().GetByCases(ctx, wsID, []int64{c1.ID, c2.ID}, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeAll})
		gt.NoError(t, err).Required()
		gt.Array(t, gotAll[c1.ID]).Length(2)
		gt.Array(t, gotAll[c2.ID]).Length(1)

		// ArchiveScopeArchivedOnly
		gotArchived, err := repo.Action().GetByCases(ctx, wsID, []int64{c1.ID, c2.ID}, interfaces.ActionListOptions{ArchiveScope: interfaces.ActionArchiveScopeArchivedOnly})
		gt.NoError(t, err).Required()
		gt.Array(t, gotArchived[c1.ID]).Length(1).Required()
		gt.Value(t, gotArchived[c1.ID][0].ID).Equal(c1archived.ID)
		gt.Array(t, gotArchived[c2.ID]).Length(1).Required()
		gt.Value(t, gotArchived[c2.ID][0].ID).Equal(c2archived.ID)
	})

	t.Run("Get returns archived actions as-is", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "case", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
		gt.NoError(t, err).Required()

		created, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID, Title: "archived", Status: types.ActionStatusTodo,
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		created.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		got, err := repo.Action().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.ArchivedAt).NotNil()
	})

	t.Run("GetBySlackMessageTS returns archived action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		ctx := context.Background()

		c, err := repo.Case().Create(ctx, wsID, &model.Case{ReporterID: "U-TEST-DEFAULT", Title: "case", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
		gt.NoError(t, err).Required()

		ts := "1700000099.000999"
		created, err := repo.Action().Create(ctx, wsID, &model.Action{
			CaseID: c.ID, Title: "archived-with-slack", Status: types.ActionStatusTodo, SlackMessageTS: ts,
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		created.ArchivedAt = &now
		_, err = repo.Action().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()

		got, err := repo.Action().GetBySlackMessageTS(ctx, wsID, ts)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.ArchivedAt).NotNil()
	})
}

func TestActionRepository_Memory(t *testing.T) {
	t.Parallel()
	runActionRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestActionRepository_Firestore(t *testing.T) {
	t.Parallel()
	runActionRepositoryTest(t, newFirestoreRepository)
}
