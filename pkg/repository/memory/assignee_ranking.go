package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// assigneeRankingRepository stores one ranking document per workspace ID.
type assigneeRankingRepository struct {
	mu   sync.RWMutex
	data map[string]*model.AssigneeRanking
}

var _ interfaces.AssigneeRankingRepository = &assigneeRankingRepository{}

func newAssigneeRankingRepository() *assigneeRankingRepository {
	return &assigneeRankingRepository{
		data: make(map[string]*model.AssigneeRanking),
	}
}

// copyAssigneeRanking deep-copies so a caller mutating the returned pointer, or
// mutating the input after Set, cannot alter stored state. The whole struct is
// copied first (so a newly added field is never silently dropped), then the
// only reference-typed field is cloned.
func copyAssigneeRanking(r *model.AssigneeRanking) *model.AssigneeRanking {
	copied := *r
	copied.UserIDs = slices.Clone(r.UserIDs)
	return &copied
}

func (r *assigneeRankingRepository) Get(ctx context.Context, workspaceID string) (*model.AssigneeRanking, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ranking, ok := r.data[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "assignee ranking not found", goerr.V("workspace_id", workspaceID))
	}
	return copyAssigneeRanking(ranking), nil
}

func (r *assigneeRankingRepository) Set(ctx context.Context, ranking *model.AssigneeRanking) error {
	if err := ranking.Validate(); err != nil {
		return goerr.Wrap(err, "assignee ranking validation failed before set")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[ranking.WorkspaceID] = copyAssigneeRanking(ranking)
	return nil
}
