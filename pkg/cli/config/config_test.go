package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestLoadFieldSchema(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name: "valid configuration with all field types",
			content: `
[labels]
case = "Risk"

[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true
description = "Classification of the case"

  [[fields.options]]
  id = "data_breach"
  name = "Data Breach"
  description = "Risk of data leakage"
  color = "#E53E3E"

  [[fields.options]]
  id = "system_failure"
  name = "System Failure"
  color = "#DD6B20"

[[fields]]
id = "likelihood"
name = "Likelihood"
type = "select"
required = true
description = "Probability of occurrence"

  [[fields.options]]
  id = "very_low"
  name = "Very Low"
  description = "Extremely unlikely to occur"
  [fields.options.metadata]
  score = 1

  [[fields.options]]
  id = "high"
  name = "High"
  [fields.options.metadata]
  score = 4

[[fields]]
id = "specific_impact"
name = "Specific Impact"
type = "text"
required = false
description = "Detailed impact description"

[[fields]]
id = "score"
name = "Score"
type = "number"
required = false

[[fields]]
id = "assignee"
name = "Assignee"
type = "user"
required = false

[[fields]]
id = "responders"
name = "Responders"
type = "multi-user"
required = false

[[fields]]
id = "due_date"
name = "Due Date"
type = "date"
required = false

[[fields]]
id = "reference_url"
name = "Reference URL"
type = "url"
required = false
`,
			wantErr: nil,
		},
		{
			name: "default labels when not specified",
			content: `
[[fields]]
id = "status"
name = "Status"
type = "select"
required = false

  [[fields.options]]
  id = "active"
  name = "Active"
`,
			wantErr: nil,
		},
		{
			name:    "config file not found",
			content: "", // Won't create the file
			wantErr: config.ErrConfigNotFound,
		},
		{
			name: "duplicate field ID",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "text"

[[fields]]
id = "category"
name = "Duplicate"
type = "text"
`,
			wantErr: config.ErrDuplicateFieldID,
		},
		{
			name: "invalid field ID format (uppercase)",
			content: `
[[fields]]
id = "CategoryID"
name = "Category"
type = "text"
`,
			wantErr: config.ErrInvalidFieldID,
		},
		{
			name: "invalid field ID format (hyphen no longer allowed)",
			content: `
[[fields]]
id = "category-id"
name = "Category"
type = "text"
`,
			wantErr: config.ErrInvalidFieldID,
		},
		{
			name: "invalid field ID format (leading digit)",
			content: `
[[fields]]
id = "1category"
name = "Category"
type = "text"
`,
			wantErr: config.ErrInvalidFieldID,
		},
		{
			name: "missing field name",
			content: `
[[fields]]
id = "category"
type = "text"
`,
			wantErr: config.ErrMissingName,
		},
		{
			name: "invalid field type",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "invalid-type"
`,
			wantErr: config.ErrInvalidFieldType,
		},
		{
			name: "select field without options",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "select"
required = true
`,
			wantErr: config.ErrMissingOptions,
		},
		{
			name: "multi-select field without options",
			content: `
[[fields]]
id = "tags"
name = "Tags"
type = "multi-select"
`,
			wantErr: config.ErrMissingOptions,
		},
		{
			name: "duplicate option ID within field",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "select"

  [[fields.options]]
  id = "option_a"
  name = "Option A"

  [[fields.options]]
  id = "option_a"
  name = "Duplicate"
`,
			wantErr: config.ErrDuplicateOptionID,
		},
		{
			name: "invalid option ID format",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "select"

  [[fields.options]]
  id = "OptionA"
  name = "Option A"
`,
			wantErr: config.ErrInvalidFieldID,
		},
		{
			name: "missing option name",
			content: `
[[fields]]
id = "category"
name = "Category"
type = "select"

  [[fields.options]]
  id = "option_a"
`,
			wantErr: config.ErrMissingName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")

			// Only create file if content is not empty
			if tt.content != "" {
				err := os.WriteFile(configPath, []byte(tt.content), 0644)
				gt.NoError(t, err).Required()
			}

			schema, err := config.LoadFieldSchema(configPath)

			if tt.wantErr != nil {
				gt.Value(t, err).NotNil()
				if err != nil {
					gt.Error(t, err).Is(tt.wantErr)
				}
				return
			}

			gt.NoError(t, err)
			if err != nil {
				return
			}

			gt.Value(t, schema).NotNil()
		})
	}
}

func TestLoadFieldSchema_ValidConfiguration(t *testing.T) {
	content := `
[labels]
case = "Risk"

[[fields]]
id = "category"
name = "Category"
type = "multi-select"
required = true

  [[fields.options]]
  id = "data_breach"
  name = "Data Breach"
  description = "Risk of personal or confidential information leakage"
  [fields.options.metadata]
  severity = "high"
  priority = 1
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	schema, err := config.LoadFieldSchema(configPath)
	gt.NoError(t, err).Required()

	// Check labels
	gt.Value(t, schema.Labels.Case).Equal("Risk")

	// Check field count
	gt.Array(t, schema.Fields).Length(1).Required()

	// Check field details
	field := schema.Fields[0]
	gt.Value(t, field.ID).Equal("category")
	gt.Value(t, field.Name).Equal("Category")
	gt.Value(t, field.Type).Equal("multi-select")
	gt.Bool(t, field.Required).True()

	// Check option count
	gt.Array(t, field.Options).Length(1).Required()

	// Check option details
	option := field.Options[0]
	gt.Value(t, option.ID).Equal("data_breach")
	gt.Value(t, option.Name).Equal("Data Breach")
	gt.Value(t, option.Description).Equal("Risk of personal or confidential information leakage")

	// Check metadata: severity / priority remain as free-form metadata.
	gt.Value(t, option.Metadata).NotNil().Required()
	severity, ok := option.Metadata["severity"].(string)
	gt.Bool(t, ok).True()
	gt.Value(t, severity).Equal("high")
	priority, ok := option.Metadata["priority"].(int64)
	gt.Bool(t, ok).True()
	gt.Value(t, priority).Equal(int64(1))
}

