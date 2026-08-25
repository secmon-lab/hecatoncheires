package tool

import (
	"encoding/json"
	"fmt"
)

// ExtractInt64 extracts an int64 value from a tool's args map, accepting int,
// int64, float64, or json.Number. gollem decodes a JSON number into a float64
// whenever that is lossless and keeps it as a json.Number otherwise (see
// gollem's internal/jsonutil), and its ToolSpec validation accepts both, so a
// tool argument wider than 53 bits reaches Run as a json.Number. int / int64
// are accepted so non-LLM callers and tests need not box every value.
//
// Returns an error when the key is missing, nil, or the value is not a number.
func ExtractInt64(args map[string]any, key string) (int64, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	case json.Number:
		// A json.Number only appears for a value a float64 cannot hold
		// exactly, so a failure here means the literal is out of int64 range
		// or is not an integer at all. Both are invalid for every argument
		// this helper reads (case / action ids, page sizes, limits), so the
		// parse error is reported rather than truncated through a float64.
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer, got %q: %w", key, n.String(), err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", key, v)
	}
}
