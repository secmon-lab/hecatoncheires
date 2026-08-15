// Package toolcall holds one tool result between the transition that produced it
// and the LLM call that reports it.
//
// It exists because of a provider requirement that the obvious implementation
// violates. A model turn holding N function calls must be answered by a SINGLE
// turn holding N function responses; Gemini rejects the request outright
// otherwise ("Please ensure that the number of function response parts is equal
// to the number of function call parts of the function call turn"). agentkit's
// managed session appends each result to the conversation as its own message
// (session.go, CallTool), and gollem's Gemini adapter maps one message to one
// turn (llm/gemini/convert_message.go, convertMessagesToGemini) — so a strategy
// that answers a parallel call turn one result at a time splits the one required
// turn into N, and every later call in the run is rejected.
//
// A Strategy therefore runs each call through the PRIMITIVE Syscalls.CallTool,
// which executes the tool without touching the conversation, keeps the results
// here on its checkpointed state, and hands all of them to the next Generate as
// inputs. gollem packs the inputs of one Generate into one turn, which is the
// shape the provider asks for.
package toolcall

import (
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
)

// Response is one tool result, in a form that survives the JSON round trip a
// checkpoint makes. gollem.FunctionResponse cannot be stored directly: it carries
// the failure as an `error`, which does not marshal.
type Response struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Data  map[string]any `json:"data,omitempty"`
	Error string         `json:"error,omitempty"`
}

// New records the outcome of call. A failed tool still produces a Response: the
// call must be answered for the conversation to stay well-formed, and the model
// is expected to react to the failure on its next turn.
func New(call gollem.FunctionCall, out map[string]any, err error) Response {
	r := Response{ID: call.ID, Name: call.Name}
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Data = out
	return r
}

// Input renders the response as the gollem input a Generate sends.
func (r Response) Input() gollem.FunctionResponse {
	out := gollem.FunctionResponse{ID: r.ID, Name: r.Name, Data: r.Data}
	if r.Error != "" {
		out.Error = goerr.New(r.Error)
	}
	return out
}

// Inputs renders every response in order. The order is the order the model asked
// for the calls, which is what a provider matching responses positionally needs.
func Inputs(responses []Response) []gollem.Input {
	inputs := make([]gollem.Input, 0, len(responses))
	for _, r := range responses {
		inputs = append(inputs, r.Input())
	}
	return inputs
}
