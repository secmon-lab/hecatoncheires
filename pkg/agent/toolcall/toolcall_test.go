package toolcall_test

import (
	"encoding/json"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/toolcall"
)

func TestSuccessCarriesTheResult(t *testing.T) {
	call := gollem.FunctionCall{ID: "c1", Name: "probe__ping"}
	r := toolcall.New(call, map[string]any{"ok": true}, nil)

	in := r.Input()
	gt.Value(t, in.ID).Equal("c1")
	gt.Value(t, in.Name).Equal("probe__ping")
	gt.Value(t, in.Data).Equal(map[string]any{"ok": true})
	gt.Value(t, in.Error).Nil()
}

// A failed tool still produces an answer: the call has to be answered for the
// conversation to stay well-formed, and the model reacts to the failure.
func TestFailureIsCarriedAsTheAnswer(t *testing.T) {
	call := gollem.FunctionCall{ID: "c1", Name: "probe__ping"}
	r := toolcall.New(call, map[string]any{"ignored": true}, goerr.New("backend unavailable"))

	gt.Value(t, r.Data).Nil()

	in := r.Input()
	gt.Value(t, in.ID).Equal("c1")
	gt.Value(t, in.Error).NotNil().Required()
	gt.String(t, in.Error.Error()).Contains("backend unavailable")
}

// The response is held on a checkpointed state, so it must survive the JSON round
// trip a checkpoint makes — including the failure, which gollem.FunctionResponse
// carries as an unmarshalable error.
func TestSurvivesTheCheckpointRoundTrip(t *testing.T) {
	original := []toolcall.Response{
		toolcall.New(gollem.FunctionCall{ID: "c1", Name: "a"}, map[string]any{"n": float64(1)}, nil),
		toolcall.New(gollem.FunctionCall{ID: "c2", Name: "b"}, nil, goerr.New("nope")),
	}

	raw, err := json.Marshal(original)
	gt.NoError(t, err).Required()
	var restored []toolcall.Response
	gt.NoError(t, json.Unmarshal(raw, &restored)).Required()

	inputs := toolcall.Inputs(restored)
	gt.Array(t, inputs).Length(2).Required()

	first := gt.Cast[gollem.FunctionResponse](t, inputs[0])
	gt.Value(t, first.ID).Equal("c1")
	gt.Value(t, first.Data).Equal(map[string]any{"n": float64(1)})
	gt.Value(t, first.Error).Nil()

	second := gt.Cast[gollem.FunctionResponse](t, inputs[1])
	gt.Value(t, second.ID).Equal("c2")
	gt.Value(t, second.Error).NotNil().Required()
	gt.String(t, second.Error.Error()).Contains("nope")
}

func TestNoResponsesRenderNoInputs(t *testing.T) {
	gt.Array(t, toolcall.Inputs(nil)).Length(0)
}
