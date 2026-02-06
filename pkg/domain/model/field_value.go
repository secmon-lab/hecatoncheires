package model

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// FieldValue represents a single custom field value embedded in a Case document.
// Each value carries its own Type for self-describing data.
type FieldValue struct {
	FieldID types.FieldID   // References FieldDefinition.ID from configuration
	Type    types.FieldType // Field type (self-descriptive)
	Value   any             // Actual value (Go type depends on field type)
}
