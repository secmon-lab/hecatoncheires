package slack

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

const (
	// DefaultCacheTTL is the default TTL for channel name cache
	DefaultCacheTTL = 45 * time.Second
)

// cacheEntry holds a cached channel name with expiration
type cacheEntry struct {
	name      string
	expiresAt time.Time
}

// client implements Service interface
type client struct {
	api      *slack.Client
	cacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry

	teamURLOnce sync.Once
	teamURL     string
	teamURLErr  error

	botUserIDOnce sync.Once
	botUserID     string
	botUserIDErr  error
}

// Option is a functional option for client configuration
type Option func(*client)

// WithCacheTTL sets the TTL for channel name cache
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *client) {
		c.cacheTTL = ttl
	}
}

// New creates a new Slack service with the provided bot token
func New(token string, opts ...Option) (Service, error) {
	if token == "" {
		return nil, goerr.New("Slack bot token is required")
	}

	c := &client{
		api:      slack.New(token),
		cacheTTL: DefaultCacheTTL,
		cache:    make(map[string]cacheEntry),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// ListJoinedChannels retrieves the list of channels the bot has joined
func (c *client) ListJoinedChannels(ctx context.Context) ([]Channel, error) {
	var channels []Channel
	var cursor string

	for {
		params := &slack.GetConversationsParameters{
			// TODO: Add "private_channel" support after resolving scope configuration
			// Requires: groups:read scope in addition to channels:read
			Types:           []string{"public_channel"},
			ExcludeArchived: true,
			Limit:           100,
			Cursor:          cursor,
		}

		convs, nextCursor, err := c.api.GetConversationsContext(ctx, params)
		if err != nil {
			// Handle rate limiting by waiting and retrying,
			// matching the pattern used by GetUsersContext in slack-go/slack.
			if rateLimitErr, ok := err.(*slack.RateLimitedError); ok {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(rateLimitErr.RetryAfter):
					continue
				}
			}
			return nil, goerr.Wrap(err, "failed to get conversations")
		}

		for _, conv := range convs {
			// Only include channels the bot is a member of
			if conv.IsMember {
				channels = append(channels, Channel{
					ID:   conv.ID,
					Name: conv.Name,
				})
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return channels, nil
}

// GetChannelNames retrieves channel names for the given IDs with caching
func (c *client) GetChannelNames(ctx context.Context, ids []string) (map[string]string, error) {
	result := make(map[string]string)
	var missingIDs []string

	now := time.Now()

	// Check cache first
	c.mu.RLock()
	for _, id := range ids {
		if entry, ok := c.cache[id]; ok && entry.expiresAt.After(now) {
			result[id] = entry.name
		} else {
			missingIDs = append(missingIDs, id)
		}
	}
	c.mu.RUnlock()

	// Fetch missing channels from API
	if len(missingIDs) > 0 {
		c.mu.Lock()
		defer c.mu.Unlock()

		for _, id := range missingIDs {
			// Double-check cache after acquiring write lock
			if entry, ok := c.cache[id]; ok && entry.expiresAt.After(now) {
				result[id] = entry.name
				continue
			}

			info, err := c.api.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{
				ChannelID: id,
			})
			if err != nil {
				// If we can't get the channel info, skip it
				// The caller will use the fallback name
				continue
			}

			name := info.Name
			result[id] = name
			c.cache[id] = cacheEntry{
				name:      name,
				expiresAt: now.Add(c.cacheTTL),
			}
		}
	}

	return result, nil
}

// GetUserInfo retrieves user information for the given user ID
func (c *client) GetUserInfo(ctx context.Context, userID string) (*User, error) {
	user, err := c.api.GetUserInfoContext(ctx, userID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get user info", goerr.V("user_id", userID))
	}

	return &User{
		ID:       user.ID,
		Name:     user.Name,
		RealName: user.RealName,
		Email:    user.Profile.Email,
		ImageURL: user.Profile.Image48,
	}, nil
}

// ListUsers retrieves all non-deleted, non-bot users in the workspace
func (c *client) ListUsers(ctx context.Context) ([]*User, error) {
	users, err := c.api.GetUsersContext(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list users")
	}

	result := make([]*User, 0, len(users))
	for _, u := range users {
		// Skip deleted users and bots
		if u.Deleted || u.IsBot {
			continue
		}

		result = append(result, &User{
			ID:       u.ID,
			Name:     u.Name,
			RealName: u.RealName,
			Email:    u.Profile.Email,
			ImageURL: u.Profile.Image48,
		})
	}

	return result, nil
}

// CreateChannel creates a new Slack channel for a case
// The channel name is automatically generated from caseID, caseName, and the given prefix
// If isPrivate is true, the channel is created as a private channel
func (c *client) CreateChannel(ctx context.Context, caseID int64, caseName string, prefix string, isPrivate bool) (string, error) {
	channelName := GenerateRiskChannelName(caseID, caseName, prefix)
	channel, err := c.api.CreateConversationContext(ctx, slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   isPrivate,
	})
	if err != nil {
		return "", goerr.Wrap(err, "failed to create Slack channel", goerr.V("channelName", channelName), goerr.V("caseID", caseID), goerr.V("caseName", caseName))
	}
	return channel.ID, nil
}

// GetConversationMembers retrieves the list of user IDs in the given channel
func (c *client) GetConversationMembers(ctx context.Context, channelID string) ([]string, error) {
	var members []string
	var cursor string

	for {
		params := &slack.GetUsersInConversationParameters{
			ChannelID: channelID,
			Limit:     1000,
			Cursor:    cursor,
		}

		userIDs, nextCursor, err := c.api.GetUsersInConversationContext(ctx, params)
		if err != nil {
			if rateLimitErr, ok := err.(*slack.RateLimitedError); ok {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(rateLimitErr.RetryAfter):
					continue
				}
			}
			return nil, goerr.Wrap(err, "failed to get conversation members",
				goerr.V("channel_id", channelID))
		}

		members = append(members, userIDs...)

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	return members, nil
}

// InviteUsersToChannel invites users to a Slack channel
func (c *client) InviteUsersToChannel(ctx context.Context, channelID string, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := c.api.InviteUsersToConversationContext(ctx, channelID, userIDs...)
	if err != nil {
		return goerr.Wrap(err, "failed to invite users to Slack channel",
			goerr.V("channel_id", channelID),
			goerr.V("user_ids", userIDs))
	}
	return nil
}

// RenameChannel renames an existing Slack channel for a case
// The channel name is automatically generated from caseID, caseName, and the given prefix
func (c *client) RenameChannel(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error {
	channelName := GenerateRiskChannelName(caseID, caseName, prefix)
	_, err := c.api.RenameConversationContext(ctx, channelID, channelName)
	if err != nil {
		return goerr.Wrap(err, "failed to rename Slack channel", goerr.V("channelID", channelID), goerr.V("channelName", channelName), goerr.V("caseID", caseID), goerr.V("caseName", caseName))
	}
	return nil
}

// AddBookmark adds a link bookmark to a Slack channel
func (c *client) AddBookmark(ctx context.Context, channelID, title, link string) error {
	_, err := c.api.AddBookmarkContext(ctx, channelID, slack.AddBookmarkParameters{
		Title: title,
		Type:  "link",
		Link:  link,
	})
	if err != nil {
		return goerr.Wrap(err, "failed to add bookmark to Slack channel",
			goerr.V("channel_id", channelID),
			goerr.V("title", title),
			goerr.V("link", link))
	}
	return nil
}

// GetTeamURL retrieves the Slack workspace URL using auth.test API.
// The result is cached permanently (sync.Once) since the team URL does not change.
func (c *client) GetTeamURL(ctx context.Context) (string, error) {
	c.teamURLOnce.Do(func() {
		resp, err := c.api.AuthTestContext(ctx)
		if err != nil {
			c.teamURLErr = goerr.Wrap(err, "failed to call auth.test")
			return
		}
		c.teamURL = strings.TrimRight(resp.URL, "/")
	})
	return c.teamURL, c.teamURLErr
}

// PostMessage posts a Block Kit message to a channel and returns the message timestamp
func (c *client) PostMessage(ctx context.Context, channelID string, blocks []slack.Block, text string) (string, error) {
	_, ts, err := c.api.PostMessageContext(ctx, channelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		return "", goerr.Wrap(err, "failed to post Slack message",
			goerr.V("channel_id", channelID))
	}
	return ts, nil
}

// UpdateMessage updates an existing Block Kit message identified by channel and timestamp
func (c *client) UpdateMessage(ctx context.Context, channelID string, timestamp string, blocks []slack.Block, text string) error {
	_, _, _, err := c.api.UpdateMessageContext(ctx, channelID, timestamp,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		return goerr.Wrap(err, "failed to update Slack message",
			goerr.V("channel_id", channelID),
			goerr.V("timestamp", timestamp))
	}
	return nil
}

// GetConversationReplies retrieves messages from a thread
func (c *client) GetConversationReplies(ctx context.Context, channelID string, threadTS string, limit int) ([]ConversationMessage, error) {
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Limit:     limit,
	}

	msgs, _, _, err := c.api.GetConversationRepliesContext(ctx, params)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get conversation replies",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS))
	}

	result := make([]ConversationMessage, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, ConversationMessage{
			UserID:    msg.User,
			UserName:  msg.Username,
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
			ThreadTS:  msg.ThreadTimestamp,
		})
	}

	return result, nil
}

