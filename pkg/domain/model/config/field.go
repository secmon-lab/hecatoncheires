package config

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// FieldOption represents an option for select/multi-select fields
type FieldOption struct {
	ID          string
	Name        string
	Description string
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
	// ReferenceWorkspace is the workspace ID whose Cases this field may
	// reference. Required (and only meaningful) for case_ref /
	// multi_case_ref types; empty for all other types. May point at the
	// field's own workspace (self-reference is allowed).
	ReferenceWorkspace string
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
