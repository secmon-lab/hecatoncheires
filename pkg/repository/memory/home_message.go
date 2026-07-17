package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// homeMessageRepository stores generated home messages append-only per user.
type homeMessageRepository struct {
	mu   sync.RWMutex
	data map[string][]*model.HomeMessage // userID -> messages (append order)
}

func newHomeMessageRepository() *homeMessageRepository {
	return &homeMessageRepository{
		data: make(map[string][]*model.HomeMessage),
	}
}

func copyHomeMessage(m *model.HomeMessage) *model.HomeMessage {
	copied := *m
	return &copied
}

func (r *homeMessageRepository) Add(ctx context.Context, msg *model.HomeMessage) error {
	if err := msg.Validate(); err != nil {
		return goerr.Wrap(err, "home message validation failed before add")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[msg.UserID] = append(r.data[msg.UserID], copyHomeMessage(msg))
	return nil
}

func (r *homeMessageRepository) ListRecent(ctx context.Context, userID string, limit int) ([]*model.HomeMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msgs := r.data[userID]
	sorted := make([]*model.HomeMessage, len(msgs))
	for i, m := range msgs {
		sorted[i] = copyHomeMessage(m)
	}
	// Newest first, tie-broken by ID (UUID v7 is lexicographically time-ordered)
	// so the ordering matches the Firestore CreatedAt-desc query.
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	if limit >= 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted, nil
}
