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
