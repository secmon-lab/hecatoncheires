package graphql_test

import (
	"encoding/json"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
)

func TestUnmarshalAny_PromotesJSONNumber(t *testing.T) {
	t.Run("integer json.Number → int64", func(t *testing.T) {
		v, err := graphql.UnmarshalAny(json.Number("42"))
		gt.NoError(t, err)
		gt.Value(t, v).Equal(int64(42))
	})

	t.Run("float json.Number → float64", func(t *testing.T) {
		v, err := graphql.UnmarshalAny(json.Number("3.14"))
		gt.NoError(t, err)
		gt.Value(t, v).Equal(3.14)
	})

	t.Run("nested array preserves promotion", func(t *testing.T) {
		v, err := graphql.UnmarshalAny([]any{json.Number("1"), json.Number("2.5"), "x"})
		gt.NoError(t, err)
		got, ok := v.([]any)
		gt.B(t, ok).True()
		gt.Value(t, got[0]).Equal(int64(1))
		gt.Value(t, got[1]).Equal(2.5)
		gt.Value(t, got[2]).Equal("x")
	})

	t.Run("nested map preserves promotion", func(t *testing.T) {
		v, err := graphql.UnmarshalAny(map[string]any{"n": json.Number("7")})
		gt.NoError(t, err)
		m, ok := v.(map[string]any)
		gt.B(t, ok).True()
		gt.Value(t, m["n"]).Equal(int64(7))
	})

	t.Run("non-numeric values pass through", func(t *testing.T) {
		v, err := graphql.UnmarshalAny("hello")
		gt.NoError(t, err)
		gt.Value(t, v).Equal("hello")

		v2, err := graphql.UnmarshalAny(true)
		gt.NoError(t, err)
		gt.Value(t, v2).Equal(true)
	})
}
