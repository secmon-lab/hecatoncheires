package model

import (
	"time"

	"github.com/google/uuid"
)

// SourceType represents the type of source
type SourceType string

const (
	SourceTypeNotionDB SourceType = "notion_db"
	SourceTypeSlack    SourceType = "slack"
)

// SourceID is a UUID-based identifier for Source
type SourceID string

// NewSourceID generates a new UUID v4 SourceID
func NewSourceID() SourceID {
	return SourceID(uuid.New().String())
}

// Source represents an external data source for risk monitoring
type Source struct {
	ID             SourceID
	Name           string
	SourceType     SourceType
	Description    string
	Enabled        bool
	NotionDBConfig *NotionDBConfig
	SlackConfig    *SlackConfig
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NotionDBConfig holds Notion DB specific configuration
type NotionDBConfig struct {
	DatabaseID    string
	DatabaseTitle string
	DatabaseURL   string
}

// SlackConfig holds Slack specific configuration
type SlackConfig struct {
	Channels []SlackChannel
}

// SlackChannel represents a Slack channel configuration
type SlackChannel struct {
	ID   string // Slack Channel ID (e.g., C01234567)
	Name string // Fallback name for display when API is unavailable
}
