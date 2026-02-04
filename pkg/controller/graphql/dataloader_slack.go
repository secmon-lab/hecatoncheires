package graphql

import (
	"context"

	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// SlackUsersLoader provides DataLoader interface for Slack users
// This is request-scoped but uses application-scoped SlackUserProvider
type SlackUsersLoader struct {
	provider *SlackUserProvider // Application-scoped provider (DB-backed)
}

// NewSlackUsersLoader creates a new SlackUsersLoader
func NewSlackUsersLoader(provider *SlackUserProvider) *SlackUsersLoader {
	return &SlackUsersLoader{
		provider: provider,
	}
}

// Load retrieves a single user by ID
func (l *SlackUsersLoader) Load(ctx context.Context, userID string) (*graphql1.SlackUser, error) {
	if l.provider == nil {
		// If provider is not available, return minimal user info
		return &graphql1.SlackUser{ID: userID}, nil
	}
	return l.provider.Get(ctx, userID)
}

// LoadMany retrieves multiple users by IDs
func (l *SlackUsersLoader) LoadMany(ctx context.Context, userIDs []string) ([]*graphql1.SlackUser, error) {
	if l.provider == nil {
		// If provider is not available, return minimal user info
		users := make([]*graphql1.SlackUser, len(userIDs))
		for i, userID := range userIDs {
			users[i] = &graphql1.SlackUser{ID: userID}
		}
		return users, nil
	}
	return l.provider.GetMany(ctx, userIDs)
}

// LoadAll retrieves all users
func (l *SlackUsersLoader) LoadAll(ctx context.Context) (map[string]*graphql1.SlackUser, error) {
	if l.provider == nil {
		return make(map[string]*graphql1.SlackUser), nil
	}
	return l.provider.GetAll(ctx)
}