// GetConversationHistory retrieves channel messages from the specified time
func (c *client) GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]ConversationMessage, error) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Oldest:    fmt.Sprintf("%d.000000", oldest.Unix()),
		Limit:     limit,
	}

	resp, err := c.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get conversation history",
			goerr.V("channel_id", channelID),
			goerr.V("oldest", oldest))
	}

	result := make([]ConversationMessage, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		result = append(result, ConversationMessage{
			UserID:    msg.User,
			UserName:  msg.Username,
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
			ThreadTS:  msg.ThreadTimestamp,
		})
	}

	return result, nil
}

// PostThreadReply posts a text message as a thread reply and returns the message timestamp
func (c *client) PostThreadReply(ctx context.Context, channelID string, threadTS string, text string) (string, error) {
	_, ts, err := c.api.PostMessageContext(ctx, channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		return "", goerr.Wrap(err, "failed to post thread reply",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS))
	}
	return ts, nil
}

// PostThreadMessage posts a Block Kit message as a thread reply and returns the message timestamp
func (c *client) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slack.Block, text string) (string, error) {
	_, ts, err := c.api.PostMessageContext(ctx, channelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	if err != nil {
		return "", goerr.Wrap(err, "failed to post thread message",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", threadTS))
	}
	return ts, nil
}

// OpenView opens a modal view in Slack using the provided trigger ID
func (c *client) OpenView(ctx context.Context, triggerID string, view slack.ModalViewRequest) error {
	_, err := c.api.OpenViewContext(ctx, triggerID, view)
	if err != nil {
		return goerr.Wrap(err, "failed to open Slack modal view",
			goerr.V("trigger_id", triggerID))
	}
	return nil
}

// GetBotUserID retrieves the bot's own user ID via auth.test API.
// The result is cached permanently (sync.Once) since the bot user ID does not change.
func (c *client) GetBotUserID(ctx context.Context) (string, error) {
	c.botUserIDOnce.Do(func() {
		resp, err := c.api.AuthTestContext(ctx)
		if err != nil {
			c.botUserIDErr = goerr.Wrap(err, "failed to call auth.test for bot user ID")
			return
		}
		c.botUserID = resp.UserID
	})
	return c.botUserID, c.botUserIDErr
}
