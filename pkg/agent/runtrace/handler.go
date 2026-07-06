// Package runtrace persists a case-scoped agent run as JobRunLog +
// JobRunEvent records so the case agent page can list it and show its
// per-call timeline.
//
// It is the single home for the gollem trace.Handler that turns LLM / tool
// call boundaries into JobRunEvent rows, shared by the Job runner
// (pkg/usecase/job) and the mention hosts (pkg/usecase/agent/casebound and
// .../threadcase). The Job runner drives its own JobRunLog lifecycle (lease,
// suspend/resume, reflection) and uses only Handler + Sequencer here; the
// mention hosts, which have no such lifecycle, use Recorder for the whole
// open/close.
package runtrace

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

// Sequencer hands out monotonically increasing JobRunEvent.Sequence values
// for a single run. The run's owner shares the SAME pointer with the Handler
// so that RUN_ERROR appends emitted by the owner and per-call appends emitted
// by the handler never collide on Sequence.
type Sequencer struct {
	mu   sync.Mutex
	next int64
}

// NewSequencer returns a sequencer whose first Next() returns 1.
func NewSequencer() *Sequencer {
	return &Sequencer{next: 1}
}

// NewSequencerStartingAt returns a sequencer whose first Next() returns start.
// Used when resuming a suspended run so the resumed turn's events continue
// past the suspended turn's events (which share the same RunID event space)
// instead of colliding on Sequence.
func NewSequencerStartingAt(start int64) *Sequencer {
	if start < 1 {
		start = 1
	}
	return &Sequencer{next: start}
}

// Next returns the next Sequence and advances the counter.
func (s *Sequencer) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.next
	s.next++
	return v
}

// Routing carries the immutable identifiers stamped on every JobRunEvent
// emitted by a Handler instance. Captured at construction time so individual
// hook calls do not need to thread them through.
type Routing struct {
	WorkspaceID string
	CaseID      int64
	JobID       string
	RunID       string
	TraceID     string
}

// handlerSpanKey is the context key under which Handler stashes per-span
// bookkeeping. Using a private struct ensures no cross-package collision with
// gollem's own span context.
type handlerSpanKey struct{}

// handlerSpan is the per-call bookkeeping the handler caches between a Start*
// hook and its matching End* hook.
type handlerSpan struct {
	kind      string // "llm" / "tool"
	startedAt time.Time

	// Tool-only.
	toolName string
	toolArgs map[string]any
}

// Handler is a gollem trace.Handler that appends one JobRunEvent per LLM call
// (LLM_REQUEST + LLM_RESPONSE) and one per tool execution (TOOL_CALL). It is
// wired once per run via gollem.WithTrace(handler) (or planexec's
// RunRequest.TraceHandler) and shares its Sequence allocator with the run's
// owner so that RUN_ERROR emits ordering-consistent with the per-call events.
type Handler struct {
	eventRepo interfaces.JobRunEventRepository
	routing   Routing
	seq       *Sequencer
	clock     func() time.Time
	truncator payloadTruncator

	// State mutated under mu by hook callbacks.
	mu                 sync.Mutex
	phase              string // current phase label; default "execute"
	agentLabel         string // current sub-agent label
	lastLLMResponseSeq int64  // most recent LLM_RESPONSE Sequence (for TOOL_CALL parents)
}

var _ trace.Handler = (*Handler)(nil)