func TestLoadWorkspaceConfigs_SingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"

[labels]
case = "Risk"

[[fields]]
id = "category"
name = "Category"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1)
	gt.Value(t, configs[0].ID).Equal("risk")
	gt.Value(t, configs[0].Name).Equal("Risk Management")
	gt.Value(t, configs[0].FieldSchema).NotNil()
	gt.Value(t, configs[0].FieldSchema.Labels.Case).Equal("Risk")
	gt.Array(t, configs[0].FieldSchema.Fields).Length(1)
}

func TestLoadWorkspaceConfigs_Directory(t *testing.T) {
	tmpDir := t.TempDir()

	risk := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "status"
name = "Status"
type = "text"
`
	recruit := `
[workspace]
id = "recruit"
name = "Recruitment"

[[fields]]
id = "role"
name = "Role"
type = "text"
`
	err := os.WriteFile(filepath.Join(tmpDir, "risk.toml"), []byte(risk), 0644)
	gt.NoError(t, err).Required()
	err = os.WriteFile(filepath.Join(tmpDir, "recruit.toml"), []byte(recruit), 0644)
	gt.NoError(t, err).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{tmpDir})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(2)

	// Collect IDs (order depends on filesystem walk order)
	ids := map[string]bool{}
	for _, c := range configs {
		ids[c.ID] = true
	}
	gt.Bool(t, ids["risk"]).True()
	gt.Bool(t, ids["recruit"]).True()
}

func TestLoadWorkspaceConfigs_MissingWorkspaceID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "onboarding.toml")
	content := `
[[fields]]
id = "task"
name = "Task"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	_, err = config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrMissingWorkspaceID)
}

func TestLoadWorkspaceConfigs_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()

	content1 := `
[workspace]
id = "risk"
name = "Risk One"

[[fields]]
id = "a"
name = "A"
type = "text"
`
	content2 := `
[workspace]
id = "risk"
name = "Risk Two"

[[fields]]
id = "b"
name = "B"
type = "text"
`
	err := os.WriteFile(filepath.Join(tmpDir, "risk1.toml"), []byte(content1), 0644)
	gt.NoError(t, err).Required()
	err = os.WriteFile(filepath.Join(tmpDir, "risk2.toml"), []byte(content2), 0644)
	gt.NoError(t, err).Required()

	_, err = config.LoadWorkspaceConfigs([]string{tmpDir})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrDuplicateWorkspaceID)
}

