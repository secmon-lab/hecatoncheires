package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	domainConfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/urfave/cli/v3"
)

// fieldIDPattern is the validation pattern for Field IDs and Option IDs.
// It allows lowercase ASCII letters, digits, and underscores, and must start
// with a letter. This restriction lets template authors reference values via
// dot notation (e.g., {{.Fields.risk_level}}), which Go's text/template only
// supports for valid Go identifiers.
var fieldIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// workspaceIDPattern is the validation pattern for Workspace IDs.
// Workspace IDs are used in Slack channel names and other infrastructure
// identifiers, so the legacy hyphen-separated form is preserved.
var workspaceIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// workspaceColorPattern is the validation pattern for the [workspace] color.
// Only the 6-digit #RRGGBB form is accepted so the frontend can derive a
// gradient from the base color without handling the 3-digit shorthand.
var workspaceColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// maxWorkspaceEmojiRunes bounds the [workspace] emoji length. A single emoji
// can span several runes (variation selectors, ZWJ sequences, skin-tone
// modifiers, flags), so the limit is generous rather than "one rune". It only
// guards against operators pasting arbitrary text into the field.
const maxWorkspaceEmojiRunes = 16

// slackChannelIDPattern is a lenient check for a Slack channel ID. Slack
// channel IDs start with C (public), G (private/group) and are uppercase
// alphanumerics. This catches obvious mistakes (channel names like
// `#team-support`) without over-constraining future ID shapes.
var slackChannelIDPattern = regexp.MustCompile(`^[CG][A-Z0-9]+$`)

// WorkspaceBaseConfig represents the [workspace] section in a TOML config
type WorkspaceBaseConfig struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"` // Human-readable description used to disambiguate workspaces (especially for AI-side workspace estimation)
	// Emoji is an optional display glyph rendered in the workspace badge.
	// Mutually exclusive with Color. Empty when unset.
	Emoji string `toml:"emoji"`
	// Color is an optional #RRGGBB hex used as the workspace badge background.
	// Mutually exclusive with Emoji. Empty when unset.
	Color string `toml:"color"`
}

// Validate checks the optional emoji/color fields of the [workspace] section.
// emoji and color are mutually exclusive; color must be a #RRGGBB hex code;
// emoji must not exceed maxWorkspaceEmojiRunes runes.
func (w *WorkspaceBaseConfig) Validate() error {
	if w.Emoji != "" && w.Color != "" {
		return goerr.Wrap(ErrWorkspaceEmojiColorConflict,
			"workspace emoji and color cannot both be set",
			goerr.V(WorkspaceIDKey, w.ID),
		)
	}
	if w.Color != "" && !workspaceColorPattern.MatchString(w.Color) {
		return goerr.Wrap(ErrInvalidWorkspaceColor,
			"workspace color must be a #RRGGBB hex code",
			goerr.V(WorkspaceIDKey, w.ID),
			goerr.V(WorkspaceColorKey, w.Color),
		)
	}
	if n := len([]rune(w.Emoji)); n > maxWorkspaceEmojiRunes {
		return goerr.Wrap(ErrInvalidWorkspaceEmoji,
			"workspace emoji is too long",
			goerr.V(WorkspaceIDKey, w.ID),
			goerr.V(WorkspaceEmojiKey, w.Emoji),
			goerr.V(WorkspaceEmojiLen, n),
		)
	}
	return nil
}

// SlackInviteSection represents the [slack.invite] section in a TOML config
type SlackInviteSection struct {
	Users  []string `toml:"users"`
	Groups []string `toml:"groups"`
}

// SlackSection represents the [slack] section in a TOML config
type SlackSection struct {
	ChannelPrefix   string             `toml:"channel_prefix"`
	TeamID          string             `toml:"team_id"`
	Invite          SlackInviteSection `toml:"invite"`
	WelcomeMessages []string           `toml:"welcome_messages"`
	// Mode selects the case-binding mode: "channel" (default) or "thread".
	Mode string `toml:"mode"`
	// Channel is the monitored Slack channel ID for thread mode (e.g. C0123...).
	Channel string `toml:"channel"`
	// AcceptBot, when true, makes bot-authored channel-root posts
	// (e.g. an intake-form app's relayed request) start a case in thread mode.
	// Default false: only human channel-root posts start a case.
	AcceptBot bool `toml:"accept_bot"`
}

