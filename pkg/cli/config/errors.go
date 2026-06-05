package config

import "github.com/m-mizutani/goerr/v2"

// Sentinel errors for configuration validation
var (
	ErrConfigNotFound        = goerr.New("configuration file not found")
	ErrInvalidConfig         = goerr.New("invalid configuration")
	ErrDuplicateFieldID      = goerr.New("duplicate field ID")
	ErrDuplicateOptionID     = goerr.New("duplicate option ID")
	ErrInvalidFieldID        = goerr.New("invalid field ID format")
	ErrInvalidFieldType      = goerr.New("invalid field type")
	ErrMissingOptions        = goerr.New("select/multi-select field requires at least one option")
	ErrInvalidMetadata       = goerr.New("invalid metadata format")
	ErrMissingName           = goerr.New("name is required")
	ErrInvalidWorkspaceID    = goerr.New("invalid workspace ID format")
	ErrMissingWorkspaceID    = goerr.New("workspace ID is required")
	ErrDuplicateWorkspaceID  = goerr.New("duplicate workspace ID")
	ErrNoConfigFiles         = goerr.New("no configuration files found")
	ErrInvalidWelcomeMessage = goerr.New("invalid Slack welcome message template")
	// ErrWorkspaceEmojiColorConflict is returned when both emoji and color are
	// set in the [workspace] section. They are mutually exclusive because the
	// UI renders either an emoji badge (neutral background) or a colored
	// initials badge, never both.
	ErrWorkspaceEmojiColorConflict = goerr.New("workspace emoji and color are mutually exclusive")
	// ErrInvalidWorkspaceColor is returned when the [workspace] color is not a
	// 6-digit #RRGGBB hex code.
	ErrInvalidWorkspaceColor = goerr.New("invalid workspace color format")
	// ErrInvalidWorkspaceEmoji is returned when the [workspace] emoji exceeds
	// the allowed rune length.
	ErrInvalidWorkspaceEmoji = goerr.New("invalid workspace emoji")
	ErrInvalidCaseMode       = goerr.New("invalid case mode")
	ErrMissingMonitorChannel = goerr.New("thread mode requires [slack] channel")
	ErrInvalidMonitorChannel = goerr.New("invalid Slack channel ID")
	ErrMissingCaseStatus     = goerr.New("thread mode requires [case.status]")
)

// Context keys for error values
const (
	ConfigPathKey     = "config_path"
	FieldIDKey        = "field_id"
	FieldTypeKey      = "field_type"
	OptionIDKey       = "option_id"
	FieldIndexKey     = "field_index"
	OptionIndexKey    = "option_index"
	WorkspaceIDKey    = "workspace_id"
	WorkspaceColorKey = "workspace_color"
	WorkspaceEmojiKey = "workspace_emoji"
	WorkspaceEmojiLen = "workspace_emoji_len"
)
