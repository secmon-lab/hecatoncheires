package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// assistLogKey is a composite key for assist log entries (workspaceID + caseID)
type assistLogKey struct {
	workspaceID string
	caseID      int64
}

type assistLogRepository struct {
	mu      sync.RWMutex
	entries map[assistLogKey][]*model.AssistLog
}

func newAssistLogRepository() *assistLogRepository {
	return &assistLogRepository{
		entries: make(map[assistLogKey][]*model.AssistLog),
	}
}

func copyAssistLog(l *model.AssistLog) *model.AssistLog {
	return &model.AssistLog{
		ID:        l.ID,
		CaseID:    l.CaseID,
		Summary:   l.Summary,
		Actions:   l.Actions,
		Reasoning: l.Reasoning,
		NextSteps: l.NextSteps,
		CreatedAt: l.CreatedAt,
	}
}

func (r *assistLogRepository) Create(ctx context.Context, workspaceID string, caseID int64, log *model.AssistLog) (*model.AssistLog, error) {
	// Guard nil up front: CaseID is assigned (and Validate called) only after
	// copyAssistLog dereferences log, so a nil log would panic before the
	// post-assignment Validate could reject it.
	if log == nil {
		return nil, goerr.Wrap(model.ErrAssistLogValidation, "assist log is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := assistLogKey{workspaceID: workspaceID, caseID: caseID}

	created := copyAssistLog(log)
	if created.ID == "" {
		created.ID = model.NewAssistLogID()
	}
	created.CaseID = caseID

	// Validate after CaseID is assigned from the parameter (the caller struct
	// may carry a zero CaseID and rely on the repository to set it).
	if err := created.Validate(); err != nil {
		return nil, goerr.Wrap(err, "assist log validation failed before create")
	}

	r.entries[key] = append(r.entries[key], created)
	return copyAssistLog(created), nil
}

func (r *assistLogRepository) List(ctx context.Context, workspaceID string, caseID int64, limit, offset int) ([]*model.AssistLog, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := assistLogKey{workspaceID: workspaceID, caseID: caseID}
	all, exists := r.entries[key]
	if !exists {
		return []*model.AssistLog{}, 0, nil
	}

	// Sort by CreatedAt descending
	sorted := make([]*model.AssistLog, len(all))
	copy(sorted, all)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	totalCount := len(sorted)

	if offset >= totalCount {
		return []*model.AssistLog{}, totalCount, nil
	}

	end := offset + limit
	if end > totalCount {
		end = totalCount
	}

	result := make([]*model.AssistLog, 0, end-offset)
	for _, l := range sorted[offset:end] {
		result = append(result, copyAssistLog(l))
	}

	return result, totalCount, nil
}
