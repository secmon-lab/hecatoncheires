package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type responseRepository struct {
	mu        sync.RWMutex
	responses map[int64]*model.Response
	nextID    int64
}

func newResponseRepository() *responseRepository {
	return &responseRepository{
		responses: make(map[int64]*model.Response),
		nextID:    1,
	}
}

// copyResponse creates a deep copy of a response
func copyResponse(resp *model.Response) *model.Response {
	responderIDs := make([]string, len(resp.ResponderIDs))
	copy(responderIDs, resp.ResponderIDs)

	return &model.Response{
		ID:           resp.ID,
		Title:        resp.Title,
		Description:  resp.Description,
		ResponderIDs: responderIDs,
		URL:          resp.URL,
		Status:       resp.Status,
		CreatedAt:    resp.CreatedAt,
		UpdatedAt:    resp.UpdatedAt,
	}
}

func (r *responseRepository) Create(ctx context.Context, response *model.Response) (*model.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copyResponse(response)
	created.ID = r.nextID
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID++

	r.responses[created.ID] = created
	return copyResponse(created), nil
}

func (r *responseRepository) Get(ctx context.Context, id int64) (*model.Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	response, exists := r.responses[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", id))
	}

	return copyResponse(response), nil
}

func (r *responseRepository) List(ctx context.Context) ([]*model.Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	responses := make([]*model.Response, 0, len(r.responses))
	for _, response := range r.responses {
		responses = append(responses, copyResponse(response))
	}

	return responses, nil
}

func (r *responseRepository) Update(ctx context.Context, response *model.Response) (*model.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.responses[response.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", response.ID))
	}

	updated := copyResponse(response)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.responses[updated.ID] = updated
	return copyResponse(updated), nil
}

func (r *responseRepository) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.responses[id]; !exists {
		return goerr.Wrap(ErrNotFound, "response not found", goerr.V("id", id))
	}

	delete(r.responses, id)
	return nil
}
