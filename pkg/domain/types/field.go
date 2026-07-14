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
	// FieldTypeCaseRef references a single Case by its numeric ID in the
	// workspace named by the field definition's ReferenceWorkspace. The stored
	// value is the referenced Case ID as a string (mirrors select / user).
	FieldTypeCaseRef FieldType = "case_ref"
	// FieldTypeMultiCaseRef references multiple Cases by ID. The stored
	// value is a []string of Case IDs (mirrors multi-select / multi-user).
	FieldTypeMultiCaseRef FieldType = "multi_case_ref"
	// FieldTypeMarkdown holds Markdown-formatted text. The stored value is a
	// plain string (same shape as text); the Web UI renders it as Markdown.
	FieldTypeMarkdown FieldType = "markdown"
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
		FieldTypeCaseRef,
		FieldTypeMultiCaseRef,
		FieldTypeMarkdown,
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
		FieldTypeURL,
		FieldTypeCaseRef,
		FieldTypeMultiCaseRef,
		FieldTypeMarkdown:
		return true
	default:
		return false
	}
}

// IsCaseRef reports whether the type references Cases (single or multi).
func (t FieldType) IsCaseRef() bool {
	return t == FieldTypeCaseRef || t == FieldTypeMultiCaseRef
}

// String returns the string representation of the field type
func (t FieldType) String() string {
	return string(t)
}
