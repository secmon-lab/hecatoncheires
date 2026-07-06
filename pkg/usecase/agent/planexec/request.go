package planexec

import (
	"context"

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
	// text only: even under Run[T] it returns RunResult.Text (not Data), since
	// side-effecting terminal actions must go through the investigate →
	// finalize loop. Hosts opt in (structured-only flows may still enable it
	// for a trivial text reply). Default false.
	AllowDirect bool

	// --- Phase execution -------------------------------------------

	// AllowSubAgentWrites lets this run's sub-agents perform writes /
	// side-effecting actions (post a message, change case status, ...) with
	// the tools the planner assigns them, instead of being restricted to
	// observation-only investigation. Default false → sub-agents are told
	// they are observation-only (the historical behaviour).
	//
	// The flag governs only the sub-agent prompt wording; the tools a
	// sub-agent can physically call are decided by ToolResolver. A caller
	// that sets this false MUST supply a read-only resolver so a sub-agent
	// is never handed a write tool it is then told not to use — that
	// prompt-vs-capability mismatch is exactly the failure this feature
	// fixes. A caller that sets it true supplies a resolver that includes
	// the write tools, and the planner assigns them per task.
	//
	// job sets true (Job deliverables are side effects, e.g. a Slack post);
	// threadcase sets true for mention turns (the sub-agent may close /
	// transition the case via case__update_case_status) and false for create
	// turns (no case to act on yet; the case is materialized by the host from
	// the structured final output).
	AllowSubAgentWrites bool

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
	//
	// The final output is chosen by the entry point, NOT by a request field:
	// Run[T] produces a validated structured *T, RunText / ResumeText produce
	// plain text. There is no OnFinalize commit callback — side effects are
	// performed by the sub-agents' tools inside the loop, and the host applies
	// whatever the returned *T describes.

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
	if r.Sink == nil {
		return goerr.New("sink is required")
	}
	return nil
}

// ResumeRequest re-enters a previously-suspended turn after the user has
// answered a Question. The turn ended earlier with
// QuestionResult{Terminate: true} (RunResult.EndedWithQuestion); the host
// persisted the question, collected the answer out-of-band, and now calls
// Resume.
//
// Resume re-enters planexec at a replan round (NOT a fresh plan), folding
// the answers into the first planner input. It deliberately carries no plan
// snapshot: the gollem conversation history keyed by RunRequest.HistoryKey
// already holds every prior round's observations, so the planner sees them
// on Load. The planner budget is fresh — the human answer is a natural
// checkpoint, and a strict carry-over would risk a stillborn resume when the
// budget was already near-exhausted at the moment the question was asked.
type ResumeRequest struct {
	// RunRequest carries the same identity / planner / tool / output
	// configuration as the original Run. HistoryKey MUST equal the
	// suspended run's key so the conversation continues. UserInput is
	// validated (non-empty) but not used to drive the first round — the
	// answers do; callers may set it to the original job prompt.
	RunRequest

	// Question is the question the planner asked before suspending,
	// reconstructed by the host from its persisted form. Used to label the
	// answers in the resumed planner input.
	Question Question

	// Answers is the user's reply, one entry per answered item. Required
	// (non-empty).
	Answers []QuestionAnswer
}

// Validate enforces the resume contract: the embedded RunRequest must be
// valid and at least one answer must be present.
func (r *ResumeRequest) Validate() error {
	if r == nil {
		return goerr.New("resume request is nil")
	}
	if err := r.RunRequest.Validate(); err != nil {
		return goerr.Wrap(err, "resume run request invalid")
	}
	if len(r.Answers) == 0 {
		return goerr.New("resume requires at least one answer")
	}
	return nil
}

// RunStatus is the terminal classification of a run.
type RunStatus int

const (
	// StatusCompleted means the loop terminated cleanly: either the planner
	// declared completion (finalize → final output produced), the round-1
	// direct path replied, or OnQuestion returned QuestionResult{Terminate:
	// true} (the host acknowledged the question and will resume in a later
	// turn — RunResult.EndedWithQuestion distinguishes this case).
	StatusCompleted RunStatus = iota
	// StatusFallbackBudget means the planner budget was exhausted before the
	// loop could terminate. RunResult.Data / Text are empty; the host should
	// render a "couldn't reach a conclusion" message of its choice.
	StatusFallbackBudget
	// StatusFallbackError means an internal error path made the run give up
	// before terminating (e.g. the direct reply failed, or the structured
	// final output failed to validate after retries). FallbackReason carries
	// the human-readable cause.
	StatusFallbackError
)
