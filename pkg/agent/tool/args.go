package tool

import "fmt"

// ExtractInt64 extracts an int64 value from a tool's args map, accepting int,
// int64, or float64. gollem decodes JSON numbers as float64, so this helper
// normalises the lookup across providers that may return int / int64 directly.
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
	default:
		return 0, fmt.Errorf("%s must be an integer, got %T", key, v)
	}
}
