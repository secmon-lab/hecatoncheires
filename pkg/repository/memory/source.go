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
	sources map[model.SourceID]*model.Source
}

func newSourceRepository() *sourceRepository {
	return &sourceRepository{
		sources: make(map[model.SourceID]*model.Source),
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

	return copied
}

func (r *sourceRepository) Create(ctx context.Context, source *model.Source) (*model.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copySource(source)
	if created.ID == "" {
		created.ID = model.NewSourceID()
	}
	created.CreatedAt = now
	created.UpdatedAt = now

	r.sources[created.ID] = created
	return copySource(created), nil
}

func (r *sourceRepository) Get(ctx context.Context, id model.SourceID) (*model.Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	source, exists := r.sources[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	return copySource(source), nil
}

func (r *sourceRepository) List(ctx context.Context) ([]*model.Source, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sources := make([]*model.Source, 0, len(r.sources))
	for _, source := range r.sources {
		sources = append(sources, copySource(source))
	}

	return sources, nil
}

func (r *sourceRepository) Update(ctx context.Context, source *model.Source) (*model.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.sources[source.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", source.ID))
	}

	updated := copySource(source)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.sources[updated.ID] = updated
	return copySource(updated), nil
}

func (r *sourceRepository) Delete(ctx context.Context, id model.SourceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.sources[id]; !exists {
		return goerr.Wrap(ErrNotFound, "source not found", goerr.V("id", id))
	}

	delete(r.sources, id)
	return nil
}