func TestLoadWorkspaceConfigs_InvalidWorkspaceID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "INVALID_ID"
name = "Bad ID"

[[fields]]
id = "a"
name = "A"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	_, err = config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceID)
}

func TestLoadWorkspaceConfigs_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := config.LoadWorkspaceConfigs([]string{tmpDir})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrNoConfigFiles)
}

func TestLoadWorkspaceConfigs_MixedFileAndDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "workspaces")
	err := os.Mkdir(subDir, 0755)
	gt.NoError(t, err).Required()

	// File directly
	file1 := filepath.Join(tmpDir, "base.toml")
	err = os.WriteFile(file1, []byte(`
[workspace]
id = "base"
name = "Base"

[[fields]]
id = "x"
name = "X"
type = "text"
`), 0644)
	gt.NoError(t, err).Required()

	// File in subdirectory
	err = os.WriteFile(filepath.Join(subDir, "extra.toml"), []byte(`
[workspace]
id = "extra"
name = "Extra"

[[fields]]
id = "y"
name = "Y"
type = "text"
`), 0644)
	gt.NoError(t, err).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{file1, subDir})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(2)

	ids := map[string]bool{}
	for _, c := range configs {
		ids[c.ID] = true
	}
	gt.Bool(t, ids["base"]).True()
	gt.Bool(t, ids["extra"]).True()
}

func TestLoadWorkspaceConfigs_WorkspaceIDTooLong(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// ID with 64 characters (exceeds 63 limit)
	longID := "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01"
	content := `
[workspace]
id = "` + longID + `"
name = "Too Long"

[[fields]]
id = "a"
name = "A"
type = "text"
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	_, err = config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceID)
}

func TestLoadWorkspaceConfigs_SlackChannelPrefix(t *testing.T) {
	t.Run("explicit prefix", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[slack]
channel_prefix = "incident"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].SlackChannelPrefix).Equal("incident")
	})

	t.Run("fallback to workspace ID when omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].SlackChannelPrefix).Equal("risk")
	})
}

func TestLoadWorkspaceConfigs_CompilePrompt(t *testing.T) {
	t.Run("parses compile prompt from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "security"
name = "Security"

[compile]
prompt = "Focus on security vulnerabilities and threat intelligence."

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].CompilePrompt).Equal("Focus on security vulnerabilities and threat intelligence.")
	})

	t.Run("empty compile prompt when section omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].CompilePrompt).Equal("")
	})
}

func TestLoadWorkspaceConfigs_AssistPrompt(t *testing.T) {
	t.Run("parses assist prompt from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "security"
name = "Security"

[assist]
prompt = "Check action deadlines and follow up on pending items."

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].AssistPrompt).Equal("Check action deadlines and follow up on pending items.")
	})

	t.Run("empty assist prompt when section omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].AssistPrompt).Equal("")
	})
}

func TestLoadWorkspaceConfigs_AssistLanguage(t *testing.T) {
	t.Run("parses assist language from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "security"
name = "Security"

[assist]
prompt = "Check deadlines."
language = "Japanese"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].AssistLanguage).Equal("Japanese")
	})

	t.Run("empty assist language when not specified", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk"

[assist]
prompt = "Check deadlines."

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].AssistLanguage).Equal("")
	})
}

func TestLoadWorkspaceConfigs_SlackInvite(t *testing.T) {
	t.Run("parses slack invite section", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[slack]
channel_prefix = "risk"

[slack.invite]
users = ["U12345678", "U87654321"]
groups = ["S0614TZR7", "@security-team"]

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Array(t, configs[0].SlackInviteUsers).Length(2)
		gt.Value(t, configs[0].SlackInviteUsers[0]).Equal("U12345678")
		gt.Value(t, configs[0].SlackInviteUsers[1]).Equal("U87654321")
		gt.Array(t, configs[0].SlackInviteGroups).Length(2)
		gt.Value(t, configs[0].SlackInviteGroups[0]).Equal("S0614TZR7")
		gt.Value(t, configs[0].SlackInviteGroups[1]).Equal("@security-team")
	})

	t.Run("empty invite section defaults to empty slices", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Array(t, configs[0].SlackInviteUsers).Length(0)
		gt.Array(t, configs[0].SlackInviteGroups).Length(0)
	})
}

func TestLoadWorkspaceConfigs_SlackWelcomeMessages(t *testing.T) {
	t.Run("parses welcome messages", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[slack]
welcome_messages = [
  "Hello {{.Case.Title}}",
  "Reporter: <@{{.Case.ReporterID}}>",
]

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Array(t, configs[0].SlackWelcomeMessages).Length(2)
		gt.Value(t, configs[0].SlackWelcomeMessages[0]).Equal("Hello {{.Case.Title}}")
		gt.Value(t, configs[0].SlackWelcomeMessages[1]).Equal("Reporter: <@{{.Case.ReporterID}}>")
	})

	t.Run("empty when omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Array(t, configs[0].SlackWelcomeMessages).Length(0)
	})

	t.Run("rejects template with parse error", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[slack]
welcome_messages = [
  "Hello {{.Case.Title",
]

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		_, err = config.LoadWorkspaceConfigs([]string{configPath})
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrInvalidWelcomeMessage)
	})
}

func TestLoadFieldSchema_DefaultLabels(t *testing.T) {
	content := `
[[fields]]
id = "status"
name = "Status"
type = "text"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	err := os.WriteFile(configPath, []byte(content), 0644)
	gt.NoError(t, err).Required()

	schema, err := config.LoadFieldSchema(configPath)
	gt.NoError(t, err).Required()

	// Check default labels
	gt.Value(t, schema.Labels.Case).Equal("Case")
}

