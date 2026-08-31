package runtrace_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// growingConversation renders the messages an agent's Nth LLM call carries: the
// same conversation, one message longer each time.
func growingConversation(n int) []trace.Message {
	msgs := make([]trace.Message, 0, n)
	for i := range n {
		msgs = append(msgs, trace.Message{
			Role:     "user",
			Contents: []trace.MessageContent{{Type: "text", Text: fmt.Sprintf("message %d", i)}},
		})
	}
	return msgs
}

// callWith drives one complete LLM call through the handler.
func callWith(ctx context.Context, h *runtrace.Handler, msgs []trace.Message, tools []trace.ToolSpec) {
	c := h.StartLLMCall(ctx)
	h.EndLLMCall(c, &trace.LLMCallData{
		Model:   "claude-opus-4-7",
		Request: &trace.LLMRequest{Messages: msgs, Tools: tools},
	}, nil)
}

// llmRequests reads back the run's LLM_REQUEST payloads in timeline order.
func llmRequests(t *testing.T, repo *memory.Memory, routing runtrace.Routing) []*model.LLMRequestPayload {
	t.Helper()
	events, err := repo.JobRunEvent().List(context.Background(), model.JobRunKey{
		WorkspaceID: routing.WorkspaceID, CaseID: routing.CaseID, JobID: routing.JobID,
	}, routing.RunID)
	gt.NoError(t, err).Required()
	var out []*model.LLMRequestPayload
	for _, ev := range events {
		if ev.LLMRequest != nil {
			out = append(out, ev.LLMRequest)
		}
	}
	return out
}

// reconstruct rebuilds one conversation's full message list at each of its
// events, the way a consumer of the timeline is expected to.
func reconstruct(reqs []*model.LLMRequestPayload, conversationID string) [][]model.LLMMessage {
	var full []model.LLMMessage
	var out [][]model.LLMMessage
	for _, p := range reqs {
		if p.ConversationID != conversationID {
			continue
		}
		prefix := min(p.MessagesPrefixLen, len(full))
		full = append(append([]model.LLMMessage{}, full[:prefix]...), p.Messages...)
		out = append(out, full)
	}
	return out
}

func conversationRouting() runtrace.Routing {
	return runtrace.Routing{
		WorkspaceID: "ws-1", CaseID: 7, JobID: "job-A",
		RunID: "run-conv", TraceID: "trace-conv",
	}
}

// TestAGrowingConversationIsRecordedOnce is the point of the whole mechanism: a
// run that calls the model N times against one conversation must not store the
// conversation N times. Every message is written exactly once, and the whole
// list is still reconstructible at every call.
func TestAGrowingConversationIsRecordedOnce(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := conversationRouting()
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, nil)

	const calls = 5
	tools := []trace.ToolSpec{{Name: "slack_search", Description: "search slack"}}
	for i := 1; i <= calls; i++ {
		callWith(ctx, h, growingConversation(i), tools)
	}

	reqs := llmRequests(t, repo, routing)
	gt.Array(t, reqs).Length(calls).Required()

	// One conversation, so one id across every event.
	convID := reqs[0].ConversationID
	gt.String(t, convID).NotEqual("")
	for _, p := range reqs {
		gt.String(t, p.ConversationID).Equal(convID)
	}

	// Total messages stored equals the conversation's final length, not the sum
	// of its lengths at each call (which would be calls*(calls+1)/2 == 15).
	stored := 0
	for _, p := range reqs {
		stored += len(p.Messages)
	}
	gt.Number(t, stored).Equal(calls)

	// The tool set is written once and suppressed while it stays the same.
	gt.Array(t, reqs[0].Tools).Length(1)
	for _, p := range reqs[1:] {
		gt.Array(t, p.Tools).Length(0)
	}

	// Every call's full request is still recoverable.
	got := reconstruct(reqs, convID)
	gt.Array(t, got).Length(calls).Required()
	for i, full := range got {
		gt.Array(t, full).Length(i + 1).Required()
		for j, m := range full {
			gt.String(t, m.Contents[0].Text).Equal(fmt.Sprintf("message %d", j))
		}
	}
}

// TestAChangedToolSetIsRecordedAgain pins that the tool-set suppression is
// checked rather than assumed: a call offering a different set records it.
func TestAChangedToolSetIsRecordedAgain(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := conversationRouting()
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, nil)

	first := []trace.ToolSpec{{Name: "slack_search", Description: "search slack"}}
	second := []trace.ToolSpec{
		{Name: "slack_search", Description: "search slack"},
		{Name: "case__update_case", Description: "update the case"},
	}
	callWith(ctx, h, growingConversation(1), first)
	callWith(ctx, h, growingConversation(2), first)
	callWith(ctx, h, growingConversation(3), second)

	reqs := llmRequests(t, repo, routing)
	gt.Array(t, reqs).Length(3).Required()
	gt.Array(t, reqs[0].Tools).Length(1)
	gt.Array(t, reqs[1].Tools).Length(0)
	gt.Array(t, reqs[2].Tools).Length(2)
}

