package types

// FieldID represents the unique identifier for a custom field
type FieldID string

// FieldType represents the type of a custom field
type FieldType string

const (
	FieldTypeText        FieldType = "text"
	FieldTypeNumber      FieldType = "number"
	FieldTypeSelect      FieldType = "select"
	FieldTypeMultiSelect FieldType = "multi-select"
	FieldTypeUser        FieldType = "user"
	FieldTypeMultiUser   FieldType = "multi-user"
	FieldTypeDate        FieldType = "date"
	FieldTypeURL         FieldType = "url"
)

// AllFieldTypes returns all valid field types
func AllFieldTypes() []FieldType {
	return []FieldType{
		FieldTypeText,
		FieldTypeNumber,
		FieldTypeSelect,
		FieldTypeMultiSelect,
		FieldTypeUser,
		FieldTypeMultiUser,
		FieldTypeDate,
		FieldTypeURL,
	}
}

// IsValid checks if the field type is valid
func (t FieldType) IsValid() bool {
	switch t {
	case FieldTypeText,
		FieldTypeNumber,
		FieldTypeSelect,
		FieldTypeMultiSelect,
		FieldTypeUser,
		FieldTypeMultiUser,
		FieldTypeDate,
		FieldTypeURL:
		return true
	default:
		return false
	}
}

// String returns the string representation of the field type
func (t FieldType) String() string {
	return string(t)
}
