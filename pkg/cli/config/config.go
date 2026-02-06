package config

import (
	"os"
	"regexp"

	"github.com/m-mizutani/goerr/v2"
	"github.com/pelletier/go-toml/v2"
	domainConfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

var fieldIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// AppConfig represents the application configuration
type AppConfig struct {
	Labels Labels            `toml:"labels"`
	Fields []FieldDefinition `toml:"fields"`
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
	Color       string         `toml:"color"`
	Metadata    map[string]any `toml:"metadata"`
}

// Validate checks if the FieldOption is valid
func (o *FieldOption) Validate(fieldID string) error {
	if !fieldIDPattern.MatchString(o.ID) {
		return goerr.Wrap(ErrInvalidFieldID, "option ID must match pattern ^[a-z0-9]+(-[a-z0-9]+)*$",
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
		return goerr.Wrap(ErrInvalidFieldID, "field ID must match pattern ^[a-z0-9]+(-[a-z0-9]+)*$",
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

	return nil
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

// ToDomainFieldSchema converts AppConfig to domain FieldSchema
func (a *AppConfig) ToDomainFieldSchema() *domainConfig.FieldSchema {
	fields := make([]domainConfig.FieldDefinition, len(a.Fields))
	for i, field := range a.Fields {
		options := make([]domainConfig.FieldOption, len(field.Options))
		for j, opt := range field.Options {
			options[j] = domainConfig.FieldOption{
				ID:          opt.ID,
				Name:        opt.Name,
				Description: opt.Description,
				Color:       opt.Color,
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

	labels := domainConfig.EntityLabels{
		Case: a.Labels.Case,
	}
	// Set default labels if not specified
	if labels.Case == "" {
		labels.Case = "Case"
	}

	return &domainConfig.FieldSchema{
		Fields: fields,
		Labels: labels,
	}
}
