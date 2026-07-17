package interfaces

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// UserPreferenceRepository persists per-user settings. It is a single document
// per user (keyed by Slack User ID), so it needs only Get/Set — no List.
type UserPreferenceRepository interface {
	// Get returns the user's preference. Returns the backend's ErrNotFound
	// (memory.ErrNotFound / firestore.ErrNotFound) when the user has none yet.
	Get(ctx context.Context, userID string) (*model.UserPreference, error)
	// Set writes the preference wholesale (Validate then persist).
	Set(ctx context.Context, pref *model.UserPreference) error
}
