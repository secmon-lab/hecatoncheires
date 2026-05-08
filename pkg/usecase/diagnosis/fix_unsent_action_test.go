package diagnosis_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/diagnosis"
)

const testWorkspaceID = "ws-diag-test"

// makeWorkspaceRegistry builds a registry pre-populated with a single
// workspace; all FixUnsentActions tests target this workspace because the
// sweep iterates every registered workspace identically.
func makeWorkspaceRegistry(t *testing.T) *model.WorkspaceRegistry {
	t.Helper()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: testWorkspaceID, Name: "Diag Test"},
	})
	return registry
}

// seedActions inserts the supplied actions directly through the repository,
// mirroring the legacy tool path that bypassed CreateAction. The resulting
// rows have whatever SlackMessageTS was set on the input, which lets the
// tests construct mixed posted/unposted populations in a single call.
func seedActions(t *testing.T, repo *memory.Memory, actions []*model.Action) []*model.Action {
	t.Helper()
	ctx := context.Background()
	created := make([]*model.Action, 0, len(actions))
	for _, a := range actions {
		got, err := repo.Action().Create(ctx, testWorkspaceID, a)
		gt.NoError(t, err).Required()
		created = append(created, got)
	}
	return created
}

func TestUseCase_FixUnsentActions(t *testing.T) {
	t.Run("returns error when registry is unset", func(t *testing.T) {
		repo := memory.New()
		uc := diagnosis.New(repo, nil, &stubActionPoster{})
		_, err := uc.FixUnsentActions(context.Background())
		gt.Value(t, err).NotNil()
	})

	t.Run("returns error when poster is unset", func(t *testing.T) {
		repo := memory.New()
		uc := diagnosis.New(repo, makeWorkspaceRegistry(t), nil)
		_, err := uc.FixUnsentActions(context.Background())
		gt.Value(t, err).NotNil()
	})

	t.Run("targets only actions with empty SlackMessageTS", func(t *testing.T) {
		repo := memory.New()
		seeded := seedActions(t, repo, []*model.Action{
			{CaseID: 1, Title: "Unsent A", Status: types.ActionStatusTodo},
			{CaseID: 1, Title: "Already posted", Status: types.ActionStatusTodo, SlackMessageTS: "1.1"},
			{CaseID: 2, Title: "Unsent B", Status: types.ActionStatusTodo},
		})
		// Build a quick set of expected unsent action IDs so we can
		// assert PostSlackMessageToAction was called with each one.
		want := map[int64]bool{seeded[0].ID: true, seeded[2].ID: true}

		poster := &stubActionPoster{
			postFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return &model.Action{}, nil
			},
		}
		uc := diagnosis.New(repo, makeWorkspaceRegistry(t), poster)

		report, err := uc.FixUnsentActions(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, report.Total).Equal(2)
		gt.Value(t, report.Fixed).Equal(2)
		gt.Value(t, report.Skipped).Equal(0)
		gt.Value(t, report.Failed).Equal(0)

		gt.Array(t, poster.calls).Length(2).Required()
		got := map[int64]bool{}
		for _, c := range poster.calls {
			gt.Value(t, c.workspaceID).Equal(testWorkspaceID)
			got[c.actionID] = true
		}
		gt.Value(t, got).Equal(want)
	})

	t.Run("buckets ErrCaseHasNoSlackChannel as Skipped", func(t *testing.T) {
		repo := memory.New()
		seedActions(t, repo, []*model.Action{
			{CaseID: 7, Title: "No channel", Status: types.ActionStatusTodo},
		})
		poster := &stubActionPoster{
			postFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return nil, usecase.ErrCaseHasNoSlackChannel
			},
		}
		uc := diagnosis.New(repo, makeWorkspaceRegistry(t), poster)

		report, err := uc.FixUnsentActions(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, report.Total).Equal(1)
		gt.Value(t, report.Fixed).Equal(0)
		gt.Value(t, report.Skipped).Equal(1)
		gt.Value(t, report.Failed).Equal(0)
	})

	t.Run("buckets ErrSlackMessageAlreadyPosted as Skipped", func(t *testing.T) {
		repo := memory.New()
		seedActions(t, repo, []*model.Action{
			{CaseID: 8, Title: "Race fix", Status: types.ActionStatusTodo},
		})
		poster := &stubActionPoster{
			postFn: func(_ context.Context, _ string, _ int64) (*model.Action, error) {
				return nil, usecase.ErrSlackMessageAlreadyPosted
			},
		}
		uc := diagnosis.New(repo, makeWorkspaceRegistry(t), poster)

		report, err := uc.FixUnsentActions(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, report.Skipped).Equal(1)
		gt.Value(t, report.Failed).Equal(0)
	})

	t.Run("continues sweep when one action's post fails", func(t *testing.T) {
		repo := memory.New()
		seeded := seedActions(t, repo, []*model.Action{
			{CaseID: 1, Title: "OK", Status: types.ActionStatusTodo},
			{CaseID: 1, Title: "BOOM", Status: types.ActionStatusTodo},
			{CaseID: 1, Title: "OK 2", Status: types.ActionStatusTodo},
		})
		boomID := seeded[1].ID

		poster := &stubActionPoster{
			postFn: func(_ context.Context, _ string, actionID int64) (*model.Action, error) {
				if actionID == boomID {
					return nil, errors.New("transient slack error")
				}
				return &model.Action{}, nil
			},
		}
		uc := diagnosis.New(repo, makeWorkspaceRegistry(t), poster)

		report, err := uc.FixUnsentActions(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, report.Total).Equal(3)
		gt.Value(t, report.Fixed).Equal(2)
		gt.Value(t, report.Skipped).Equal(0)
		gt.Value(t, report.Failed).Equal(1)
		gt.Array(t, poster.calls).Length(3)
	})

	t.Run("no-op when registry has no workspaces", func(t *testing.T) {
		repo := memory.New()
		poster := &stubActionPoster{}
		uc := diagnosis.New(repo, model.NewWorkspaceRegistry(), poster)

		report, err := uc.FixUnsentActions(context.Background())
		gt.NoError(t, err).Required()
		gt.Value(t, report.Total).Equal(0)
		gt.Array(t, poster.calls).Length(0)
	})
}
