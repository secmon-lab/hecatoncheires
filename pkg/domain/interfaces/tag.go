package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// TagRepository defines the interface for Tag data access. Every method is
// workspace-scoped; tags are first-class, workspace-wide classification labels
// referenced by Knowledge entries via TagID.
type TagRepository interface {
	// Create persists a new tag. The caller assigns the TagID (via
	// model.NewTagID) and stamps CreatedAt / UpdatedAt before calling; the
	// repository does not generate IDs or timestamps.
	Create(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error)

	// Get retrieves a tag by ID within a workspace.
	Get(ctx context.Context, workspaceID string, id model.TagID) (*model.Tag, error)

	// List retrieves every tag of a workspace, sorted by CreatedAt ascending.
	List(ctx context.Context, workspaceID string) ([]*model.Tag, error)

	// Update persists changes to an existing tag (only Name is mutable). The
	// caller's pointer is the source of truth for every field.
	Update(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error)

	// Delete removes a tag by ID within a workspace. The caller (usecase) is
	// responsible for refusing deletion of a tag still referenced by any
	// Knowledge; the repository performs the raw delete.
	Delete(ctx context.Context, workspaceID string, id model.TagID) error
}