// CaseSection represents the [case] section in a TOML config. It mirrors
// [action] but configures the status set that attaches to Cases in thread
// mode. It is required for thread-mode workspaces and ignored otherwise.
type CaseSection struct {
	Initial string                  `toml:"initial"`
	Closed  []string                `toml:"closed"`
	Status  []ActionStatusConfigRow `toml:"status"`
	Prompts CasePromptsSection      `toml:"prompts"`
}

// CasePromptsSection represents the [case.prompts] sub-table: workspace-
// specific additional prompts for the case agent, keyed by lifecycle phase.
// Only `create` (the thread-mode initialization agent) is consumed today;
// `mention` / `close` are reserved for future phases.
type CasePromptsSection struct {
	Create string `toml:"create"`
}

// CompileSection represents the [compile] section in a TOML config
type CompileSection struct {
	Prompt string `toml:"prompt"`
}

// AssistSection represents the [assist] section in a TOML config
type AssistSection struct {
	Prompt   string `toml:"prompt"`
	Language string `toml:"language"`
}

// AppConfig represents the application configuration.
// It holds TOML-parsed fields and provides CLI Flags()/Configure() methods.
type AppConfig struct {
	Workspace WorkspaceBaseConfig `toml:"workspace"`
	Labels    Labels              `toml:"labels"`
	Fields    []FieldDefinition   `toml:"fields"`
	Slack     SlackSection        `toml:"slack"`
	Compile   CompileSection      `toml:"compile"`
	Assist    AssistSection       `toml:"assist"`
	Action    *ActionSection      `toml:"action"`
	Case      *CaseSection        `toml:"case"`
	Memo      *MemoSection        `toml:"memo"`
	Jobs      []JobSection        `toml:"job"`
}

// MemoSection represents the [memo] section in a TOML config. When omitted
// (nil) or with no fields, the workspace does not enable the memo feature.
type MemoSection struct {
	// Description is the workspace's "strong definition" of the memo, injected
	// into the agent system prompt and shown in the WebUI.
	Description string `toml:"description"`
	// Fields are the memo custom field definitions ([[memo.fields]]), reusing
	// the same FieldDefinition schema as Case fields.
	Fields []FieldDefinition `toml:"fields"`
}

// ActionSection represents the [action] section in a TOML config.
// When omitted (nil), the workspace inherits the default action status set.
type ActionSection struct {
	Initial string                  `toml:"initial"`
	Closed  []string                `toml:"closed"`
	Status  []ActionStatusConfigRow `toml:"status"`
}

// ActionStatusConfigRow represents a single [[action.status]] entry.
type ActionStatusConfigRow struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Color       string `toml:"color"`
	Emoji       string `toml:"emoji"`
}

// WorkspaceConfig represents a fully resolved workspace configuration
type WorkspaceConfig struct {
	ID                   string
	Name                 string
	Description          string
	Emoji                string
	Color                string
	SlackChannelPrefix   string
	SlackTeamID          string
	SlackInviteUsers     []string
	SlackInviteGroups    []string
	SlackWelcomeMessages []string
	FieldSchema          *domainConfig.FieldSchema
	MemoConfig           *domainConfig.MemoConfig
	ActionStatusSet      *model.ActionStatusSet
	CompilePrompt        string
	AssistPrompt         string
	AssistLanguage       string
	// CaseCreatePrompt is the workspace-specific additional prompt for the
	// thread-mode case initialization agent, from [case.prompts].create.
	CaseCreatePrompt    string
	Jobs                []*model.Job
	CaseMode            model.CaseMode
	SlackMonitorChannel string
	AcceptBot           bool
	CaseStatusSet       *model.ActionStatusSet
}

// Labels represents entity display labels
type Labels struct {
	Case string `toml:"case"`
}

