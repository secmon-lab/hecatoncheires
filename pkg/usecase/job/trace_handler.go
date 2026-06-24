package job

import (
	"context"
	"encoding/json"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gollem-dev/gollem/trace"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// runSequencer hands out monotonically increasing JobRunEvent.Sequence
// values for a single Run. JobRunner.Run owns one and shares the SAME
// pointer with the trace handler so that RUN_ERROR appends emitted by
// the runner and per-call appends emitted by the handler never collide
// on Sequence.
type runSequencer struct {
	mu   sync.Mutex
	next int64
}

// newRunSequencer returns a sequencer whose first Next() returns 1.
func newRunSequencer() *runSequencer {
	return &runSequencer{next: 1}
}

// newRunSequencerStartingAt returns a sequencer whose first Next() returns
// start. Used when resuming a suspended run so the resumed turn's events
// continue past the suspended turn's events (which share the same RunID
// event space) instead of colliding on Sequence.
func newRunSequencerStartingAt(start int64) *runSequencer {
	if start < 1 {
		start = 1
	}
	return &runSequencer{next: start}
}

// Next returns the next Sequence and advances the counter.
func (s *runSequencer) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.next
	s.next++
	return v
}

// jobRunRouting carries the immutable identifiers stamped on every
// JobRunEvent emitted by a handler instance. Captured at handler
// construction time so individual hook calls do not need to thread
// them through.
type jobRunRouting struct {
	WorkspaceID string
	CaseID      int64
	JobID       string
	RunID       string
	TraceID     string
}

// handlerSpanKey is the context key under which jobRunTraceHandler
// stashes per-span bookkeeping. Using a private struct ensures no
// cross-package collision with gollem's own span context.
type handlerSpanKey struct{}

// handlerSpan is the per-call bookkeeping the handler caches between a
// Start* hook and its matching End* hook.
type handlerSpan struct {
	kind      string // "llm" / "tool"
	startedAt time.Time

	// Tool-only.
	toolName string
	toolArgs map[string]any
}

// jobRunTraceHandler is a gollem trace.Handler that appends one
// JobRunEvent per LLM call (LLM_REQUEST + LLM_RESPONSE) and one per
// tool execution (TOOL_CALL). It is wired once per Run via
// gollem.WithTrace(handler) and shares its Sequence allocator with
// JobRunner.Run so that RUN_ERROR emits ordering-consistent with the
// per-call events.
type jobRunTraceHandler struct {
	eventRepo interfaces.JobRunEventRepository
	routing   jobRunRouting
	seq       *runSequencer
	clock     func() time.Time
	truncator payloadTruncator

	// State mutated under mu by hook callbacks.
	mu                 sync.Mutex
	phase              string // current phase label; v1 always "execute"
	agentLabel         string // current sub-agent label; v1 always ""
	lastLLMResponseSeq int64  // most recent LLM_RESPONSE Sequence (for TOOL_CALL parents)
}

var _ trace.Handler = (*jobRunTraceHandler)(nil)

