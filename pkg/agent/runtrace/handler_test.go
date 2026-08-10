package runtrace_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func newHandlerFixture(t *testing.T) (*runtrace.Handler, *memory.Memory) {
	t.Helper()
	repo := memory.New()
	routing := runtrace.Routing{
		WorkspaceID: "ws1",
		CaseID:      42,
		JobID:       "job-A",
		RunID:       "run-1",
		TraceID:     "trace-1",
	}
	clock := fixedClock(time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	h := runtrace.NewHandler(repo.JobRunEvent(), routing, clock)
	return h, repo
}

// TestHandler_TwoHandlersShareOneTimeline pins the property that replaced the
// in-process Sequencer: two Handlers on the same run — a resumed turn's, or
// another instance's claim of the same durable run — append into one ordered
// timeline without agreeing on a counter. An in-process counter would have both
// starting at 1 and the whole timeline would collapse into an arbitrary order.
func TestHandler_TwoHandlersShareOneTimeline(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	routing := runtrace.Routing{
		WorkspaceID: "ws-1", CaseID: 42, JobID: "job-A",
		RunID: "run-shared", TraceID: "trace-shared",
	}
	clock := fixedClock(time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))

	first := runtrace.NewHandler(repo.JobRunEvent(), routing, clock)
	second := runtrace.NewHandler(repo.JobRunEvent(), routing, clock)

	for _, h := range []*runtrace.Handler{first, second, first} {
		c := h.StartLLMCall(ctx)
		h.EndLLMCall(c, &trace.LLMCallData{Model: "claude-opus-4-7"}, nil)
	}

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{
		WorkspaceID: "ws-1", CaseID: 42, JobID: "job-A",
	}, "run-shared")
	gt.NoError(t, err).Required()
	// Three calls, each a request + a response.
	gt.Array(t, events).Length(6).Required()

	seen := make(map[int64]bool, len(events))
	for i, ev := range events {
		gt.Bool(t, seen[ev.Sequence]).False() // no two events share a number
		seen[ev.Sequence] = true
		if i > 0 {
			gt.Bool(t, events[i-1].Sequence < ev.Sequence).True()
		}
	}
}

