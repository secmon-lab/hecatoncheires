package model

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// FieldValue represents a single custom field value embedded in a Case document.
// Each value carries its own Type for self-describing data.
type FieldValue struct {
	FieldID types.FieldID   // References FieldDefinition.ID from configuration
	Type    types.FieldType // Field type (self-descriptive)
	Value   any             // Actual value (Go type depends on field type)
}

// IsValueInSet checks whether the field value is contained in the given valid set.
// For select fields, the value must be a string present in validSet.
// For multi-select fields, all elements must be strings present in validSet.
// For other field types, it always returns true.
func (fv FieldValue) IsValueInSet(fieldType types.FieldType, validSet map[string]bool) bool {
	switch fieldType {
	case types.FieldTypeSelect:
		s, ok := fv.Value.(string)
		if !ok {
			return false
		}
		return validSet[s]

	case types.FieldTypeMultiSelect:
		switch v := fv.Value.(type) {
		case []string:
			for _, s := range v {
				if !validSet[s] {
					return false
				}
			}
			return true
		case []interface{}:
			for _, elem := range v {
				s, ok := elem.(string)
				if !ok {
					return false
				}
				if !validSet[s] {
					return false
				}
			}
			return true
		default:
			return false
		}

	default:
		return true
	}
}
