package agentarchive

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem/trace"
)

// MemoryTraceRepository is an in-process trace.Repository for tests. It keeps
// each saved trace (one entry per traceID) so tests can read them back.
type MemoryTraceRepository struct {
	mu      sync.RWMutex
	entries map[string]map[string][]byte // sessionID -> traceID -> json
}

var _ trace.Repository = (*MemoryTraceRepository)(nil)

// NewMemoryTraceRepository creates an empty MemoryTraceRepository.
func NewMemoryTraceRepository() *MemoryTraceRepository {
	return &MemoryTraceRepository{entries: make(map[string]map[string][]byte)}
}

// Save records the trace under (session_id, trace_id).
func (r *MemoryTraceRepository) Save(_ context.Context, t *trace.Trace) error {
	if t == nil {
		return goerr.New("trace is nil")
	}
	sessionID := t.Metadata.Labels[SessionIDLabel]
	if sessionID == "" {
		return goerr.New("trace metadata.Labels[session_id] is required")
	}
	if t.TraceID == "" {
		return goerr.New("trace ID is required")
	}
	data, err := json.Marshal(t)
	if err != nil {
		return goerr.Wrap(err, "failed to encode trace")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries[sessionID] == nil {
		r.entries[sessionID] = make(map[string][]byte)
	}
	r.entries[sessionID][t.TraceID] = data
	return nil
}

// Load returns the persisted trace for the given (sessionID, traceID), or nil
// if absent. Intended for tests only.
func (r *MemoryTraceRepository) Load(sessionID, traceID string) *trace.Trace {
	r.mu.RLock()
	defer r.mu.RUnlock()
	data, ok := r.entries[sessionID][traceID]
	if !ok {
		return nil
	}
	var t trace.Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}
	return &t
}

// TraceIDs returns all traceIDs persisted for the given sessionID.
func (r *MemoryTraceRepository) TraceIDs(sessionID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.entries[sessionID]))
	for id := range r.entries[sessionID] {
		ids = append(ids, id)
	}
	return ids
}
