package graphql

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// SlackUserProvider provides Slack user data from the database
// No caching or TTL management - data is refreshed by background worker
// This is read-only and always fetches from the database
type SlackUserProvider struct {
	repo interfaces.Repository
}

// NewSlackUserProvider creates a new SlackUserProvider
func NewSlackUserProvider(repo interfaces.Repository) *SlackUserProvider {
	return &SlackUserProvider{
		repo: repo,
	}
}

// Get retrieves a single user by ID from the database
func (p *SlackUserProvider) Get(ctx context.Context, userID string) (*graphql1.SlackUser, error) {
	user, err := p.repo.SlackUser().GetByID(ctx, model.SlackUserID(userID))
	if err != nil {
		// Return minimal user info if not found (Graceful Degradation)
		return &graphql1.SlackUser{ID: userID}, nil
	}

	return convertToGraphQL(user), nil
}

// GetMany retrieves multiple users by IDs from the database
// Uses GetByIDs for batch retrieval to prevent N+1 queries
func (p *SlackUserProvider) GetMany(ctx context.Context, userIDs []string) ([]*graphql1.SlackUser, error) {
	// Convert string IDs to model.SlackUserID
	ids := make([]model.SlackUserID, len(userIDs))
	for i, id := range userIDs {
		ids[i] = model.SlackUserID(id)
	}

	// N+1 Prevention: Use GetByIDs for batch retrieval
	usersMap, err := p.repo.SlackUser().GetByIDs(ctx, ids)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get users by IDs")
	}

	// Convert to GraphQL models, preserving order
	users := make([]*graphql1.SlackUser, len(userIDs))
	for i, userID := range userIDs {
		if user, ok := usersMap[model.SlackUserID(userID)]; ok {
			users[i] = convertToGraphQL(user)
		} else {
			// Return minimal user info if not found (Graceful Degradation)
			users[i] = &graphql1.SlackUser{ID: userID}
		}
	}

	return users, nil
}

// GetAll retrieves all users from the database
func (p *SlackUserProvider) GetAll(ctx context.Context) (map[string]*graphql1.SlackUser, error) {
	users, err := p.repo.SlackUser().GetAll(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get all users")
	}

	// Convert to map of GraphQL models
	result := make(map[string]*graphql1.SlackUser, len(users))
	for _, user := range users {
		result[string(user.ID)] = convertToGraphQL(user)
	}

	return result, nil
}

// convertToGraphQL converts a domain SlackUser to a GraphQL SlackUser
func convertToGraphQL(user *model.SlackUser) *graphql1.SlackUser {
	var imageURL *string
	if user.ImageURL != "" {
		imageURL = &user.ImageURL
	}

	return &graphql1.SlackUser{
		ID:       string(user.ID),
		Name:     user.Name,
		RealName: user.RealName,
		ImageURL: imageURL,
	}
}
