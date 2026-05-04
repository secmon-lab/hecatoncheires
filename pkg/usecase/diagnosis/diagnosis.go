// Package diagnosis hosts data-repair and inspection routines that operate
// across the persistent state owned by the rest of the application.
//
// The package exists to keep one-shot maintenance jobs (Slack post repair,
// data backfill, etc.) on the usecase side of the architecture instead of
// in the CLI layer. Every job composes existing usecase / repository
// surfaces; nothing in this package talks to external services directly.
package diagnosis

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionPoster is the narrow surface of the ActionUseCase that the
// fix-unsent-action job depends on. Defined here (rather than imported from
// pkg/usecase) so:
//   - this subpackage stays free of an upward import on its parent, and
//   - tests can substitute a fake without spinning up a full ActionUseCase
//     plus its Slack/Repository wiring.
type ActionPoster interface {
	PostSlackMessageToAction(ctx context.Context, workspaceID string, actionID int64) (*model.Action, error)
}

// UseCase exposes diagnosis routines.
type UseCase struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	actionUC ActionPoster
}

// New constructs a UseCase. registry is required so the FixUnsentActions
// sweep knows which workspaces to enumerate; actionUC is required so the
// repair routes through the unified post-Slack entry point.
func New(repo interfaces.Repository, registry *model.WorkspaceRegistry, actionUC ActionPoster) *UseCase {
	return &UseCase{
		repo:     repo,
		registry: registry,
		actionUC: actionUC,
	}
}