// newJobRunTraceHandler constructs a handler bound to the given Run.
// clock defaults to time.Now when nil. truncator defaults to a built-in
// implementation that enforces model.MaxInlineBytes when nil.
func newJobRunTraceHandler(
	eventRepo interfaces.JobRunEventRepository,
	routing jobRunRouting,
	seq *runSequencer,
	clock func() time.Time,
	truncator payloadTruncator,
) *jobRunTraceHandler {
	if eventRepo == nil {
		panic("jobRunTraceHandler: eventRepo is nil")
	}
	if seq == nil {
		panic("jobRunTraceHandler: seq is nil")
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if truncator == nil {
		truncator = defaultPayloadTruncator{}
	}
	return &jobRunTraceHandler{
		eventRepo: eventRepo,
		routing:   routing,
		seq:       seq,
		clock:     clock,
		truncator: truncator,
		phase:     "execute",
	}
}

// StartAgentExecute is a no-op: Run lifecycle is tracked by JobRunLog,
// not the per-event timeline.
func (h *jobRunTraceHandler) StartAgentExecute(ctx context.Context) context.Context {
	return ctx
}

// EndAgentExecute is a no-op for the same reason.
func (h *jobRunTraceHandler) EndAgentExecute(ctx context.Context, err error) {}

// StartLLMCall records the start time in a context-scoped span. No
// event is appended yet; we have no LLMCallData until End.
func (h *jobRunTraceHandler) StartLLMCall(ctx context.Context) context.Context {
	return context.WithValue(ctx, handlerSpanKey{}, &handlerSpan{
		kind:      "llm",
		startedAt: h.clock(),
	})
}

// EndLLMCall appends two events: LLM_REQUEST built from data.Request +
// data.Model, then LLM_RESPONSE built from data.Response + tokens +
// elapsed wall-clock. If err is non-nil the response event still
// captures whatever partial data was returned; the surrounding Run
// failure is recorded separately via EmitRunError.
func (h *jobRunTraceHandler) EndLLMCall(ctx context.Context, data *trace.LLMCallData, err error) {
	span := spanFromContext(ctx)
	endedAt := h.clock()
	var durationMs int64
	if span != nil && !span.startedAt.IsZero() {
		durationMs = max(endedAt.Sub(span.startedAt).Milliseconds(), 0)
	}

	if data == nil {
		// Nothing to record; we still skip rather than fabricate.
		return
	}

	// LLM_REQUEST
	reqEv := h.baseEvent(model.JobRunEventKindLLMRequest, endedAt)
	reqEv.LLMRequest = h.truncator.LLMRequestFromTrace(data)
	h.append(ctx, reqEv)

	// LLM_RESPONSE
	respEv := h.baseEvent(model.JobRunEventKindLLMResponse, endedAt)
	respEv.LLMResponse = h.truncator.LLMResponseFromTrace(data, durationMs)
	h.append(ctx, respEv)

	// Track the response Sequence so subsequent TOOL_CALL events can
	// point ParentSequence at it.
	h.mu.Lock()
	h.lastLLMResponseSeq = respEv.Sequence
	h.mu.Unlock()
}

// StartToolExec caches the tool name + args + start timestamp on a
// context-scoped span. No event is appended yet; we need the result
// before we can emit a complete TOOL_CALL.
func (h *jobRunTraceHandler) StartToolExec(ctx context.Context, toolName string, args map[string]any) context.Context {
	return context.WithValue(ctx, handlerSpanKey{}, &handlerSpan{
		kind:      "tool",
		startedAt: h.clock(),
		toolName:  toolName,
		toolArgs:  args,
	})
}

// EndToolExec appends a single TOOL_CALL event with ParentSequence
// pointing at the most recent LLM_RESPONSE seen by this handler.
func (h *jobRunTraceHandler) EndToolExec(ctx context.Context, result map[string]any, err error) {
	span := spanFromContext(ctx)
	endedAt := h.clock()

	ev := h.baseEvent(model.JobRunEventKindToolCall, endedAt)
	h.mu.Lock()
	ev.ParentSequence = h.lastLLMResponseSeq
	h.mu.Unlock()

	var startedAt time.Time
	var toolName string
	var args map[string]any
	if span != nil && span.kind == "tool" {
		startedAt = span.startedAt
		toolName = span.toolName
		args = span.toolArgs
	}
	if startedAt.IsZero() {
		startedAt = endedAt
	}

	ev.ToolCall = h.truncator.ToolCallFromTrace(toolName, args, result, err, startedAt, endedAt)
	h.append(ctx, ev)
}

// StartSubAgent flags the handler so subsequent events carry the named
// AgentLabel. v1 SingleLoopJobExecutor never triggers this, but the
// hook is wired so a future plan-execute runtime light up automatically.
func (h *jobRunTraceHandler) StartSubAgent(ctx context.Context, name string) context.Context {
	h.mu.Lock()
	h.agentLabel = name
	h.mu.Unlock()
	return ctx
}

// EndSubAgent clears the AgentLabel back to empty.
func (h *jobRunTraceHandler) EndSubAgent(ctx context.Context, err error) {
	h.mu.Lock()
	h.agentLabel = ""
	h.mu.Unlock()
}

// StartChildAgent mirrors StartSubAgent for the child-agent variant.
func (h *jobRunTraceHandler) StartChildAgent(ctx context.Context, name string) context.Context {
	return h.StartSubAgent(ctx, name)
}

// EndChildAgent mirrors EndSubAgent.
func (h *jobRunTraceHandler) EndChildAgent(ctx context.Context, err error) {
	h.EndSubAgent(ctx, err)
}

// AddEvent is a no-op in v1; reserved for future debug-event payloads.
func (h *jobRunTraceHandler) AddEvent(ctx context.Context, kind string, data any) {}

// Finish is a no-op: events have been appended per-call. The hook is
// kept for trace.Handler contract compatibility.
func (h *jobRunTraceHandler) Finish(ctx context.Context) error { return nil }

// phaseReflection labels the events emitted by the post-execution reflection
// agent so they are distinguishable from the main "execute" phase events in the
// JobRunEvent timeline.
const phaseReflection = "reflection"

// enterReflectionPhase relabels subsequent events as the reflection phase.
// JobRunner.Run calls it once, after the executor returns and before invoking
// the reflector, so the reflection agent's LLM / tool events are attributed to
// "reflection". Safe because the run is single-threaded at that point.
func (h *jobRunTraceHandler) enterReflectionPhase() {
	h.mu.Lock()
	h.phase = phaseReflection
	h.mu.Unlock()
}

// EmitRunError appends a RUN_ERROR event using the shared sequencer.
// Called by JobRunner.Run on lifecycle failures (prepare / execute /
// finish stages).
func (h *jobRunTraceHandler) EmitRunError(ctx context.Context, stage, message string) error {
	ev := h.baseEvent(model.JobRunEventKindRunError, h.clock())
	ev.RunError = &model.RunErrorPayload{
		Stage:   stage,
		Message: truncateString(message, model.MaxInlineBytes),
	}
	if err := h.eventRepo.Append(ctx, ev); err != nil {
		return goerr.Wrap(err, "append run_error event",
			goerr.V("run_id", h.routing.RunID))
	}
	return nil
}

// baseEvent stamps the common identifier / phase / sequence / occurred-at
// fields onto a fresh event. The caller fills in the kind-specific
// payload. EventID is a freshly minted UUIDv7 (timestamp-prefixed for
// Firestore-console readability); the authoritative monotonic order is
// the Sequence field, not the doc ID.
func (h *jobRunTraceHandler) baseEvent(kind model.JobRunEventKind, at time.Time) *model.JobRunEvent {
	h.mu.Lock()
	phase := h.phase
	agentLabel := h.agentLabel
	h.mu.Unlock()
	return &model.JobRunEvent{
		WorkspaceID: h.routing.WorkspaceID,
		CaseID:      h.routing.CaseID,
		JobID:       h.routing.JobID,
		RunID:       h.routing.RunID,
		TraceID:     h.routing.TraceID,
		EventID:     uuid.Must(uuid.NewV7()).String(),
		Sequence:    h.seq.Next(),
		OccurredAt:  at,
		Kind:        kind,
		Phase:       phase,
		AgentLabel:  agentLabel,
	}
}

// append persists the event. Per-event failures are non-fatal: log via
// errutil.Handle so a single bad event does not drop the rest of the
// Run's trail.
func (h *jobRunTraceHandler) append(ctx context.Context, ev *model.JobRunEvent) {
	if err := h.eventRepo.Append(ctx, ev); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "append job run event",
			goerr.V("run_id", h.routing.RunID),
			goerr.V("sequence", ev.Sequence),
			goerr.V("kind", string(ev.Kind))),
			"job: persist agent trace event")
		return
	}
	if logger := logging.From(ctx); logger != nil {
		logger.Debug("appended job run event",
			"run_id", h.routing.RunID,
			"sequence", ev.Sequence,
			"kind", string(ev.Kind))
	}
}

