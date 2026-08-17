// Package toolargs repairs the one argument mistake a model cannot repair by
// itself: sending a single value where the tool spec declares an array.
//
// gollem validates a call's arguments against the ToolSpec before the tool runs
// and rejects the whole call on a mismatch ("expected array type"), so a tool
// never gets the chance to interpret what it was sent. That rejection is fed
// back to the model, but in production (memo__apply_memo_changes, ARGUS-8S) the
// model answered it by re-emitting the same call, and the writes the call
// carried were never applied while the run reported success.
//
// Two readings need no guessing: a lone value where an array is required is the
// batch of one the model meant, and a string holding a JSON array is that array
// double-encoded. Nothing else is interpreted, and the repair is kept only when
// it makes the whole call valid — otherwise the call goes to gollem exactly as
// the model sent it, so the rejection and the shape
// toolArgsFeedbackMiddleware (pkg/agent/kernel) reports describe what the model
// actually did, not a half-applied guess at it.
package toolargs

import (
	"encoding/json"
	"maps"

	"github.com/gollem-dev/gollem"
)

// Coerce returns call with the argument shapes above rewritten into what the
// tool declares, descending into declared object properties and array items. The
// call is returned unchanged when nothing needs rewriting, when the rewrite does
// not make the call valid, or when no tool of that name is present. The input
// arguments are never mutated, because the call is held on a Strategy's
// checkpointed state and a replayed transition coerces the same original again.
func Coerce(tools []gollem.Tool, call gollem.FunctionCall) gollem.FunctionCall {
	if len(call.Arguments) == 0 {
		return call
	}
	spec, ok := findSpec(tools, call.Name)
	if !ok {
		return call
	}

	var coerced map[string]any
	for name, param := range spec.Parameters {
		if param == nil {
			continue
		}
		value, ok := call.Arguments[name]
		if !ok {
			continue
		}
		next, changed := coerceValue(param, value)
		if !changed {
			continue
		}
		if coerced == nil {
			coerced = maps.Clone(call.Arguments)
		}
		coerced[name] = next
	}
	if coerced == nil {
		return call
	}
	if err := spec.ValidateArgs(coerced); err != nil {
		// Still rejected, so the rewrite bought nothing and would only misreport
		// what the model sent. Hand back the original and let it be refused.
		return call
	}

	call.Arguments = coerced
	return call
}

// findSpec returns the spec of the named tool. Spec() is the only way to learn a
// gollem.Tool's name, so the scan stops at the first match rather than building
// every spec.
func findSpec(tools []gollem.Tool, name string) (gollem.ToolSpec, bool) {
	for _, t := range tools {
		if t == nil {
			continue
		}
		spec := t.Spec()
		if spec.Name == name {
			return spec, true
		}
	}
	return gollem.ToolSpec{}, false
}

// coerceValue returns the value to send for param, and whether it differs from
// what was received. It builds new containers instead of writing into the ones
// it was given.
func coerceValue(param *gollem.Parameter, value any) (any, bool) {
	if param == nil || value == nil {
		return value, false
	}

	switch param.Type {
	case gollem.TypeArray:
		if arr, ok := value.([]any); ok {
			return coerceItems(param.Items, arr)
		}
		if arr, ok := decodeJSONArray(value); ok {
			next, _ := coerceItems(param.Items, arr)
			return next, true
		}
		item, _ := coerceValue(param.Items, value)
		return []any{item}, true

	case gollem.TypeObject:
		obj, ok := value.(map[string]any)
		if !ok {
			return value, false
		}
		return coerceProperties(param.Properties, obj)

	default:
		return value, false
	}
}

// decodeJSONArray reads a string argument that holds a JSON array — a model
// serialising the array it was asked for instead of emitting it. Anything that
// is not such a string, including a string holding a JSON scalar or object, is
// left to the single-element wrap.
func decodeJSONArray(value any) ([]any, bool) {
	s, ok := value.(string)
	if !ok {
		return nil, false
	}
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, false
	}
	return arr, true
}

func coerceItems(items *gollem.Parameter, arr []any) (any, bool) {
	if items == nil {
		return arr, false
	}
	var out []any
	for i, item := range arr {
		next, changed := coerceValue(items, item)
		if !changed {
			continue
		}
		if out == nil {
			out = make([]any, len(arr))
			copy(out, arr)
		}
		out[i] = next
	}
	if out == nil {
		return arr, false
	}
	return out, true
}

func coerceProperties(props map[string]*gollem.Parameter, obj map[string]any) (any, bool) {
	var out map[string]any
	for name, prop := range props {
		value, ok := obj[name]
		if !ok || prop == nil {
			continue
		}
		next, changed := coerceValue(prop, value)
		if !changed {
			continue
		}
		if out == nil {
			out = maps.Clone(obj)
		}
		out[name] = next
	}
	if out == nil {
		return obj, false
	}
	return out, true
}
