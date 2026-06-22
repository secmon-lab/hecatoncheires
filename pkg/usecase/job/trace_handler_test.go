package job_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func newHandlerFixture(t *testing.T) (
	*job.JobRunTraceHandlerForTest,
	*memory.Memory,
	job.RunSequencerForTest,
) {
	t.Helper()
	repo := memory.New()
	routing := job.JobRunRoutingForTest{
		WorkspaceID: "ws1",
		CaseID:      42,
		JobID:       "job-A",
		RunID:       "run-1",
		TraceID:     "trace-1",
	}
	seq := job.NewRunSequencerForTest()
	clock := fixedClock(time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))
	h := job.NewJobRunTraceHandlerForTest(repo.JobRunEvent(), routing, seq, clock, nil)
	return h, repo, seq
}

func TestRunSequencer_Next_MonotonicUnderConcurrency(t *testing.T) {
	seq := job.NewRunSequencerForTest()
	const N = 1000
	var wg sync.WaitGroup
	results := make([]int64, N)
	for i := range N {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = seq.Next()
		}(i)
	}
	wg.Wait()

	seen := make(map[int64]bool, N)
	var minV, maxV int64 = int64(^uint64(0) >> 1), 0
	for _, v := range results {
		if seen[v] {
			t.Fatalf("duplicate sequence %d", v)
		}
		seen[v] = true
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	gt.Number(t, minV).Equal(1)
	gt.Number(t, maxV).Equal(N)
}

func TestJobRunTraceHandler_LLMCall_AppendsRequestAndResponse(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
	ctx := context.Background()

	ctxLLM := h.StartLLMCall(ctx)
	data := &trace.LLMCallData{
		Model:        "claude-opus-4-7",
		InputTokens:  120,
		OutputTokens: 60,
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
	// DurationMs is computed from the clock; fixedClock returns the same
	// time, so the difference is 0 ms.
	gt.Number(t, respEv.LLMResponse.DurationMs).Equal(0)
}

func TestJobRunTraceHandler_ToolExec_AppendsToolCallWithParent(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_ToolExec_Error(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_NSerialToolExecs_MonotonicSeq(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_EmitRunError_SharesSequencer(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_SubAgentLabel_RoundTrips(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_TruncatesLongFields(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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

func TestJobRunTraceHandler_EnterReflectionPhase_SetsPhase(t *testing.T) {
	h, repo, _ := newHandlerFixture(t)
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
	h.EnterReflectionPhaseForTest()

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
