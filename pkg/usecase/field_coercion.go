package usecase

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// coerceFieldValue normalizes a JSON-decoded any value to the canonical Go
// representation expected for the given FieldType. Returns ok=false on any
// type mismatch (e.g., AI returned a boolean for a number field). Used by
// the slackDraftHandler when persisting the planner's materialize payload
// against a workspace's FieldSchema.
func coerceFieldValue(v any, t types.FieldType) (any, bool) {
	switch t {
	case types.FieldTypeText, types.FieldTypeURL, types.FieldTypeUser, types.FieldTypeDate, types.FieldTypeSelect:
		s, ok := v.(string)
		return s, ok
	case types.FieldTypeNumber:
		switch n := v.(type) {
		case float64:
			return n, true
		case int:
			return float64(n), true
		case int64:
			return float64(n), true
		default:
			return nil, false
		}
	case types.FieldTypeMultiSelect, types.FieldTypeMultiUser:
		arr, ok := v.([]any)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return v, true
	}
}