func TestLoadWorkspaceConfigs_SlackTeamID(t *testing.T) {
	t.Run("explicit team_id", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[slack]
team_id = "T0123456789"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].SlackTeamID).Equal("T0123456789")
	})

	t.Run("omitted team_id defaults to empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "a"
name = "A"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].SlackTeamID).Equal("")
	})
}

func TestLoadWorkspaceConfigs_ActionStatuses_Default(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "risk"
name = "Risk"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()

	set := configs[0].ActionStatusSet
	gt.Value(t, set).NotNil().Required()
	gt.Value(t, set.InitialID()).Equal("BACKLOG")
	gt.Bool(t, set.IsClosed("COMPLETED")).True()
	gt.Bool(t, set.IsClosed("BACKLOG")).False()
}

func TestLoadWorkspaceConfigs_ActionStatuses_Custom(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "ops"
name = "Ops"

[action]
initial = "queued"
closed = ["done", "abandoned"]

[[action.status]]
id = "queued"
name = "Queued"
color = "idle"
emoji = "📋"

[[action.status]]
id = "working"
name = "Working"
color = "active"

[[action.status]]
id = "waiting_user"
name = "Waiting on user"
color = "waiting"

[[action.status]]
id = "done"
name = "Done"
color = "success"
emoji = "✅"

[[action.status]]
id = "abandoned"
name = "Abandoned"
color = "neutral_done"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()

	set := configs[0].ActionStatusSet
	gt.Value(t, set).NotNil().Required()
	gt.Value(t, set.InitialID()).Equal("queued")
	gt.Value(t, set.ClosedIDs()).Equal([]string{"done", "abandoned"})
	gt.Bool(t, set.IsValid("waiting_user")).True()
	def, ok := set.Get("queued")
	gt.Bool(t, ok).True()
	gt.Value(t, def.Name).Equal("Queued")
	gt.Value(t, def.Emoji).Equal("📋")
}

func TestLoadWorkspaceConfigs_ActionStatuses_InvalidColor(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "ops"

[action]
initial = "x"

[[action.status]]
id = "x"
name = "X"
color = "rainbow"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Error(t, err)
}

func TestLoadWorkspaceConfigs_ActionStatuses_InitialMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "ops"

[action]
initial = "ghost"

[[action.status]]
id = "real"
name = "Real"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Error(t, err)
}

func TestLoadWorkspaceConfigs_ActionStatuses_ClosedMissing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "ops"

[action]
initial = "x"
closed = ["ghost"]

[[action.status]]
id = "x"
name = "X"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Error(t, err)
}

