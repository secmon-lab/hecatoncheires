package graphql

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// SlackUsersCache provides a TTL-based cache for Slack users
// This is an application-scoped cache that persists across requests
type SlackUsersCache struct {
	slackService slack.Service
	mu           sync.RWMutex
	cache        map[string]*graphql1.SlackUser
	cachedAt     time.Time
	ttl          time.Duration
}

// NewSlackUsersCache creates a new SlackUsersCache
func NewSlackUsersCache(slackService slack.Service) *SlackUsersCache {
	return &SlackUsersCache{
		slackService: slackService,
		cache:        make(map[string]*graphql1.SlackUser),
		ttl:          1 * time.Minute,
	}
}

// Get retrieves a single user by ID
func (c *SlackUsersCache) Get(ctx context.Context, userID string) (*graphql1.SlackUser, error) {
	if err := c.ensureFresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	user, ok := c.cache[userID]
	if !ok {
		return &graphql1.SlackUser{ID: userID}, nil // Return minimal user info if not found
	}

	return user, nil
}

// GetMany retrieves multiple users by IDs
func (c *SlackUsersCache) GetMany(ctx context.Context, userIDs []string) ([]*graphql1.SlackUser, error) {
	if err := c.ensureFresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	users := make([]*graphql1.SlackUser, len(userIDs))
	for i, userID := range userIDs {
		if user, ok := c.cache[userID]; ok {
			users[i] = user
		} else {
			users[i] = &graphql1.SlackUser{ID: userID} // Return minimal user info if not found
		}
	}

	return users, nil
}

// GetAll retrieves all cached users
func (c *SlackUsersCache) GetAll(ctx context.Context) (map[string]*graphql1.SlackUser, error) {
	if err := c.ensureFresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy of the cache
	result := make(map[string]*graphql1.SlackUser, len(c.cache))
	for k, v := range c.cache {
		result[k] = v
	}

	return result, nil
}

// ensureFresh checks if the cache is fresh and refreshes it if needed
func (c *SlackUsersCache) ensureFresh(ctx context.Context) error {
	c.mu.RLock()
	needsRefresh := time.Since(c.cachedAt) > c.ttl
	c.mu.RUnlock()

	if needsRefresh {
		return c.refresh(ctx)
	}

	return nil
}

// refresh fetches fresh user data from Slack API
// Uses double-check locking to prevent thundering herd problem
func (c *SlackUsersCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: another goroutine might have refreshed while waiting for lock
	if time.Since(c.cachedAt) <= c.ttl {
		return nil
	}

	users, err := c.slackService.ListUsers(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to list Slack users")
	}

	newCache := make(map[string]*graphql1.SlackUser, len(users))
	for _, user := range users {
		var imageURL *string
		if user.ImageURL != "" {
			imageURL = &user.ImageURL
		}
		newCache[user.ID] = &graphql1.SlackUser{
			ID:       user.ID,
			Name:     user.Name,
			RealName: user.RealName,
			ImageURL: imageURL,
		}
	}

	c.cache = newCache
	c.cachedAt = time.Now()

	return nil
}
