package model

import "time"

// FieldValue represents a single custom field value associated with a Case
// Field values are stored in a separate Firestore collection (case_field_values)
// rather than being embedded in the parent Case document
type FieldValue struct {
	CaseID    int64  // Case ID (custom fields are only for Cases, not Actions)
	FieldID   string // References FieldDefinition.ID from configuration
	Value     any    // Actual value (Go type depends on field type)
	UpdatedAt time.Time
}
