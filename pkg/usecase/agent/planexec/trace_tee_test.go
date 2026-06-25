package planexec_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// recordingTraceHandler records every hook invocation so tests can
// assert the fan-out reached this backend. It also writes a unique key
// into the context on each Start* hook so ctx chaining is observable.
// All state is mutex-guarded so it can be shared across the parallel
// sub-agent goroutines planexec dispatches.
type recordingTraceHandler struct {
	name string

	mu            sync.Mutex
	agentExecutes int
	llmStarts     int
	llmEnds       int
	toolStarts    int
	toolEnds      int
	toolNames     []string
	addEvents     []string
	finishes      int
	finishErr     error

	// seenOtherKeyOnEnd records whether, on EndLLMCall, the context still
	// carried the value a *different* handler wrote on StartLLMCall —
	// proving the chained context survived across all backends.
	seenOtherKeyOnEnd bool
	otherKey          any
}

type ctxKey string

func (h *recordingTraceHandler) StartAgentExecute(ctx context.Context) context.Context {
	h.mu.Lock()
	h.agentExecutes++
	h.mu.Unlock()
	return context.WithValue(ctx, ctxKey(h.name), true)
}
func (h *recordingTraceHandler) EndAgentExecute(ctx context.Context, err error) {}

func (h *recordingTraceHandler) StartLLMCall(ctx context.Context) context.Context {
	h.mu.Lock()
	h.llmStarts++
	h.mu.Unlock()
	return context.WithValue(ctx, ctxKey(h.name), true)
}

func (h *recordingTraceHandler) EndLLMCall(ctx context.Context, data *trace.LLMCallData, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.llmEnds++
	if h.otherKey != nil {
		if v, ok := ctx.Value(h.otherKey).(bool); ok && v {
			h.seenOtherKeyOnEnd = true
		}
	}
}

func (h *recordingTraceHandler) StartToolExec(ctx context.Context, toolName string, args map[string]any) context.Context {
	h.mu.Lock()
	h.toolStarts++
	h.toolNames = append(h.toolNames, toolName)
	h.mu.Unlock()
	return ctx
}

func (h *recordingTraceHandler) EndToolExec(ctx context.Context, result map[string]any, err error) {
	h.mu.Lock()
	h.toolEnds++
	h.mu.Unlock()
}

func (h *recordingTraceHandler) StartSubAgent(ctx context.Context, name string) context.Context {
	return ctx
}
func (h *recordingTraceHandler) EndSubAgent(ctx context.Context, err error) {}
func (h *recordingTraceHandler) StartChildAgent(ctx context.Context, name string) context.Context {
	return ctx
}
func (h *recordingTraceHandler) EndChildAgent(ctx context.Context, err error) {}

func (h *recordingTraceHandler) AddEvent(ctx context.Context, kind string, data any) {
	h.mu.Lock()
	h.addEvents = append(h.addEvents, kind)
	h.mu.Unlock()
}

func (h *recordingTraceHandler) Finish(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.finishes++
	return h.finishErr
}

// snapshotAgentExecutes copies the StartAgentExecute count under the lock
// for race-free assertions after async.Wait(). Under a mock LLM client
// the per-call LLM hooks (StartLLMCall / EndLLMCall) are never fired —
// only the real claude/gemini/openai clients call them — so the number
// of agent executions is the observable signal that proves which agents
// were wired to the handler.
func (h *recordingTraceHandler) snapshotAgentExecutes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.agentExecutes
}

func (h *recordingTraceHandler) snapshotFinishes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finishes
}

func TestCombineTrace_CollapsesNoneAndSingle(t *testing.T) {
	gt.Value(t, planexec.CombineTraceForTest()).Nil()
	gt.Value(t, planexec.CombineTraceForTest(nil, nil)).Nil()

	only := &recordingTraceHandler{name: "only"}
	// A single non-nil handler is returned unchanged (no wrapper), so the
	// common path keeps zero overhead.
	got := planexec.CombineTraceForTest(nil, only, nil)
	gt.Value(t, got).Equal(trace.Handler(only))
}

func TestCombineTrace_BroadcastsToEveryBackend(t *testing.T) {
	a := &recordingTraceHandler{name: "a"}
	b := &recordingTraceHandler{name: "b"}
	// trace.Multi gives each backend its OWN isolated context. b's
	// EndLLMCall must see the key b wrote on its StartLLMCall (per-handler
	// threading works)...
	b.otherKey = ctxKey("b")
	// ...but a must NOT see b's key (no cross-backend contamination).
	a.otherKey = ctxKey("b")

	h := planexec.CombineTraceForTest(a, b)
	gt.Value(t, h).NotNil().Required()

	ctx := context.Background()
	ctx = h.StartAgentExecute(ctx)

	llmCtx := h.StartLLMCall(ctx)
	h.EndLLMCall(llmCtx, &trace.LLMCallData{Model: "test-model"}, nil)

	toolCtx := h.StartToolExec(ctx, "slack_search", map[string]any{"q": "x"})
	h.EndToolExec(toolCtx, map[string]any{"ok": true}, nil)

	h.AddEvent(ctx, "note", "payload")

	// Both backends saw every hook exactly once.
	for _, rec := range []*recordingTraceHandler{a, b} {
		gt.Number(t, rec.llmStarts).Equal(1)
		gt.Number(t, rec.llmEnds).Equal(1)
		gt.Number(t, rec.toolStarts).Equal(1)
		gt.Number(t, rec.toolEnds).Equal(1)
		gt.Array(t, rec.toolNames).Length(1).Required()
		gt.String(t, rec.toolNames[0]).Equal("slack_search")
		gt.Array(t, rec.addEvents).Length(1).Required()
		gt.String(t, rec.addEvents[0]).Equal("note")
	}

	// Per-handler context isolation: b sees its own Start value on End,
	// a never sees b's.
	gt.Bool(t, b.seenOtherKeyOnEnd).True()
	gt.Bool(t, a.seenOtherKeyOnEnd).False()
}

func TestCombineTrace_FinishJoinsErrors(t *testing.T) {
	errA := errors.New("backend a failed")
	errB := errors.New("backend b failed")
	a := &recordingTraceHandler{name: "a", finishErr: errA}
	b := &recordingTraceHandler{name: "b"}
	c := &recordingTraceHandler{name: "c", finishErr: errB}

	h := planexec.CombineTraceForTest(a, b, c)
	err := h.Finish(context.Background())

	// Every backend's Finish ran even though the first one errored.
	gt.Number(t, a.finishes).Equal(1)
	gt.Number(t, b.finishes).Equal(1)
	gt.Number(t, c.finishes).Equal(1)

	// Both errors are surfaced (joined), not just the first.
	gt.Value(t, err).NotNil().Required()
	gt.Bool(t, errors.Is(err, errA)).True()
	gt.Bool(t, errors.Is(err, errB)).True()
}
