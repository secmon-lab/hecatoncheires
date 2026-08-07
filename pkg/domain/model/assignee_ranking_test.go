package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestAssigneeRankingValidate(t *testing.T) {
	t.Parallel()

	t.Run("accepts a populated ranking", func(t *testing.T) {
		r := &model.AssigneeRanking{
			WorkspaceID: "ws-a",
			UserIDs:     []string{"U1", "U2"},
			ComputedAt:  time.Now(),
		}
		gt.NoError(t, r.Validate())
	})

	t.Run("accepts an empty user list", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", UserIDs: []string{}}
		gt.NoError(t, r.Validate())
	})

	t.Run("accepts a nil user list", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a"}
		gt.NoError(t, r.Validate())
	})

	t.Run("rejects nil", func(t *testing.T) {
		var r *model.AssigneeRanking
		gt.Error(t, r.Validate()).Is(model.ErrAssigneeRankingValidation)
	})

	t.Run("rejects an empty workspace id", func(t *testing.T) {
		r := &model.AssigneeRanking{UserIDs: []string{"U1"}}
		gt.Error(t, r.Validate()).Is(model.ErrAssigneeRankingValidation)
	})

	t.Run("rejects an empty user id element", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", UserIDs: []string{"U1", "", "U2"}}
		gt.Error(t, r.Validate()).Is(model.ErrAssigneeRankingValidation)
	})
}

func TestAssigneeRankingIsFresh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	window := time.Hour

	t.Run("a never-computed ranking is stale", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", UserIDs: []string{"U1"}}
		gt.Bool(t, r.IsFresh(now, window)).False()
	})

	t.Run("nil is stale", func(t *testing.T) {
		var r *model.AssigneeRanking
		gt.Bool(t, r.IsFresh(now, window)).False()
	})

	t.Run("computed within the window is fresh", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", ComputedAt: now.Add(-59 * time.Minute)}
		gt.Bool(t, r.IsFresh(now, window)).True()
	})

	t.Run("computed exactly a window ago is stale", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", ComputedAt: now.Add(-window)}
		gt.Bool(t, r.IsFresh(now, window)).False()
	})

	t.Run("computed longer ago than the window is stale", func(t *testing.T) {
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", ComputedAt: now.Add(-3 * time.Hour)}
		gt.Bool(t, r.IsFresh(now, window)).False()
	})

	t.Run("a future ComputedAt is treated as fresh", func(t *testing.T) {
		// Clock skew between instances can push ComputedAt ahead of now. Serving
		// the cached value is the safe reading: the ranking exists and a refresh
		// would only rewrite the same thing.
		r := &model.AssigneeRanking{WorkspaceID: "ws-a", ComputedAt: now.Add(time.Minute)}
		gt.Bool(t, r.IsFresh(now, window)).True()
	})
}
