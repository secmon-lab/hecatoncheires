package config

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// FieldOption represents an option for select/multi-select fields
type FieldOption struct {
	ID          string
	Name        string
	Description string
	Color       string         // Optional: hex color code
	Metadata    map[string]any // Optional: arbitrary metadata (e.g., {"score": 4})
}

// FieldDefinition defines a custom field's schema
type FieldDefinition struct {
	ID          string
	Name        string
	Type        types.FieldType
	Required    bool
	Description string
	Options     []FieldOption // Only used for select and multi-select types
}

// EntityLabels holds display labels for entities
type EntityLabels struct {
	Case string // Default: "Case"
}

// FieldSchema holds the complete field configuration
type FieldSchema struct {
	Fields []FieldDefinition
	Labels EntityLabels
}
