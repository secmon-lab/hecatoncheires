package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
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
  id = "data-breach"
  name = "Data Breach"
  description = "Risk of data leakage"
  color = "#E53E3E"

  [[fields.options]]
  id = "system-failure"
  name = "System Failure"
  color = "#DD6B20"

[[fields]]
id = "likelihood"
name = "Likelihood"
type = "select"
required = true
description = "Probability of occurrence"

  [[fields.options]]
  id = "very-low"
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
id = "specific-impact"
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
id = "due-date"
name = "Due Date"
type = "date"
required = false

[[fields]]
id = "reference-url"
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
			name: "invalid field ID format (underscore)",
			content: `
[[fields]]
id = "category_id"
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
  id = "option-a"
  name = "Option A"

  [[fields.options]]
  id = "option-a"
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
  id = "option-a"
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
  id = "data-breach"
  name = "Data Breach"
  color = "#E53E3E"
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
	gt.Value(t, option.ID).Equal("data-breach")
	gt.Value(t, option.Name).Equal("Data Breach")
	gt.Value(t, option.Color).Equal("#E53E3E")

	// Check metadata
	gt.Value(t, option.Metadata).NotNil().Required()
	if severity, ok := option.Metadata["severity"].(string); !ok || severity != "high" {
		t.Errorf("Option.Metadata[severity] = %v, want high", option.Metadata["severity"])
	}
	if priority, ok := option.Metadata["priority"].(int64); !ok || priority != 1 {
		t.Errorf("Option.Metadata[priority] = %v, want 1", option.Metadata["priority"])
	}
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
