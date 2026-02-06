package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type actionRepository struct {
	mu      sync.RWMutex
	actions map[int64]*model.Action
	nextID  int64
}

func newActionRepository() *actionRepository {
	return &actionRepository{
		actions: make(map[int64]*model.Action),
		nextID:  1,
	}
}

// copyAction creates a deep copy of an action
func copyAction(a *model.Action) *model.Action {
	assigneeIDs := make([]string, len(a.AssigneeIDs))
	copy(assigneeIDs, a.AssigneeIDs)

	return &model.Action{
		ID:             a.ID,
		CaseID:         a.CaseID,
		Title:          a.Title,
		Description:    a.Description,
		AssigneeIDs:    assigneeIDs,
		SlackMessageTS: a.SlackMessageTS,
		Status:         a.Status,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}
}

func (r *actionRepository) Create(ctx context.Context, action *model.Action) (*model.Action, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copyAction(action)
	created.ID = r.nextID
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID++

	r.actions[created.ID] = created
	return copyAction(created), nil
}

func (r *actionRepository) Get(ctx context.Context, id int64) (*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	action, exists := r.actions[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	return copyAction(action), nil
}

func (r *actionRepository) List(ctx context.Context) ([]*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	actions := make([]*model.Action, 0, len(r.actions))
	for _, action := range r.actions {
		actions = append(actions, copyAction(action))
	}

	return actions, nil
}

func (r *actionRepository) Update(ctx context.Context, action *model.Action) (*model.Action, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.actions[action.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", action.ID))
	}

	updated := copyAction(action)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.actions[updated.ID] = updated
	return copyAction(updated), nil
}

func (r *actionRepository) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actions[id]; !exists {
		return goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	delete(r.actions, id)
	return nil
}

func (r *actionRepository) GetByCase(ctx context.Context, caseID int64) ([]*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	actions := make([]*model.Action, 0)
	for _, action := range r.actions {
		if action.CaseID == caseID {
			actions = append(actions, copyAction(action))
		}
	}

	return actions, nil
}

func (r *actionRepository) GetByCases(ctx context.Context, caseIDs []int64) (map[int64][]*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a map for quick lookup
	caseIDMap := make(map[int64]bool)
	for _, caseID := range caseIDs {
		caseIDMap[caseID] = true
	}

	// Initialize result map with empty slices for each case ID
	result := make(map[int64][]*model.Action)
	for _, caseID := range caseIDs {
		result[caseID] = make([]*model.Action, 0)
	}

	// Collect actions for each case
	for _, action := range r.actions {
		if caseIDMap[action.CaseID] {
			result[action.CaseID] = append(result[action.CaseID], copyAction(action))
		}
	}

	return result, nil
}
