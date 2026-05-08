package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ActionStepRepository persists per-Action ActionStep documents.
type ActionStepRepository interface {
	// Put inserts or replaces a step. The ID must be unique within the action.
	Put(ctx context.Context, workspaceID string, step *model.ActionStep) error

	// Get retrieves a single step by id.
	Get(ctx context.Context, workspaceID string, actionID int64, stepID string) (*model.ActionStep, error)

	// List returns all steps for an action ordered by CreatedAt ascending.
	List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error)

	// Delete removes a single step. Deleting a non-existent step is a no-op.
	Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error
}
