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
	cases  map[string]map[int64]*model.Case
	nextID map[string]int64
}

func newCaseRepository() *caseRepository {
	return &caseRepository{
		cases:  make(map[string]map[int64]*model.Case),
		nextID: make(map[string]int64),
	}
}

func (r *caseRepository) ensureWorkspace(workspaceID string) {
	if _, exists := r.cases[workspaceID]; !exists {
		r.cases[workspaceID] = make(map[int64]*model.Case)
	}
	if _, exists := r.nextID[workspaceID]; !exists {
		r.nextID[workspaceID] = 1
	}
}

// copyFieldValue creates a deep copy of a field value
func copyFieldValue(fv model.FieldValue) model.FieldValue {
	copied := model.FieldValue{
		FieldID: fv.FieldID,
		Type:    fv.Type,
	}
	switch v := fv.Value.(type) {
	case []string:
		s := make([]string, len(v))
		copy(s, v)
		copied.Value = s
	case []interface{}:
		s := make([]interface{}, len(v))
		copy(s, v)
		copied.Value = s
	default:
		copied.Value = fv.Value
	}
	return copied
}

// copyCase creates a deep copy of a case
func copyCase(c *model.Case) *model.Case {
	assigneeIDs := make([]string, len(c.AssigneeIDs))
	copy(assigneeIDs, c.AssigneeIDs)

	var fieldValues map[string]model.FieldValue
	if c.FieldValues != nil {
		fieldValues = make(map[string]model.FieldValue, len(c.FieldValues))
		for k, v := range c.FieldValues {
			fieldValues[k] = copyFieldValue(v)
		}
	}

	return &model.Case{
		ID:             c.ID,
		Title:          c.Title,
		Description:    c.Description,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: c.SlackChannelID,
		FieldValues:    fieldValues,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func (r *caseRepository) Create(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)

	now := time.Now().UTC()
	created := copyCase(c)
	created.ID = r.nextID[workspaceID]
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID[workspaceID]++

	r.cases[workspaceID][created.ID] = created
	return copyCase(created), nil
}

func (r *caseRepository) Get(ctx context.Context, workspaceID string, id int64) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	c, exists := ws[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	return copyCase(c), nil
}

func (r *caseRepository) List(ctx context.Context, workspaceID string) ([]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return []*model.Case{}, nil
	}

	cases := make([]*model.Case, 0, len(ws))
	for _, c := range ws {
		cases = append(cases, copyCase(c))
	}

	return cases, nil
}

func (r *caseRepository) Update(ctx context.Context, workspaceID string, c *model.Case) (*model.Case, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
	}

	existing, exists := ws[c.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", c.ID))
	}

	updated := copyCase(c)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.cases[workspaceID][updated.ID] = updated
	return copyCase(updated), nil
}

func (r *caseRepository) Delete(ctx context.Context, workspaceID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	if _, exists := ws[id]; !exists {
		return goerr.Wrap(ErrNotFound, "case not found", goerr.V("id", id))
	}

	delete(r.cases[workspaceID], id)
	return nil
}
