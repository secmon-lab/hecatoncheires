package interfaces_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func memoAt(createdAt time.Time, archivedAt *time.Time) *model.Memo {
	return &model.Memo{
		ID:          model.NewMemoID(),
		WorkspaceID: "ws-test",
		CaseID:      1,
		Title:       "memo",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
		ArchivedAt:  archivedAt,
	}
}

func TestMemoListOptions_Allows(t *testing.T) {
	base := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	before := base.Add(-time.Hour)
	after := base.Add(time.Hour)

	older := memoAt(before, nil)
	onBoundary := memoAt(base, nil)
	newer := memoAt(after, nil)

	t.Run("no window allows every active memo", func(t *testing.T) {
		opts := interfaces.MemoListOptions{}
		gt.Bool(t, opts.Allows(older)).True()
		gt.Bool(t, opts.Allows(onBoundary)).True()
		gt.Bool(t, opts.Allows(newer)).True()
	})

	t.Run("CreatedAfter includes the boundary", func(t *testing.T) {
		opts := interfaces.MemoListOptions{CreatedAfter: &base}
		gt.Bool(t, opts.Allows(older)).False()
		gt.Bool(t, opts.Allows(onBoundary)).True()
		gt.Bool(t, opts.Allows(newer)).True()
	})

	t.Run("CreatedBefore excludes the boundary", func(t *testing.T) {
		opts := interfaces.MemoListOptions{CreatedBefore: &base}
		gt.Bool(t, opts.Allows(older)).True()
		gt.Bool(t, opts.Allows(onBoundary)).False()
		gt.Bool(t, opts.Allows(newer)).False()
	})

	t.Run("both bounds keep only the half-open interval", func(t *testing.T) {
		opts := interfaces.MemoListOptions{CreatedAfter: &base, CreatedBefore: &after}
		gt.Bool(t, opts.Allows(older)).False()
		gt.Bool(t, opts.Allows(onBoundary)).True()
		gt.Bool(t, opts.Allows(newer)).False()
	})

	t.Run("archive scope is combined with the window", func(t *testing.T) {
		archivedAt := base
		archived := memoAt(base, &archivedAt)

		activeOnly := interfaces.MemoListOptions{CreatedAfter: &base}
		gt.Bool(t, activeOnly.Allows(archived)).False()
		gt.Bool(t, activeOnly.Allows(onBoundary)).True()

		all := interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeAll,
			CreatedAfter: &base,
		}
		gt.Bool(t, all.Allows(archived)).True()

		archivedOnly := interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeArchivedOnly,
			CreatedAfter: &base,
		}
		gt.Bool(t, archivedOnly.Allows(archived)).True()
		gt.Bool(t, archivedOnly.Allows(onBoundary)).False()

		// An archived memo outside the window is still excluded.
		oldArchivedAt := before
		oldArchived := memoAt(before, &oldArchivedAt)
		gt.Bool(t, all.Allows(oldArchived)).False()
	})

	t.Run("nil memo is never allowed", func(t *testing.T) {
		gt.Bool(t, interfaces.MemoListOptions{}.Allows(nil)).False()
	})
}

func TestMemoListOptions_Validate(t *testing.T) {
	base := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	later := base.Add(time.Hour)
	earlier := base.Add(-time.Hour)

	t.Run("unbounded window is valid", func(t *testing.T) {
		gt.NoError(t, interfaces.MemoListOptions{}.Validate())
	})

	t.Run("single bound is valid", func(t *testing.T) {
		gt.NoError(t, interfaces.MemoListOptions{CreatedAfter: &base}.Validate())
		gt.NoError(t, interfaces.MemoListOptions{CreatedBefore: &base}.Validate())
	})

	t.Run("ordered bounds are valid", func(t *testing.T) {
		gt.NoError(t, interfaces.MemoListOptions{CreatedAfter: &base, CreatedBefore: &later}.Validate())
	})

	t.Run("equal bounds are rejected", func(t *testing.T) {
		err := interfaces.MemoListOptions{CreatedAfter: &base, CreatedBefore: &base}.Validate()
		gt.Error(t, err).Is(interfaces.ErrMemoListOptions)
	})

	t.Run("inverted bounds are rejected", func(t *testing.T) {
		err := interfaces.MemoListOptions{CreatedAfter: &base, CreatedBefore: &earlier}.Validate()
		gt.Error(t, err).Is(interfaces.ErrMemoListOptions)
	})
}
