package diagnosis_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/diagnosis"
)

// stubActionPoster captures PostSlackMessageToAction invocations so tests
// can verify the diagnosis sweep targets the right (workspace, action) pairs
// and reacts correctly to each documented outcome.
type stubActionPoster struct {
	postFn func(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error)
	calls  []posterCall
}

type posterCall struct {
	workspaceID string
	actionID    int64
}

func (s *stubActionPoster) PostSlackMessageToAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error) {
	s.calls = append(s.calls, posterCall{workspaceID: workspaceID, actionID: actionID})
	if s.postFn != nil {
		return s.postFn(ctx, workspaceID, actionID)
	}
	return nil, nil
}

func TestNew(t *testing.T) {
	t.Run("constructs UseCase with all dependencies", func(t *testing.T) {
		repo := memory.New()
		registry := model.NewWorkspaceRegistry()
		poster := &stubActionPoster{}

		uc := diagnosis.New(repo, registry, poster)
		gt.Value(t, uc).NotNil()
	})
}
