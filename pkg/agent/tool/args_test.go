package tool_test

import (
	"encoding/json"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

func TestExtractInt64(t *testing.T) {
	for name, v := range map[string]any{
		"int":         7,
		"int64":       int64(7),
		"float64":     float64(7),
		"json.Number": json.Number("7"),
	} {
		t.Run("accepts "+name, func(t *testing.T) {
			n, err := tool.ExtractInt64(map[string]any{"case_id": v}, "case_id")
			gt.NoError(t, err).Required()
			gt.Number(t, n).Equal(7)
		})
	}

	// gollem keeps a literal a float64 cannot hold exactly as a json.Number, so
	// an id beyond 2^53 arrives with its exact value rather than rounded.
	t.Run("a json.Number wider than a float64 keeps its exact value", func(t *testing.T) {
		n, err := tool.ExtractInt64(map[string]any{"case_id": json.Number("9007199254740993")}, "case_id")
		gt.NoError(t, err).Required()
		gt.Number(t, n).Equal(9007199254740993)
	})

	t.Run("a float64 is truncated toward zero", func(t *testing.T) {
		n, err := tool.ExtractInt64(map[string]any{"limit": 3.7}, "limit")
		gt.NoError(t, err).Required()
		gt.Number(t, n).Equal(3)
	})

	t.Run("a json.Number outside int64 range is rejected", func(t *testing.T) {
		_, err := tool.ExtractInt64(map[string]any{"case_id": json.Number("99999999999999999999")}, "case_id")
		gt.Value(t, err).NotNil()
	})

	t.Run("a missing or nil key is rejected", func(t *testing.T) {
		_, err := tool.ExtractInt64(map[string]any{}, "case_id")
		gt.Value(t, err).NotNil()

		_, err = tool.ExtractInt64(map[string]any{"case_id": nil}, "case_id")
		gt.Value(t, err).NotNil()
	})

	t.Run("a non-numeric value is rejected", func(t *testing.T) {
		for _, v := range []any{"12", true, []any{1}} {
			_, err := tool.ExtractInt64(map[string]any{"case_id": v}, "case_id")
			gt.Value(t, err).NotNil()
		}
	})
}