// FieldOption represents an option for select/multi-select fields
type FieldOption struct {
	ID          string         `toml:"id"`
	Name        string         `toml:"name"`
	Description string         `toml:"description"`
	Metadata    map[string]any `toml:"metadata"`
}

// Validate checks if the FieldOption is valid
func (o *FieldOption) Validate(fieldID string) error {
	if !fieldIDPattern.MatchString(o.ID) {
		return goerr.Wrap(ErrInvalidFieldID, "option ID must match pattern ^[a-z][a-z0-9_]*$",
			goerr.V(FieldIDKey, fieldID),
			goerr.V(OptionIDKey, o.ID))
	}
	if o.Name == "" {
		return goerr.Wrap(ErrMissingName, "option name is required",
			goerr.V(FieldIDKey, fieldID),
			goerr.V(OptionIDKey, o.ID))
	}
	return nil
}

// FieldDefinition represents a custom field definition
type FieldDefinition struct {
	ID          string        `toml:"id"`
	Name        string        `toml:"name"`
	Type        string        `toml:"type"`
	Required    bool          `toml:"required"`
	Description string        `toml:"description"`
	Options     []FieldOption `toml:"options"`
}

// Validate checks if the FieldDefinition is valid
func (f *FieldDefinition) Validate() error {
	// Check field ID format
	if !fieldIDPattern.MatchString(f.ID) {
		return goerr.Wrap(ErrInvalidFieldID, "field ID must match pattern ^[a-z][a-z0-9_]*$",
			goerr.V(FieldIDKey, f.ID))
	}

	// Check name is required
	if f.Name == "" {
		return goerr.Wrap(ErrMissingName, "field name is required",
			goerr.V(FieldIDKey, f.ID))
	}

	// Check field type is valid
	fieldType := types.FieldType(f.Type)
	if !fieldType.IsValid() {
		return goerr.Wrap(ErrInvalidFieldType, "field type must be one of the valid types",
			goerr.V(FieldIDKey, f.ID),
			goerr.V(FieldTypeKey, f.Type))
	}

	// Check options requirement for select/multi-select
	if fieldType == types.FieldTypeSelect || fieldType == types.FieldTypeMultiSelect {
		if len(f.Options) == 0 {
			return goerr.Wrap(ErrMissingOptions, "select and multi-select fields must have at least one option",
				goerr.V(FieldIDKey, f.ID),
				goerr.V(FieldTypeKey, f.Type))
		}

		// Check option ID uniqueness within the field
		optionIDs := make(map[string]bool)
		for idx, opt := range f.Options {
			if err := opt.Validate(f.ID); err != nil {
				return goerr.Wrap(err, "invalid option",
					goerr.V(FieldIDKey, f.ID),
					goerr.V(OptionIndexKey, idx))
			}
			if optionIDs[opt.ID] {
				return goerr.Wrap(ErrDuplicateOptionID, "duplicate option ID within field",
					goerr.V(FieldIDKey, f.ID),
					goerr.V(OptionIDKey, opt.ID))
			}
			optionIDs[opt.ID] = true
		}
	}

	return nil
}

