package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// TagUseCase orchestrates workspace-wide Tag operations. Tags are first-class
// classification labels referenced by Knowledge entries via TagID.
type TagUseCase struct {
	repo interfaces.Repository
}

// NewTagUseCase constructs a TagUseCase.
func NewTagUseCase(repo interfaces.Repository) *TagUseCase {
	return &TagUseCase{repo: repo}
}

// ErrTagInUse is returned when deleting a tag that is still referenced by at
// least one knowledge entry. The delete is refused so no dangling reference can
// be created.
var ErrTagInUse = goerr.New("tag is in use")

// CreateTag creates a new tag. Name is optional and is trimmed; the ID is a
// freshly generated immutable TagID.
func (uc *TagUseCase) CreateTag(ctx context.Context, workspaceID, name string) (*model.Tag, error) {
	now := time.Now().UTC()
	tag := &model.Tag{
		ID:          model.NewTagID(),
		WorkspaceID: workspaceID,
		Name:        strings.TrimSpace(name),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := tag.Validate(); err != nil {
		return nil, err
	}
	created, err := uc.repo.Tag().Create(ctx, workspaceID, tag)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create tag", goerr.V("workspace_id", workspaceID))
	}
	return created, nil
}

// GetTag retrieves a tag by ID.
func (uc *TagUseCase) GetTag(ctx context.Context, workspaceID string, id model.TagID) (*model.Tag, error) {
	t, err := uc.repo.Tag().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get tag",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id))
	}
	return t, nil
}

// ListTags returns every tag of a workspace, sorted by CreatedAt ascending.
func (uc *TagUseCase) ListTags(ctx context.Context, workspaceID string) ([]*model.Tag, error) {
	items, err := uc.repo.Tag().List(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list tags", goerr.V("workspace_id", workspaceID))
	}
	return items, nil
}

// UpdateTag renames an existing tag (only Name is mutable; the ID never changes).
func (uc *TagUseCase) UpdateTag(ctx context.Context, workspaceID string, id model.TagID, name string) (*model.Tag, error) {
	tag, err := uc.repo.Tag().Get(ctx, workspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to load tag for update",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id))
	}
	tag.Name = strings.TrimSpace(name)
	tag.UpdatedAt = time.Now().UTC()
	if err := tag.Validate(); err != nil {
		return nil, err
	}
	updated, err := uc.repo.Tag().Update(ctx, workspaceID, tag)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update tag",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id))
	}
	return updated, nil
}

// DeleteTag removes a tag, but only when no knowledge entry references it. A tag
// still in use is refused with ErrTagInUse so a delete can never strand a
// knowledge entry with a dangling tag id.
func (uc *TagUseCase) DeleteTag(ctx context.Context, workspaceID string, id model.TagID) error {
	items, err := uc.repo.Knowledge().List(ctx, workspaceID, interfaces.KnowledgeListOptions{TagIDs: []model.TagID{id}})
	if err != nil {
		return goerr.Wrap(err, "failed to check tag references before delete",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id))
	}
	if len(items) > 0 {
		return goerr.Wrap(ErrTagInUse, "tag is referenced by knowledge entries",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id), goerr.V("reference_count", len(items)))
	}
	if err := uc.repo.Tag().Delete(ctx, workspaceID, id); err != nil {
		return goerr.Wrap(err, "failed to delete tag",
			goerr.V("workspace_id", workspaceID), goerr.V("tag_id", id))
	}
	return nil
}
