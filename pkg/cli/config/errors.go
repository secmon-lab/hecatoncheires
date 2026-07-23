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
	ErrInvalidCaseTrigger    = goerr.New("invalid case trigger")
	ErrMissingMonitorChannel = goerr.New("thread mode requires [slack] channel")
	ErrInvalidMonitorChannel = goerr.New("invalid Slack channel ID")
	ErrMissingCaseStatus     = goerr.New("thread mode requires [case.status]")
	// ErrReactionRequiresThreadMode is returned when [slack] reaction is set on a
	// workspace that is not in thread mode. Reaction-triggered case creation needs
	// a destination thread, which only thread mode provides.
	ErrReactionRequiresThreadMode = goerr.New("[slack] reaction requires mode = \"thread\"")
	// ErrInvalidReactionEmoji is returned when [slack] reaction, after stripping
	// surrounding colons, is empty or contains characters outside a Slack emoji
	// name.
	ErrInvalidReactionEmoji = goerr.New("invalid Slack reaction emoji name")
	// ErrDuplicateReactionEmoji is returned when the same reaction emoji is
	// configured on more than one workspace, which would make emoji-to-workspace
	// resolution ambiguous.
	ErrDuplicateReactionEmoji = goerr.New("duplicate Slack reaction emoji across workspaces")

	// --- Workspace channel / agent ([slack] workspace_channel + [slack.workspace_agent]) ---

	// ErrWorkspaceChannelRequiresChannelMode is returned when workspace_channel or
	// [slack.workspace_agent] is set on a workspace that is not in channel mode.
	// The workspace channel hosts a cross-case agent over per-case channels, which
	// only exist in channel mode.
	ErrWorkspaceChannelRequiresChannelMode = goerr.New("[slack] workspace_channel / [slack.workspace_agent] require channel mode")
	// ErrMissingWorkspaceChannel is returned when [slack.workspace_agent] is set
	// but [slack] workspace_channel is empty: the agent runs in the workspace
	// channel and is meaningless without it.
	ErrMissingWorkspaceChannel = goerr.New("[slack.workspace_agent] requires [slack] workspace_channel")
	// ErrInvalidWorkspaceChannel is returned when [slack] workspace_channel is not
	// a Slack channel ID (e.g. a channel name).
	ErrInvalidWorkspaceChannel = goerr.New("invalid Slack workspace_channel ID")
	// ErrDuplicateWorkspaceChannel is returned when the same channel ID is used as
	// a workspace_channel on more than one workspace, or collides with another
	// workspace's monitored channel, which would make channel-to-workspace routing
	// ambiguous.
	ErrDuplicateWorkspaceChannel = goerr.New("duplicate Slack workspace_channel across workspaces")
	// ErrWorkspaceAgentPromptConflict is returned when [slack.workspace_agent]
	// sets both prompt and prompt_file (mutually exclusive).
	ErrWorkspaceAgentPromptConflict = goerr.New("[slack.workspace_agent] prompt and prompt_file are mutually exclusive")
	// ErrWorkspaceAgentPromptEmpty is returned when [slack.workspace_agent]
	// prompt_file resolves to an empty file.
	ErrWorkspaceAgentPromptEmpty = goerr.New("[slack.workspace_agent] prompt_file is empty")
	// ErrMissingReferenceWorkspace is returned when a case_ref /
	// multi_case_ref field omits reference_workspace.
	ErrMissingReferenceWorkspace = goerr.New("case_ref field requires reference_workspace")
	// ErrUnexpectedReferenceWorkspace is returned when reference_workspace is set
	// on a field whose type is not a case_ref type.
	ErrUnexpectedReferenceWorkspace = goerr.New("reference_workspace is only valid for case_ref fields")
	// ErrUnknownReferenceWorkspace is returned when reference_workspace points at
	// a workspace ID that is not defined across the loaded configs.
	ErrUnknownReferenceWorkspace = goerr.New("reference_workspace points to an unknown workspace")
	// ErrRequiredCaseRefUnsupported is returned when a case_ref / multi_case_ref
	// field is marked required: the Slack case-creation modal cannot collect a
	// case reference, so a required one would make the case un-creatable.
	ErrRequiredCaseRefUnsupported = goerr.New("case_ref fields cannot be required")

	// --- Global config ([[workspace_group]]) ---

	// ErrMissingWorkspaceGroupID is returned when a [[workspace_group]] omits id.
	ErrMissingWorkspaceGroupID = goerr.New("workspace group ID is required")
	// ErrInvalidWorkspaceGroupID is returned when a workspace group id does not
	// match the allowed pattern or exceeds the length limit.
	ErrInvalidWorkspaceGroupID = goerr.New("invalid workspace group ID format")
	// ErrDuplicateWorkspaceGroupID is returned when the same workspace group id
	// is defined more than once across the global config files.
	ErrDuplicateWorkspaceGroupID = goerr.New("duplicate workspace group ID")
	// ErrDuplicateGroupMember is returned when the same workspace id appears more
	// than once in a single group's members list.
	ErrDuplicateGroupMember = goerr.New("duplicate workspace group member")
	// ErrUnknownGroupMember is returned when a group member references a
	// workspace id that is not defined across the loaded workspace configs.
	ErrUnknownGroupMember = goerr.New("workspace group member references an unknown workspace")
	// ErrGlobalConfigContainsWorkspace is returned when a --global-config file
	// contains a [workspace] section. Workspace definitions belong under
	// --config (1 file = 1 workspace); the global config is for deployment-wide
	// settings only, so mixing the two is rejected loudly rather than ignored.
	ErrGlobalConfigContainsWorkspace = goerr.New("global config file must not contain a [workspace] section")

	// --- Global config ([export]) ---

	// ErrInvalidExportConfig is returned when the [export] section is malformed,
	// e.g. a missing [export.bigquery] project.
	ErrInvalidExportConfig = goerr.New("invalid export configuration")
	// ErrInvalidExportDataset is returned when an [[export.bigquery.workspace]]
	// dataset name is not a valid BigQuery dataset ID (BigQuery forbids hyphens).
	ErrInvalidExportDataset = goerr.New("invalid export dataset name")
	// ErrDuplicateExportWorkspace is returned when the same workspace id — or the
	// same dataset name — appears more than once in the export config.
	ErrDuplicateExportWorkspace = goerr.New("duplicate export workspace mapping")
	// ErrUnknownExportWorkspace is returned when an export workspace mapping
	// references a workspace id not defined across the loaded workspace configs.
	ErrUnknownExportWorkspace = goerr.New("export workspace mapping references an unknown workspace")
	// ErrDuplicateExportConfig is returned when more than one global config file
	// defines an [export] section.
	ErrDuplicateExportConfig = goerr.New("duplicate [export] section across global config files")
)

// Context keys for error values
const (
	ConfigPathKey       = "config_path"
	FieldIDKey          = "field_id"
	FieldTypeKey        = "field_type"
	OptionIDKey         = "option_id"
	FieldIndexKey       = "field_index"
	OptionIndexKey      = "option_index"
	WorkspaceIDKey      = "workspace_id"
	WorkspaceColorKey   = "workspace_color"
	WorkspaceEmojiKey   = "workspace_emoji"
	WorkspaceEmojiLen   = "workspace_emoji_len"
	WorkspaceGroupIDKey = "workspace_group_id"
	GroupMemberKey      = "group_member"
	ExportDatasetKey    = "dataset"
)
