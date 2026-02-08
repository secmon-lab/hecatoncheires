package memory

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type knowledgeRepository struct {
	mu        sync.RWMutex
	knowledge map[string]map[model.KnowledgeID]*model.Knowledge
}

func newKnowledgeRepository() *knowledgeRepository {
	return &knowledgeRepository{
		knowledge: make(map[string]map[model.KnowledgeID]*model.Knowledge),
	}
}

func (r *knowledgeRepository) ensureWorkspace(workspaceID string) {
	if _, exists := r.knowledge[workspaceID]; !exists {
		r.knowledge[workspaceID] = make(map[model.KnowledgeID]*model.Knowledge)
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

func (r *knowledgeRepository) Create(ctx context.Context, workspaceID string, knowledge *model.Knowledge) (*model.Knowledge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureWorkspace(workspaceID)

	now := time.Now().UTC()
	created := copyKnowledge(knowledge)
	if created.ID == "" {
		created.ID = model.NewKnowledgeID()
	}
	created.CreatedAt = now
	created.UpdatedAt = now

	r.knowledge[workspaceID][created.ID] = created
	return copyKnowledge(created), nil
}

func (r *knowledgeRepository) Get(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	knowledge, exists := ws[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	return copyKnowledge(knowledge), nil
}

func (r *knowledgeRepository) ListByCaseID(ctx context.Context, workspaceID string, caseID int64) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return []*model.Knowledge{}, nil
	}

	var result []*model.Knowledge
	for _, k := range ws {
		if k.CaseID == caseID {
			result = append(result, copyKnowledge(k))
		}
	}

	return result, nil
}

func (r *knowledgeRepository) ListByCaseIDs(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a map of case IDs for fast lookup
	caseIDSet := make(map[int64]bool, len(caseIDs))
	for _, id := range caseIDs {
		caseIDSet[id] = true
	}

	// Group knowledges by case ID
	result := make(map[int64][]*model.Knowledge, len(caseIDs))

	ws, exists := r.knowledge[workspaceID]
	if exists {
		for _, k := range ws {
			if caseIDSet[k.CaseID] {
				result[k.CaseID] = append(result[k.CaseID], copyKnowledge(k))
			}
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

func (r *knowledgeRepository) ListBySourceID(ctx context.Context, workspaceID string, sourceID model.SourceID) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return []*model.Knowledge{}, nil
	}

	var result []*model.Knowledge
	for _, k := range ws {
		if k.SourceID == sourceID {
			result = append(result, copyKnowledge(k))
		}
	}

	return result, nil
}

func (r *knowledgeRepository) ListWithPagination(ctx context.Context, workspaceID string, limit, offset int) ([]*model.Knowledge, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return []*model.Knowledge{}, 0, nil
	}

	// Collect all knowledge entries and sort by CreatedAt descending
	all := make([]*model.Knowledge, 0, len(ws))
	for _, k := range ws {
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

func (r *knowledgeRepository) Delete(ctx context.Context, workspaceID string, id model.KnowledgeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	if _, exists := ws[id]; !exists {
		return goerr.Wrap(ErrNotFound, "knowledge not found", goerr.V("id", id))
	}

	delete(r.knowledge[workspaceID], id)
	return nil
}

func (r *knowledgeRepository) FindByEmbedding(ctx context.Context, workspaceID string, embedding []float32, limit int) ([]*model.Knowledge, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ws, exists := r.knowledge[workspaceID]
	if !exists {
		return []*model.Knowledge{}, nil
	}

	type scored struct {
		knowledge *model.Knowledge
		score     float64
	}

	var candidates []scored
	for _, k := range ws {
		if len(k.Embedding) == 0 {
			continue
		}
		s := cosineSimilarity(embedding, k.Embedding)
		candidates = append(candidates, scored{knowledge: copyKnowledge(k), score: s})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}

	result := make([]*model.Knowledge, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].knowledge
	}

	return result, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return dot / denom
}
