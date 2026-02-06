package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type caseRepository struct {
	mu     sync.RWMutex
	cases  map[int64]*model.Case
	nextID int64
}

func newCaseRepository() *caseRepository {
	return &caseRepository{
		cases:  make(map[int64]*model.Case),
		nextID: 1,
	}
}

// copyCase creates a deep copy of a case
func copyCase(c *model.Case) *model.Case {
	assigneeIDs := make([]string, len(c.AssigneeIDs))
	copy(assigneeIDs, c.AssigneeIDs)

	return &model.Case{
		ID:             c.ID,
		Title:          c.Title,
		Description:    c.Description,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: c.SlackChannelID,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func (r *caseRepository) Create(ctx context.Context, c *model.Case) (*model.Case, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copyCase(c)
	created.ID = r.nextID
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID++

	r.cases[created.ID] = created
	return copyCase(created), nil
}

func (r *caseRepository) Get(ctx context.Context, id int64) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, exists := r.cases[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	return copyCase(c), nil
}

func (r *caseRepository) List(ctx context.Context) ([]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cases := make([]*model.Case, 0, len(r.cases))
	for _, c := range r.cases {
		cases = append(cases, copyCase(c))
	}

	return cases, nil
}

func (r *caseRepository) Update(ctx context.Context, c *model.Case) (*model.Case, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.cases[c.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
	}

	updated := copyCase(c)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.cases[updated.ID] = updated
	return copyCase(updated), nil
}

func (r *caseRepository) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.cases[id]; !exists {
		return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	delete(r.cases, id)
	return nil
}
