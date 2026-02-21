package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type sourceRepository struct {
	mu      sync.RWMutex
	sources map[string]map[model.SourceID]*model.Source
}

func newSourceRepository() *sourceRepository {
	return &sourceRepository{
		sources: make(map[string]map[model.SourceID]*model.Source),
	}
}

func (r *sourceRepository) ensureWorkspace(workspaceID string) {
	if _, exists := r.sources[workspaceID]; !exists {
		r.sources[workspaceID] = make(map[model.SourceID]*model.Source)
	}
}

// copySource creates a deep copy of a source
func copySource(source *model.Source) *model.Source {
	copied := &model.Source{
		ID:          source.ID,
		Name:        source.Name,
		SourceType:  source.SourceType,
		Description: source.Description,
		Enabled:     source.Enabled,
		CreatedAt:   source.CreatedAt,
		UpdatedAt:   source.UpdatedAt,
	}

	if source.NotionDBConfig != nil {
		copied.NotionDBConfig = &model.NotionDBConfig{
			DatabaseID:    source.NotionDBConfig.DatabaseID,
			DatabaseTitle: source.NotionDBConfig.DatabaseTitle,
			DatabaseURL:   source.NotionDBConfig.DatabaseURL,
		}
	}

	if source.NotionPageConfig != nil {
		copied.NotionPageConfig = &model.NotionPageConfig{
			PageID:    source.NotionPageConfig.PageID,
			PageTitle: source.NotionPageConfig.PageTitle,
			PageURL:   source.NotionPageConfig.PageURL,
			Recursive: source.NotionPageConfig.Recursive,
			MaxDepth:  source.NotionPageConfig.MaxDepth,
		}
	}

	if source.SlackConfig != nil {
		channels := make([]model.SlackChannel, len(source.SlackConfig.Channels))
		for i, ch := range source.SlackConfig.Channels {
			channels[i] = model.SlackChannel{
				ID:   ch.ID,
				Name: ch.Name,
			}
		}
		copied.SlackConfig = &model.SlackConfig{
			Channels: channels,
		}
	}

	return copied
}

func (r *sourceRepository) Create(ctx context.Context, workspaceID string, source *model.Source) (*model.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)

	now := time.Now().UTC()
	created := copySource(source)
	if created.ID == "" {
		created.ID = model.NewSourceID()
	}
	created.CreatedAt = now
	created.UpdatedAt = now

	r.sources[workspaceID][created.ID] = created
	return copySource(created), nil
}

func (r *sourceRepository) Get(ctx context.Context, workspaceID string, id model.SourceID) (*model.Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.sources[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	source, exists := ws[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	return copySource(source), nil
}

func (r *sourceRepository) List(ctx context.Context, workspaceID string) ([]*model.Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.sources[workspaceID]
	if !exists {
		return []*model.Source{}, nil
	}

	sources := make([]*model.Source, 0, len(ws))
	for _, source := range ws {
		sources = append(sources, copySource(source))
	}

	return sources, nil
}

func (r *sourceRepository) Update(ctx context.Context, workspaceID string, source *model.Source) (*model.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.sources[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", source.ID))
	}

	existing, exists := ws[source.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", source.ID))
	}

	updated := copySource(source)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.sources[workspaceID][updated.ID] = updated
	return copySource(updated), nil
}

func (r *sourceRepository) Delete(ctx context.Context, workspaceID string, id model.SourceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.sources[workspaceID]
	if !exists {
		return goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	if _, exists := ws[id]; !exists {
		return goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	delete(r.sources[workspaceID], id)
	return nil
}
