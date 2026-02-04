package model

import "time"

// SlackUserID represents a unique identifier for a Slack user
type SlackUserID string

// SlackUser represents a Slack workspace user stored in the database
type SlackUser struct {
	ID        SlackUserID
	Name      string    // Slack username (e.g., "john.doe")
	RealName  string    // Display name (e.g., "John Doe")
	Email     string    // Email address (for future features)
	ImageURL  string    // Avatar URL (empty string = no image)
	UpdatedAt time.Time // Last synchronized from Slack
}

// SlackUserMetadata tracks the health and status of Slack user synchronization
type SlackUserMetadata struct {
	LastRefreshSuccess time.Time // Last successful refresh time
	LastRefreshAttempt time.Time // Last refresh attempt time (success or failure)
	UserCount          int       // Number of users at last successful refresh
}
