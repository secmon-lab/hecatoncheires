package agentarchive

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
)

// MemoryHistoryRepository is an in-process gollem.HistoryRepository for
// tests and local development. It stores serialized history blobs to mirror
// the wire format of the production Cloud Storage backend.
type MemoryHistoryRepository struct {
	mu      sync.RWMutex
	entries map[string][]byte
}

var _ gollem.HistoryRepository = (*MemoryHistoryRepository)(nil)

// NewMemoryHistoryRepository creates an empty MemoryHistoryRepository.
func NewMemoryHistoryRepository() *MemoryHistoryRepository {
	return &MemoryHistoryRepository{entries: make(map[string][]byte)}
}

// Load returns the history previously saved under sessionID, or (nil, nil) if
// none exists.
func (r *MemoryHistoryRepository) Load(_ context.Context, sessionID string) (*gollem.History, error) {
	if sessionID == "" {
		return nil, goerr.New("sessionID is required")
	}
	r.mu.RLock()
	data, ok := r.entries[sessionID]
	r.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	var h gollem.History
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, goerr.Wrap(err, "failed to decode in-memory history")
	}
	return &h, nil
}

// Save stores history under sessionID, overwriting any existing entry.
//
// gollem may call Save with a nil history when the underlying session has no
// recorded turns yet (e.g. tests with a stub session). Treat that as a no-op
// rather than an error so Execute does not fail spuriously.
func (r *MemoryHistoryRepository) Save(_ context.Context, sessionID string, history *gollem.History) error {
	if sessionID == "" {
		return goerr.New("sessionID is required")
	}
	if history == nil {
		return nil
	}
	data, err := json.Marshal(history)
	if err != nil {
		return goerr.Wrap(err, "failed to encode history")
	}
	r.mu.Lock()
	r.entries[sessionID] = data
	r.mu.Unlock()
	return nil
}
