package usecase_test

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
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// rankingRegistry registers testWorkspaceID and nothing else, so
// ListFrequentAssignees' workspace existence check resolves for that ID and
// rejects any other. Production always supplies a registry (usecase.New), so
// the tests below exercise that shape rather than the nil-registry fallback.
func rankingRegistry() *model.WorkspaceRegistry {
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: testWorkspaceID, Name: "Test Workspace"},
	})
	return registry
}

// seedRankingCase persists one OPEN case with the given assignees. It writes
// through the repository rather than CreateCase so the test controls IsPrivate
// and the assignee set directly.
func seedRankingCase(t *testing.T, repo interfaces.Repository, workspaceID string, isPrivate bool, assigneeIDs ...string) {
	t.Helper()
	now := time.Now()
	_, err := repo.Case().Create(context.Background(), workspaceID, &model.Case{
		Title:       fmt.Sprintf("case-%d", time.Now().UnixNano()),
		Status:      types.CaseStatusOpen,
		ReporterID:  "U-reporter",
		AssigneeIDs: assigneeIDs,
		IsPrivate:   isPrivate,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	gt.NoError(t, err).Required()
}

func TestCaseUseCase_ListFrequentAssignees(t *testing.T) {
	t.Run("serves a fresh ranking without recomputing", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		// The stored ranking names someone who appears in no Case at all, and
		// omits the only user who does. If the result matches the stored value,
		// the cache was served rather than recomputed.
		seedRankingCase(t, repo, testWorkspaceID, false, "U-actual")
		gt.NoError(t, repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: testWorkspaceID,
			UserIDs:     []string{"U-cached"},
			ComputedAt:  time.Now(),
		})).Required()

		got, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0]).Equal("U-cached")

		// Even after the async tail drains, the fresh ranking must be untouched.
		async.Wait()
		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-cached")
	})

	t.Run("cold cache returns empty and computes in the background", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-busy")
		seedRankingCase(t, repo, testWorkspaceID, false, "U-busy", "U-quiet")

		got, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)

		async.Wait()
		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(2).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-busy")
		gt.Value(t, stored.UserIDs[1]).Equal("U-quiet")
		gt.Bool(t, stored.ComputedAt.IsZero()).False()

		// The next call now serves the computed ranking.
		next, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, next).Length(2).Required()
		gt.Value(t, next[0]).Equal("U-busy")
	})

	t.Run("stale cache is returned as-is and refreshed in the background", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-current")
		gt.NoError(t, repo.AssigneeRanking().Set(ctx, &model.AssigneeRanking{
			WorkspaceID: testWorkspaceID,
			UserIDs:     []string{"U-outdated"},
			ComputedAt:  time.Now().Add(-3 * time.Hour),
		})).Required()

		got, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(1).Required()
		gt.Value(t, got[0]).Equal("U-outdated")

		async.Wait()
		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-current")
	})

	t.Run("orders by assignment count, ties by user ID", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		// U-three: 3 cases, U-two: 2, U-one-b / U-one-a: 1 each (tie).
		seedRankingCase(t, repo, testWorkspaceID, false, "U-three", "U-two")
		seedRankingCase(t, repo, testWorkspaceID, false, "U-three", "U-two")
		seedRankingCase(t, repo, testWorkspaceID, false, "U-three", "U-one-b")
		seedRankingCase(t, repo, testWorkspaceID, false, "U-one-a")

		_, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(4).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-three")
		gt.Value(t, stored.UserIDs[1]).Equal("U-two")
		gt.Value(t, stored.UserIDs[2]).Equal("U-one-a")
		gt.Value(t, stored.UserIDs[3]).Equal("U-one-b")
	})

	t.Run("excludes assignees that appear only on private cases", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-public")
		// Assigned to more cases than U-public, but all of them private.
		seedRankingCase(t, repo, testWorkspaceID, true, "U-secret")
		seedRankingCase(t, repo, testWorkspaceID, true, "U-secret")

		_, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-public")
	})

	t.Run("truncates to the ranking size", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		// 20 distinct assignees, each on exactly one case. Zero-padded IDs make
		// the ID tie-break order predictable.
		for i := range 20 {
			seedRankingCase(t, repo, testWorkspaceID, false, fmt.Sprintf("U-%02d", i))
		}

		_, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(12).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-00")
		gt.Value(t, stored.UserIDs[11]).Equal("U-11")
	})

	t.Run("skips empty assignee IDs", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-real", "")

		_, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-real")
	})

	t.Run("stores an empty ranking with a timestamp when nothing qualifies", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		got, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
		async.Wait()

		// A workspace with no qualifying case must still record ComputedAt, or
		// every request would re-scan it forever.
		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(0)
		gt.Bool(t, stored.ComputedAt.IsZero()).False()
	})

	t.Run("ignores DRAFT cases", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-open")
		_, err := repo.Case().Create(ctx, testWorkspaceID, &model.Case{
			Title:       "draft",
			Status:      types.CaseStatusDraft,
			ReporterID:  "U-reporter",
			AssigneeIDs: []string{"U-draft"},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		})
		gt.NoError(t, err).Required()

		_, err = uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-open")
	})

	t.Run("rejects an unconfigured workspace without writing anything", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.NewCaseUseCase(repo, rankingRegistry(), nil, nil, "")
		ctx := context.Background()
		unknownWorkspaceID := fmt.Sprintf("ws-unknown-%d", time.Now().UnixNano())

		_, err := uc.ListFrequentAssignees(ctx, unknownWorkspaceID)
		gt.Error(t, err).Is(model.ErrWorkspaceNotFound)

		// The point of the check is the absence of the write: a cold cache would
		// otherwise persist a ranking document for every id a caller invents.
		async.Wait()
		_, getErr := repo.AssigneeRanking().Get(ctx, unknownWorkspaceID)
		gt.Error(t, getErr).Is(memory.ErrNotFound)
	})

	t.Run("skips the workspace check when no registry is configured", func(t *testing.T) {
		repo := memory.New()
		// Callers constructed without workspace configuration keep working: the
		// registry is the only source of truth for which workspaces exist, so a
		// nil one cannot reject anything.
		uc := usecase.NewCaseUseCase(repo, nil, nil, nil, "")
		ctx := context.Background()

		seedRankingCase(t, repo, testWorkspaceID, false, "U-open")

		got, err := uc.ListFrequentAssignees(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
		async.Wait()

		stored, err := repo.AssigneeRanking().Get(ctx, testWorkspaceID)
		gt.NoError(t, err).Required()
		gt.Array(t, stored.UserIDs).Length(1).Required()
		gt.Value(t, stored.UserIDs[0]).Equal("U-open")
	})
}
