package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type knowledgeRepository struct {
	mu        sync.RWMutex
	knowledge map[model.KnowledgeID]*model.Knowledge
}

func newKnowledgeRepository() *knowledgeRepository {
	return &knowledgeRepository{
		knowledge: make(map[model.KnowledgeID]*model.Knowledge),
	}
}

// copyKnowledge creates a deep copy of a knowledge entry
func copyKnowledge(k *model.Knowledge) *model.Knowledge {
	copied := &model.Knowledge{
		ID:        k.ID,
		CaseID:    k.CaseID,
		SourceID:  k.SourceID,
		SourceURL: k.SourceURL,
		Title:     k.Title,
		Summary:   k.Summary,
		SourcedAt: k.SourcedAt,
		CreatedAt: k.CreatedAt,
		UpdatedAt: k.UpdatedAt,
	}

	if k.Embedding != nil {
		copied.Embedding = make([]float32, len(k.Embedding))
		copy(copied.Embedding, k.Embedding)
	}

	return copied
}

func (r *knowledgeRepository) Create(ctx context.Context, knowledge *model.Knowledge) (*model.Knowledge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copyKnowledge(knowledge)
	if created.ID == "" {
		created.ID = model.NewKnowledgeID()
	}
	created.CreatedAt = now
	created.UpdatedAt = now

	r.knowledge[created.ID] = created
	return copyKnowledge(created), nil
}

func (r *knowledgeRepository) Get(ctx context.Context, id model.KnowledgeID) (*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	knowledge, exists := r.knowledge[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	return copyKnowledge(knowledge), nil
}

func (r *knowledgeRepository) ListByCaseID(ctx context.Context, caseID int64) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Knowledge
	for _, k := range r.knowledge {
		if k.CaseID == caseID {
			result = append(result, copyKnowledge(k))
		}
	}

	return result, nil
}

func (r *knowledgeRepository) ListByCaseIDs(ctx context.Context, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a map of case IDs for fast lookup
	caseIDSet := make(map[int64]bool, len(caseIDs))
	for _, id := range caseIDs {
		caseIDSet[id] = true
	}

	// Group knowledges by case ID
	result := make(map[int64][]*model.Knowledge, len(caseIDs))
	for _, k := range r.knowledge {
		if caseIDSet[k.CaseID] {
			result[k.CaseID] = append(result[k.CaseID], copyKnowledge(k))
		}
	}

	// Ensure all requested IDs are in the result map (even if empty)
	for _, caseID := range caseIDs {
		if _, exists := result[caseID]; !exists {
			result[caseID] = []*model.Knowledge{}
		}
	}

	return result, nil
}

func (r *knowledgeRepository) ListBySourceID(ctx context.Context, sourceID model.SourceID) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Knowledge
	for _, k := range r.knowledge {
		if k.SourceID == sourceID {
			result = append(result, copyKnowledge(k))
		}
	}

	return result, nil
}

func (r *knowledgeRepository) ListWithPagination(ctx context.Context, limit, offset int) ([]*model.Knowledge, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all knowledge entries and sort by CreatedAt descending
	all := make([]*model.Knowledge, 0, len(r.knowledge))
	for _, k := range r.knowledge {
		all = append(all, copyKnowledge(k))
	}

	// Sort by CreatedAt descending
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].CreatedAt.After(all[i].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	totalCount := len(all)

	// Apply pagination
	if offset >= len(all) {
		return []*model.Knowledge{}, totalCount, nil
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}

	return all[offset:end], totalCount, nil
}

func (r *knowledgeRepository) Delete(ctx context.Context, id model.KnowledgeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.knowledge[id]; !exists {
		return goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	delete(r.knowledge, id)
	return nil
}