// TestAnUnrelatedConversationDoesNotCorruptTheDiff pins the guard that makes
// the diff safe when the scoping is imperfect. A conversation that is not a
// continuation of what was recorded is recorded in full instead of as a diff,
// so no message is ever lost — the outcome is a larger record, never a wrong
// one.
func TestAnUnrelatedConversationDoesNotCorruptTheDiff(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := conversationRouting()
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, nil)

	// Two calls of the agent's own conversation, then a short unrelated one on
	// the same handler with no scope of its own, then the agent continues.
	callWith(ctx, h, growingConversation(3), nil)
	unrelated := []trace.Message{{
		Role:     "user",
		Contents: []trace.MessageContent{{Type: "text", Text: "summarise this page"}},
	}}
	callWith(ctx, h, unrelated, nil)
	callWith(ctx, h, growingConversation(4), nil)

	reqs := llmRequests(t, repo, routing)
	gt.Array(t, reqs).Length(3).Required()

	// The interloper is recorded whole, and so is the call after it: neither
	// claims a prefix it cannot have.
	gt.Number(t, reqs[1].MessagesPrefixLen).Equal(0)
	gt.Array(t, reqs[1].Messages).Length(1)
	gt.Number(t, reqs[2].MessagesPrefixLen).Equal(0)
	gt.Array(t, reqs[2].Messages).Length(4).Required()
	for j, m := range reqs[2].Messages {
		gt.String(t, m.Contents[0].Text).Equal(fmt.Sprintf("message %d", j))
	}
}

// TestANestedToolLLMCallIsItsOwnConversation pins the case the runtime actually
// produces: a tool that reaches an LLM itself (a knowledge tool's embedding,
// webfetch's page analysis) shares the run's handler, and its request must be
// diffed against the tool's own message list rather than against the agent's.
func TestANestedToolLLMCallIsItsOwnConversation(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := conversationRouting()
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, nil)

	callWith(ctx, h, growingConversation(2), nil)

	// The tool span is what carries the tool's conversation.
	toolCtx := h.StartToolExec(ctx, "webfetch", map[string]any{"url": "https://example.com"})
	toolMsgs := []trace.Message{{
		Role:     "user",
		Contents: []trace.MessageContent{{Type: "text", Text: "screen this page"}},
	}}
	callWith(toolCtx, h, toolMsgs, nil)
	callWith(toolCtx, h, append(append([]trace.Message{}, toolMsgs...), trace.Message{
		Role:     "assistant",
		Contents: []trace.MessageContent{{Type: "text", Text: "the page is safe"}},
	}), nil)
	h.EndToolExec(toolCtx, map[string]any{"safe": true}, nil)

	// The agent's conversation continues where it left off.
	callWith(ctx, h, growingConversation(3), nil)

	reqs := llmRequests(t, repo, routing)
	gt.Array(t, reqs).Length(4).Required()

	agentID := reqs[0].ConversationID
	toolID := reqs[1].ConversationID
	gt.String(t, toolID).NotEqual(agentID)
	gt.String(t, reqs[2].ConversationID).Equal(toolID)
	gt.String(t, reqs[3].ConversationID).Equal(agentID)

	// The tool's second call diffed against the tool's first, not the agent's.
	gt.Number(t, reqs[2].MessagesPrefixLen).Equal(1)
	gt.Array(t, reqs[2].Messages).Length(1)

	// The agent's third call diffed against the agent's second: the tool's
	// calls in between changed nothing.
	gt.Number(t, reqs[3].MessagesPrefixLen).Equal(2)
	gt.Array(t, reqs[3].Messages).Length(1).Required()
	gt.String(t, reqs[3].Messages[0].Contents[0].Text).Equal("message 2")

	gt.Array(t, reconstruct(reqs, agentID)).Length(2)
	gt.Array(t, reconstruct(reqs, toolID)).Length(2)
}

// TestConcurrentSubAgentConversationsStayApart pins that several conversations
// running at the same time on one handler each keep their own diff. On the
// durable runtime a sub-agent is a separate Process with a Handler of its own,
// but a gollem-driven agent shares one, and parallel tool calls do too.
func TestConcurrentSubAgentConversationsStayApart(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := conversationRouting()
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, nil)

	const branches = 4
	const callsPerBranch = 4
	var wg sync.WaitGroup
	for b := range branches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			branchCtx := h.StartSubAgent(ctx, fmt.Sprintf("task-%d", b))
			for i := 1; i <= callsPerBranch; i++ {
				msgs := make([]trace.Message, 0, i)
				for j := range i {
					msgs = append(msgs, trace.Message{
						Role: "user",
						Contents: []trace.MessageContent{
							{Type: "text", Text: fmt.Sprintf("branch %d message %d", b, j)},
						},
					})
				}
				callWith(branchCtx, h, msgs, nil)
			}
		}()
	}
	wg.Wait()

	reqs := llmRequests(t, repo, routing)
	gt.Array(t, reqs).Length(branches * callsPerBranch).Required()

	byConversation := map[string][]*model.LLMRequestPayload{}
	for _, p := range reqs {
		byConversation[p.ConversationID] = append(byConversation[p.ConversationID], p)
	}
	gt.Map(t, byConversation).Length(branches).Required()

	for id, group := range byConversation {
		gt.Array(t, group).Length(callsPerBranch).Required()
		full := reconstruct(reqs, id)
		gt.Array(t, full).Length(callsPerBranch).Required()
		// Each branch reconstructs to its own messages, in order, with nothing
		// from another branch mixed in. The branch a conversation belongs to is
		// named by its first message.
		last := full[callsPerBranch-1]
		gt.Array(t, last).Length(callsPerBranch).Required()
		var branch int
		_, err := fmt.Sscanf(last[0].Contents[0].Text, "branch %d message 0", &branch)
		gt.NoError(t, err).Required()
		for j, m := range last {
			gt.String(t, m.Contents[0].Text).
				Equal(fmt.Sprintf("branch %d message %d", branch, j))
		}
	}
}
