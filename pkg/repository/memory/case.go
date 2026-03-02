package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
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

	channelUserIDs := make([]string, len(c.ChannelUserIDs))
	copy(channelUserIDs, c.ChannelUserIDs)

	return &model.Case{
		ID:             c.ID,
		Title:          c.Title,
		Description:    c.Description,
		Status:         c.Status,
		AssigneeIDs:    assigneeIDs,
		SlackChannelID: c.SlackChannelID,
		IsPrivate:      c.IsPrivate,
		ChannelUserIDs: channelUserIDs,
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

func (r *caseRepository) List(ctx context.Context, workspaceID string, opts ...interfaces.ListCaseOption) ([]*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return []*model.Case{}, nil
	}

	cfg := interfaces.BuildListCaseConfig(opts...)

	cases := make([]*model.Case, 0, len(ws))
	for _, c := range ws {
		// Apply status filter
		if statusFilter := cfg.Status(); statusFilter != nil {
			if c.Status.Normalize() != *statusFilter {
				continue
			}
		}
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

func (r *caseRepository) GetBySlackChannelID(ctx context.Context, workspaceID string, channelID string) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.cases[workspaceID]
	if !exists {
		return nil, nil
	}

	for _, c := range ws {
		if c.SlackChannelID == channelID {
			return copyCase(c), nil
		}
	}

	return nil, nil
}

func (r *caseRepository) CountFieldValues(_ context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (int64, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws := r.cases[workspaceID]
	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	var total, valid int64
	for _, c := range ws {
		fv, ok := c.FieldValues[fieldID]
		if !ok || fv.Type != fieldType {
			continue
		}
		total++
		if fv.IsValueInSet(fieldType, validSet) {
			valid++
		}
	}

	return total, valid, nil
}

func (r *caseRepository) FindCaseWithInvalidFieldValue(_ context.Context, workspaceID string, fieldID string, fieldType types.FieldType, validValues []string) (*model.Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws := r.cases[workspaceID]
	validSet := make(map[string]bool, len(validValues))
	for _, v := range validValues {
		validSet[v] = true
	}

	for _, c := range ws {
		fv, ok := c.FieldValues[fieldID]
		if !ok || fv.Type != fieldType {
			continue
		}
		if !fv.IsValueInSet(fieldType, validSet) {
			return copyCase(c), nil
		}
	}

	return nil, nil
}
