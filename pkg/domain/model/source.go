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
	SourceTypeGitHub     SourceType = "github"
)

// ErrInvalidGitHubRepo is returned when the input cannot be parsed as a GitHub repository
var ErrInvalidGitHubRepo = goerr.New("invalid GitHub repository")

// ErrSourceValidation is returned when a Source fails its persistence-boundary invariants.
var ErrSourceValidation = goerr.New("source validation failed")

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
	GitHubConfig     *GitHubConfig
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Validate enforces the invariants required before any persistence write.
// The repository assigns the storage ID (NewSourceID when empty), so ID is not
// checked here; Name and SourceType are the source's mandatory identity/kind
// fields (every create path supplies a non-empty default for both).
func (s *Source) Validate() error {
	if s == nil {
		return goerr.Wrap(ErrSourceValidation, "source is nil")
	}
	if s.Name == "" {
		return goerr.Wrap(ErrSourceValidation, "source Name is required")
	}
	if s.SourceType == "" {
		return goerr.Wrap(ErrSourceValidation, "source SourceType is required")
	}
	return nil
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

// notionHosts is the allow-list of hosts recognized as Notion web URLs.
// "notion.so" / "www.notion.so" are the classic hosts; "app.notion.com"
// is the newer host used by shared / copied page links (e.g.
// "https://app.notion.com/p/workspace/Title-<id>").
var notionHosts = map[string]struct{}{
	"notion.so":      {},
	"www.notion.so":  {},
	"app.notion.com": {},
}

// ParseNotionID extracts a Notion ID from either a raw ID or a Notion URL.
// The returned ID is always in UUID format (8-4-4-4-12) as required by the Notion API.
// Accepted formats:
//   - Raw ID: "abc123def456789012345678901234567"
//   - UUID format: "12345678-90ab-cdef-1234-567890abcdef"
//   - Notion URL: "https://www.notion.so/workspace/abc123def456...?v=..."
//   - Notion URL: "https://www.notion.so/abc123def456..."
//   - Notion URL: "https://app.notion.com/p/workspace/Title-abc123def456..."
func ParseNotionID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrInvalidNotionID
	}

	var hex string
	var err error

	// If the input looks like a URL, parse it. The scheme is matched
	// case-insensitively since URL schemes are not case-sensitive.
	lower := strings.ToLower(input)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
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

	// Hostnames are case-insensitive; normalize before the allow-list check.
	host := strings.ToLower(u.Hostname())
	if _, ok := notionHosts[host]; !ok {
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
	// Extract the last 32 hex characters from the segment. Hex IDs are
	// case-insensitive, so normalize to lower case (matching the raw-ID path).
	clean := strings.ToLower(strings.ReplaceAll(lastSegment, "-", ""))
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

// GitHubConfig holds GitHub specific configuration
type GitHubConfig struct {
	Repositories []GitHubRepository
}

// GitHubRepository represents a GitHub repository reference
type GitHubRepository struct {
	Owner string
	Repo  string
}

// githubURLPattern matches GitHub repository URLs
var githubURLPattern = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)

// githubRepoPattern matches owner/repo format
var githubRepoPattern = regexp.MustCompile(`^([a-zA-Z0-9\-_.]+)/([a-zA-Z0-9\-_.]+)$`)

// ParseGitHubRepo extracts owner and repo from either "owner/repo" or a GitHub URL.
// Accepted formats:
//   - "owner/repo"
//   - "https://github.com/owner/repo"
//   - "https://github.com/owner/repo.git"
func ParseGitHubRepo(input string) (owner, repo string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", ErrInvalidGitHubRepo
	}

	// Try URL format first
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		matches := githubURLPattern.FindStringSubmatch(input)
		if matches == nil {
			return "", "", ErrInvalidGitHubRepo
		}
		return matches[1], matches[2], nil
	}

	// Try owner/repo format
	matches := githubRepoPattern.FindStringSubmatch(input)
	if matches == nil {
		return "", "", ErrInvalidGitHubRepo
	}
	return matches[1], matches[2], nil
}