// NewHandler constructs a Handler bound to the given run. clock defaults to
// time.Now().UTC() when nil.
func NewHandler(
	eventRepo interfaces.JobRunEventRepository,
	routing Routing,
	seq *Sequencer,
	clock func() time.Time,
) *Handler {
	if eventRepo == nil {
		panic("runtrace.Handler: eventRepo is nil")
	}
	if seq == nil {
		panic("runtrace.Handler: seq is nil")
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Handler{
		eventRepo: eventRepo,
		routing:   routing,
		seq:       seq,
		clock:     clock,
		truncator: defaultPayloadTruncator{},
		phase:     phaseExecute,
	}
}

// phaseExecute is the default phase label carried by every event until the
// owner transitions the handler to another phase.
const phaseExecute = "execute"

// StartAgentExecute is a no-op: run lifecycle is tracked by JobRunLog, not the
// per-event timeline.
func (h *Handler) StartAgentExecute(ctx context.Context) context.Context {
	return ctx
}

// EndAgentExecute is a no-op for the same reason.
func (h *Handler) EndAgentExecute(ctx context.Context, err error) {}

// StartLLMCall records the start time in a context-scoped span. No event is
// appended yet; we have no LLMCallData until End.
func (h *Handler) StartLLMCall(ctx context.Context) context.Context {
	return context.WithValue(ctx, handlerSpanKey{}, &handlerSpan{
		kind:      "llm",
		startedAt: h.clock(),
	})
}

// EndLLMCall appends two events: LLM_REQUEST built from data.Request +
// data.Model, then LLM_RESPONSE built from data.Response + tokens + elapsed
// wall-clock. If err is non-nil the response event still captures whatever
// partial data was returned; the surrounding run failure is recorded
// separately via EmitRunError.
func (h *Handler) EndLLMCall(ctx context.Context, data *trace.LLMCallData, err error) {
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

	// Track the response Sequence so subsequent TOOL_CALL events can point
	// ParentSequence at it.
	h.mu.Lock()
	h.lastLLMResponseSeq = respEv.Sequence
	h.mu.Unlock()
}

// StartToolExec caches the tool name + args + start timestamp on a
// context-scoped span. No event is appended yet; we need the result before we
// can emit a complete TOOL_CALL.
func (h *Handler) StartToolExec(ctx context.Context, toolName string, args map[string]any) context.Context {
	return context.WithValue(ctx, handlerSpanKey{}, &handlerSpan{
		kind:      "tool",
		startedAt: h.clock(),
		toolName:  toolName,
		toolArgs:  args,
	})
}

// EndToolExec appends a single TOOL_CALL event with ParentSequence pointing at
// the most recent LLM_RESPONSE seen by this handler.
func (h *Handler) EndToolExec(ctx context.Context, result map[string]any, err error) {
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
// AgentLabel. The single-loop path never triggers this; planexec sub-agents do.
func (h *Handler) StartSubAgent(ctx context.Context, name string) context.Context {
	h.mu.Lock()
	h.agentLabel = name
	h.mu.Unlock()
	return ctx
}

// EndSubAgent clears the AgentLabel back to empty.
func (h *Handler) EndSubAgent(ctx context.Context, err error) {
	h.mu.Lock()
	h.agentLabel = ""
	h.mu.Unlock()
}

// StartChildAgent mirrors StartSubAgent for the child-agent variant.
func (h *Handler) StartChildAgent(ctx context.Context, name string) context.Context {
	return h.StartSubAgent(ctx, name)
}

// EndChildAgent mirrors EndSubAgent.
func (h *Handler) EndChildAgent(ctx context.Context, err error) {
	h.EndSubAgent(ctx, err)
}

// AddEvent is a no-op; reserved for future debug-event payloads.
func (h *Handler) AddEvent(ctx context.Context, kind string, data any) {}

// Finish is a no-op: events have been appended per-call. The hook is kept for
// trace.Handler contract compatibility.
func (h *Handler) Finish(ctx context.Context) error { return nil }

// phaseReflection labels the events emitted by the post-execution reflection
// agent so they are distinguishable from the main "execute" phase events in
// the JobRunEvent timeline.
const phaseReflection = "reflection"

// EnterReflectionPhase relabels subsequent events as the reflection phase.
// The Job runner calls it once, after the executor returns and before invoking
// the reflector, so the reflection agent's LLM / tool events are attributed to
// "reflection". Safe because the run is single-threaded at that point.
func (h *Handler) EnterReflectionPhase() {
	h.mu.Lock()
	h.phase = phaseReflection
	h.mu.Unlock()
}

// EmitRunError appends a RUN_ERROR event using the shared sequencer. Called by
// the run's owner on lifecycle failures (prepare / execute / finish stages).
func (h *Handler) EmitRunError(ctx context.Context, stage, message string) error {
	ev := h.baseEvent(model.JobRunEventKindRunError, h.clock())
	ev.RunError = &model.RunErrorPayload{
		Stage:   stage,
		Message: Truncate(message, model.MaxInlineBytes),
	}
	if err := h.eventRepo.Append(ctx, ev); err != nil {
		return goerr.Wrap(err, "append run_error event",
			goerr.V("run_id", h.routing.RunID))
	}
	return nil
}

// baseEvent stamps the common identifier / phase / sequence / occurred-at
// fields onto a fresh event. The caller fills in the kind-specific payload.
// EventID is a freshly minted UUIDv7 (timestamp-prefixed for Firestore-console
// readability); the authoritative monotonic order is the Sequence field, not
// the doc ID.
func (h *Handler) baseEvent(kind model.JobRunEventKind, at time.Time) *model.JobRunEvent {
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
// errutil.Handle so a single bad event does not drop the rest of the run's
// trail.
func (h *Handler) append(ctx context.Context, ev *model.JobRunEvent) {
	if err := h.eventRepo.Append(ctx, ev); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "append job run event",
			goerr.V("run_id", h.routing.RunID),
			goerr.V("sequence", ev.Sequence),
			goerr.V("kind", string(ev.Kind))),
			"runtrace: persist agent trace event")
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

// payloadTruncator builds the kind-specific payloads from gollem trace data,
// applying length caps so the resulting Firestore doc stays well under the
// 1 MiB limit.
type payloadTruncator interface {
	LLMRequestFromTrace(data *trace.LLMCallData) *model.LLMRequestPayload
	LLMResponseFromTrace(data *trace.LLMCallData, durationMs int64) *model.LLMResponsePayload
	ToolCallFromTrace(toolName string, args, result map[string]any, err error, startedAt, endedAt time.Time) *model.ToolCallPayload
}

// defaultPayloadTruncator is the production implementation: it copies gollem
// trace data into our payload types, applying Truncate to every text/JSON
// field.
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
				Name:        Truncate(ts.Name, model.MaxInlineBytes),
				Description: Truncate(ts.Description, model.MaxInlineBytes),
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
			p.Texts = append(p.Texts, Truncate(txt, model.MaxInlineBytes))
		}
		p.FunctionCalls = make([]model.LLMFunctionCall, 0, len(data.Response.FunctionCalls))
		for _, fc := range data.Response.FunctionCalls {
			if fc == nil {
				continue
			}
			p.FunctionCalls = append(p.FunctionCalls, model.LLMFunctionCall{
				ID:            fc.ID,
				Name:          fc.Name,
				ArgumentsJSON: Truncate(marshalJSON(fc.Arguments), model.MaxInlineBytes),
			})
		}
	}
	return p
}