func spanFromContext(ctx context.Context) *handlerSpan {
	s, _ := ctx.Value(handlerSpanKey{}).(*handlerSpan)
	return s
}

// payloadTruncator builds the kind-specific payloads from gollem trace
// data, applying length caps so the resulting Firestore doc stays well
// under the 1 MiB limit. Implemented as an interface so tests can plug
// in a no-op truncator for assertion clarity.
type payloadTruncator interface {
	LLMRequestFromTrace(data *trace.LLMCallData) *model.LLMRequestPayload
	LLMResponseFromTrace(data *trace.LLMCallData, durationMs int64) *model.LLMResponsePayload
	ToolCallFromTrace(toolName string, args, result map[string]any, err error, startedAt, endedAt time.Time) *model.ToolCallPayload
}

// defaultPayloadTruncator is the production implementation: it copies
// gollem trace data into our payload types, applying truncateString to
// every text/JSON field.
type defaultPayloadTruncator struct{}

func (defaultPayloadTruncator) LLMRequestFromTrace(data *trace.LLMCallData) *model.LLMRequestPayload {
	if data == nil {
		return &model.LLMRequestPayload{}
	}
	p := &model.LLMRequestPayload{Model: data.Model}
	if data.Request != nil {
		p.Messages = make([]model.LLMMessage, 0, len(data.Request.Messages))
		for _, m := range data.Request.Messages {
			p.Messages = append(p.Messages, convertMessage(m))
		}
		p.Tools = make([]model.LLMToolSpec, 0, len(data.Request.Tools))
		for _, ts := range data.Request.Tools {
			p.Tools = append(p.Tools, model.LLMToolSpec{
				Name:        truncateString(ts.Name, model.MaxInlineBytes),
				Description: truncateString(ts.Description, model.MaxInlineBytes),
			})
		}
	}
	return p
}

