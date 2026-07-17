package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// userPreferenceRepository stores one preference document per user ID.
type userPreferenceRepository struct {
	mu   sync.RWMutex
	data map[string]*model.UserPreference
}

func newUserPreferenceRepository() *userPreferenceRepository {
	return &userPreferenceRepository{
		data: make(map[string]*model.UserPreference),
	}
}

// copyUserPreference deep-copies so a caller mutating the returned pointer, or
// mutating the input after Set, cannot alter stored state. The whole struct is
// copied first (so a newly added field is never silently dropped), then the
// only reference-typed field is cloned.
func copyUserPreference(p *model.UserPreference) *model.UserPreference {
	copied := *p
	copied.FavoriteWorkspaceIDs = slices.Clone(p.FavoriteWorkspaceIDs)
	return &copied
}

func (r *userPreferenceRepository) Get(ctx context.Context, userID string) (*model.UserPreference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.data[userID]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "user preference not found", goerr.V("user_id", userID))
	}
	return copyUserPreference(p), nil
}

func (r *userPreferenceRepository) Set(ctx context.Context, pref *model.UserPreference) error {
	if err := pref.Validate(); err != nil {
		return goerr.Wrap(err, "user preference validation failed before set")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[pref.UserID] = copyUserPreference(pref)
	return nil
}