func TestHandler_LLMCall_AppendsRequestAndResponse(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	ctxLLM := h.StartLLMCall(ctx)
	data := &trace.LLMCallData{
		Model:                    "claude-opus-4-7",
		InputTokens:              120,
		OutputTokens:             60,
		CacheCreationInputTokens: 70,
		CacheReadInputTokens:     30,
		Request: &trace.LLMRequest{
			Messages: []trace.Message{
				{
					Role: "user",
					Contents: []trace.MessageContent{
						{Type: "text", Text: "investigate case 42"},
					},
				},
			},
			Tools: []trace.ToolSpec{
				{Name: "slack_search", Description: "search slack"},
			},
		},
		Response: &trace.LLMResponse{
			Texts: []string{"let me look"},
			FunctionCalls: []*trace.FunctionCall{
				{ID: "abc", Name: "slack_search", Arguments: map[string]any{"q": "foo"}},
			},
		},
	}
	h.EndLLMCall(ctxLLM, data, nil)

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()

	reqEv := events[0]
	gt.Value(t, reqEv.Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Number(t, reqEv.Sequence).Equal(1)
	gt.String(t, reqEv.Phase).Equal("execute")
	gt.String(t, reqEv.AgentLabel).Equal("")
	gt.Value(t, reqEv.LLMRequest).NotNil()
	gt.String(t, reqEv.LLMRequest.Model).Equal("claude-opus-4-7")
	gt.Array(t, reqEv.LLMRequest.Messages).Length(1).Required()
	gt.String(t, reqEv.LLMRequest.Messages[0].Role).Equal("user")
	gt.Array(t, reqEv.LLMRequest.Messages[0].Contents).Length(1).Required()
	gt.String(t, reqEv.LLMRequest.Messages[0].Contents[0].Type).Equal("text")
	gt.String(t, reqEv.LLMRequest.Messages[0].Contents[0].Text).Equal("investigate case 42")
	gt.Array(t, reqEv.LLMRequest.Tools).Length(1).Required()
	gt.String(t, reqEv.LLMRequest.Tools[0].Name).Equal("slack_search")
	gt.String(t, reqEv.LLMRequest.Tools[0].Description).Equal("search slack")

	respEv := events[1]
	gt.Value(t, respEv.Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Number(t, respEv.Sequence).Equal(2)
	gt.Value(t, respEv.LLMResponse).NotNil()
	gt.String(t, respEv.LLMResponse.Model).Equal("claude-opus-4-7")
	gt.Array(t, respEv.LLMResponse.Texts).Length(1).Required()
	gt.String(t, respEv.LLMResponse.Texts[0]).Equal("let me look")
	gt.Array(t, respEv.LLMResponse.FunctionCalls).Length(1).Required()
	gt.String(t, respEv.LLMResponse.FunctionCalls[0].ID).Equal("abc")
	gt.String(t, respEv.LLMResponse.FunctionCalls[0].Name).Equal("slack_search")
	gt.String(t, respEv.LLMResponse.FunctionCalls[0].ArgumentsJSON).Equal(`{"q":"foo"}`)
	gt.Number(t, respEv.LLMResponse.InputTokens).Equal(120)
	gt.Number(t, respEv.LLMResponse.OutputTokens).Equal(60)
	gt.Number(t, respEv.LLMResponse.CacheCreationInputTokens).Equal(70)
	gt.Number(t, respEv.LLMResponse.CacheReadInputTokens).Equal(30)
	// DurationMs is computed from the clock; fixedClock returns the same time,
	// so the difference is 0 ms.
	gt.Number(t, respEv.LLMResponse.DurationMs).Equal(0)
}

// llmCall is a minimal LLMCallData carrying only the token figures, for tests
// that care about accumulation rather than payload content.
func llmCall(input, output int) *trace.LLMCallData {
	return llmCallWithCache(input, output, 0, 0)
}

// llmCallWithCache is llmCall plus the prompt-cache split of the input figure.
func llmCallWithCache(input, output, cacheCreation, cacheRead int) *trace.LLMCallData {
	return &trace.LLMCallData{
		Model:                    "claude-opus-4-7",
		InputTokens:              input,
		OutputTokens:             output,
		CacheCreationInputTokens: cacheCreation,
		CacheReadInputTokens:     cacheRead,
		Response:                 &trace.LLMResponse{Texts: []string{"ok"}},
	}
}

// steppingClock advances by step on every read, so the wall-clock a
// Start*/End* pair measures is exactly one step.
type steppingClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

// newSteppingHandler builds a handler whose clock advances by step per read.
func newSteppingHandler(t *testing.T, step time.Duration) (*runtrace.Handler, *memory.Memory) {
	t.Helper()
	repo := memory.New()
	clock := &steppingClock{now: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC), step: step}
	h := runtrace.NewHandler(repo.JobRunEvent(), runtrace.Routing{
		WorkspaceID: "ws1", CaseID: 42, JobID: "job-A", RunID: "run-1", TraceID: "trace-1",
	}, clock.Now)
	return h, repo
}

// llmCallOfferingTools is llmCall plus the tool specs the model was offered,
// which is what makes a tool name eligible as a per-tool key.
func llmCallOfferingTools(input, output int, toolNames ...string) *trace.LLMCallData {
	data := llmCall(input, output)
	specs := make([]trace.ToolSpec, 0, len(toolNames))
	for _, name := range toolNames {
		specs = append(specs, trace.ToolSpec{Name: name, Description: "d"})
	}
	data.Request = &trace.LLMRequest{Tools: specs}
	return data
}

// The measured latency is what makes a run's elapsed time decomposable, so the
// handler must sum the spans it already measures per call.
func TestHandler_CallStats_SumsMeasuredDurations(t *testing.T) {
	h, _ := newSteppingHandler(t, 100*time.Millisecond)
	ctx := context.Background()

	// Each Start/End pair spans one clock step. baseEvent also reads the clock,
	// but only through a value already taken, so the span stays one step.
	h.EndLLMCall(h.StartLLMCall(ctx), llmCallOfferingTools(10, 2, "slack_search", "notion_read"), nil)
	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(20, 4), nil)
	h.EndToolExec(h.StartToolExec(ctx, "slack_search", nil), map[string]any{"hits": 1}, nil)
	h.EndToolExec(h.StartToolExec(ctx, "slack_search", nil), map[string]any{"hits": 2}, nil)
	h.EndToolExec(h.StartToolExec(ctx, "notion_read", nil), nil, errors.New("page not found"))

	stats := h.CallStats()
	gt.Number(t, stats.LLMCalls).Equal(2)
	gt.Number(t, stats.LLMDurationMs).Equal(200)
	gt.Number(t, stats.ToolCalls).Equal(3)
	gt.Number(t, stats.ToolDurationMs).Equal(300)

	gt.Value(t, stats.ToolByName["slack_search"]).Equal(runtrace.ToolCallStats{Calls: 2, DurationMs: 200})
	// A failed execution still consumed wall-clock, so it is counted.
	gt.Value(t, stats.ToolByName["notion_read"]).Equal(runtrace.ToolCallStats{Calls: 1, DurationMs: 100})
}

