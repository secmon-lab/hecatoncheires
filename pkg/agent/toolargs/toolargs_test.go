package toolargs_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/toolargs"
)

// memoLikeTool mirrors the shape that produced the production failure: three
// array-typed top-level parameters, one of which nests objects holding a further
// array of objects holding an array of strings.
type memoLikeTool struct{}

func (memoLikeTool) Spec() gollem.ToolSpec {
	fields := &gollem.Parameter{
		Type: gollem.TypeArray,
		Items: &gollem.Parameter{
			Type: gollem.TypeObject,
			Properties: map[string]*gollem.Parameter{
				"field_id": {Type: gollem.TypeString},
				"value":    {Type: gollem.TypeString},
				"values":   {Type: gollem.TypeArray, Items: &gollem.Parameter{Type: gollem.TypeString}},
			},
		},
	}
	return gollem.ToolSpec{
		Name: "memo__apply_memo_changes",
		Parameters: map[string]*gollem.Parameter{
			"creates": {
				Type: gollem.TypeArray,
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"title":  {Type: gollem.TypeString},
						"fields": fields,
					},
				},
			},
			"archives": {Type: gollem.TypeArray, Items: &gollem.Parameter{Type: gollem.TypeString}},
			"note":     {Type: gollem.TypeString},
		},
	}
}

func (memoLikeTool) Run(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func tools() []gollem.Tool { return []gollem.Tool{nil, memoLikeTool{}} }

func TestCoerce(t *testing.T) {
	spec := memoLikeTool{}.Spec()

	t.Run("wraps a single value sent for an array argument", func(t *testing.T) {
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"archives": "memo-1"},
		}

		got := toolargs.Coerce(tools(), call)

		gt.NoError(t, spec.ValidateArgs(got.Arguments))
		gt.Value(t, got.Arguments["archives"]).Equal([]any{"memo-1"})
	})

	t.Run("wraps a single object sent for an array of objects", func(t *testing.T) {
		entry := map[string]any{"title": "Observed beaconing"}
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"creates": entry},
		}

		got := toolargs.Coerce(tools(), call)

		gt.NoError(t, spec.ValidateArgs(got.Arguments))
		gt.Value(t, got.Arguments["creates"]).Equal([]any{entry})
	})

	t.Run("wraps a nested array argument inside an array item", func(t *testing.T) {
		call := gollem.FunctionCall{
			Name: "memo__apply_memo_changes",
			Arguments: map[string]any{
				"creates": []any{
					map[string]any{
						"title": "Observed beaconing",
						"fields": map[string]any{
							"field_id": "severity",
							"values":   "high",
						},
					},
				},
			},
		}

		got := toolargs.Coerce(tools(), call)

		gt.NoError(t, spec.ValidateArgs(got.Arguments))
		creates := gt.Cast[[]any](t, got.Arguments["creates"])
		gt.Array(t, creates).Length(1).Required()
		create := gt.Cast[map[string]any](t, creates[0])
		gt.Value(t, create["title"]).Equal("Observed beaconing")
		fields := gt.Cast[[]any](t, create["fields"])
		gt.Array(t, fields).Length(1).Required()
		field := gt.Cast[map[string]any](t, fields[0])
		gt.Value(t, field["field_id"]).Equal("severity")
		gt.Value(t, field["values"]).Equal([]any{"high"})
	})

	t.Run("leaves arguments that already match their declared type untouched", func(t *testing.T) {
		call := gollem.FunctionCall{
			Name: "memo__apply_memo_changes",
			Arguments: map[string]any{
				"archives": []any{"memo-1", "memo-2"},
				"note":     "done",
			},
		}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, got.Arguments["archives"]).Equal([]any{"memo-1", "memo-2"})
		gt.Value(t, got.Arguments["note"]).Equal("done")
	})

	t.Run("does not mutate the call it was given", func(t *testing.T) {
		args := map[string]any{"archives": "memo-1"}
		call := gollem.FunctionCall{Name: "memo__apply_memo_changes", Arguments: args}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, args["archives"]).Equal("memo-1")
		gt.Value(t, got.Arguments["archives"]).Equal([]any{"memo-1"})
	})

	t.Run("leaves a call for an unknown tool alone", func(t *testing.T) {
		call := gollem.FunctionCall{Name: "other__tool", Arguments: map[string]any{"archives": "memo-1"}}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, got.Arguments["archives"]).Equal("memo-1")
	})

	t.Run("leaves a null argument as null", func(t *testing.T) {
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"archives": nil},
		}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, got.Arguments["archives"]).Nil()
	})

	t.Run("decodes an array the model sent as a JSON string", func(t *testing.T) {
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"archives": `["memo-1","memo-2"]`},
		}

		got := toolargs.Coerce(tools(), call)

		gt.NoError(t, spec.ValidateArgs(got.Arguments))
		gt.Value(t, got.Arguments["archives"]).Equal([]any{"memo-1", "memo-2"})
	})

	t.Run("leaves a mismatch it cannot read as a batch of one to be rejected", func(t *testing.T) {
		// A string where an object is declared is not a batch of one, so it is
		// passed through and gollem still rejects it — the model is told what it
		// sent by toolArgsFeedbackMiddleware instead of having it guessed at.
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"creates": []any{"Observed beaconing"}},
		}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, got.Arguments["creates"]).Equal([]any{"Observed beaconing"})
		gt.Error(t, spec.ValidateArgs(got.Arguments)).Is(gollem.ErrToolArgsValidation)
	})

	t.Run("keeps the arguments as sent when wrapping does not make the call valid", func(t *testing.T) {
		// [42] is an array, but its item is still not the declared string. The
		// call is rejected either way, and the shape reported back to the model
		// has to be the one it sent — an argument it never wrote would send it
		// looking for a mistake it did not make.
		call := gollem.FunctionCall{
			Name:      "memo__apply_memo_changes",
			Arguments: map[string]any{"archives": float64(42)},
		}

		got := toolargs.Coerce(tools(), call)

		gt.Value(t, got.Arguments["archives"]).Equal(float64(42))
		gt.Error(t, spec.ValidateArgs(got.Arguments)).Is(gollem.ErrToolArgsValidation)
	})
}
