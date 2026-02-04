package slack

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"
)

const (
	// DefaultCacheTTL is the default TTL for channel name cache
	DefaultCacheTTL = 45 * time.Second
	// DefaultChannelPrefix is the default prefix for risk channels
	DefaultChannelPrefix = "risk"
)

// cacheEntry holds a cached channel name with expiration
type cacheEntry struct {
	name      string
	expiresAt time.Time
}

// client implements Service interface
type client struct {
	api           *slack.Client
	cacheTTL      time.Duration
	channelPrefix string

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

// Option is a functional option for client configuration
type Option func(*client)

// WithCacheTTL sets the TTL for channel name cache
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *client) {
		c.cacheTTL = ttl
	}
}

// WithChannelPrefix sets the prefix for risk channels
func WithChannelPrefix(prefix string) Option {
	return func(c *client) {
		c.channelPrefix = prefix
	}
}

// New creates a new Slack service with the provided bot token
func New(token string, opts ...Option) (Service, error) {
	if token == "" {
		return nil, goerr.New("Slack bot token is required")
	}

	c := &client{
		api:           slack.New(token),
		cacheTTL:      DefaultCacheTTL,
		channelPrefix: DefaultChannelPrefix,
		cache:         make(map[string]cacheEntry),
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

// CreateChannel creates a new public Slack channel for a risk
// The channel name is automatically generated from riskID and riskName with the configured prefix
func (c *client) CreateChannel(ctx context.Context, riskID int64, riskName string) (string, error) {
	channelName := GenerateRiskChannelName(riskID, riskName, c.channelPrefix)
	channel, err := c.api.CreateConversationContext(ctx, slack.CreateConversationParams{
		ChannelName: channelName,
		IsPrivate:   false,
	})
	if err != nil {
		return "", goerr.Wrap(err, "failed to create Slack channel", goerr.V("channelName", channelName), goerr.V("riskID", riskID), goerr.V("riskName", riskName))
	}
	return channel.ID, nil
}

// RenameChannel renames an existing Slack channel for a risk
// The channel name is automatically generated from riskID and riskName with the configured prefix
func (c *client) RenameChannel(ctx context.Context, channelID string, riskID int64, riskName string) error {
	channelName := GenerateRiskChannelName(riskID, riskName, c.channelPrefix)
	_, err := c.api.RenameConversationContext(ctx, channelID, channelName)
	if err != nil {
		return goerr.Wrap(err, "failed to rename Slack channel", goerr.V("channelID", channelID), goerr.V("channelName", channelName), goerr.V("riskID", riskID), goerr.V("riskName", riskName))
	}
	return nil
}