// A tool name reaches the handler from the provider's function call, so an
// unrecognised one must not become a log field of its own.
func TestHandler_CallStats_BucketsUnofferedToolNames(t *testing.T) {
	h, _ := newSteppingHandler(t, 50*time.Millisecond)
	ctx := context.Background()

	h.EndLLMCall(h.StartLLMCall(ctx), llmCallOfferingTools(10, 2, "slack_search"), nil)
	h.EndToolExec(h.StartToolExec(ctx, "slack_search", nil), nil, nil)
	// Never offered to the model.
	h.EndToolExec(h.StartToolExec(ctx, "rm_minus_rf", nil), nil, nil)
	// Span lost, so no name at all.
	h.EndToolExec(ctx, nil, nil)

	stats := h.CallStats()
	gt.Number(t, stats.ToolCalls).Equal(3)
	gt.Value(t, stats.ToolByName["slack_search"].Calls).Equal(int64(1))
	gt.Value(t, stats.ToolByName["unregistered"].Calls).Equal(int64(2))
	_, leaked := stats.ToolByName["rm_minus_rf"]
	gt.Bool(t, leaked).False()
	_, empty := stats.ToolByName[""]
	gt.Bool(t, empty).False()
}

// A call that failed before returning data still waited on the provider, so its
// wall-clock must not be dropped along with its (absent) payload.
func TestHandler_CallStats_CountsDurationOfCallWithNoData(t *testing.T) {
	h, _ := newSteppingHandler(t, 250*time.Millisecond)
	ctx := context.Background()

	h.EndLLMCall(h.StartLLMCall(ctx), nil, errors.New("failed to create chat completion stream"))

	stats := h.CallStats()
	gt.Number(t, stats.LLMCalls).Equal(1)
	gt.Number(t, stats.LLMDurationMs).Equal(250)
}

// The caller keeps the returned map (it goes onto a log line), so it must not
// alias the handler's own state.
func TestHandler_CallStats_ReturnsACopy(t *testing.T) {
	h, _ := newSteppingHandler(t, 10*time.Millisecond)
	ctx := context.Background()

	h.EndLLMCall(h.StartLLMCall(ctx), llmCallOfferingTools(1, 1, "slack_search"), nil)
	h.EndToolExec(h.StartToolExec(ctx, "slack_search", nil), nil, nil)

	first := h.CallStats()
	first.ToolByName["slack_search"] = runtrace.ToolCallStats{Calls: 999}
	first.ToolByName["injected"] = runtrace.ToolCallStats{Calls: 1}

	second := h.CallStats()
	gt.Value(t, second.ToolByName["slack_search"].Calls).Equal(int64(1))
	_, injected := second.ToolByName["injected"]
	gt.Bool(t, injected).False()
}

