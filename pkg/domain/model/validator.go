package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// ValidateCaseFields validates field values for a Case and returns enriched values with Type injected from config.
// The input map is not modified; a new map is returned with Type set on each FieldValue.
func (v *FieldValidator) ValidateCaseFields(fieldValues map[string]FieldValue) (map[string]FieldValue, error) {
	return v.validateCaseFields(fieldValues, false)
}

// ValidateCaseFieldsPartial is the partial-update variant: only the supplied
// field values are type-checked, and missing required fields do NOT fail.
// Use this for UpdateCase where the caller may submit a subset of fields and
// the remaining values are preserved untouched on the existing Case.
func (v *FieldValidator) ValidateCaseFieldsPartial(fieldValues map[string]FieldValue) (map[string]FieldValue, error) {
	return v.validateCaseFields(fieldValues, true)
}

// ValidateCaseFieldsAll is the full-validation variant that does NOT fail
// fast. It walks every supplied value and every required field, accumulating
// all violations, and returns them as a single ErrCaseFieldValidation whose
// message lists every violation (one per line). Unlike the fail-fast
// ValidateCaseFields, an unknown field id is reported as a violation (not
// silently preserved) so the thread-mode create agent learns it referenced a
// field that does not exist. On success it returns the enriched values (Type
// injected) and a nil error.
func (v *FieldValidator) ValidateCaseFieldsAll(fieldValues map[string]FieldValue) (map[string]FieldValue, error) {
	return v.validateCaseFieldsStrict(fieldValues, true)
}

// ValidateCaseFieldsPartialStrict is the agent-and-API boundary variant: like
// ValidateCaseFieldsPartial it does NOT require missing required fields (a
// partial update need not resubmit every field), but — unlike the lenient
// partial variant — an unknown field id is reported as a violation rather than
// silently preserved, and every violation is accumulated into a single
// ErrCaseFieldValidation so the caller can fix them in one shot. Use this for
// UpdateCase / MaterializeThreadCase where the submitted values come from an
// untrusted source (LLM / API client) that must not write a field id the
// workspace does not define.
func (v *FieldValidator) ValidateCaseFieldsPartialStrict(fieldValues map[string]FieldValue) (map[string]FieldValue, error) {
	return v.validateCaseFieldsStrict(fieldValues, false)
}

// validateCaseFieldsStrict is the shared accumulate-all-violations core behind
// ValidateCaseFieldsAll (requireRequired=true) and ValidateCaseFieldsPartialStrict
// (requireRequired=false). Both reject unknown field ids; they differ only in
// whether a missing required field is a violation.
func (v *FieldValidator) validateCaseFieldsStrict(fieldValues map[string]FieldValue, requireRequired bool) (map[string]FieldValue, error) {
	fieldDefMap := make(map[string]config.FieldDefinition)
	for _, fd := range v.schema.Fields {
		fieldDefMap[fd.ID] = fd
	}

	result := make(map[string]FieldValue, len(fieldValues))
	var violations []string

	for fieldID, fv := range fieldValues {
		fieldDef, ok := fieldDefMap[fieldID]
		if !ok {
			violations = append(violations,
				fmt.Sprintf("field %q: not defined in the workspace schema", fieldID))
			continue
		}
		fv.Type = fieldDef.Type
		result[fieldID] = fv
		if err := v.validateFieldValue(fieldDef, fv); err != nil {
			violations = append(violations, fmt.Sprintf("field %q: %s", fieldID, err.Error()))
		}
	}

	if requireRequired {
		for _, fieldDef := range v.schema.Fields {
			if fieldDef.Required {
				if _, ok := result[fieldDef.ID]; !ok {
					violations = append(violations,
						fmt.Sprintf("field %q: required but missing", fieldDef.ID))
				}
			}
		}
	}

	if len(violations) > 0 {
		return nil, goerr.Wrap(ErrCaseFieldValidation,
			"case field validation failed:\n- "+strings.Join(violations, "\n- "),
			goerr.V("violations", violations))
	}
	return result, nil
}

