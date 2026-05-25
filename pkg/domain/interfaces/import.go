package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ImportRepository persists ImportSession documents that drive the
// YAML → Case/Action wizard. The session is created (status=pending) by
// createCaseImport and advanced exactly once to applied / failed by
// executeCaseImport; no Delete method exists because the spec keeps
// sessions indefinitely and exposes them only by URL.
type ImportRepository interface {
	// Create persists a new ImportSession. Implementations MUST call
	// Validate on the session before write.
	Create(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error)

	// Update overwrites an existing ImportSession in place. Used by the
	// execute path to advance status, fill ExecutedAt, and stamp per-Case
	// results into the snapshot. Implementations MUST call Validate
	// before write.
	Update(ctx context.Context, workspaceID string, s *model.ImportSession) (*model.ImportSession, error)

	// Get retrieves an ImportSession by ID. Returns ErrNotFound (the
	// repository's standard sentinel) when missing.
	Get(ctx context.Context, workspaceID string, id model.ImportSessionID) (*model.ImportSession, error)
}