// Validate checks if the AppConfig is valid
func (a *AppConfig) Validate() error {
	// Check field ID uniqueness
	fieldIDs := make(map[string]bool)
	for idx, field := range a.Fields {
		if err := field.Validate(); err != nil {
			return goerr.Wrap(err, "invalid field",
				goerr.V(FieldIndexKey, idx))
		}
		if fieldIDs[field.ID] {
			return goerr.Wrap(ErrDuplicateFieldID, "duplicate field ID",
				goerr.V(FieldIDKey, field.ID))
		}
		fieldIDs[field.ID] = true
	}

	// [memo] is optional. When supplied, validate its field definitions with the
	// same rules as case fields (ID pattern, name, type, option requirements) and
	// enforce field-ID uniqueness so schema errors surface at startup.
	if a.Memo != nil {
		memoFieldIDs := make(map[string]bool)
		for idx, field := range a.Memo.Fields {
			if err := field.Validate(); err != nil {
				return goerr.Wrap(err, "invalid memo field",
					goerr.V(FieldIndexKey, idx))
			}
			if memoFieldIDs[field.ID] {
				return goerr.Wrap(ErrDuplicateFieldID, "duplicate memo field ID",
					goerr.V(FieldIDKey, field.ID))
			}
			memoFieldIDs[field.ID] = true
		}
	}

	// [action] is optional. When supplied, build the ActionStatusSet eagerly so
	// schema errors surface at startup.
	if _, err := a.resolveActionStatusSet(); err != nil {
		return goerr.Wrap(err, "invalid [action] section")
	}

	// [[job]] entries are optional. When supplied, validate eagerly so
	// schema / cron / duration errors surface at startup. baseDir is empty:
	// this structural pass must not read prompt_file contents (the config
	// path is unknown here); the file read happens in loadSingleWorkspaceConfig.
	if _, err := a.resolveJobs(""); err != nil {
		return goerr.Wrap(err, "invalid [[job]] section")
	}

	// [slack] mode / channel and [case.status] validation. Resolve eagerly so
	// case-mode misconfigurations surface at startup.
	if err := a.validateCaseMode(); err != nil {
		return err
	}

	return nil
}

// validateCaseMode validates the [slack] mode / channel pairing and the
// [case.status] requirement for thread mode. It also resolves the case status
// set eagerly so schema errors surface at startup.
func (a *AppConfig) validateCaseMode() error {
	mode := model.CaseMode(a.Slack.Mode)
	if a.Slack.Mode != "" && !mode.IsValid() {
		return goerr.Wrap(ErrInvalidCaseMode, "[slack] mode must be \"channel\" or \"thread\"",
			goerr.V("mode", a.Slack.Mode))
	}

	if !mode.IsThread() {
		// Channel mode: monitored channel and [case.status] are not used.
		return nil
	}

	// Thread mode requirements.
	if a.Slack.Channel == "" {
		return goerr.Wrap(ErrMissingMonitorChannel, "[slack] channel is required when mode = \"thread\"")
	}
	if !slackChannelIDPattern.MatchString(a.Slack.Channel) {
		return goerr.Wrap(ErrInvalidMonitorChannel,
			"[slack] channel must be a Slack channel ID (e.g. C0123456789), not a channel name",
			goerr.V("channel", a.Slack.Channel))
	}
	if a.Case == nil || len(a.Case.Status) == 0 {
		return goerr.Wrap(ErrMissingCaseStatus, "[case.status] is required when mode = \"thread\"")
	}
	if _, err := a.resolveCaseStatusSet(); err != nil {
		return goerr.Wrap(err, "invalid [case] section")
	}
	return nil
}

// resolveCaseStatusSet builds the case status set from the [case] section.
// Returns nil (no error) when [case] is omitted; callers requiring a set must
// check for thread mode separately.
func (a *AppConfig) resolveCaseStatusSet() (*model.ActionStatusSet, error) {
	if a.Case == nil {
		return nil, nil
	}
	defs := make([]model.ActionStatusDefinition, 0, len(a.Case.Status))
	for _, row := range a.Case.Status {
		defs = append(defs, model.ActionStatusDefinition{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			Color:       row.Color,
			Emoji:       row.Emoji,
		})
	}
	return model.NewActionStatusSet(a.Case.Initial, a.Case.Closed, defs)
}

// resolveActionStatusSet returns the ActionStatusSet implied by the [action]
// section, or the default set when [action] is omitted.
func (a *AppConfig) resolveActionStatusSet() (*model.ActionStatusSet, error) {
	if a.Action == nil {
		return model.DefaultActionStatusSet(), nil
	}

	defs := make([]model.ActionStatusDefinition, 0, len(a.Action.Status))
	for _, row := range a.Action.Status {
		defs = append(defs, model.ActionStatusDefinition{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			Color:       row.Color,
			Emoji:       row.Emoji,
		})
	}
	return model.NewActionStatusSet(a.Action.Initial, a.Action.Closed, defs)
}