// A nil handler is the resume prepare-failure path, where no trace was set up.
func TestHandler_CallStats_NilReceiver(t *testing.T) {
	var h *runtrace.Handler
	gt.Value(t, h.CallStats()).Equal(runtrace.CallStats{})
}

func TestHandler_RunTotals_AccumulatesTokensAcrossCalls(t *testing.T) {
	h, _ := newHandlerFixture(t)
	ctx := context.Background()

	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{})

	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(120, 60), nil)
	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(30, 5), nil)

	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{
		InputTokens:  150,
		OutputTokens: 65,
		LLMCalls:     2,
	})
}

func TestHandler_RunTotals_AccumulatesPromptCacheTokens(t *testing.T) {
	h, _ := newHandlerFixture(t)
	ctx := context.Background()

	// First call writes the cache, the second reads it back. InputTokens is the
	// provider's total and already includes the cache figures, so the three
	// counters accumulate independently rather than one being derived from
	// the others.
	h.EndLLMCall(h.StartLLMCall(ctx), llmCallWithCache(1000, 40, 900, 0), nil)
	h.EndLLMCall(h.StartLLMCall(ctx), llmCallWithCache(1200, 30, 0, 900), nil)
	// A provider that reports no cache usage leaves the split at zero.
	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(100, 10), nil)

	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{
		InputTokens:              2300,
		OutputTokens:             80,
		CacheCreationInputTokens: 900,
		CacheReadInputTokens:     900,
		LLMCalls:                 3,
	})
}

func TestHandler_RunTotals_CountsCallThatReportedNoData(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	// A provider that could not open the stream calls EndLLMCall with a nil
	// LLMCallData. Reaching the model is a step the run took, so it is counted
	// even though there is no request / response body to record.
	h.EndLLMCall(h.StartLLMCall(ctx), nil, errors.New("failed to create chat completion stream"))
	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{LLMCalls: 1})

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(0)

	// A failed call that did report data still contributes its tokens.
	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(10, 0), errors.New("upstream refused"))
	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{InputTokens: 10, LLMCalls: 2})
}

func TestHandler_RunTotals_CountsConcurrentSubAgentCalls(t *testing.T) {
	h, _ := newHandlerFixture(t)
	ctx := context.Background()

	// planexec wires one handler into every parallel sub-agent, so concurrent
	// EndLLMCall must not lose counts.
	const N = 50
	var wg sync.WaitGroup
	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.EndLLMCall(h.StartLLMCall(ctx), llmCall(2, 3), nil)
		}()
	}
	wg.Wait()

	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{
		InputTokens:  2 * N,
		OutputTokens: 3 * N,
		LLMCalls:     N,
	})
}

func TestHandler_RunTotals_CountsToolExecutions(t *testing.T) {
	h, _ := newHandlerFixture(t)
	ctx := context.Background()

	// A TOOL_CALL needs a preceding LLM_RESPONSE for its ParentSequence.
	h.EndLLMCall(h.StartLLMCall(ctx), llmCall(10, 2), nil)
	h.EndToolExec(h.StartToolExec(ctx, "slack_search", map[string]any{"q": "a"}), map[string]any{"hits": 1}, nil)
	h.EndToolExec(h.StartToolExec(ctx, "notion_read", nil), nil, errors.New("page not found"))

	// A failed tool execution is still a step the agent took, so it counts.
	gt.Value(t, runtrace.HandlerRunTotalsForTest(h)).Equal(runtrace.RunTotalsForTest{
		InputTokens:  10,
		OutputTokens: 2,
		LLMCalls:     1,
		ToolCalls:    2,
	})
}

