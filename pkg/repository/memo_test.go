package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runMemoRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("Create and Get round-trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		// Create a parent case first so the workspace/case hierarchy exists.
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Parent case",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		archivedAt := time.Now().UTC()
		memoID := model.NewMemoID()
		now := time.Now().UTC()

		input := &model.Memo{
			ID:          memoID,
			WorkspaceID: wsID,
			CaseID:      c.ID,
			Title:       "Round-trip memo",
			CreatorID:   "U-CREATOR",
			FieldValues: map[string]model.FieldValue{
				"select_field": {
					FieldID: "select_field",
					Type:    types.FieldTypeSelect,
					Value:   "option-A",
				},
				"number_field": {
					FieldID: "number_field",
					Type:    types.FieldTypeNumber,
					Value:   float64(42),
				},
				"multi_select_field": {
					FieldID: "multi_select_field",
					Type:    types.FieldTypeMultiSelect,
					Value:   []string{"tag1", "tag2"},
				},
				"url_field": {
					FieldID: "url_field",
					Type:    types.FieldTypeURL,
					Value:   "https://example.com",
				},
			},
			ArchivedAt: &archivedAt,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		created, err := repo.Memo().Create(ctx, wsID, input)
		gt.NoError(t, err).Required()

		// Verify returned value from Create.
		gt.Value(t, created.ID).Equal(memoID)
		gt.Value(t, created.WorkspaceID).Equal(wsID)
		gt.Value(t, created.CaseID).Equal(c.ID)
		gt.Value(t, created.Title).Equal("Round-trip memo")
		gt.Value(t, created.CreatorID).Equal("U-CREATOR")
		gt.Value(t, created.ArchivedAt).NotNil()

		// Read back via Get and verify all fields.
		got, err := repo.Memo().Get(ctx, wsID, c.ID, memoID)
		gt.NoError(t, err).Required()

		gt.Value(t, got.ID).Equal(memoID)
		gt.Value(t, got.WorkspaceID).Equal(wsID)
		gt.Value(t, got.CaseID).Equal(c.ID)
		gt.Value(t, got.Title).Equal("Round-trip memo")
		gt.Value(t, got.CreatorID).Equal("U-CREATOR")
		gt.Value(t, got.ArchivedAt).NotNil()
		gt.Bool(t, got.CreatedAt.Sub(now) < time.Second).True()
		gt.Bool(t, got.UpdatedAt.Sub(now) < time.Second).True()

		// Assert every FieldValue.
		gt.Number(t, len(got.FieldValues)).Equal(4)

		selectFV, ok := got.FieldValues["select_field"]
		gt.Bool(t, ok).True()
		gt.Value(t, selectFV.Type).Equal(types.FieldTypeSelect)
		gt.Value(t, selectFV.Value).Equal("option-A")

		numberFV, ok := got.FieldValues["number_field"]
		gt.Bool(t, ok).True()
		gt.Value(t, numberFV.Type).Equal(types.FieldTypeNumber)
		gt.Value(t, numberFV.Value).Equal(float64(42))

		multiSelectFV, ok := got.FieldValues["multi_select_field"]
		gt.Bool(t, ok).True()
		gt.Value(t, multiSelectFV.Type).Equal(types.FieldTypeMultiSelect)
		ms := toStringSlice(t, multiSelectFV.Value)
		gt.Array(t, ms).Length(2)
		gt.Value(t, ms[0]).Equal("tag1")
		gt.Value(t, ms[1]).Equal("tag2")

		urlFV, ok := got.FieldValues["url_field"]
		gt.Bool(t, ok).True()
		gt.Value(t, urlFV.Type).Equal(types.FieldTypeURL)
		gt.Value(t, urlFV.Value).Equal("https://example.com")
	})

	t.Run("Get returns not-found error for missing id", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for missing memo test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		missing := model.NewMemoID()
		_, err = repo.Memo().Get(ctx, wsID, c.ID, missing)
		gt.Value(t, err).NotNil()

		// Verify the error wraps a not-found sentinel from either backend.
		// Both memory.ErrNotFound and firestore.ErrNotFound have the same
		// message "not found" — we check via errors.Is against each package.
		isNotFound := errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
		gt.Bool(t, isNotFound).True()
	})

	t.Run("GetByIDs returns only found memos and omits missing ids", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for GetByIDs test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		id1 := model.NewMemoID()
		m1, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: id1, WorkspaceID: wsID, CaseID: c.ID,
			Title: "Memo one", CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()

		id2 := model.NewMemoID()
		m2, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: id2, WorkspaceID: wsID, CaseID: c.ID,
			Title: "Memo two", CreatedAt: now, UpdatedAt: now,
		})
		gt.NoError(t, err).Required()

		missingID := model.NewMemoID()

		got, err := repo.Memo().GetByIDs(ctx, wsID, c.ID, []model.MemoID{id1, missingID, id2})
		gt.NoError(t, err).Required()

		gt.Number(t, len(got)).Equal(2)
		gt.Map(t, got).HasKey(id1)
		gt.Map(t, got).HasKey(id2)

		_, hasMissing := got[missingID]
		gt.Bool(t, hasMissing).False()

		gt.Value(t, got[id1].Title).Equal(m1.Title)
		gt.Value(t, got[id2].Title).Equal(m2.Title)
	})

	t.Run("GetByIDs returns empty map for empty id slice", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for empty GetByIDs test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		got, err := repo.Memo().GetByIDs(ctx, wsID, c.ID, []model.MemoID{})
		gt.NoError(t, err).Required()
		gt.Number(t, len(got)).Equal(0)
	})

	t.Run("List archive scopes return the correct subsets sorted by CreatedAt", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for List archive scope test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		// Use distinct CreatedAt times so ordering is deterministic.
		base := time.Now().UTC()

		id1 := model.NewMemoID()
		m1, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: id1, WorkspaceID: wsID, CaseID: c.ID,
			Title:     "Active memo 1",
			CreatedAt: base,
			UpdatedAt: base,
		})
		gt.NoError(t, err).Required()

		id2 := model.NewMemoID()
		m2, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: id2, WorkspaceID: wsID, CaseID: c.ID,
			Title:     "Active memo 2",
			CreatedAt: base.Add(time.Millisecond),
			UpdatedAt: base.Add(time.Millisecond),
		})
		gt.NoError(t, err).Required()

		id3 := model.NewMemoID()
		archivedAt := base.Add(2 * time.Millisecond)
		m3, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID: id3, WorkspaceID: wsID, CaseID: c.ID,
			Title:      "Archived memo",
			ArchivedAt: &archivedAt,
			CreatedAt:  base.Add(2 * time.Millisecond),
			UpdatedAt:  base.Add(2 * time.Millisecond),
		})
		gt.NoError(t, err).Required()

		// ActiveOnly (default — zero value)
		active, err := repo.Memo().List(ctx, wsID, c.ID, interfaces.MemoListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, active).Length(2).Required()
		gt.Value(t, active[0].ID).Equal(m1.ID)
		gt.Value(t, active[1].ID).Equal(m2.ID)

		// ArchivedOnly
		archived, err := repo.Memo().List(ctx, wsID, c.ID, interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeArchivedOnly,
		})
		gt.NoError(t, err).Required()
		gt.Array(t, archived).Length(1).Required()
		gt.Value(t, archived[0].ID).Equal(m3.ID)

		// All
		all, err := repo.Memo().List(ctx, wsID, c.ID, interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeAll,
		})
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(3).Required()
		gt.Value(t, all[0].ID).Equal(m1.ID)
		gt.Value(t, all[1].ID).Equal(m2.ID)
		gt.Value(t, all[2].ID).Equal(m3.ID)
	})

	t.Run("Update persists mutations and is read back correctly", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for Update test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		memoID := model.NewMemoID()
		created, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID:          memoID,
			WorkspaceID: wsID,
			CaseID:      c.ID,
			Title:       "Original title",
			FieldValues: map[string]model.FieldValue{
				"status_field": {
					FieldID: "status_field",
					Type:    types.FieldTypeSelect,
					Value:   "open",
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		})
		gt.NoError(t, err).Required()

		// Mutate title, field, and set ArchivedAt.
		archivedAt := time.Now().UTC()
		updatedAt := time.Now().UTC()
		created.Title = "Updated title"
		created.FieldValues["status_field"] = model.FieldValue{
			FieldID: "status_field",
			Type:    types.FieldTypeSelect,
			Value:   "closed",
		}
		created.ArchivedAt = &archivedAt
		created.UpdatedAt = updatedAt

		updated, err := repo.Memo().Update(ctx, wsID, created)
		gt.NoError(t, err).Required()
		gt.Value(t, updated.Title).Equal("Updated title")
		gt.Value(t, updated.ArchivedAt).NotNil()

		// Read back via Get and confirm persistence.
		got, err := repo.Memo().Get(ctx, wsID, c.ID, memoID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Title).Equal("Updated title")
		gt.Value(t, got.ArchivedAt).NotNil()
		gt.Bool(t, got.UpdatedAt.Sub(updatedAt) < time.Second).True()

		statusFV, ok := got.FieldValues["status_field"]
		gt.Bool(t, ok).True()
		gt.Value(t, statusFV.Value).Equal("closed")
	})

	t.Run("Update returns not-found error for missing memo", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for missing Update test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		ghost := &model.Memo{
			ID:          model.NewMemoID(),
			WorkspaceID: wsID,
			CaseID:      c.ID,
			Title:       "Ghost memo",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err = repo.Memo().Update(ctx, wsID, ghost)
		gt.Value(t, err).NotNil()
		isNotFound := errors.Is(err, memory.ErrNotFound) || errors.Is(err, firestore.ErrNotFound)
		gt.Bool(t, isNotFound).True()
	})

	t.Run("Get returns archived memo as-is", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		c, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID: "U-REPORTER",
			Title:      "Case for archived Get test",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		})
		gt.NoError(t, err).Required()

		now := time.Now().UTC()
		memoID := model.NewMemoID()
		archivedAt := now
		created, err := repo.Memo().Create(ctx, wsID, &model.Memo{
			ID:          memoID,
			WorkspaceID: wsID,
			CaseID:      c.ID,
			Title:       "Already archived",
			ArchivedAt:  &archivedAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		gt.NoError(t, err).Required()

		got, err := repo.Memo().Get(ctx, wsID, c.ID, memoID)
		gt.NoError(t, err).Required()
		gt.Value(t, got.ID).Equal(created.ID)
		gt.Value(t, got.ArchivedAt).NotNil()
	})
}

func TestMemoRepository_Memory(t *testing.T) {
	runMemoRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestMemoRepository_Firestore(t *testing.T) {
	runMemoRepositoryTest(t, newFirestoreRepository)
}
