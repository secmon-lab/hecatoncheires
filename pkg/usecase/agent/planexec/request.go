package planexec

import (
	"context"
	"encoding/json"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
)

// RunRequest is the argument bundle handed to Runner.Run. Required and
// optional fields are documented per-field; Validate() enforces the
// required-field contract at the entry point so host wiring bugs (Sink
// nil, OnQuestion missing when AllowQuestion=true, ...) fail loud before
// the first LLM call.
type RunRequest struct {
	// --- Identity / persistence -------------------------------------

	// HistoryKey is the gollem.WithHistoryRepository key. Required.
	// Used to bind every planner / replan call to one continuous
	// conversation history.
	HistoryKey string

	// TraceID is the trace.WithTraceID value handed to the trace.Recorder.
	// Required.
	TraceID string

	// TraceMetadata is attached to the trace.Recorder as labels.
	// Optional — zero value is fine.
	TraceMetadata trace.TraceMetadata

	// TraceHandler, when non-nil, is wired (in addition to the run's
	// internal archive recorder) into every gollem agent the run drives —
	// planner, sub-agents, direct, and final — so a host can capture
	// per-call LLM / tool events for its own timeline. The Job host
	// supplies its jobRunTraceHandler here so planexec Jobs populate the
	// JobRunEvent timeline the same way the single-loop executor does.
	// Optional; nil preserves the archive-only behaviour (the proposal
	// host passes nil).
	TraceHandler trace.Handler

	// LanguageLabel is interpolated into the planner system prompt's
	// "respond in" directive (e.g. "Japanese", "English"). Optional —
	// empty disables the directive.
	LanguageLabel string

	// --- Initial input ---------------------------------------------

	// UserInput is the very first user message handed to the planner.
	// Required.
	UserInput string

	// --- Planner customisation -------------------------------------

	// SystemPrompt is the planner's base system prompt. Required.
	// Host-specific guidance lives here; planexec appends only the
	// budget / action-shape boilerplate it owns.
	SystemPrompt string

	// PlannerTools are tools the planner itself is allowed to call
	// during a round (e.g. proposal's wsmeta tools). Optional — nil
	// means the planner is tool-less.
	PlannerTools []gollem.Tool

	// AllowDirect lets the planner short-circuit round 1: when the request
	// is trivially answerable without any investigation phase, the planner
	// may emit a `direct` payload instead of `tasks`, and the runtime
	// answers in a single tool-enabled ReAct loop. The direct path is plain
	// text only — FinalOutputSchema / OnFinalize are NOT used on it (those
	// are reserved for the investigate path's structured terminal). Hosts
	// opt in (proposal-style structured-only flows leave this false).
	// Default false.
	AllowDirect bool

	// --- Phase execution -------------------------------------------

	// ToolResolver maps TaskPlan.Tools entries into concrete gollem
	// tool slices for each sub-agent. Required.
	ToolResolver ToolResolver

	// KnownToolIDs is the enum the planner sees in the JSON schema's
	// task.tools field. Required (must be non-empty).
	KnownToolIDs []string

	// --- Question (HITL with the user, not tool approval) ----------

	// AllowQuestion toggles the question section in the planner prompt
	// and schema. proposal sets true; job sets false.
	AllowQuestion bool

	// OnQuestion is the host callback fired when the planner emits a
	// Question. Required iff AllowQuestion is true.
	OnQuestion func(ctx context.Context, q Question) (QuestionResult, error)

	// --- Final output ----------------------------------------------

	// FinalOutputSchema is the gollem response schema applied to the
	// final-response LLM call after the loop exits. nil → plain text
	// in RunResult.FinalText; non-nil → JSON in RunResult.FinalRaw.
	// proposal passes its materialize schema; job passes nil.
	FinalOutputSchema *gollem.Parameter

	// OnFinalize is invoked with the structured final output once the
	// planner loop decides to terminate. The host parses + validates the
	// output AND performs the terminal side effect (e.g. create the Case).
	// When it returns a non-nil error — whether validation failed OR the
	// side effect (persistence) failed — the runner folds the error back
	// into the next planner round (charged against PlannerLoopMax) so the
	// planner can investigate / ask / re-emit until it succeeds or the
	// round budget is exhausted. nil → the turn completes. Optional; only
	// meaningful when FinalOutputSchema != nil. When nil, the final output
	// is returned as-is (existing proposal / job behaviour is unaffected).
	OnFinalize func(ctx context.Context, raw json.RawMessage) error

	// --- Output ----------------------------------------------------

	// Sink is the progress / state output channel. Required (use
	// SinkFuncs{} for a no-op).
	Sink Sink
}