func TestLoadWorkspaceConfigs_ActionStatuses_DuplicateID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	content := `
[workspace]
id = "ops"

[action]
initial = "x"

[[action.status]]
id = "x"
name = "X"

[[action.status]]
id = "x"
name = "X again"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Error(t, err)
}

// When the top-level description is omitted, the resulting field option has
// an empty Description. Free-form metadata is preserved verbatim — it is the
// caller's responsibility (TOML author) to put description at the top level.
func TestToDomainFieldSchema_OptionWithoutDescription(t *testing.T) {
	content := `
[labels]
case = "Risk"

[[fields]]
id = "category"
name = "Category"
type = "select"
required = true

  [[fields.options]]
  id = "compliance"
  name = "Compliance"
  [fields.options.metadata]
  score = 3
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	schema, err := config.LoadFieldSchema(configPath)
	gt.NoError(t, err).Required()
	gt.Array(t, schema.Fields).Length(1).Required()
	gt.Array(t, schema.Fields[0].Options).Length(1).Required()
	gt.Value(t, schema.Fields[0].Options[0].Description).Equal("")
	// Metadata stays untouched.
	score, ok := schema.Fields[0].Options[0].Metadata["score"].(int64)
	gt.Bool(t, ok).True()
	gt.Value(t, score).Equal(int64(3))
}

func TestLoadWorkspaceConfigs_Emoji(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
emoji = "🛡️"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()
	gt.Value(t, configs[0].Emoji).Equal("🛡️")
	gt.Value(t, configs[0].Color).Equal("")
}

func TestLoadWorkspaceConfigs_Color(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "incident.toml")
	content := `
[workspace]
id = "incident"
name = "Incident Response"
color = "#c8501c"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()
	gt.Value(t, configs[0].Color).Equal("#c8501c")
	gt.Value(t, configs[0].Emoji).Equal("")
}

func TestLoadWorkspaceConfigs_EmojiColorConflict(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
emoji = "🛡️"
color = "#c8501c"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrWorkspaceEmojiColorConflict)
}

func TestLoadWorkspaceConfigs_InvalidColor(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
color = "blue"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceColor)
}

func TestLoadWorkspaceConfigs_InvalidColorThreeDigit(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
color = "#fff"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceColor)
}

func TestLoadWorkspaceConfigs_EmojiTooLong(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
emoji = "this is clearly not a single emoji glyph"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrInvalidWorkspaceEmoji)
}

func TestLoadWorkspaceConfigs_NoEmojiNoColor(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	content := `
[workspace]
id = "risk"
name = "Risk Management"
`
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	configs, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.NoError(t, err).Required()
	gt.Array(t, configs).Length(1).Required()
	gt.Value(t, configs[0].Emoji).Equal("")
	gt.Value(t, configs[0].Color).Equal("")
}

func TestLoadWorkspaceConfigs_CaseMode(t *testing.T) {
	writeAndLoad := func(t *testing.T, content string) ([]*config.WorkspaceConfig, error) {
		t.Helper()
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.toml")
		gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()
		return config.LoadWorkspaceConfigs([]string{configPath})
	}

	t.Run("defaults to channel mode when omitted", func(t *testing.T) {
		configs, err := writeAndLoad(t, `
[workspace]
id = "risk"