func TestAddRunTotals(t *testing.T) {
	ctx := context.Background()

	t.Run("adds to the totals already on the log", func(t *testing.T) {
		h, _ := newHandlerFixture(t)
		h.EndLLMCall(h.StartLLMCall(ctx), llmCallWithCache(40, 7, 12, 8), nil)
		h.EndToolExec(h.StartToolExec(ctx, "slack_search", nil), nil, nil)

		// A resumed run's log carries the suspended turn's totals.
		log := &model.JobRunLog{
			InputTokens: 100, OutputTokens: 20,
			CacheCreationInputTokens: 30, CacheReadInputTokens: 50,
			LLMCallCount: 3, ToolCallCount: 5,
		}
		runtrace.AddRunTotals(log, h)

		gt.Number(t, log.InputTokens).Equal(140)
		gt.Number(t, log.OutputTokens).Equal(27)
		gt.Number(t, log.CacheCreationInputTokens).Equal(42)
		gt.Number(t, log.CacheReadInputTokens).Equal(58)
		gt.Number(t, log.LLMCallCount).Equal(4)
		gt.Number(t, log.ToolCallCount).Equal(6)
	})

	t.Run("leaves the log untouched when there is no handler", func(t *testing.T) {
		log := &model.JobRunLog{
			InputTokens: 100, OutputTokens: 20,
			CacheCreationInputTokens: 30, CacheReadInputTokens: 50,
			LLMCallCount: 3, ToolCallCount: 5,
		}
		runtrace.AddRunTotals(log, nil)

		gt.Number(t, log.InputTokens).Equal(100)
		gt.Number(t, log.OutputTokens).Equal(20)
		gt.Number(t, log.CacheCreationInputTokens).Equal(30)
		gt.Number(t, log.CacheReadInputTokens).Equal(50)
		gt.Number(t, log.LLMCallCount).Equal(3)
		gt.Number(t, log.ToolCallCount).Equal(5)
	})
}

func TestHandler_ToolExec_AppendsToolCallWithParent(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	// Run an LLM call first so we have a parent for the tool call.
	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	// Now invoke the tool.
	args := map[string]any{"q": "foo"}
	ctxTool := h.StartToolExec(ctx, "slack_search", args)
	h.EndToolExec(ctxTool, map[string]any{"hits": 3}, nil)

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()

	toolEv := events[2]
	gt.Value(t, toolEv.Kind).Equal(model.JobRunEventKindToolCall)
	gt.Number(t, toolEv.Sequence).Equal(3)
	// ParentSequence points at the LLM_RESPONSE (seq=2).
	gt.Number(t, toolEv.ParentSequence).Equal(2)
	gt.Value(t, toolEv.ToolCall).NotNil()
	gt.String(t, toolEv.ToolCall.ToolName).Equal("slack_search")
	gt.String(t, toolEv.ToolCall.ArgumentsJSON).Equal(`{"q":"foo"}`)
	gt.String(t, toolEv.ToolCall.ResultJSON).Equal(`{"hits":3}`)
	gt.Bool(t, toolEv.ToolCall.IsError).False()
	gt.String(t, toolEv.ToolCall.ErrorMessage).Equal("")
}

func TestHandler_ToolExec_Error(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	// We need a parent LLM_RESPONSE first.
	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	ctxTool := h.StartToolExec(ctx, "slack_search", map[string]any{"q": "x"})
	h.EndToolExec(ctxTool, nil, errors.New("network down"))

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()

	toolEv := events[2]
	gt.Value(t, toolEv.Kind).Equal(model.JobRunEventKindToolCall)
	gt.Bool(t, toolEv.ToolCall.IsError).True()
	gt.String(t, toolEv.ToolCall.ErrorMessage).Equal("network down")
	gt.String(t, toolEv.ToolCall.ResultJSON).Equal("")
}

func TestHandler_NSerialToolExecs_MonotonicSeq(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	// One LLM call as parent.
	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	// Three serial tool execs.
	for i := range 3 {
		ctxTool := h.StartToolExec(ctx, "search", map[string]any{"i": i})
		h.EndToolExec(ctxTool, map[string]any{"i": i}, nil)
	}

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(5).Required()

	// LLM_REQUEST + LLM_RESPONSE + 3 TOOL_CALL
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindToolCall)
	gt.Value(t, events[3].Kind).Equal(model.JobRunEventKindToolCall)
	gt.Value(t, events[4].Kind).Equal(model.JobRunEventKindToolCall)
	for i := int64(0); i < int64(len(events)); i++ {
		gt.Number(t, events[i].Sequence).Equal(i + 1)
	}
	// All TOOL_CALL events share the same ParentSequence (= LLM_RESPONSE seq=2).
	gt.Number(t, events[2].ParentSequence).Equal(2)
	gt.Number(t, events[3].ParentSequence).Equal(2)
	gt.Number(t, events[4].ParentSequence).Equal(2)
}

