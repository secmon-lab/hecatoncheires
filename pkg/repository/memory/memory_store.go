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

// memoryKey is a composite key for memory entries (workspaceID + caseID)
type memoryKey struct {
	workspaceID string
	caseID      int64
}

type memoryRepository struct {
	mu      sync.RWMutex
	entries map[memoryKey]map[model.MemoryID]*model.Memory
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		entries: make(map[memoryKey]map[model.MemoryID]*model.Memory),
	}
}

func (r *memoryRepository) ensureKey(key memoryKey) {
	if _, exists := r.entries[key]; !exists {
		r.entries[key] = make(map[model.MemoryID]*model.Memory)
	}
}

func copyMemory(m *model.Memory) *model.Memory {
	copied := &model.Memory{
		ID:        m.ID,
		CaseID:    m.CaseID,
		Claim:     m.Claim,
		CreatedAt: m.CreatedAt,
	}
	if m.Embedding != nil {
		copied.Embedding = make([]float32, len(m.Embedding))
		copy(copied.Embedding, m.Embedding)
	}
	return copied
}

func (r *memoryRepository) Create(ctx context.Context, workspaceID string, caseID int64, mem *model.Memory) (*model.Memory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := memoryKey{workspaceID: workspaceID, caseID: caseID}
	r.ensureKey(key)

	created := copyMemory(mem)
	if created.ID == "" {
		created.ID = model.NewMemoryID()
	}
	created.CaseID = caseID
	created.CreatedAt = time.Now().UTC()

	r.entries[key][created.ID] = created
	return copyMemory(created), nil
}

func (r *memoryRepository) Get(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) (*model.Memory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := memoryKey{workspaceID: workspaceID, caseID: caseID}
	bucket, exists := r.entries[key]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
	}

	mem, exists := bucket[memoryID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
	}

	return copyMemory(mem), nil
}

func (r *memoryRepository) Delete(ctx context.Context, workspaceID string, caseID int64, memoryID model.MemoryID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := memoryKey{workspaceID: workspaceID, caseID: caseID}
	bucket, exists := r.entries[key]
	if !exists {
		return goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
	}

	if _, exists := bucket[memoryID]; !exists {
		return goerr.Wrap(ErrNotFound, "memory not found", goerr.V("memoryID", memoryID))
	}

	delete(bucket, memoryID)
	return nil
}

func (r *memoryRepository) List(ctx context.Context, workspaceID string, caseID int64) ([]*model.Memory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := memoryKey{workspaceID: workspaceID, caseID: caseID}
	bucket, exists := r.entries[key]
	if !exists {
		return []*model.Memory{}, nil
	}

	result := make([]*model.Memory, 0, len(bucket))
	for _, m := range bucket {
		result = append(result, copyMemory(m))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

func (r *memoryRepository) FindByEmbedding(ctx context.Context, workspaceID string, caseID int64, embedding []float32, limit int) ([]*model.Memory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := memoryKey{workspaceID: workspaceID, caseID: caseID}
	bucket, exists := r.entries[key]
	if !exists {
		return []*model.Memory{}, nil
	}

	type scored struct {
		memory *model.Memory
		score  float64
	}

	var candidates []scored
	for _, m := range bucket {
		if len(m.Embedding) == 0 {
			continue
		}
		s := memoryCosineSimilarity(embedding, m.Embedding)
		candidates = append(candidates, scored{memory: copyMemory(m), score: s})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}

	result := make([]*model.Memory, limit)
	for i := 0; i < limit; i++ {
		result[i] = candidates[i].memory
	}

	return result, nil
}

func memoryCosineSimilarity(a, b []float32) float64 {
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