func (v *FieldValidator) validateCaseFields(fieldValues map[string]FieldValue, partial bool) (map[string]FieldValue, error) {
	// Build a map of field definitions by ID for quick lookup
	fieldDefMap := make(map[string]config.FieldDefinition)
	for _, fd := range v.schema.Fields {
		fieldDefMap[fd.ID] = fd
	}

	result := make(map[string]FieldValue, len(fieldValues))

	// Validate each field value and inject Type into result
	for fieldID, fv := range fieldValues {
		// Check if field ID is defined in schema
		fieldDef, ok := fieldDefMap[fieldID]
		if !ok {
			// Preserve unknown fields (for forward compatibility when field is removed from schema)
			result[fieldID] = fv
			continue
		}

		// Inject Type from config into new value
		fv.Type = fieldDef.Type
		result[fieldID] = fv

		// Validate field value type and constraints
		if err := v.validateFieldValue(fieldDef, fv); err != nil {
			return nil, goerr.Wrap(err, "field validation failed",
				goerr.V(FieldIDKey, fieldID))
		}
	}

	if partial {
		return result, nil
	}

	// Check for missing required fields
	for _, fieldDef := range v.schema.Fields {
		if fieldDef.Required {
			if _, ok := result[fieldDef.ID]; !ok {
				return nil, goerr.Wrap(ErrMissingRequired, "required field not provided",
					goerr.V(FieldIDKey, fieldDef.ID))
			}
		}
	}

	return result, nil
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
	case types.FieldTypeCaseRef:
		return v.validateCaseRef(fieldDef, fv)
	case types.FieldTypeMultiCaseRef:
		return v.validateMultiCaseRef(fieldDef, fv)
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
	switch x := fv.Value.(type) {
	case float64, int, int64, int32:
		return nil
	case json.Number:
		// gqlgen feeds Any-typed numeric inputs as json.Number; accept them
		// after confirming they parse.
		if _, err := x.Float64(); err == nil {
			return nil
		}
		return goerr.Wrap(ErrInvalidFieldType, "value must be number",
			goerr.V(ExpectedTypeKey, types.FieldTypeNumber),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
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

// validateCaseRef validates a single case_ref field value. It only
// checks the shape (a string parseable as a Case ID); existence, privacy and
// draft checks require I/O and are performed in the usecase layer
// (verifyCaseRefsExist), mirroring how user / multi-user existence is
// verified outside this pure validator.
func (v *FieldValidator) validateCaseRef(fieldDef config.FieldDefinition, fv FieldValue) error {
	id, ok := fv.Value.(string)
	if !ok {
		return goerr.Wrap(ErrInvalidFieldType, "value must be string (case ID)",
			goerr.V(ExpectedTypeKey, types.FieldTypeCaseRef),
			goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return goerr.Wrap(ErrInvalidFieldType, "case reference must be a numeric case ID",
			goerr.V(FieldValueKey, id))
	}
	return nil
}

// validateMultiCaseRef validates a multi_case_ref field value: an
// array of strings each parseable as a Case ID. As with validateCaseRef,
// existence / privacy / draft checks are deferred to the usecase layer.
func (v *FieldValidator) validateMultiCaseRef(fieldDef config.FieldDefinition, fv FieldValue) error {
	ids, ok := fv.Value.([]string)
	if !ok {
		if values, ok := fv.Value.([]interface{}); ok {
			ids = make([]string, len(values))
			for i, val := range values {
				strVal, ok := val.(string)
				if !ok {
					return goerr.Wrap(ErrInvalidFieldType, "multi_case_ref value must be array of strings",
						goerr.V(ExpectedTypeKey, types.FieldTypeMultiCaseRef),
						goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
				}
				ids[i] = strVal
			}
		} else {
			return goerr.Wrap(ErrInvalidFieldType, "value must be array of strings (case IDs)",
				goerr.V(ExpectedTypeKey, types.FieldTypeMultiCaseRef),
				goerr.V(ActualTypeKey, fmt.Sprintf("%T", fv.Value)))
		}
	}
	for _, id := range ids {
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return goerr.Wrap(ErrInvalidFieldType, "case reference must be a numeric case ID",
				goerr.V(FieldValueKey, id))
		}
	}
	return nil
}
