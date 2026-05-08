package slack

import (
	"context"
	"sort"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// MaxCollectedMessages is the maximum number of messages collected for a single
// case-draft generation. Older messages beyond this limit are dropped.
const MaxCollectedMessages = 64

// ChannelLookbackWindow is the time window used when collecting recent messages
// from a non-thread mention. Messages older than this are excluded.
const ChannelLookbackWindow = 3 * time.Hour

// MessageCollector gathers Slack messages around a mention to be used as
// source material for an AI-generated Case draft.
//
// Two collection modes are supported:
//   - Thread: when the mention happened inside a thread, the most recent
//     `MaxCollectedMessages` messages of that thread (including the mention
//     itself) are returned.
//   - Channel-recent: when the mention happened in a channel's main timeline,
//     the most recent `MaxCollectedMessages` messages within the last
//     `ChannelLookbackWindow` (anchored to the mention timestamp) are returned.
//
// Messages from bots (including the mentioning bot itself) are intentionally
// included; the collector does not filter by author.
type MessageCollector struct {
	svc Service
}

// NewMessageCollector returns a collector backed by the given Slack service.
func NewMessageCollector(svc Service) *MessageCollector {
	return &MessageCollector{svc: svc}
}

// threadFetchHardCap is the maximum number of thread messages we ever ask
// Slack for in a single call. Slack's conversations.replies returns messages
// oldest-first up to `limit`; to faithfully implement the "latest N" rule we
// fetch a much larger window and then keep the trailing slice.
//
// 1000 covers virtually all real-world threads while keeping the request
// bounded. Threads exceeding this cap fall back to the oldest 1000 messages,
// then trimmed to the latest MaxCollectedMessages of those.
const threadFetchHardCap = 1000

// CollectThread fetches up to MaxCollectedMessages most recent messages from
// the given thread and resolves their permalinks.
//
// threadTS is the parent message timestamp of the thread. Both the thread's
// parent and replies are included. Messages are returned in chronological
// (oldest-first) order.
func (c *MessageCollector) CollectThread(ctx context.Context, channelID, threadTS string) ([]model.DraftMessage, error) {
	msgs, err := c.svc.GetConversationReplies(ctx, channelID, threadTS, threadFetchHardCap)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch thread replies",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS))
	}

	if len(msgs) > MaxCollectedMessages {
		msgs = msgs[len(msgs)-MaxCollectedMessages:]
	}

	return c.resolveAndConvert(ctx, channelID, msgs)
}

// CollectChannelRecent fetches up to MaxCollectedMessages messages from the
// channel's main timeline that were posted within ChannelLookbackWindow before
// the mention. Returned messages are in chronological (oldest-first) order.
func (c *MessageCollector) CollectChannelRecent(ctx context.Context, channelID string, mentionTime time.Time) ([]model.DraftMessage, error) {
	oldest := mentionTime.Add(-ChannelLookbackWindow)

	msgs, err := c.svc.GetConversationHistory(ctx, channelID, oldest, MaxCollectedMessages)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to fetch channel history",
			goerr.V("channel_id", channelID),
			goerr.V("oldest", oldest))
	}

	// conversations.history returns newest-first; sort to chronological order
	// so consumers see a natural reading order.
	sort.SliceStable(msgs, func(i, j int) bool {
		return msgs[i].Timestamp < msgs[j].Timestamp
	})

	if len(msgs) > MaxCollectedMessages {
		msgs = msgs[len(msgs)-MaxCollectedMessages:]
	}

	return c.resolveAndConvert(ctx, channelID, msgs)
}

func (c *MessageCollector) resolveAndConvert(ctx context.Context, channelID string, msgs []ConversationMessage) ([]model.DraftMessage, error) {
	out := make([]model.DraftMessage, 0, len(msgs))
	for _, m := range msgs {
		link, err := c.svc.GetPermalink(ctx, channelID, m.Timestamp)
		if err != nil {
			// Non-fatal: text is the primary signal for the Materializer, the
			// permalink is reference-only. Surface via errutil.Handle so it is
			// logged/alerted but does not abort collection.
			errutil.Handle(ctx, goerr.Wrap(err, "failed to get permalink",
				goerr.V("channel_id", channelID),
				goerr.V("message_ts", m.Timestamp),
			), "permalink fetch failed during draft collection")
			link = ""
		}
		out = append(out, model.DraftMessage{
			UserID:    m.UserID,
			Text:      m.Text,
			TS:        m.Timestamp,
			Permalink: link,
		})
	}
	return out, nil
}
