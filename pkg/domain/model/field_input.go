package model

import (
	"strconv"
	"strings"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// FieldInput is the LLM-facing shape of one custom-field assignment: a field
// id plus either a scalar value or a list of values. It is the agent-boundary
// form that planners / tools emit before the value is coerced into a typed
// FieldValue.
type FieldInput struct {
	// FieldID references FieldDefinition.ID from the workspace schema.
	FieldID string
	// Value carries the scalar form (text / number / url / date / single
	// select option id / single user id).
	Value string
	// Values carries the multi form (multi-select option ids / multi-user ids).
	Values []string
}

// CoerceFieldInputs maps FieldInput entries into FieldValue keyed by field id,
// resolving each field's storage shape from the schema (multi -> []string,
// number -> float64). It performs TYPE COERCION ONLY, not business validation:
// an unknown field id is passed through with its raw value and Type unset so
// the downstream validator (ValidateCaseFieldsPartialStrict / ...All) reports
// the schema violation with a single consistent message. An unparseable number
// is the one shape the coercion cannot represent, so it is returned as a
// violation string (rather than written as a string the storage layer would
// reject) giving the caller a precise "must be a number" hint.
//
// Empty scalar values are intentionally PRESERVED (as an explicit "set to
// empty") — callers that must instead drop empties (e.g. the materialize path
// guarding against clobbering an existing value) do their own filtering and do
// not route through this helper.
func CoerceFieldInputs(schema *config.FieldSchema, in []FieldInput) (map[string]FieldValue, []string) {
	out := make(map[string]FieldValue, len(in))
	if len(in) == 0 {
		return out, nil
	}

	typeByID := make(map[string]types.FieldType)
	if schema != nil {
		for _, f := range schema.Fields {
			typeByID[f.ID] = f.Type
		}
	}

	var violations []string
	for _, fi := range in {
		ft, known := typeByID[fi.FieldID]
		if !known {
			// Pass the raw value through with Type unset; the downstream
			// validator owns the "not defined in the workspace schema" report
			// so the message stays consistent across callers.
			out[fi.FieldID] = FieldValue{FieldID: types.FieldID(fi.FieldID), Value: firstNonEmptyValue(fi.Value, fi.Values)}
			continue
		}

		var val any
		switch ft {
		case types.FieldTypeMultiSelect, types.FieldTypeMultiUser, types.FieldTypeMultiCaseRef:
			switch {
			case len(fi.Values) > 0:
				val = fi.Values
			case fi.Value != "":
				val = []string{fi.Value}
			default:
				val = []string{}
			}
		case types.FieldTypeNumber:
			trimmed := strings.TrimSpace(fi.Value)
			if trimmed == "" {
				// An optional number left empty by the caller: drop it rather
				// than emit a spurious "not a number" violation. A missing
				// REQUIRED number is still caught by the downstream validator.
				continue
			}
			n, err := strconv.ParseFloat(trimmed, 64)
			if err != nil {
				violations = append(violations,
					"field "+strconv.Quote(fi.FieldID)+": value must be a number, got "+strconv.Quote(fi.Value))
				continue
			}
			val = n
		default:
			val = fi.Value
		}
		out[fi.FieldID] = FieldValue{FieldID: types.FieldID(fi.FieldID), Type: ft, Value: val}
	}
	return out, violations
}

// firstNonEmptyValue returns the scalar when present, else the slice — used
// only to preserve an unknown field's raw value for the validator's error
// message.
func firstNonEmptyValue(scalar string, slice []string) any {
	if scalar != "" {
		return scalar
	}
	if len(slice) > 0 {
		return slice
	}
	return ""
}