// LoadFieldSchema loads the field schema configuration from a TOML file
// Returns an error if the file does not exist (config.toml is required)
func LoadFieldSchema(path string) (*domainConfig.FieldSchema, error) {
	// Check if config file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, goerr.Wrap(ErrConfigNotFound, "config.toml not found. Please create a configuration file.",
			goerr.V(ConfigPathKey, path))
	}

	// #nosec G304 - path is expected to be provided by CLI argument
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to read config file",
			goerr.V(ConfigPathKey, path))
	}

	var config AppConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, goerr.Wrap(err, "failed to parse TOML config",
			goerr.V(ConfigPathKey, path))
	}

	if err := config.Validate(); err != nil {
		return nil, goerr.Wrap(err, "config validation failed",
			goerr.V(ConfigPathKey, path))
	}

	return config.ToDomainFieldSchema(), nil
}

// LoadWorkspaceConfigs loads workspace configurations from multiple paths.
// Each path can be a file or directory. Directories are walked recursively for .toml files.
func LoadWorkspaceConfigs(paths []string) ([]*WorkspaceConfig, error) {
	var tomlFiles []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to stat config path", goerr.V(ConfigPathKey, p))
		}

		if info.IsDir() {
			err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(d.Name(), ".toml") {
					tomlFiles = append(tomlFiles, path)
				}
				return nil
			})
			if err != nil {
				return nil, goerr.Wrap(err, "failed to walk config directory", goerr.V(ConfigPathKey, p))
			}
		} else {
			tomlFiles = append(tomlFiles, p)
		}
	}

	if len(tomlFiles) == 0 {
		return nil, goerr.Wrap(ErrNoConfigFiles, "no .toml files found in specified paths")
	}

	var configs []*WorkspaceConfig
	seenIDs := make(map[string]string) // workspaceID → file path
	for _, f := range tomlFiles {
		wc, err := loadSingleWorkspaceConfig(f)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to load workspace config", goerr.V(ConfigPathKey, f))
		}

		if existing, ok := seenIDs[wc.ID]; ok {
			return nil, goerr.Wrap(ErrDuplicateWorkspaceID, "duplicate workspace ID",
				goerr.V(WorkspaceIDKey, wc.ID),
				goerr.V("first_file", existing),
				goerr.V("second_file", f),
			)
		}
		seenIDs[wc.ID] = f
		configs = append(configs, wc)
	}

	return configs, nil
}

