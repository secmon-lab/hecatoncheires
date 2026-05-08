package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

type actionMessageRepository struct {
	mu       sync.RWMutex
	messages map[string][]*slack.Message // key: "workspaceID/actionID"
}

var _ interfaces.ActionMessageRepository = &actionMessageRepository{}

func newActionMessageRepository() *actionMessageRepository {
	return &actionMessageRepository{
		messages: make(map[string][]*slack.Message),
	}
}

func actionMessageKey(workspaceID string, actionID int64) string {
	return fmt.Sprintf("%s/%d", workspaceID, actionID)
}

func (r *actionMessageRepository) Put(_ context.Context, workspaceID string, actionID int64, msg *slack.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := actionMessageKey(workspaceID, actionID)

	msgs := r.messages[key]
	for i, m := range msgs {
		if m.ID() == msg.ID() {
			msgs = append(msgs[:i], msgs[i+1:]...)
			break
		}
	}

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
		msg.Files(),
	)
	r.messages[key] = append(msgs, copied)
	return nil
}

func (r *actionMessageRepository) List(_ context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*slack.Message, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	key := actionMessageKey(workspaceID, actionID)
	msgs := r.messages[key]

	sorted := make([]*slack.Message, len(msgs))
	copy(sorted, msgs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt().After(sorted[j].CreatedAt())
	})

	startIdx := 0
	if cursor != "" {
		for i, m := range sorted {
			if m.ID() == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	end := startIdx + limit
	hasMore := end < len(sorted)
	if end > len(sorted) {
		end = len(sorted)
	}

	result := make([]*slack.Message, 0, end-startIdx)
	for _, m := range sorted[startIdx:end] {
		copied := slack.NewMessageFromData(
			m.ID(), m.ChannelID(), m.ThreadTS(), m.TeamID(),
			m.UserID(), m.UserName(), m.Text(), m.EventTS(), m.CreatedAt(),
			m.Files(),
		)
		result = append(result, copied)
	}

	var nextCursor string
	if hasMore && len(result) > 0 {
		nextCursor = result[len(result)-1].ID()
	}

	return result, nextCursor, nil
}
