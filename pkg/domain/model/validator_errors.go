package model

import "github.com/m-mizutani/goerr/v2"

// Validation errors
var (
	ErrInvalidFieldType = goerr.New("invalid field type")
	ErrInvalidOptionID  = goerr.New("invalid option ID")
	ErrMissingRequired  = goerr.New("required field is missing")
)

// Context keys for error values
const (
	FieldIDKey      = "field_id"
	ExpectedTypeKey = "expected_type"
	ActualTypeKey   = "actual_type"
	OptionIDKey     = "option_id"
	FieldValueKey   = "field_value"
)