func loadSingleWorkspaceConfig(path string) (*WorkspaceConfig, error) {
	// #nosec G304 - path is expected to be provided by CLI argument
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to read config file", goerr.V(ConfigPathKey, path))
	}

	var appCfg AppConfig
	if err := toml.Unmarshal(data, &appCfg); err != nil {
		return nil, goerr.Wrap(err, "failed to parse TOML config", goerr.V(ConfigPathKey, path))
	}

	if err := appCfg.Validate(); err != nil {
		return nil, goerr.Wrap(err, "config validation failed", goerr.V(ConfigPathKey, path))
	}

	// Resolve workspace ID and name from [workspace] section
	wsID := appCfg.Workspace.ID
	if wsID == "" {
		return nil, goerr.Wrap(ErrMissingWorkspaceID,
			"[workspace] id is required in config file",
			goerr.V(ConfigPathKey, path),
		)
	}
	wsName := appCfg.Workspace.Name
	if wsName == "" {
		wsName = wsID
	}

	// Validate workspace ID
	if !workspaceIDPattern.MatchString(wsID) || len(wsID) > 63 {
		return nil, goerr.Wrap(ErrInvalidWorkspaceID,
			"workspace ID must match ^[a-z0-9]+(-[a-z0-9]+)*$ and be at most 63 characters",
			goerr.V(WorkspaceIDKey, wsID),
			goerr.V(ConfigPathKey, path),
		)
	}

	// Validate optional emoji/color (mutually exclusive, color format, emoji length)
	if err := appCfg.Workspace.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid [workspace] emoji/color", goerr.V(ConfigPathKey, path))
	}

	// Use workspace ID as default Slack channel prefix if not specified
	slackPrefix := appCfg.Slack.ChannelPrefix
	if slackPrefix == "" {
		slackPrefix = wsID
	}

	// Validate welcome message templates eagerly so misconfigurations are
	// caught at startup rather than at the first case creation.
	for i, msg := range appCfg.Slack.WelcomeMessages {
		if _, parseErr := template.New("welcome").Parse(msg); parseErr != nil {
			return nil, goerr.Wrap(ErrInvalidWelcomeMessage,
				"failed to parse Slack welcome message template",
				goerr.V(ConfigPathKey, path),
				goerr.V("index", i),
				goerr.V("template", msg),
				goerr.V("parse_error", parseErr.Error()),
			)
		}
	}

	statusSet, err := appCfg.resolveActionStatusSet()
	if err != nil {
		return nil, goerr.Wrap(err, "failed to resolve action status set", goerr.V(ConfigPathKey, path))
	}

	// Resolve relative prompt_file paths against the config file's directory.
	jobs, err := appCfg.resolveJobs(filepath.Dir(path))
	if err != nil {
		return nil, goerr.Wrap(err, "failed to resolve jobs", goerr.V(ConfigPathKey, path))
	}

	caseMode := model.CaseMode(appCfg.Slack.Mode).Normalize()
	caseStatusSet, err := appCfg.resolveCaseStatusSet()
	if err != nil {
		return nil, goerr.Wrap(err, "failed to resolve case status set", goerr.V(ConfigPathKey, path))
	}

	caseCreatePrompt := ""
	if appCfg.Case != nil {
		caseCreatePrompt = appCfg.Case.Prompts.Create
	}
	if len(caseCreatePrompt) > model.AgentAdditionalPromptMaxLen {
		return nil, goerr.New("[case.prompts].create exceeds the maximum length",
			goerr.V(ConfigPathKey, path),
			goerr.V("len", len(caseCreatePrompt)),
			goerr.V("max", model.AgentAdditionalPromptMaxLen))
	}

	// Warn about channel-mode-only settings supplied to a thread-mode workspace
	// (and vice versa) so operators notice ignored configuration at startup.
	if caseMode.IsThread() {
		if slackPrefix != wsID || len(appCfg.Slack.Invite.Users) > 0 ||
			len(appCfg.Slack.Invite.Groups) > 0 || len(appCfg.Slack.WelcomeMessages) > 0 {
			logging.Default().Warn("thread-mode workspace ignores channel-mode Slack settings (channel_prefix / invite / welcome_messages)",
				"workspace_id", wsID, "config_path", path)
		}
	} else if appCfg.Case != nil {
		logging.Default().Warn("channel-mode workspace ignores [case.status]",
			"workspace_id", wsID, "config_path", path)
	}

	return &WorkspaceConfig{
		ID:                   wsID,
		Name:                 wsName,
		Description:          appCfg.Workspace.Description,
		Emoji:                appCfg.Workspace.Emoji,
		Color:                appCfg.Workspace.Color,
		SlackChannelPrefix:   slackPrefix,
		SlackTeamID:          appCfg.Slack.TeamID,
		SlackInviteUsers:     appCfg.Slack.Invite.Users,
		SlackInviteGroups:    appCfg.Slack.Invite.Groups,
		SlackWelcomeMessages: appCfg.Slack.WelcomeMessages,
		FieldSchema:          appCfg.ToDomainFieldSchema(),
		MemoConfig:           appCfg.ToDomainMemoConfig(),
		ActionStatusSet:      statusSet,
		CompilePrompt:        appCfg.Compile.Prompt,
		AssistPrompt:         appCfg.Assist.Prompt,
		AssistLanguage:       appCfg.Assist.Language,
		CaseCreatePrompt:     caseCreatePrompt,
		Jobs:                 jobs,
		CaseMode:             caseMode,
		SlackMonitorChannel:  appCfg.Slack.Channel,
		AcceptBot:            appCfg.Slack.AcceptBot,
		CaseStatusSet:        caseStatusSet,
	}, nil
}