[[fields]]
id = "a"
name = "A"
type = "text"
`)
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].CaseMode).Equal(model.CaseModeChannel)
		gt.Value(t, configs[0].SlackMonitorChannel).Equal("")
		gt.Value(t, configs[0].CaseStatusSet).Nil()
	})

	t.Run("thread mode resolves monitored channel and case status set", func(t *testing.T) {
		configs, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"

[case]
initial = "TRIAGE"
closed = ["DONE"]

  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
  color = "active"

  [[case.status]]
  id = "DONE"
  name = "Done"
  color = "success"

[[fields]]
id = "a"
name = "A"
type = "text"
`)
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].CaseMode).Equal(model.CaseModeThread)
		gt.Value(t, configs[0].SlackMonitorChannel).Equal("C0123ABC")
		set := configs[0].CaseStatusSet
		gt.Value(t, set).NotNil().Required()
		gt.Value(t, set.InitialID()).Equal("TRIAGE")
		gt.Bool(t, set.IsClosed("DONE")).True()
		gt.Bool(t, set.IsClosed("TRIAGE")).False()
		gt.Array(t, set.IDs()).Length(2)
		// accept_bot defaults to false when omitted.
		gt.Value(t, configs[0].AcceptBot).Equal(false)
		// trigger defaults to instant when omitted.
		gt.Value(t, configs[0].CaseTrigger).Equal(model.CaseTriggerInstant)
	})

	t.Run("thread mode selects mention trigger", func(t *testing.T) {
		configs, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"
trigger = "mention"

[case]
initial = "TRIAGE"
closed = ["DONE"]

  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
  color = "active"

  [[case.status]]
  id = "DONE"
  name = "Done"
  color = "success"

[[fields]]
id = "a"
name = "A"
type = "text"
`)
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].CaseTrigger).Equal(model.CaseTriggerMention)
	})

	t.Run("invalid trigger is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"
trigger = "bogus"

[case]
initial = "TRIAGE"
  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrInvalidCaseTrigger)).True()
	})

	threadWithReaction := func(reaction string) string {
		return `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"
reaction = "` + reaction + `"

[case]
initial = "TRIAGE"
closed = ["DONE"]

  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
  color = "active"

  [[case.status]]
  id = "DONE"
  name = "Done"
  color = "success"

[[fields]]
id = "a"
name = "A"
type = "text"
`
	}

	t.Run("thread mode parses and normalizes the reaction emoji", func(t *testing.T) {
		configs, err := writeAndLoad(t, threadWithReaction(":incident:"))
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].ReactionEmoji).Equal("incident")
	})

	t.Run("reaction without colons is accepted as-is", func(t *testing.T) {
		configs, err := writeAndLoad(t, threadWithReaction("white_check_mark"))
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].ReactionEmoji).Equal("white_check_mark")
	})

	t.Run("a skin-tone modifier is stripped to the base emoji", func(t *testing.T) {
		configs, err := writeAndLoad(t, threadWithReaction("wave::skin-tone-2"))
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].ReactionEmoji).Equal("wave")
	})

	t.Run("reaction on channel mode is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "channel"
reaction = "incident"

[[fields]]
id = "a"
name = "A"
type = "text"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrReactionRequiresThreadMode)).True()
	})

	t.Run("invalid reaction emoji is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, threadWithReaction("Not Valid!"))
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrInvalidReactionEmoji)).True()
	})

	t.Run("duplicate reaction across workspaces is rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		one := `
[workspace]
id = "ws1"

[slack]
mode = "thread"
channel = "C0111111"
reaction = "incident"

[case]
initial = "TRIAGE"
  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
`
		two := `
[workspace]
id = "ws2"

[slack]
mode = "thread"
channel = "C0222222"
reaction = ":incident:"

[case]
initial = "TRIAGE"
  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
`
		gt.NoError(t, os.WriteFile(filepath.Join(tmpDir, "ws1.toml"), []byte(one), 0644)).Required()
		gt.NoError(t, os.WriteFile(filepath.Join(tmpDir, "ws2.toml"), []byte(two), 0644)).Required()
		_, err := config.LoadWorkspaceConfigs([]string{tmpDir})
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrDuplicateReactionEmoji)).True()
	})

	t.Run("thread mode opts into bot posts via accept_bot", func(t *testing.T) {
		configs, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"
accept_bot = true

[case]
initial = "TRIAGE"
closed = ["DONE"]

  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
  color = "active"

  [[case.status]]
  id = "DONE"
  name = "Done"
  color = "success"

[[fields]]
id = "a"
name = "A"
type = "text"
`)
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].AcceptBot).Equal(true)
	})

	t.Run("invalid mode is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "bogus"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrInvalidCaseMode)).True()
	})

	t.Run("thread mode without channel is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"

[case]
initial = "TRIAGE"
  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrMissingMonitorChannel)).True()
	})

	t.Run("thread mode with channel name (not ID) is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "#team-support"

[case]
initial = "TRIAGE"
  [[case.status]]
  id = "TRIAGE"
  name = "Triage"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrInvalidMonitorChannel)).True()
	})

	t.Run("thread mode without [case.status] is rejected", func(t *testing.T) {
		_, err := writeAndLoad(t, `
[workspace]
id = "support"

[slack]
mode = "thread"
channel = "C0123ABC"
`)
		gt.Error(t, err).Required()
		gt.Bool(t, errors.Is(err, config.ErrMissingCaseStatus)).True()
	})
}

