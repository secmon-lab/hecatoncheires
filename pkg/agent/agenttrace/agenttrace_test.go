package agenttrace_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/agenttrace"
)

func mustContent(t *testing.T, build func() (gollem.MessageContent, error)) gollem.MessageContent {
	t.Helper()
	c, err := build()
	gt.NoError(t, err).Required()
	return c
}

func textContent(t *testing.T, text string) gollem.MessageContent {
	t.Helper()
	return mustContent(t, func() (gollem.MessageContent, error) { return gollem.NewTextContent(text) })
}

type stubTool struct {
	name string
	desc string
}

func (s stubTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: s.name, Description: s.desc}
}

func (s stubTool) Run(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

// TestMessagesRendersHistoryThenInput pins the shape a trace consumer reads:
// the carried conversation first, this call's input last.
func TestMessagesRendersHistoryThenInput(t *testing.T) {
	h := &gollem.History{
		LLType:  gollem.LLMTypeOpenAI,
		Version: gollem.HistoryVersion,
		Messages: []gollem.Message{
			{
				Role:     gollem.RoleUser,
				Contents: []gollem.MessageContent{textContent(t, "earlier question")},
			},
			{
				Role: gollem.RoleAssistant,
				Contents: []gollem.MessageContent{
					mustContent(t, func() (gollem.MessageContent, error) {
						return gollem.NewToolCallContent("call-1", "slack__search", map[string]any{"q": "deploy"})
					}),
				},
			},
			{
				Role: gollem.RoleTool,
				Contents: []gollem.MessageContent{
					mustContent(t, func() (gollem.MessageContent, error) {
						return gollem.NewToolResponseContent("call-1", "slack__search", map[string]any{"hits": float64(2)}, false)
					}),
				},
			},
		},
	}

	got := agenttrace.Messages(h, []gollem.Input{gollem.Text("new question")})

	gt.Array(t, got).Length(4).Required()

	gt.String(t, got[0].Role).Equal("user")
	gt.String(t, got[0].Contents[0].Type).Equal("text")
	gt.String(t, got[0].Contents[0].Text).Equal("earlier question")

	gt.String(t, got[1].Role).Equal("assistant")
	gt.String(t, got[1].Contents[0].Type).Equal("tool_call")
	gt.String(t, got[1].Contents[0].ID).Equal("call-1")
	gt.String(t, got[1].Contents[0].Name).Equal("slack__search")
	gt.Value(t, got[1].Contents[0].Arguments["q"]).Equal("deploy")

	gt.String(t, got[2].Role).Equal("tool")
	gt.String(t, got[2].Contents[0].Type).Equal("tool_response")
	gt.String(t, got[2].Contents[0].ToolCallID).Equal("call-1")
	gt.Value(t, got[2].Contents[0].Result["hits"]).Equal(float64(2))

	gt.String(t, got[3].Role).Equal("user")
	gt.String(t, got[3].Contents[0].Text).Equal("new question")
}

func TestMessagesWithoutHistoryOrInput(t *testing.T) {
	t.Run("no history renders only the input", func(t *testing.T) {
		got := agenttrace.Messages(nil, []gollem.Input{gollem.Text("only question")})
		gt.Array(t, got).Length(1).Required()
		gt.String(t, got[0].Contents[0].Text).Equal("only question")
	})

	t.Run("no input renders only the history", func(t *testing.T) {
		h := &gollem.History{
			LLType:  gollem.LLMTypeOpenAI,
			Version: gollem.HistoryVersion,
			Messages: []gollem.Message{{
				Role:     gollem.RoleUser,
				Contents: []gollem.MessageContent{textContent(t, "carried")},
			}},
		}
		got := agenttrace.Messages(h, nil)
		gt.Array(t, got).Length(1).Required()
		gt.String(t, got[0].Contents[0].Text).Equal("carried")
	})

	t.Run("neither renders nothing", func(t *testing.T) {
		gt.Array(t, agenttrace.Messages(nil, nil)).Length(0)
	})
}

// TestMessagesUndecodableContent pins that a block the tracer cannot read is
// rendered as a placeholder rather than dropped. Dropping it would make the
// recorded conversation disagree with the one the model actually saw.
func TestMessagesUndecodableContent(t *testing.T) {
	h := &gollem.History{
		LLType:  gollem.LLMTypeOpenAI,
		Version: gollem.HistoryVersion,
		Messages: []gollem.Message{{
			Role: gollem.RoleAssistant,
			Contents: []gollem.MessageContent{
				{Type: gollem.MessageContentTypeToolCall, Data: []byte("not json")},
			},
		}},
	}

	got := agenttrace.Messages(h, nil)
	gt.Array(t, got).Length(1).Required()
	gt.Array(t, got[0].Contents).Length(1).Required()
	gt.String(t, got[0].Contents[0].Text).Equal("(undecodable tool_call content)")
}

func TestLLMCallData(t *testing.T) {
	req := &agentkit.GenerateRequest{
		Input:        []gollem.Input{gollem.Text("what happened")},
		SystemPrompt: "you are the case agent",
		Tools:        []gollem.Tool{stubTool{name: "slack__search", desc: "search slack"}},
	}
	res := &agentkit.GenerateResult{
		Texts:                    []string{"here is the answer"},
		FunctionCalls:            []*gollem.FunctionCall{{ID: "c1", Name: "slack__search", Arguments: map[string]any{"q": "x"}}},
		InputTokens:              120,
		OutputTokens:             34,
		CacheReadInputTokens:     100,
		CacheCreationInputTokens: 20,
	}

	got := agenttrace.LLMCallData(req, res, "claude-opus-4-6")

	gt.String(t, got.Model).Equal("claude-opus-4-6")
	gt.Value(t, got.InputTokens).Equal(120)
	gt.Value(t, got.OutputTokens).Equal(34)
	gt.Value(t, got.CacheReadInputTokens).Equal(100)
	gt.Value(t, got.CacheCreationInputTokens).Equal(20)

	gt.String(t, got.Request.SystemPrompt).Equal("you are the case agent")
	gt.Array(t, got.Request.Messages).Length(1).Required()
	gt.String(t, got.Request.Messages[0].Contents[0].Text).Equal("what happened")
	gt.Array(t, got.Request.Tools).Length(1).Required()
	gt.String(t, got.Request.Tools[0].Name).Equal("slack__search")
	gt.String(t, got.Request.Tools[0].Description).Equal("search slack")

	gt.Array(t, got.Response.Texts).Equal([]string{"here is the answer"})
	gt.Array(t, got.Response.FunctionCalls).Length(1).Required()
	gt.String(t, got.Response.FunctionCalls[0].ID).Equal("c1")
	gt.String(t, got.Response.FunctionCalls[0].Name).Equal("slack__search")
}

// TestLLMCallDataWithoutResult pins that a failed call still records what was
// sent — that is the case an operator most needs the prompt for.
func TestLLMCallDataWithoutResult(t *testing.T) {
	req := &agentkit.GenerateRequest{
		Input:        []gollem.Input{gollem.Text("what happened")},
		SystemPrompt: "you are the case agent",
	}

	got := agenttrace.LLMCallData(req, nil, "")

	gt.String(t, got.Request.SystemPrompt).Equal("you are the case agent")
	gt.Array(t, got.Request.Messages).Length(1).Required()
	gt.String(t, got.Request.Messages[0].Contents[0].Text).Equal("what happened")
	gt.Value(t, got.InputTokens).Equal(0)
	gt.Array(t, got.Response.Texts).Length(0)
	gt.Array(t, got.Response.FunctionCalls).Length(0)
}

// TestLLMCallDataWithNilRequest pins that the converter cannot panic on the
// degenerate input a middleware could hand it.
func TestLLMCallDataWithNilRequest(t *testing.T) {
	got := agenttrace.LLMCallData(nil, nil, "")
	gt.Value(t, got).NotNil()
	gt.String(t, got.Request.SystemPrompt).Equal("")
	gt.Array(t, got.Request.Messages).Length(0)
}
