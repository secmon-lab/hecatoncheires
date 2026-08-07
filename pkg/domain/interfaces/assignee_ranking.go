package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// AssigneeRankingRepository persists the per-workspace assignee ranking that
// orders the WebUI assignee picker. It is a single document per workspace, so
// it needs only Get/Set — no List.
type AssigneeRankingRepository interface {
	// Get returns the workspace's stored ranking. Returns the backend's
	// ErrNotFound (memory.ErrNotFound / firestore.ErrNotFound) when the
	// workspace has none yet.
	Get(ctx context.Context, workspaceID string) (*model.AssigneeRanking, error)

	// Set writes the ranking wholesale (Validate then persist). Concurrent
	// writers are deliberately not coordinated: the value is derived data, so
	// two instances recomputing at the same moment simply overwrite each other
	// with near-identical rankings.
	Set(ctx context.Context, ranking *model.AssigneeRanking) error
}