// TestHandler_EmitRunError_OrdersAfterTheCallsItFollows pins that the owner's
// RUN_ERROR lands after the per-call events it followed. The two appenders no
// longer share a counter — the repository allocates — so this is what stands in
// for that former guarantee.
func TestHandler_EmitRunError_OrdersAfterTheCallsItFollows(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	gt.NoError(t, h.EmitRunError(ctx, "execute", "boom")).Required()

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Number(t, events[0].Sequence).Equal(1)
	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Number(t, events[1].Sequence).Equal(2)
	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindRunError)
	gt.Number(t, events[2].Sequence).Equal(3)
	gt.String(t, events[2].RunError.Stage).Equal("execute")
	gt.String(t, events[2].RunError.Message).Equal("boom")
}

func TestHandler_SubAgentLabel_RoundTrips(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	ctxSub := h.StartSubAgent(ctx, "planner")
	// LLM_REQUEST/RESPONSE while sub-agent label is active.
	ctxLLM := h.StartLLMCall(ctxSub)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)
	h.EndSubAgent(ctxSub, nil)

	// After EndSubAgent the label should be cleared.
	ctxLLM2 := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM2, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(4).Required()
	// First pair was tagged "planner".
	gt.String(t, events[0].AgentLabel).Equal("planner")
	gt.String(t, events[1].AgentLabel).Equal("planner")
	// Second pair is back to empty.
	gt.String(t, events[2].AgentLabel).Equal("")
	gt.String(t, events[3].AgentLabel).Equal("")
}

func TestHandler_TruncatesLongFields(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	big := strings.Repeat("a", model.MaxInlineBytes+100)
	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:   "m",
		Request: &trace.LLMRequest{},
		Response: &trace.LLMResponse{
			Texts: []string{big},
		},
	}, nil)

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()
	gt.Array(t, events[1].LLMResponse.Texts).Length(1).Required()
	gt.Number(t, len(events[1].LLMResponse.Texts[0])).Equal(model.MaxInlineBytes)
}

func TestHandler_EnterReflectionPhase_SetsPhase(t *testing.T) {
	h, repo := newHandlerFixture(t)
	ctx := context.Background()

	// Before entering the reflection phase, events carry the default "execute" phase.
	ctxLLM := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	events, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()
	gt.String(t, events[0].Phase).Equal("execute")
	gt.String(t, events[1].Phase).Equal("execute")

	// Transition to reflection phase — subsequent events must carry "reflection".
	h.EnterReflectionPhase()

	ctxLLM2 := h.StartLLMCall(ctx)
	h.EndLLMCall(ctxLLM2, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{},
	}, nil)

	events2, err := repo.JobRunEvent().List(ctx, model.JobRunKey{WorkspaceID: "ws1", CaseID: 42, JobID: "job-A"}, "run-1")
	gt.NoError(t, err).Required()
	gt.Array(t, events2).Length(4).Required()
	// The two new events (LLM_REQUEST + LLM_RESPONSE) are in the reflection phase.
	gt.String(t, events2[2].Phase).Equal("reflection")
	gt.String(t, events2[3].Phase).Equal("reflection")
}

func TestTruncate(t *testing.T) {
	// Shorter than the cap is returned unchanged.
	gt.String(t, runtrace.Truncate("hello", 100)).Equal("hello")
	// Exactly the cap is unchanged.
	gt.String(t, runtrace.Truncate("hello", 5)).Equal("hello")
	// A max of 0 yields empty.
	gt.String(t, runtrace.Truncate("hello", 0)).Equal("")
	// Multi-byte runes are never split: cutting a 3-byte rune at byte 1 snaps
	// back to a rune boundary (here, the empty string).
	jp := "あ" // 3 bytes
	gt.Number(t, len(runtrace.Truncate(jp, 1))).Equal(0)
}
