package graphql

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// MarshalTime serializes time.Time to RFC3339 string
func MarshalTime(t time.Time) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		data, _ := json.Marshal(t.Format(time.RFC3339))
		_, _ = w.Write(data)
	})
}

// UnmarshalTime deserializes RFC3339 string to time.Time
func UnmarshalTime(v interface{}) (time.Time, error) {
	if str, ok := v.(string); ok {
		return time.Parse(time.RFC3339, str)
	}
	return time.Time{}, fmt.Errorf("time must be a string")
}

// MarshalJSON serializes map[string]any to JSON
func MarshalJSON(v map[string]any) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		data, _ := json.Marshal(v)
		_, _ = w.Write(data)
	})
}

// UnmarshalJSON deserializes JSON to map[string]any
func UnmarshalJSON(v interface{}) (map[string]any, error) {
	switch val := v.(type) {
	case map[string]any:
		return val, nil
	case string:
		var result map[string]any
		if err := json.Unmarshal([]byte(val), &result); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("JSON must be an object or string, got %T", v)
	}
}

// MarshalAny serializes any value to JSON
func MarshalAny(v any) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		data, _ := json.Marshal(v)
		_, _ = w.Write(data)
	})
}

// UnmarshalAny deserializes any JSON value. gqlgen feeds numeric inputs as
// json.Number; downstream validators expect concrete numeric types, so we
// promote json.Number to float64 (or int64 when integral) here. Recursive
// for nested arrays/objects.
func UnmarshalAny(v interface{}) (any, error) {
	return convertJSONNumber(v), nil
}

func convertJSONNumber(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convertJSONNumber(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = convertJSONNumber(e)
		}
		return out
	default:
		return v
	}
}
