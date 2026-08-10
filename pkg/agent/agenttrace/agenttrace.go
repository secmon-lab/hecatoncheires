// Package agenttrace converts agentkit's effect requests and results into
// gollem's trace representation.
//
// It exists because agentkit builds its own gollem session per Generate and
// therefore never runs gollem.WithTrace. Everything the trace consumers in this
// repository read — the Cloud Storage archive and the JobRunEvent timeline —
// comes from trace.Handler callbacks, so the kernel middleware drives those
// callbacks itself and uses this package to build their payloads.
package agenttrace

import (
	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
)

// LLMCallData builds the span payload for one Generate. res may be nil when the
// call failed, in which case the request side is still recorded — a failed call
// is exactly the one an operator wants the prompt for.
func LLMCallData(req *agentkit.GenerateRequest, res *agentkit.GenerateResult) *trace.LLMCallData {
	data := &trace.LLMCallData{
		Request: &trace.LLMRequest{
			Messages: Messages(historyOf(req), inputOf(req)),
			Tools:    toolSpecs(req),
		},
		Response: &trace.LLMResponse{},
	}
	if req != nil {
		data.Request.SystemPrompt = req.SystemPrompt
	}
	if res != nil {
		data.InputTokens = res.InputTokens
		data.OutputTokens = res.OutputTokens
		data.CacheReadInputTokens = res.CacheReadInputTokens
		data.CacheCreationInputTokens = res.CacheCreationInputTokens
		data.Response.Texts = res.Texts
		data.Response.FunctionCalls = functionCalls(res.FunctionCalls)
	}
	return data
}

func historyOf(req *agentkit.GenerateRequest) *gollem.History {
	if req == nil {
		return nil
	}
	return req.History
}

func inputOf(req *agentkit.GenerateRequest) []gollem.Input {
	if req == nil {
		return nil
	}
	return req.Input
}

func toolSpecs(req *agentkit.GenerateRequest) []trace.ToolSpec {
	if req == nil || len(req.Tools) == 0 {
		return nil
	}
	specs := make([]trace.ToolSpec, 0, len(req.Tools))
	for _, tl := range req.Tools {
		if tl == nil {
			continue
		}
		s := tl.Spec()
		specs = append(specs, trace.ToolSpec{Name: s.Name, Description: s.Description})
	}
	return specs
}

func functionCalls(calls []*gollem.FunctionCall) []*trace.FunctionCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]*trace.FunctionCall, 0, len(calls))
	for _, c := range calls {
		if c == nil {
			continue
		}
		out = append(out, &trace.FunctionCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
	}
	return out
}

// Messages renders the conversation a Generate was issued against: the carried
// history followed by this call's new input.
//
// A content block that cannot be decoded is rendered as a placeholder rather
// than dropped or turned into an error. Tracing is observation: losing a block's
// detail must never fail the transition that produced it, and silently omitting
// it would make the recorded conversation disagree with the one the model saw.
func Messages(h *gollem.History, input []gollem.Input) []trace.Message {
	var out []trace.Message
	if h != nil {
		for i := range h.Messages {
			out = append(out, convertMessage(&h.Messages[i]))
		}
	}
	if msg, ok := inputMessage(input); ok {
		out = append(out, msg)
	}
	return out
}

func convertMessage(m *gollem.Message) trace.Message {
	contents := make([]trace.MessageContent, 0, len(m.Contents))
	for i := range m.Contents {
		contents = append(contents, convertContent(&m.Contents[i]))
	}
	return trace.Message{Role: string(m.Role), Contents: contents}
}

func convertContent(c *gollem.MessageContent) trace.MessageContent {
	switch c.Type {
	case gollem.MessageContentTypeText:
		v, err := c.GetTextContent()
		if err != nil {
			return undecodable(c.Type)
		}
		return trace.NewTextContent(v.Text)
	case gollem.MessageContentTypeThinking:
		v, err := c.GetThinkingContent()
		if err != nil {
			return undecodable(c.Type)
		}
		return trace.NewThinkingContent(v.Text)
	case gollem.MessageContentTypeToolCall:
		v, err := c.GetToolCallContent()
		if err != nil {
			return undecodable(c.Type)
		}
		return trace.NewToolCallContent(v.ID, v.Name, v.Arguments)
	case gollem.MessageContentTypeToolResponse:
		v, err := c.GetToolResponseContent()
		if err != nil {
			return undecodable(c.Type)
		}
		return trace.NewToolResponseContent(v.ToolCallID, v.Name, v.Response)
	case gollem.MessageContentTypeImage:
		v, err := c.GetImageContent()
		if err != nil {
			return undecodable(c.Type)
		}
		return trace.NewMediaContent("image", v.MediaType)
	case gollem.MessageContentTypePDF:
		return trace.NewMediaContent("pdf", "application/pdf")
	default:
		return undecodable(c.Type)
	}
}

// undecodable renders a block the tracer could not read. The type is kept so a
// reader can tell an unreadable tool call from an unreadable image.
func undecodable(t gollem.MessageContentType) trace.MessageContent {
	return trace.NewTextContent("(undecodable " + string(t) + " content)")
}

// inputMessage folds this call's inputs into one user message. gollem.Input is
// an interface whose only text-bearing implementation is gollem.Text; anything
// else is recorded by type name so the trace does not silently claim the model
// was sent nothing.
func inputMessage(input []gollem.Input) (trace.Message, bool) {
	if len(input) == 0 {
		return trace.Message{}, false
	}
	contents := make([]trace.MessageContent, 0, len(input))
	for _, in := range input {
		switch v := in.(type) {
		case gollem.Text:
			contents = append(contents, trace.NewTextContent(string(v)))
		default:
			contents = append(contents, trace.NewTextContent("(non-text input)"))
		}
	}
	return trace.Message{Role: string(gollem.RoleUser), Contents: contents}, true
}
