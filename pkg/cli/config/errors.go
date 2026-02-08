package config

import "github.com/m-mizutani/goerr/v2"

// Sentinel errors for configuration validation
var (
	ErrConfigNotFound       = goerr.New("configuration file not found")
	ErrInvalidConfig        = goerr.New("invalid configuration")
	ErrDuplicateFieldID     = goerr.New("duplicate field ID")
	ErrDuplicateOptionID    = goerr.New("duplicate option ID")
	ErrInvalidFieldID       = goerr.New("invalid field ID format")
	ErrInvalidFieldType     = goerr.New("invalid field type")
	ErrMissingOptions       = goerr.New("select/multi-select field requires at least one option")
	ErrInvalidMetadata      = goerr.New("invalid metadata format")
	ErrMissingName          = goerr.New("name is required")
	ErrInvalidWorkspaceID   = goerr.New("invalid workspace ID format")
	ErrMissingWorkspaceID   = goerr.New("workspace ID is required")
	ErrDuplicateWorkspaceID = goerr.New("duplicate workspace ID")
	ErrNoConfigFiles        = goerr.New("no configuration files found")
)

// Context keys for error values
const (
	ConfigPathKey  = "config_path"
	FieldIDKey     = "field_id"
	FieldTypeKey   = "field_type"
	OptionIDKey    = "option_id"
	FieldIndexKey  = "field_index"
	OptionIndexKey = "option_index"
	WorkspaceIDKey = "workspace_id"
)
