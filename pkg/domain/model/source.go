package model

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
)

// ErrInvalidNotionID is returned when the input cannot be parsed as a Notion ID
var ErrInvalidNotionID = goerr.New("invalid Notion ID")

// SourceType represents the type of source
type SourceType string

const (
	SourceTypeNotionDB   SourceType = "notion_db"
	SourceTypeNotionPage SourceType = "notion_page"
	SourceTypeSlack      SourceType = "slack"
)

// SourceID is a UUID-based identifier for Source
type SourceID string

// NewSourceID generates a new UUID v4 SourceID
func NewSourceID() SourceID {
	return SourceID(uuid.New().String())
}

// Source represents an external data source for risk monitoring
type Source struct {
	ID               SourceID
	Name             string
	SourceType       SourceType
	Description      string
	Enabled          bool
	NotionDBConfig   *NotionDBConfig
	NotionPageConfig *NotionPageConfig
	SlackConfig      *SlackConfig
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NotionDBConfig holds Notion DB specific configuration
type NotionDBConfig struct {
	DatabaseID    string
	DatabaseTitle string
	DatabaseURL   string
}

// NotionPageConfig holds Notion Page specific configuration
type NotionPageConfig struct {
	PageID    string
	PageTitle string
	PageURL   string
	Recursive bool
	MaxDepth  int
}

// hexPattern matches 32 hex characters (a Notion database ID without dashes)
var hexPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ParseNotionID extracts a Notion ID from either a raw ID or a Notion URL.
// The returned ID is always in UUID format (8-4-4-4-12) as required by the Notion API.
// Accepted formats:
//   - Raw ID: "abc123def456789012345678901234567"
//   - UUID format: "12345678-90ab-cdef-1234-567890abcdef"
//   - Notion URL: "https://www.notion.so/workspace/abc123def456...?v=..."
//   - Notion URL: "https://www.notion.so/abc123def456..."
func ParseNotionID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrInvalidNotionID
	}

	var hex string
	var err error

	// If the input looks like a URL, parse it
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		hex, err = parseNotionURL(input)
	} else {
		// Try as a raw ID (with or without dashes)
		hex, err = normalizeNotionID(input)
	}

	if err != nil {
		return "", err
	}

	return toUUIDFormat(hex), nil
}

func parseNotionURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidNotionID
	}

	host := u.Hostname()
	if host != "www.notion.so" && host != "notion.so" {
		return "", ErrInvalidNotionID
	}

	// The ID is the last 32 hex chars in the last path segment
	// e.g. /workspace/Title-abc123def456789012345678901234567
	path := strings.TrimRight(u.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) == 0 {
		return "", ErrInvalidNotionID
	}

	lastSegment := segments[len(segments)-1]

	// The ID may be appended after a title with a hyphen separator.
	// Extract the last 32 hex characters from the segment.
	clean := strings.ReplaceAll(lastSegment, "-", "")
	if len(clean) >= 32 {
		candidate := clean[len(clean)-32:]
		if hexPattern.MatchString(candidate) {
			return candidate, nil
		}
	}

	return "", ErrInvalidNotionID
}

func normalizeNotionID(input string) (string, error) {
	clean := strings.ReplaceAll(input, "-", "")
	clean = strings.ToLower(clean)
	if hexPattern.MatchString(clean) {
		return clean, nil
	}
	return "", ErrInvalidNotionID
}

// toUUIDFormat converts a 32-char hex string to UUID format (8-4-4-4-12)
func toUUIDFormat(hex string) string {
	return hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:32]
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
