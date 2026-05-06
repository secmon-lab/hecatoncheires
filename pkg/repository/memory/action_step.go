package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type actionStepRepository struct {
	mu    sync.RWMutex
	steps map[string][]*model.ActionStep // key: "{workspaceID}/{actionID}"
}

var _ interfaces.ActionStepRepository = &actionStepRepository{}

func newActionStepRepository() *actionStepRepository {
	return &actionStepRepository{steps: make(map[string][]*model.ActionStep)}
}

func actionStepKey(workspaceID string, actionID int64) string {
	return fmt.Sprintf("%s/%d", workspaceID, actionID)
}

func copyActionStep(s *model.ActionStep) *model.ActionStep {
	c := *s
	if s.DoneAt != nil {
		t := *s.DoneAt
		c.DoneAt = &t
	}
	return &c
}

func (r *actionStepRepository) Put(ctx context.Context, workspaceID string, step *model.ActionStep) error {
	if step == nil {
		return goerr.New("action step is nil")
	}
	if step.ID == "" {
		return goerr.New("action step id is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionStepKey(workspaceID, step.ActionID)
	existing := r.steps[key]
	for i, s := range existing {
		if s.ID == step.ID {
			existing[i] = copyActionStep(step)
			r.steps[key] = existing
			return nil
		}
	}
	r.steps[key] = append(existing, copyActionStep(step))
	return nil
}

func (r *actionStepRepository) Get(ctx context.Context, workspaceID string, actionID int64, stepID string) (*model.ActionStep, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.steps[actionStepKey(workspaceID, actionID)] {
		if s.ID == stepID {
			return copyActionStep(s), nil
		}
	}
	return nil, goerr.Wrap(ErrNotFound, "action step not found",
		goerr.V("workspace_id", workspaceID),
		goerr.V("action_id", actionID),
		goerr.V("step_id", stepID))
}

func (r *actionStepRepository) List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	source := r.steps[actionStepKey(workspaceID, actionID)]
	if len(source) == 0 {
		return []*model.ActionStep{}, nil
	}
	out := make([]*model.ActionStep, 0, len(source))
	for _, s := range source {
		out = append(out, copyActionStep(s))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r *actionStepRepository) Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionStepKey(workspaceID, actionID)
	existing := r.steps[key]
	for i, s := range existing {
		if s.ID == stepID {
			r.steps[key] = append(existing[:i], existing[i+1:]...)
			return nil
		}
	}
	return nil
}
