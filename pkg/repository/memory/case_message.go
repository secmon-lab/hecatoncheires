package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

type caseMessageRepository struct {
	mu       sync.RWMutex
	messages map[string][]*slack.Message // key: "workspaceID/caseID"
}

var _ interfaces.CaseMessageRepository = &caseMessageRepository{}

func newCaseMessageRepository() *caseMessageRepository {
	return &caseMessageRepository{
		messages: make(map[string][]*slack.Message),
	}
}

func caseMessageKey(workspaceID string, caseID int64) string {
	return fmt.Sprintf("%s/%d", workspaceID, caseID)
}

func (r *caseMessageRepository) Put(_ context.Context, workspaceID string, caseID int64, msg *slack.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := caseMessageKey(workspaceID, caseID)

	// Upsert: remove existing message with same ID
	msgs := r.messages[key]
	for i, m := range msgs {
		if m.ID() == msg.ID() {
			msgs = append(msgs[:i], msgs[i+1:]...)
			break
		}
	}

	// Re-create message to avoid external mutation
	copied := slack.NewMessageFromData(
		msg.ID(),
		msg.ChannelID(),
		msg.ThreadTS(),
		msg.TeamID(),
		msg.UserID(),
		msg.UserName(),
		msg.Text(),
		msg.EventTS(),
		msg.CreatedAt(),
	)
	r.messages[key] = append(msgs, copied)
	return nil
}

func (r *caseMessageRepository) List(_ context.Context, workspaceID string, caseID int64, limit int, cursor string) ([]*slack.Message, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	key := caseMessageKey(workspaceID, caseID)
	msgs := r.messages[key]

	// Sort by CreatedAt desc
	sorted := make([]*slack.Message, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt().After(sorted[j].CreatedAt())
	})

	// Apply cursor
	startIdx := 0
	if cursor != "" {
		for i, m := range sorted {
			if m.ID() == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Paginate
	end := startIdx + limit
	var hasMore bool
	if end < len(sorted) {
		hasMore = true
	}
	if end > len(sorted) {
		end = len(sorted)
	}

	result := make([]*slack.Message, 0, end-startIdx)
	for _, m := range sorted[startIdx:end] {
		copied := slack.NewMessageFromData(
			m.ID(), m.ChannelID(), m.ThreadTS(), m.TeamID(),
			m.UserID(), m.UserName(), m.Text(), m.EventTS(), m.CreatedAt(),
		)
		result = append(result, copied)
	}

	var nextCursor string
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].ID()
	}

	return result, nextCursor, nil
}

func (r *caseMessageRepository) Prune(_ context.Context, workspaceID string, caseID int64, before time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := caseMessageKey(workspaceID, caseID)
	msgs := r.messages[key]

	var remaining []*slack.Message
	deleted := 0
	for _, m := range msgs {
		if m.CreatedAt().Before(before) {
			deleted++
		} else {
			remaining = append(remaining, m)
		}
	}

	r.messages[key] = remaining
	return deleted, nil
}
