package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type slackUserRepository struct {
	mu       sync.RWMutex
	users    map[model.SlackUserID]*model.SlackUser
	metadata *model.SlackUserMetadata
}

func newSlackUserRepository() *slackUserRepository {
	return &slackUserRepository{
		users: make(map[model.SlackUserID]*model.SlackUser),
		metadata: &model.SlackUserMetadata{
			LastRefreshSuccess: time.Time{},
			LastRefreshAttempt: time.Time{},
			UserCount:          0,
		},
	}
}

// GetAll retrieves all Slack users from memory
func (r *slackUserRepository) GetAll(ctx context.Context) ([]*model.SlackUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*model.SlackUser, 0, len(r.users))
	for _, user := range r.users {
		// Return a deep copy to prevent external modifications
		userCopy := *user
		users = append(users, &userCopy)
	}

	return users, nil
}

// GetByID retrieves a single Slack user by ID
func (r *slackUserRepository) GetByID(ctx context.Context, id model.SlackUserID) (*model.SlackUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "slack user not found", goerr.V("id", id))
	}

	// Return a deep copy to prevent external modifications
	userCopy := *user
	return &userCopy, nil
}

// GetByIDs retrieves multiple Slack users by IDs
func (r *slackUserRepository) GetByIDs(ctx context.Context, ids []model.SlackUserID) (map[model.SlackUserID]*model.SlackUser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[model.SlackUserID]*model.SlackUser, len(ids))
	for _, id := range ids {
		if user, ok := r.users[id]; ok {
			// Return a deep copy to prevent external modifications
			userCopy := *user
			result[id] = &userCopy
		}
		// Missing users are not included in the result map (not an error)
	}

	return result, nil
}

// SaveMany saves multiple Slack users (upsert operation)
func (r *slackUserRepository) SaveMany(ctx context.Context, users []*model.SlackUser) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, user := range users {
		// Store a deep copy to prevent external modifications
		userCopy := *user
		r.users[user.ID] = &userCopy
	}

	return nil
}

// DeleteAll deletes all Slack users from memory
func (r *slackUserRepository) DeleteAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.users = make(map[model.SlackUserID]*model.SlackUser)
	return nil
}

// GetMetadata retrieves refresh metadata
func (r *slackUserRepository) GetMetadata(ctx context.Context) (*model.SlackUserMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a deep copy to prevent external modifications
	metadataCopy := *r.metadata
	return &metadataCopy, nil
}

// SaveMetadata saves refresh metadata
func (r *slackUserRepository) SaveMetadata(ctx context.Context, metadata *model.SlackUserMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store a deep copy to prevent external modifications
	metadataCopy := *metadata
	r.metadata = &metadataCopy
	return nil
}
