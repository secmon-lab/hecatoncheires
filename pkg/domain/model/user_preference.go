package model

import (
	"slices"
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// ErrUserPreferenceValidation is returned when a UserPreference fails validation.
var ErrUserPreferenceValidation = goerr.New("user preference validation failed")

// UserPreference holds per-user settings that span workspaces. Keyed by the
// user's Slack ID (the auth token's Sub). Currently it only carries the
// favorite workspace list surfaced on the home dashboard.
type UserPreference struct {
	// UserID is the Slack User ID (auth token Sub). Document key. Required.
	UserID string
	// FavoriteWorkspaceIDs are the workspace IDs the user starred, in the
	// order the caller supplies (deduplicated). May be empty.
	FavoriteWorkspaceIDs []string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Validate enforces the identity invariant the repository relies on before
// every write.
func (p *UserPreference) Validate() error {
	if p == nil {
		return goerr.Wrap(ErrUserPreferenceValidation, "user preference is nil")
	}
	if p.UserID == "" {
		return goerr.Wrap(ErrUserPreferenceValidation, "user ID is required")
	}
	if slices.Contains(p.FavoriteWorkspaceIDs, "") {
		return goerr.Wrap(ErrUserPreferenceValidation, "favorite workspace id must not be empty")
	}
	return nil
}