// Flags returns CLI flags for workspace configuration.
func (a *AppConfig) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "config",
			Usage:   "Paths to configuration files or directories (TOML). Can be specified multiple times.",
			Value:   []string{"./config.toml"},
			Sources: cli.EnvVars("HECATONCHEIRES_CONFIG"),
		},
	}
}

// Configure loads workspace configs from CLI-provided paths and builds a WorkspaceRegistry.
// It reads "config" from the cli.Command since StringSliceFlag does not support Destination.
func (a *AppConfig) Configure(c *cli.Command) ([]*WorkspaceConfig, *model.WorkspaceRegistry, error) {
	paths := c.StringSlice("config")

	workspaceConfigs, err := LoadWorkspaceConfigs(paths)
	if err != nil {
		return nil, nil, err
	}

	registry := model.NewWorkspaceRegistry()
	for _, wc := range workspaceConfigs {
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{
				ID:          wc.ID,
				Name:        wc.Name,
				Description: wc.Description,
				Emoji:       wc.Emoji,
				Color:       wc.Color,
			},
			FieldSchema:           wc.FieldSchema,
			MemoConfig:            wc.MemoConfig,
			ActionStatusSet:       wc.ActionStatusSet,
			SlackChannelPrefix:    wc.SlackChannelPrefix,
			SlackTeamID:           wc.SlackTeamID,
			SlackInviteUsers:      wc.SlackInviteUsers,
			SlackInviteGroups:     wc.SlackInviteGroups,
			SlackWelcomeMessages:  wc.SlackWelcomeMessages,
			CompilePrompt:         wc.CompilePrompt,
			AssistPrompt:          wc.AssistPrompt,
			AssistLanguage:        wc.AssistLanguage,
			CaseCreatePrompt:      wc.CaseCreatePrompt,
			Jobs:                  wc.Jobs,
			CaseMode:              wc.CaseMode,
			SlackMonitorChannelID: wc.SlackMonitorChannel,
			AcceptBot:             wc.AcceptBot,
			CaseStatusSet:         wc.CaseStatusSet,
		})
		logging.Default().Info("Registered workspace", "id", wc.ID, "name", wc.Name, "case_mode", wc.CaseMode.Normalize())
	}

	return workspaceConfigs, registry, nil
}

// toDomainFields converts TOML FieldDefinition rows into their domain form.
// Shared by case field schema and memo field schema conversion.
func toDomainFields(in []FieldDefinition) []domainConfig.FieldDefinition {
	fields := make([]domainConfig.FieldDefinition, len(in))
	for i, field := range in {
		options := make([]domainConfig.FieldOption, len(field.Options))
		for j, opt := range field.Options {
			options[j] = domainConfig.FieldOption{
				ID:          opt.ID,
				Name:        opt.Name,
				Description: opt.Description,
				Metadata:    opt.Metadata,
			}
		}

		fields[i] = domainConfig.FieldDefinition{
			ID:          field.ID,
			Name:        field.Name,
			Type:        types.FieldType(field.Type),
			Required:    field.Required,
			Description: field.Description,
			Options:     options,
		}
	}
	return fields
}

// ToDomainFieldSchema converts AppConfig to domain FieldSchema
func (a *AppConfig) ToDomainFieldSchema() *domainConfig.FieldSchema {
	labels := domainConfig.EntityLabels{
		Case: a.Labels.Case,
	}
	// Set default labels if not specified
	if labels.Case == "" {
		labels.Case = "Case"
	}

	return &domainConfig.FieldSchema{
		Fields: toDomainFields(a.Fields),
		Labels: labels,
	}
}

// ToDomainMemoConfig converts the [memo] section to a domain MemoConfig.
// Returns nil when [memo] is omitted, leaving the memo feature disabled for the
// workspace.
func (a *AppConfig) ToDomainMemoConfig() *domainConfig.MemoConfig {
	if a.Memo == nil {
		return nil
	}
	return &domainConfig.MemoConfig{
		Description: a.Memo.Description,
		FieldSchema: &domainConfig.FieldSchema{
			Fields: toDomainFields(a.Memo.Fields),
		},
	}
}