func TestLoadWorkspaceConfigs_CaseCreatePrompt(t *testing.T) {
	t.Run("loads [case.prompts].create", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "support.toml")
		content := `
[workspace]
id = "support"
name = "Support"

[slack]
mode = "thread"
channel = "C0123456789"

[case]
initial = "TRIAGE"
closed = ["DONE"]

[[case.status]]
id = "TRIAGE"
name = "Triage"

[[case.status]]
id = "DONE"
name = "Done"

[case.prompts]
create = "Always fill the severity field for security cases."
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.String(t, configs[0].CaseCreatePrompt).Equal("Always fill the severity field for security cases.")
	})

	t.Run("absent [case.prompts] yields empty prompt", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "support.toml")
		content := `
[workspace]
id = "support"
name = "Support"

[slack]
mode = "thread"
channel = "C0123456789"

[case]
initial = "TRIAGE"
closed = ["DONE"]

[[case.status]]
id = "TRIAGE"
name = "Triage"

[[case.status]]
id = "DONE"
name = "Done"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.String(t, configs[0].CaseCreatePrompt).Equal("")
	})
}

func TestLoadWorkspaceConfigs_Memo(t *testing.T) {
	t.Run("parses memo section with description and fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "risk.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "category"
name = "Category"
type = "text"

[memo]
description = "Investigation memory for this case."

[[memo.fields]]
id = "memo_type"
name = "Type"
type = "select"
required = true
options = [
  { id = "fact", name = "Fact" },
  { id = "hypothesis", name = "Hypothesis" },
]

[[memo.fields]]
id = "body"
name = "Body"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()

		mc := configs[0].MemoConfig
		gt.Value(t, mc).NotNil().Required()
		gt.String(t, mc.Description).Equal("Investigation memory for this case.")
		gt.Bool(t, mc.Enabled()).True()
		gt.Value(t, mc.FieldSchema).NotNil().Required()
		gt.Array(t, mc.FieldSchema.Fields).Length(2).Required()
		gt.String(t, mc.FieldSchema.Fields[0].ID).Equal("memo_type")
		gt.Bool(t, mc.FieldSchema.Fields[0].Required).True()
		gt.Array(t, mc.FieldSchema.Fields[0].Options).Length(2)
		gt.String(t, mc.FieldSchema.Fields[1].ID).Equal("body")
	})

	t.Run("memo omitted leaves config nil and feature disabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "risk.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[[fields]]
id = "category"
name = "Category"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		configs, err := config.LoadWorkspaceConfigs([]string{configPath})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1).Required()
		gt.Value(t, configs[0].MemoConfig).Nil()
	})

	t.Run("invalid memo field type fails validation", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "risk.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[memo]
description = "x"

[[memo.fields]]
id = "bad"
name = "Bad"
type = "not_a_type"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		_, err = config.LoadWorkspaceConfigs([]string{configPath})
		gt.Error(t, err)
	})

	t.Run("duplicate memo field id fails validation", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "risk.toml")
		content := `
[workspace]
id = "risk"
name = "Risk Management"

[memo]
description = "x"

[[memo.fields]]
id = "dup"
name = "One"
type = "text"

[[memo.fields]]
id = "dup"
name = "Two"
type = "text"
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		gt.NoError(t, err).Required()

		_, err = config.LoadWorkspaceConfigs([]string{configPath})
		gt.Error(t, err)
	})
}

// TestFieldDefinition_Validate_CaseRef tests the per-field validation
// rules introduced with the case_ref / multi_case_ref types.
func TestFieldDefinition_Validate_CaseRef(t *testing.T) {
	t.Run("case_ref without reference_workspace is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "linked"
name = "Linked Case"
type = "case_ref"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrMissingReferenceWorkspace)
	})

	t.Run("case_ref with reference_workspace is accepted", func(t *testing.T) {
		content := `
