package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
)

type slackRepository struct {
	mu       sync.RWMutex
	channels map[string]*channelData
}

type channelData struct {
	teamID   string
	messages map[string]*messageData
}

type messageData struct {
	msg       *slack.Message
	createdAt time.Time
}

var _ interfaces.SlackRepository = &slackRepository{}

func newSlackRepository() *slackRepository {
	return &slackRepository{
		channels: make(map[string]*channelData),
	}
}

func (r *slackRepository) PutMessage(ctx context.Context, msg *slack.Message) error {
	if msg == nil {
		return goerr.New("message is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	channelID := msg.ChannelID()

	// Initialize channel if it doesn't exist
	if _, exists := r.channels[channelID]; !exists {
		r.channels[channelID] = &channelData{
			teamID:   msg.TeamID(),
			messages: make(map[string]*messageData),
		}
	}

	// Store message (upsert)
	r.channels[channelID].messages[msg.ID()] = &messageData{
		msg:       msg,
		createdAt: msg.CreatedAt(),
	}

	return nil
}

func (r *slackRepository) ListMessages(ctx context.Context, channelID string, start, end time.Time, limit int, cursor string) ([]*slack.Message, string, error) {
	if channelID == "" {
		return nil, "", goerr.New("channelID is required")
	}

	if limit <= 0 {
		limit = 100
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	channel, exists := r.channels[channelID]
	if !exists {
		return []*slack.Message{}, "", nil
	}

	// Collect all messages in the time range
	var allMessages []*messageData
	for _, msgData := range channel.messages {
		if msgData.createdAt.After(start) || msgData.createdAt.Equal(start) {
			if msgData.createdAt.Before(end) {
				allMessages = append(allMessages, msgData)
			}
		}
	}

	// Sort by createdAt descending (newest first)
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].createdAt.After(allMessages[j].createdAt)
	})

	// Apply cursor if provided
	startIdx := 0
	if cursor != "" {
		for i, msgData := range allMessages {
			if msgData.msg.ID() == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply limit
	endIdx := startIdx + limit
	if endIdx > len(allMessages) {
		endIdx = len(allMessages)
	}

	// Prepare result
	var messages []*slack.Message
	for i := startIdx; i < endIdx; i++ {
		messages = append(messages, allMessages[i].msg)
	}

	// Determine next cursor
	// If there are more messages after the current page, use the last message ID as cursor
	var nextCursor string
	if endIdx < len(allMessages) && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID()
	}

	return messages, nextCursor, nil
}

func (r *slackRepository) PruneMessages(ctx context.Context, channelID string, before time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	totalDeleted := 0

	if channelID == "" {
		// Delete from all channels
		for cid := range r.channels {
			deleted := r.pruneChannelUnsafe(cid, before)
			totalDeleted += deleted
		}
	} else {
		// Delete from specific channel
		deleted := r.pruneChannelUnsafe(channelID, before)
		totalDeleted += deleted
	}

	return totalDeleted, nil
}

// pruneChannelUnsafe must be called with lock held
func (r *slackRepository) pruneChannelUnsafe(channelID string, before time.Time) int {
	channel, exists := r.channels[channelID]
	if !exists {
		return 0
	}

	deleted := 0
	for msgID, msgData := range channel.messages {
		if msgData.createdAt.Before(before) {
			delete(channel.messages, msgID)
			deleted++
		}
	}

	return deleted
}
