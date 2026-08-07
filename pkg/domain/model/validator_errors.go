package model

import "github.com/m-mizutani/goerr/v2"

// Validation errors
var (
	ErrInvalidFieldType = goerr.New("invalid field type")
	ErrInvalidOptionID  = goerr.New("invalid option ID")
	ErrMissingRequired  = goerr.New("required field is missing")
	// ErrCaseFieldValidation wraps the aggregated, non-fail-fast validation
	// result produced by ValidateCaseFieldsAll. Its message lists every
	// violation (one per line) so the caller can feed the complete set back
	// to an LLM in a single round.
	ErrCaseFieldValidation = goerr.New("case field validation failed")
	// ErrUnknownFieldID is reported by ValidateCaseFieldsAll when a supplied
	// field id is not present in the workspace schema. (The fail-fast
	// ValidateCaseFields preserves unknown fields for forward compatibility;
	// the create path rejects them instead of silently dropping data.)
	ErrUnknownFieldID = goerr.New("unknown field ID")
	// ErrStoredFieldTypeMismatch is reported by ValidateStored when a persisted
	// FieldValue carries a Type that no longer matches the workspace schema's
	// type for that field. It is distinct from ErrInvalidFieldType because the
	// stored value's shape may still be acceptable under the new type (text and
	// markdown are both strings), so a type change like that would otherwise
	// pass unnoticed.
	ErrStoredFieldTypeMismatch = goerr.New("stored field type does not match schema type")
)

// Context keys for error values
const (
	FieldIDKey      = "field_id"
	ExpectedTypeKey = "expected_type"
	ActualTypeKey   = "actual_type"
	OptionIDKey     = "option_id"
	FieldValueKey   = "field_value"
)
