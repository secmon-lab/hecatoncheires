package model

import (
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// FieldValidator validates field values against field schema
type FieldValidator struct {
	schema *config.FieldSchema
}

// NewFieldValidator creates a new FieldValidator with the given schema
func NewFieldValidator(schema *config.FieldSchema) *FieldValidator {
	return &FieldValidator{
		schema: schema,
	}
}

// ValidateCaseFields validates field values for a Case
// Returns an error if any field value is invalid
func (v *FieldValidator) ValidateCaseFields(fieldValues []FieldValue) error {
	// Build a map of field definitions by ID for quick lookup
	fieldDefMap := make(map[string]config.FieldDefinition)
	for _, fd := range v.schema.Fields {
		fieldDefMap[fd.ID] = fd
	}

	// Track which fields have been provided
	providedFields := make(map[string]bool)

	// Validate each field value
	for _, fv := range fieldValues {
		// Check if field ID is defined in schema
		fieldDef, ok := fieldDefMap[fv.FieldID]
		if !ok {
			// Skip unknown fields (for forward compatibility when field is removed from schema)
			continue
		}

		// Mark field as provided
		providedFields[fv.FieldID] = true

		// Validate field value type and constraints
		if err := v.validateFieldValue(fieldDef, fv); err != nil {
			return goerr.Wrap(err, "field validation failed",
				goerr.V(FieldIDKey, fv.FieldID))
		}
	}

	// Check for missing required fields
	for _, fieldDef := range v.schema.Fields {
		if fieldDef.Required && !providedFields[fieldDef.ID] {
			return goerr.Wrap(ErrMissingRequired, "required field not provided",
				goerr.V(FieldIDKey, fieldDef.ID))
		}
	}

	return nil
}

// validateFieldValue validates a single field value against its definition
func (v *FieldValidator) validateFieldValue(fieldDef config.FieldDefinition, fv FieldValue) error {
	switch fieldDef.Type {
	case types.FieldTypeText:
		return v.validateText(fieldDef, fv)
	case types.FieldTypeNumber:
		return v.validateNumber(fieldDef, fv)
	case types.FieldTypeSelect:
		return v.validateSelect(fieldDef, fv)
	case types.FieldTypeMultiSelect:
		return v.validateMultiSelect(fieldDef, fv)
	case types.FieldTypeUser:
		return v.validateUser(fieldDef, fv)
	case types.FieldTypeMultiUser:
		return v.validateMultiUser(fieldDef, fv)
	case types.FieldTypeDate:
		return v.validateDate(fieldDef, fv)
	case types.FieldTypeURL:
		return v.validateURL(fieldDef, fv)
	default:
		return goerr.Wrap(ErrInvalidFieldType, "unsupported field type",
			goerr.V(FieldIDKey, fieldDef.ID),
			goerr.V(ExpectedTypeKey, fieldDef.Type))
	}
}

// validateText validates a text field value
func (v *FieldValidator) validateText(fieldDef config.FieldDefinition, fv FieldValue) error {
	_, ok := fv.Value.(string)
	if !ok {
		return goerr.Wrap(ErrInvalidFieldType, "value must be string",
			goerr.V(ExpectedTypeKey, types.FieldTypeText),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
	return nil
}

// validateNumber validates a number field value
func (v *FieldValidator) validateNumber(fieldDef config.FieldDefinition, fv FieldValue) error {
	switch fv.Value.(type) {
	case float64, int, int64, int32:
		return nil
	default:
		return goerr.Wrap(ErrInvalidFieldType, "value must be number",
			goerr.V(ExpectedTypeKey, types.FieldTypeNumber),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
}

// validateSelect validates a select field value
func (v *FieldValidator) validateSelect(fieldDef config.FieldDefinition, fv FieldValue) error {
	optionID, ok := fv.Value.(string)
	if !ok {
		return goerr.Wrap(ErrInvalidFieldType, "value must be string (option ID)",
			goerr.V(ExpectedTypeKey, types.FieldTypeSelect),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}

	// Check if option ID exists in field definition
	validOptionID := false
	for _, opt := range fieldDef.Options {
		if opt.ID == optionID {
			validOptionID = true
			break
		}
	}

	if !validOptionID {
		return goerr.Wrap(ErrInvalidOptionID, "option ID not found in field definition",
			goerr.V(OptionIDKey, optionID),
			goerr.V(FieldIDKey, fieldDef.ID))
	}

	return nil
}

// validateMultiSelect validates a multi-select field value
func (v *FieldValidator) validateMultiSelect(fieldDef config.FieldDefinition, fv FieldValue) error {
	optionIDs, ok := fv.Value.([]string)
	if !ok {
		// Try to convert []interface{} to []string
		if values, ok := fv.Value.([]interface{}); ok {
			optionIDs = make([]string, len(values))
			for i, val := range values {
				strVal, ok := val.(string)
				if !ok {
					return goerr.Wrap(ErrInvalidFieldType, "multi-select value must be array of strings",
						goerr.V(ExpectedTypeKey, types.FieldTypeMultiSelect),
						goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
				}
				optionIDs[i] = strVal
			}
		} else {
			return goerr.Wrap(ErrInvalidFieldType, "value must be array of strings (option IDs)",
				goerr.V(ExpectedTypeKey, types.FieldTypeMultiSelect),
				goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
		}
	}

	// Build valid option ID map
	validOptions := make(map[string]bool)
	for _, opt := range fieldDef.Options {
		validOptions[opt.ID] = true
	}

	// Check each option ID
	for _, optionID := range optionIDs {
		if !validOptions[optionID] {
			return goerr.Wrap(ErrInvalidOptionID, "option ID not found in field definition",
				goerr.V(OptionIDKey, optionID),
				goerr.V(FieldIDKey, fieldDef.ID))
		}
	}

	return nil
}

// validateUser validates a user field value
func (v *FieldValidator) validateUser(fieldDef config.FieldDefinition, fv FieldValue) error {
	_, ok := fv.Value.(string)
	if !ok {
		return goerr.Wrap(ErrInvalidFieldType, "value must be string (user ID)",
			goerr.V(ExpectedTypeKey, types.FieldTypeUser),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
	return nil
}

// validateMultiUser validates a multi-user field value
func (v *FieldValidator) validateMultiUser(fieldDef config.FieldDefinition, fv FieldValue) error {
	_, ok := fv.Value.([]string)
	if !ok {
		// Try to convert []interface{} to []string
		if values, ok := fv.Value.([]interface{}); ok {
			for _, val := range values {
				if _, ok := val.(string); !ok {
					return goerr.Wrap(ErrInvalidFieldType, "multi-user value must be array of strings",
						goerr.V(ExpectedTypeKey, types.FieldTypeMultiUser),
						goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
				}
			}
		} else {
			return goerr.Wrap(ErrInvalidFieldType, "value must be array of strings (user IDs)",
				goerr.V(ExpectedTypeKey, types.FieldTypeMultiUser),
				goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
		}
	}
	return nil
}

// validateDate validates a date field value
func (v *FieldValidator) validateDate(fieldDef config.FieldDefinition, fv FieldValue) error {
	// Date can be stored as string (RFC3339) or time.Time
	switch val := fv.Value.(type) {
	case string:
		// Validate RFC3339 format
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return goerr.Wrap(ErrInvalidFieldType, "date value must be RFC3339 format string",
				goerr.V(FieldValueKey, val))
		}
		return nil
	case time.Time:
		return nil
	default:
		return goerr.Wrap(ErrInvalidFieldType, "value must be RFC3339 string or time.Time",
			goerr.V(ExpectedTypeKey, types.FieldTypeDate),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
}

// validateURL validates a URL field value
func (v *FieldValidator) validateURL(fieldDef config.FieldDefinition, fv FieldValue) error {
	_, ok := fv.Value.(string)
	if !ok {
		return goerr.Wrap(ErrInvalidFieldType, "value must be string (URL)",
			goerr.V(ExpectedTypeKey, types.FieldTypeURL),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
	// Note: We don't validate URL format here, just check it's a string
	// URL format validation can be added in the future if needed
	return nil
}