[[fields]]
id = "linked"
name = "Linked Case"
type = "case_ref"
reference_workspace = "other"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		schema, err := config.LoadFieldSchema(path)
		gt.NoError(t, err).Required()
		gt.Array(t, schema.Fields).Length(1).Required()
		gt.Value(t, schema.Fields[0].ReferenceWorkspace).Equal("other")
	})

	t.Run("required case_ref is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "linked"
name = "Linked Case"
type = "case_ref"
required = true
reference_workspace = "other"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Error(t, err).Is(config.ErrRequiredCaseRefUnsupported)
	})

	t.Run("required multi_case_ref is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "links"
name = "Linked Cases"
type = "multi_case_ref"
required = true
reference_workspace = "other"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Error(t, err).Is(config.ErrRequiredCaseRefUnsupported)
	})

	t.Run("multi_case_ref without reference_workspace is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "links"
name = "Linked Cases"
type = "multi_case_ref"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrMissingReferenceWorkspace)
	})

	t.Run("text field with reference_workspace is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "summary"
name = "Summary"
type = "text"
reference_workspace = "other"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrUnexpectedReferenceWorkspace)
	})

	t.Run("select field with reference_workspace is rejected", func(t *testing.T) {
		content := `
[[fields]]
id = "sev"
name = "Severity"
type = "select"
reference_workspace = "x"

  [[fields.options]]
  id = "low"
  name = "Low"
`
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "ws.toml")
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()

		_, err := config.LoadFieldSchema(path)
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrUnexpectedReferenceWorkspace)
	})
}

// TestLoadWorkspaceConfigs_CaseRef tests the cross-workspace validation
// of case_ref fields (reference_workspace must name a loaded workspace).
func TestLoadWorkspaceConfigs_CaseRef(t *testing.T) {
	writeToml := func(t *testing.T, dir, name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		gt.NoError(t, os.WriteFile(path, []byte(content), 0644)).Required()
		return path
	}

	wsA := func(refWorkspace string) string {
		return `
[workspace]
id = "ws-a"
name = "Workspace A"

[[fields]]
id = "ref"
name = "Ref"
type = "case_ref"
reference_workspace = "` + refWorkspace + `"
`
	}

	wsB := `
[workspace]
id = "ws-b"
name = "Workspace B"

[[fields]]
id = "title"
name = "Title"
type = "text"
`

	t.Run("cross-workspace reference to existing workspace is accepted", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeToml(t, tmpDir, "a.toml", wsA("ws-b"))
		writeToml(t, tmpDir, "b.toml", wsB)

		configs, err := config.LoadWorkspaceConfigs([]string{tmpDir})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(2)
	})

	t.Run("self-reference (pointing at own workspace) is accepted", func(t *testing.T) {
		tmpDir := t.TempDir()
		// ws-a references itself
		writeToml(t, tmpDir, "a.toml", wsA("ws-a"))

		configs, err := config.LoadWorkspaceConfigs([]string{tmpDir})
		gt.NoError(t, err).Required()
		gt.Array(t, configs).Length(1)
		gt.Value(t, configs[0].ID).Equal("ws-a")
	})

	t.Run("reference to unknown workspace is rejected", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeToml(t, tmpDir, "a.toml", wsA("ws-missing"))

		_, err := config.LoadWorkspaceConfigs([]string{tmpDir})
		gt.Value(t, err).NotNil()
		gt.Error(t, err).Is(config.ErrUnknownReferenceWorkspace)
	})
}

// TestLoadWorkspaceConfigs_CaseRefInMemo tests that case_ref
// fields inside [memo] are rejected, because memo fields are not wired to the
// picker, agent tools, or existence / privacy verification for Cases.
func TestLoadWorkspaceConfigs_CaseRefInMemo(t *testing.T) {
	content := `
[workspace]
id = "risk"
name = "Risk"

[memo]
description = "Memo"

[[memo.fields]]
id = "linked"
name = "Linked Case"
type = "case_ref"
reference_workspace = "risk"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "risk.toml")
	gt.NoError(t, os.WriteFile(configPath, []byte(content), 0644)).Required()

	_, err := config.LoadWorkspaceConfigs([]string{configPath})
	gt.Value(t, err).NotNil()
	gt.Error(t, err).Is(config.ErrUnexpectedReferenceWorkspace)
}
