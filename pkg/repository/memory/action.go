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
	actions map[string]map[int64]*model.Action
	nextID  map[string]int64
}

func newActionRepository() *actionRepository {
	return &actionRepository{
		actions: make(map[string]map[int64]*model.Action),
		nextID:  make(map[string]int64),
	}
}

func (r *actionRepository) ensureWorkspace(workspaceID string) {
	if _, exists := r.actions[workspaceID]; !exists {
		r.actions[workspaceID] = make(map[int64]*model.Action)
	}
	if _, exists := r.nextID[workspaceID]; !exists {
		r.nextID[workspaceID] = 1
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

func (r *actionRepository) Create(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)

	now := time.Now().UTC()
	created := copyAction(action)
	created.ID = r.nextID[workspaceID]
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID[workspaceID]++

	r.actions[workspaceID][created.ID] = created
	return copyAction(created), nil
}

func (r *actionRepository) Get(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.actions[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	action, exists := ws[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	return copyAction(action), nil
}

func (r *actionRepository) List(ctx context.Context, workspaceID string) ([]*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.actions[workspaceID]
	if !exists {
		return []*model.Action{}, nil
	}

	actions := make([]*model.Action, 0, len(ws))
	for _, action := range ws {
		actions = append(actions, copyAction(action))
	}

	return actions, nil
}

func (r *actionRepository) Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.actions[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", action.ID))
	}

	existing, exists := ws[action.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", action.ID))
	}

	updated := copyAction(action)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.actions[workspaceID][updated.ID] = updated
	return copyAction(updated), nil
}

func (r *actionRepository) Delete(ctx context.Context, workspaceID string, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.actions[workspaceID]
	if !exists {
		return goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	if _, exists := ws[id]; !exists {
		return goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
	}

	delete(r.actions[workspaceID], id)
	return nil
}

func (r *actionRepository) GetByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.actions[workspaceID]
	if !exists {
		return []*model.Action{}, nil
	}

	actions := make([]*model.Action, 0)
	for _, action := range ws {
		if action.CaseID == caseID {
			actions = append(actions, copyAction(action))
		}
	}

	return actions, nil
}

func (r *actionRepository) GetByCases(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Action, error) {
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

	ws, exists := r.actions[workspaceID]
	if !exists {
		return result, nil
	}

	// Collect actions for each case
	for _, action := range ws {
		if caseIDMap[action.CaseID] {
			result[action.CaseID] = append(result[action.CaseID], copyAction(action))
		}
	}

	return result, nil
}
