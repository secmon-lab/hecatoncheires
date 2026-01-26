package graphql

import (
	"context"
	"sync"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// KnowledgesByRiskLoader batches and caches knowledge retrieval by risk IDs within a single request
type KnowledgesByRiskLoader struct {
	repo interfaces.Repository

	// Request-scoped cache
	mu    sync.RWMutex
	cache map[int64][]*model.Knowledge

	// Batching support
	batchMu sync.Mutex
	batch   []int64
	waiting []chan map[int64][]*model.Knowledge
}

// NewKnowledgesByRiskLoader creates a new KnowledgesByRiskLoader
func NewKnowledgesByRiskLoader(repo interfaces.Repository) *KnowledgesByRiskLoader {
	return &KnowledgesByRiskLoader{
		repo:  repo,
		cache: make(map[int64][]*model.Knowledge),
	}
}

// Load retrieves knowledges for a single risk ID
// Uses batching and caching to avoid N+1 queries
func (l *KnowledgesByRiskLoader) Load(ctx context.Context, riskID int64) ([]*model.Knowledge, error) {
	// Check cache first
	l.mu.RLock()
	if knowledges, ok := l.cache[riskID]; ok {
		l.mu.RUnlock()
		return knowledges, nil
	}
	l.mu.RUnlock()

	// Not in cache, add to batch
	resultCh := make(chan map[int64][]*model.Knowledge, 1)

	l.batchMu.Lock()
	l.batch = append(l.batch, riskID)
	l.waiting = append(l.waiting, resultCh)

	// If this is the first item in the batch, start a goroutine to execute it
	if len(l.batch) == 1 {
		batch := l.batch
		waiting := l.waiting
		l.batch = nil
		l.waiting = nil
		l.batchMu.Unlock()

		go l.executeBatch(ctx, batch, waiting)
	} else {
		l.batchMu.Unlock()
	}

	// Wait for batch result
	result := <-resultCh
	return result[riskID], nil
}

// executeBatch executes a batch of risk IDs
func (l *KnowledgesByRiskLoader) executeBatch(ctx context.Context, riskIDs []int64, waiting []chan map[int64][]*model.Knowledge) {
	// Execute batch query
	results, err := l.repo.Knowledge().ListByRiskIDs(ctx, riskIDs)
	if err != nil {
		// On error, return empty results
		results = make(map[int64][]*model.Knowledge, len(riskIDs))
		for _, id := range riskIDs {
			results[id] = []*model.Knowledge{}
		}
	}

	// Update cache
	l.mu.Lock()
	for riskID, knowledges := range results {
		l.cache[riskID] = knowledges
	}
	l.mu.Unlock()

	// Notify all waiting channels
	for _, ch := range waiting {
		ch <- results
		close(ch)
	}
}

// LoadMany retrieves knowledges for multiple risk IDs
func (l *KnowledgesByRiskLoader) LoadMany(ctx context.Context, riskIDs []int64) (map[int64][]*model.Knowledge, error) {
	// Check cache for all IDs
	uncachedIDs := []int64{}
	result := make(map[int64][]*model.Knowledge, len(riskIDs))

	l.mu.RLock()
	for _, riskID := range riskIDs {
		if knowledges, ok := l.cache[riskID]; ok {
			result[riskID] = knowledges
		} else {
			uncachedIDs = append(uncachedIDs, riskID)
		}
	}
	l.mu.RUnlock()

	// If all are cached, return immediately
	if len(uncachedIDs) == 0 {
		return result, nil
	}

	// Fetch uncached IDs
	uncachedResults, err := l.repo.Knowledge().ListByRiskIDs(ctx, uncachedIDs)
	if err != nil {
		return nil, err
	}

	// Update cache and merge results
	l.mu.Lock()
	for riskID, knowledges := range uncachedResults {
		l.cache[riskID] = knowledges
		result[riskID] = knowledges
	}
	l.mu.Unlock()

	return result, nil
}
