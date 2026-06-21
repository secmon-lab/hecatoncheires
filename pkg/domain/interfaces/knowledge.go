package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// KnowledgeListOptions controls how List filters knowledge entries.
type KnowledgeListOptions struct {
	// Tags applies an AND filter: only entries carrying every listed tag are
	// returned. An empty slice returns all entries. Filtering is done in memory
	// (no Firestore composite index) — see the repository implementations.
	Tags []string
}

// KnowledgeRepository defines the interface for Knowledge data access. Every
// method is workspace-scoped; knowledge is shared across the whole workspace and
// is not tied to a case.
type KnowledgeRepository interface {
	// Create persists a new knowledge entry. The caller assigns the KnowledgeID
	// (via model.NewKnowledgeID) before calling; the repository does not generate
	// IDs.
	Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error)

	// Get retrieves a knowledge entry by ID within a workspace.
	Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error)

	// List retrieves the knowledge entries of a workspace, filtered by
	// opts.Tags (AND). Results are sorted by CreatedAt ascending.
	List(ctx context.Context, workspaceID string, opts KnowledgeListOptions) ([]*model.Knowledge, error)

	// Update persists changes to an existing knowledge entry. The caller's
	// pointer is the source of truth for every field.
	Update(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error)

	// Delete removes a knowledge entry by ID within a workspace.
	Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error
}
