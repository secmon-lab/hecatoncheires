package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// tagRepository stores tags indexed by workspaceID -> id.
type tagRepository struct {
	mu   sync.RWMutex
	data map[string]map[model.TagID]*model.Tag
}

func newTagRepository() *tagRepository {
	return &tagRepository{
		data: make(map[string]map[model.TagID]*model.Tag),
	}
}

func (r *tagRepository) ensureWorkspace(workspaceID string) {
	if _, ok := r.data[workspaceID]; !ok {
		r.data[workspaceID] = make(map[model.TagID]*model.Tag)
	}
}

// copyTag creates a full copy so mutations by the caller after Create/Update
// cannot silently alter the stored value.
func copyTag(t *model.Tag) *model.Tag {
	return &model.Tag{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		Name:        t.Name,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func (r *tagRepository) Create(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error) {
	if err := tag.Validate(); err != nil {
		return nil, goerr.Wrap(err, "tag validation failed before create")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)
	stored := copyTag(tag)
	r.data[workspaceID][tag.ID] = stored
	return copyTag(stored), nil
}

func (r *tagRepository) Get(ctx context.Context, workspaceID string, id model.TagID) (*model.Tag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}
	t, ok := ws[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}
	return copyTag(t), nil
}

func (r *tagRepository) List(ctx context.Context, workspaceID string) ([]*model.Tag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return []*model.Tag{}, nil
	}

	items := make([]*model.Tag, 0, len(ws))
	for _, t := range ws {
		items = append(items, copyTag(t))
	}

	// Sort by CreatedAt ascending, tie-breaking on ID for determinism.
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return items, nil
}

func (r *tagRepository) Update(ctx context.Context, workspaceID string, tag *model.Tag) (*model.Tag, error) {
	if err := tag.Validate(); err != nil {
		return nil, goerr.Wrap(err, "tag validation failed before update")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
	}
	if _, ok := ws[tag.ID]; !ok {
		return nil, goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", tag.ID), goerr.V("workspace_id", workspaceID))
	}

	stored := copyTag(tag)
	r.data[workspaceID][tag.ID] = stored
	return copyTag(stored), nil
}

func (r *tagRepository) Delete(ctx context.Context, workspaceID string, id model.TagID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, ok := r.data[workspaceID]
	if !ok {
		return goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}
	if _, ok := ws[id]; !ok {
		return goerr.Wrap(ErrNotFound, "tag not found",
			goerr.V("tag_id", id), goerr.V("workspace_id", workspaceID))
	}
	delete(ws, id)
	return nil
}
