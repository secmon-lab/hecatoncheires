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

type actionEventRepository struct {
	mu     sync.RWMutex
	events map[string][]*model.ActionEvent // key: "{workspaceID}/{actionID}"
}

var _ interfaces.ActionEventRepository = &actionEventRepository{}

func newActionEventRepository() *actionEventRepository {
	return &actionEventRepository{events: make(map[string][]*model.ActionEvent)}
}

func actionEventKey(workspaceID string, actionID int64) string {
	return fmt.Sprintf("%s/%d", workspaceID, actionID)
}

func copyActionEvent(e *model.ActionEvent) *model.ActionEvent {
	c := *e
	return &c
}

func (r *actionEventRepository) Put(ctx context.Context, workspaceID string, actionID int64, event *model.ActionEvent) error {
	if event == nil {
		return goerr.New("action event is nil")
	}
	if event.ID == "" {
		return goerr.New("action event id is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionEventKey(workspaceID, actionID)
	existing := r.events[key]
	for i, e := range existing {
		if e.ID == event.ID {
			existing[i] = copyActionEvent(event)
			r.events[key] = existing
			return nil
		}
	}
	r.events[key] = append(existing, copyActionEvent(event))
	return nil
}

func (r *actionEventRepository) List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionEvent, string, error) {
	if limit <= 0 {
		limit = 100
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	events := r.events[actionEventKey(workspaceID, actionID)]
	if len(events) == 0 {
		return []*model.ActionEvent{}, "", nil
	}

	sorted := make([]*model.ActionEvent, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	startIdx := 0
	if cursor != "" {
		found := -1
		for i, e := range sorted {
			if e.ID == cursor {
				found = i
				break
			}
		}
		if found < 0 {
			return []*model.ActionEvent{}, "", nil
		}
		startIdx = found + 1
	}

	end := startIdx + limit
	hasMore := end < len(sorted)
	if end > len(sorted) {
		end = len(sorted)
	}

	result := make([]*model.ActionEvent, 0, end-startIdx)
	for _, e := range sorted[startIdx:end] {
		result = append(result, copyActionEvent(e))
	}

	var nextCursor string
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].ID
	}
	return result, nextCursor, nil
}
