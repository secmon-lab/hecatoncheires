package model

import (
	"time"

	"github.com/google/uuid"
)

// SourceType represents the type of source
type SourceType string

const (
	SourceTypeNotionDB SourceType = "notion_db"
	// Future: SourceTypeGitHubIssues, SourceTypeJira, etc.
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
	// Future: GitHubConfig *GitHubConfig
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NotionDBConfig holds Notion DB specific configuration
type NotionDBConfig struct {
	DatabaseID    string
	DatabaseTitle string
	DatabaseURL   string
}