func (defaultPayloadTruncator) ToolCallFromTrace(toolName string, args, result map[string]any, err error, startedAt, endedAt time.Time) *model.ToolCallPayload {
	tc := &model.ToolCallPayload{
		ToolName:      toolName,
		ArgumentsJSON: Truncate(marshalJSON(args), model.MaxInlineBytes),
		ResultJSON:    Truncate(marshalJSON(result), model.MaxInlineBytes),
		StartedAt:     startedAt,
		EndedAt:       endedAt,
	}
	if err != nil {
		tc.IsError = true
		tc.ErrorMessage = Truncate(err.Error(), model.MaxInlineBytes)
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
		Text:       Truncate(c.Text, model.MaxInlineBytes),
		ID:         c.ID,
		Name:       c.Name,
		ToolCallID: c.ToolCallID,
		MediaType:  c.MediaType,
		URL:        c.URL,
		Title:      c.Title,
	}
	if c.Arguments != nil {
		block.ArgumentsJSON = Truncate(marshalJSON(c.Arguments), model.MaxInlineBytes)
	}
	if c.Result != nil {
		block.ResultJSON = Truncate(marshalJSON(c.Result), model.MaxInlineBytes)
	}
	return block
}

// marshalJSON serialises v to JSON, falling back to an empty string on error
// or nil-ish input. We never want a serialisation hiccup to abort the trace. A
// typed nil map (`map[string]any(nil)`) is treated as empty rather than
// literal "null" so empty payloads round-trip cleanly.
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

// Truncate returns s capped at max bytes, snapped back to a UTF-8 rune
// boundary so the result never contains a partial multi-byte character.
// Firestore rejects strings that are not valid UTF-8, and a blind byte-slice
// can leave a trailing fragment of a JP/CJK rune which would make the doc
// unwritable.
func Truncate(s string, max int) string {
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