// Validate enforces the required-field contract. Runner.Run MUST call
// this at the top before doing any work.
func (r *RunRequest) Validate() error {
	if r == nil {
		return goerr.New("run request is nil")
	}
	if r.HistoryKey == "" {
		return goerr.New("history key is required")
	}
	if r.TraceID == "" {
		return goerr.New("trace id is required")
	}
	if r.UserInput == "" {
		return goerr.New("user input is required")
	}
	if r.SystemPrompt == "" {
		return goerr.New("system prompt is required")
	}
	if r.ToolResolver == nil {
		return goerr.New("tool resolver is required")
	}
	if len(r.KnownToolIDs) == 0 {
		return goerr.New("known tool ids must not be empty")
	}
	if r.AllowQuestion && r.OnQuestion == nil {
		return goerr.New("on question callback is required when AllowQuestion is true")
	}
	if r.OnFinalize != nil && r.FinalOutputSchema == nil {
		return goerr.New("final output schema is required when OnFinalize is set")
	}
	if r.Sink == nil {
		return goerr.New("sink is required")
	}
	return nil
}

// RunStatus is the terminal classification of a Runner.Run.
type RunStatus int

const (
	// StatusCompleted means the loop exited naturally and the
	// final-response phase produced FinalText or FinalRaw (depending on
	// FinalOutputSchema). Also used when OnQuestion returned
	// QuestionResult{Terminate: true} — the host has acknowledged the
	// question and will resume in a later turn.
	StatusCompleted RunStatus = iota
	// StatusFallbackBudget means the planner / sub-agent budget was
	// exhausted before the loop could terminate. FinalText / FinalRaw
	// are empty; the host should render a "couldn't reach a conclusion"
	// message of its choice.
	StatusFallbackBudget
	// StatusFallbackError means an internal error path made the loop
	// give up before terminating (e.g. final-response LLM call failed
	// after the loop body succeeded). FallbackReason carries the
	// human-readable cause.
	StatusFallbackError
)

// RunResult is the outcome of Runner.Run. Exactly one of FinalText /
// FinalRaw is populated when Status == StatusCompleted, governed by
// RunRequest.FinalOutputSchema:
//   - schema == nil → FinalText holds the plain-text final response.
//   - schema != nil → FinalRaw holds the raw JSON bytes; the host
//     unmarshals into its concrete payload struct.
//
// AllResults is the per-phase observation trail surfaced to the host
// regardless of status, so a fallback can still log what was learnt.
type RunResult struct {
	Status         RunStatus
	FinalText      string
	FinalRaw       json.RawMessage
	AllResults     []PhaseSummary
	FallbackReason string
	// EndedWithQuestion is true when the loop exited because OnQuestion
	// returned QuestionResult{Terminate: true}. Status is still
	// StatusCompleted in this case; the host uses this flag to
	// distinguish "we answered the user" vs "we asked the user".
	EndedWithQuestion bool
	// Direct is true when the turn terminated via the round-1 direct path
	// (no investigation phase ran). FinalText holds the plain-text reply and
	// FinalRaw is nil regardless of FinalOutputSchema. Hosts that normally
	// parse FinalRaw (structured final) MUST treat FinalText as the
	// user-facing reply when Direct is true.
	Direct bool
}