func (defaultPayloadTruncator) LLMResponseFromTrace(data *trace.LLMCallData, durationMs int64) *model.LLMResponsePayload {
	if data == nil {
		return &model.LLMResponsePayload{DurationMs: durationMs}
	}
	p := &model.LLMResponsePayload{
		Model:        data.Model,
		InputTokens:  int64(data.InputTokens),
		OutputTokens: int64(data.OutputTokens),
		DurationMs:   durationMs,
	}
	if data.Response != nil {
		p.Texts = make([]string, 0, len(data.Response.Texts))
		for _, txt := range data.Response.Texts {
			p.Texts = append(p.Texts, truncateString(txt, model.MaxInlineBytes))
		}
		p.FunctionCalls = make([]model.LLMFunctionCall, 0, len(data.Response.FunctionCalls))
		for _, fc := range data.Response.FunctionCalls {
			if fc == nil {
				continue
			}
			p.FunctionCalls = append(p.FunctionCalls, model.LLMFunctionCall{
				ID:            fc.ID,
				Name:          fc.Name,
				ArgumentsJSON: truncateString(marshalJSON(fc.Arguments), model.MaxInlineBytes),
			})
		}
	}
	return p
}

func (defaultPayloadTruncator) ToolCallFromTrace(toolName string, args, result map[string]any, err error, startedAt, endedAt time.Time) *model.ToolCallPayload {
	tc := &model.ToolCallPayload{
		ToolName:      toolName,
		ArgumentsJSON: truncateString(marshalJSON(args), model.MaxInlineBytes),
		ResultJSON:    truncateString(marshalJSON(result), model.MaxInlineBytes),
		StartedAt:     startedAt,
		EndedAt:       endedAt,
	}
	if err != nil {
		tc.IsError = true
		tc.ErrorMessage = truncateString(err.Error(), model.MaxInlineBytes)
	}
	return tc
}

func convertMessage(m trace.Message) model.LLMMessage {
	out := model.LLMMessage{
		Role:     m.Role,
		Contents: make([]model.LLMContentBlock, 0, len(m.Contents)),
	}
	for _, c := range m.Contents {
		out.Contents = append(out.Contents, convertContent(c))
	}
	return out
}

func convertContent(c trace.MessageContent) model.LLMContentBlock {
	block := model.LLMContentBlock{
		Type:       c.Type,
		Text:       truncateString(c.Text, model.MaxInlineBytes),
		ID:         c.ID,
		Name:       c.Name,
		ToolCallID: c.ToolCallID,
		MediaType:  c.MediaType,
		URL:        c.URL,
		Title:      c.Title,
	}
	if c.Arguments != nil {
		block.ArgumentsJSON = truncateString(marshalJSON(c.Arguments), model.MaxInlineBytes)
	}
	if c.Result != nil {
		block.ResultJSON = truncateString(marshalJSON(c.Result), model.MaxInlineBytes)
	}
	return block
}

// marshalJSON serialises v to JSON, falling back to an empty string on
// error or nil-ish input. We never want a serialisation hiccup to abort
// the trace. A typed nil map (`map[string]any(nil)`) is treated as empty
// rather than literal "null" so empty payloads round-trip cleanly.
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	if m, ok := v.(map[string]any); ok && m == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// truncateString returns s capped at max bytes, snapped back to a UTF-8
// rune boundary so the result never contains a partial multi-byte
// character. Firestore rejects strings that are not valid UTF-8, and a
// blind byte-slice can leave a trailing fragment of a JP/CJK rune which
// would make the doc unwritable.
func truncateString(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
