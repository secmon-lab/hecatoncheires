package slack

import (
	"context"
)

// Service provides interface to Slack API for Source operations
type Service interface {
	// ListJoinedChannels retrieves the list of channels the bot has joined
	// Used for channel selection UI
	ListJoinedChannels(ctx context.Context) ([]Channel, error)

	// GetChannelNames retrieves channel names for the given IDs (with caching)
	// Used for displaying channel names in the UI
	GetChannelNames(ctx context.Context, ids []string) (map[string]string, error)

	// GetUserInfo retrieves user information for the given user ID
	GetUserInfo(ctx context.Context, userID string) (*User, error)

	// ListUsers retrieves all non-deleted, non-bot users in the workspace
	ListUsers(ctx context.Context) ([]*User, error)

	// CreateChannel creates a new public Slack channel for a case
	// The channel name is automatically generated from caseID, caseName, and the given prefix
	// Returns the channel ID on success
	CreateChannel(ctx context.Context, caseID int64, caseName string, prefix string) (string, error)

	// RenameChannel renames an existing Slack channel for a case
	// The channel name is automatically generated from caseID, caseName, and the given prefix
	RenameChannel(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error

	// InviteUsersToChannel invites users to a Slack channel
	// Silently skips if userIDs is empty
	InviteUsersToChannel(ctx context.Context, channelID string, userIDs []string) error
}

// Channel represents a Slack channel
type Channel struct {
	ID   string
	Name string
}

// User represents a Slack user
type User struct {
	ID       string
	Name     string
	RealName string
	Email    string
	ImageURL string
}
