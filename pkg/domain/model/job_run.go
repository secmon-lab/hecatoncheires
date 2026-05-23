package model

import (
	"time"

	"github.com/m-mizutani/goerr/v2"
)

// JobRunStatus enumerates the terminal outcome of the most recent Job run
// for a (workspace, case, job) tuple. It is set by JobRunRepository.RecordRun
// after the agent finishes (or fails). RUNNING is reserved for a future
// extension if we ever expose mid-flight observability — v1 does not write
// RUNNING; the in-flight signal is the lease_until column instead.
type JobRunStatus string

const (
	JobRunStatusSuccess JobRunStatus = "SUCCESS"
	JobRunStatusFailed  JobRunStatus = "FAILED"
)

// IsValid reports whether the status is a known enum member.
func (s JobRunStatus) IsValid() bool {
	switch s {
	case JobRunStatusSuccess, JobRunStatusFailed:
		return true
	default:
		return false
	}
}

// String returns the string form for logging.
func (s JobRunStatus) String() string { return string(s) }

// JobRunKey identifies a single (workspace, case, job) lock and run-record
// tuple. The tuple is the lock granularity for both lease acquisition and
// last-run bookkeeping.
type JobRunKey struct {
	WorkspaceID string
	CaseID      int64
	JobID       string
}

// Validate enforces that all three components are populated. Empty
// components produce ambiguous storage paths and corrupt the lock map.
func (k JobRunKey) Validate() error {
	if k.WorkspaceID == "" {
		return goerr.New("workspace id is empty")
	}
	if k.CaseID == 0 {
		return goerr.New("case id is zero")
	}
	if k.JobID == "" {
		return goerr.New("job id is empty")
	}
	return nil
}

// JobRun records the most recent state of a Job × Case pair: when it
// last ran, what happened, and (when in flight) the lease that prevents
// concurrent execution.
//
// The same document doubles as the lock record so a lease can be
// atomically taken or released in the same transaction that updates
// run history.
type JobRun struct {
	Key         JobRunKey
	LastRunAt   time.Time
	LastStatus  JobRunStatus
	LastError   string
	LastRunID   string
	LastTraceID string

	// LeaseUntil is the wall-clock time at which the current run lock
	// expires. Zero value means no live lease (= idle). A lease may be
	// reclaimed by another acquirer once LeaseUntil < now (clock
	// agreement is assumed; lease durations are large enough to absorb
	// minor skew).
	LeaseUntil time.Time
}

// IsLeased reports whether the JobRun currently holds a non-expired
// lease as of `now`. Used by acquirers to decide whether to take the
// lock or step aside.
func (r *JobRun) IsLeased(now time.Time) bool {
	if r == nil {
		return false
	}
	return now.Before(r.LeaseUntil)
}

// JobRunStage is the lifecycle stage of an individual Run (one invocation
// of a Job against a Case). Unlike JobRunStatus on the JobRun lock doc,
// JobRunStage includes RUNNING so a Run record exists while the agent is
// still executing — that way a crash mid-Run leaves a Stage=RUNNING log
// with the events captured up to that point.
type JobRunStage string

const (
	JobRunStageRunning JobRunStage = "RUNNING"
	JobRunStageSuccess JobRunStage = "SUCCESS"
	JobRunStageFailed  JobRunStage = "FAILED"
)

// IsValid reports whether the stage is a known enum member.
func (s JobRunStage) IsValid() bool {
	switch s {
	case JobRunStageRunning, JobRunStageSuccess, JobRunStageFailed:
		return true
	default:
		return false
	}
}

// String returns the string form for logging.
func (s JobRunStage) String() string { return string(s) }

// MaxInlineBytes is the soft cap on any single string/JSON payload field
// stored in Firestore (well under the 1 MiB doc limit). Payloads longer
// than this are truncated from the tail before persisting; this log is
// for rough flow tracing, not exact reproduction.
const MaxInlineBytes = 800 * 1024

// JobRunLog records ONE invocation of a Job against a Case. Stored at
// workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}/logs/{RunID}.
//
// Every identifier is held as a TOP-LEVEL scalar field (no nested Key
// struct) so that a Firestore -> BigQuery export yields a flat row that
// joins directly on WorkspaceID / CaseID / JobID / RunID / TraceID.
type JobRunLog struct {
	// Identifiers (all flat, all required, all populated on every write).
	WorkspaceID string
	CaseID      int64
	JobID       string
	RunID       string
	TraceID     string

	// Lifecycle.
	Stage     JobRunStage
	StartedAt time.Time
	EndedAt   time.Time // zero while RUNNING
	Error     string    // empty unless Stage == FAILED

	// Runtime identification (forward-compatible with plan-execute).
	// v1 always writes ExecutorKind="single_loop".
	ExecutorKind    string
	ExecutorVersion string

	// Provenance — copied from the triggering Event so a single Get tells
	// you why this Run happened.
	EventType      string
	EventTriggerAt time.Time

	// SystemPrompt is held once per Run, here, rather than inside every
	// LLMRequest event (it doesn't change turn-to-turn within a Run).
	// Truncated from the tail to MaxInlineBytes if longer.
	SystemPrompt string
}

// Validate enforces invariants on a JobRunLog. The repository calls this
// before every write (Create / Finish) so that a usecase bug that forgets
// to populate an identifier fails loudly at the first write instead of
// silently producing unattributable data.
func (l *JobRunLog) Validate() error {
	if l == nil {
		return goerr.New("job run log is nil")
	}
	if l.WorkspaceID == "" {
		return goerr.New("workspace id is empty")
	}
	if l.CaseID == 0 {
		return goerr.New("case id is zero")
	}
	if l.JobID == "" {
		return goerr.New("job id is empty")
	}
	if l.RunID == "" {
		return goerr.New("run id is empty")
	}
	if l.TraceID == "" {
		return goerr.New("trace id is empty")
	}
	if !l.Stage.IsValid() {
		return goerr.New("invalid stage", goerr.V("stage", string(l.Stage)))
	}
	if l.StartedAt.IsZero() {
		return goerr.New("started at is zero")
	}
	if l.ExecutorKind == "" {
		return goerr.New("executor kind is empty")
	}
	switch l.Stage {
	case JobRunStageRunning:
		if !l.EndedAt.IsZero() {
			return goerr.New("ended at must be zero while running")
		}
	case JobRunStageSuccess:
		if l.EndedAt.IsZero() {
			return goerr.New("ended at must be set on success")
		}
		if l.Error != "" {
			return goerr.New("error must be empty on success")
		}
	case JobRunStageFailed:
		if l.EndedAt.IsZero() {
			return goerr.New("ended at must be set on failure")
		}
	}
	if len(l.SystemPrompt) > MaxInlineBytes {
		return goerr.New("system prompt exceeds MaxInlineBytes (truncate before save)",
			goerr.V("len", len(l.SystemPrompt)))
	}
	return nil
}

// JobRunEventKind enumerates the per-call event types appended during a
// Run. Each maps to a specific gollem trace.Handler hook boundary:
//
//   - LLM_REQUEST  : input snapshot captured at EndLLMCall
//   - LLM_RESPONSE : output snapshot captured at EndLLMCall
//   - TOOL_CALL    : single tool execution captured at EndToolExec
//   - RUN_ERROR    : terminal failure emitted by JobRunner via EmitRunError
type JobRunEventKind string

const (
	JobRunEventKindLLMRequest  JobRunEventKind = "LLM_REQUEST"
	JobRunEventKindLLMResponse JobRunEventKind = "LLM_RESPONSE"
	JobRunEventKindToolCall    JobRunEventKind = "TOOL_CALL"
	JobRunEventKindRunError    JobRunEventKind = "RUN_ERROR"
)

// IsValid reports whether the kind is a known enum member.
func (k JobRunEventKind) IsValid() bool {
	switch k {
	case JobRunEventKindLLMRequest,
		JobRunEventKindLLMResponse,
		JobRunEventKindToolCall,
		JobRunEventKindRunError:
		return true
	default:
		return false
	}
}

// String returns the string form for logging.
func (k JobRunEventKind) String() string { return string(k) }

// JobRunEvent is one entry in the per-Run timeline. Stored at
// jobRuns/{JobID}/logs/{RunID}/events/{Sequence}. Identifiers are flat
// for BigQuery-friendliness; TraceID is duplicated here so trace-level
// analytics can group events without joining back to the log.
type JobRunEvent struct {
	// Identifiers (flat, all populated on every write).
	WorkspaceID string
	CaseID      int64
	JobID       string
	RunID       string
	TraceID     string

	// Position.
	// Sequence and ParentSequence are int64 (not uint64) so the
	// Firestore SDK can serialise them; the SDK rejects uint64. The
	// value space is still effectively unbounded for this workload
	// (max int64 = 9.2e18 events per Run).
	Sequence   int64 // monotonic within the Run; also the zero-padded doc ID
	OccurredAt time.Time
	Kind       JobRunEventKind

	// ParentSequence links a child event to its parent. Currently used
	// by ToolCall events to point back at the LLMResponse whose tool_use
	// they implement. Zero means top-level (LLMRequest / LLMResponse /
	// RunError are always top-level).
	ParentSequence int64

	// Forward-compatible runtime tagging. v1 SingleLoopJobExecutor emits
	// Phase="execute" and AgentLabel="" for every event. A future
	// plan-execute runtime may emit Phase="plan" / "execute" / "review"
	// and AgentLabel="planner" / "executor" / etc.
	Phase      string
	AgentLabel string

	// Payload (exactly one is non-nil and matches Kind).
	LLMRequest  *LLMRequestPayload
	LLMResponse *LLMResponsePayload
	ToolCall    *ToolCallPayload
	RunError    *RunErrorPayload
}

const phaseLabelMaxLen = 64

// Validate enforces invariants on a JobRunEvent. The sink calls this
// before every Append so that a payload-kind mismatch surfaces as an
// error at the boundary instead of producing unreadable docs.
func (e *JobRunEvent) Validate() error {
	if e == nil {
		return goerr.New("job run event is nil")
	}
	if e.WorkspaceID == "" {
		return goerr.New("workspace id is empty")
	}
	if e.CaseID == 0 {
		return goerr.New("case id is zero")
	}
	if e.JobID == "" {
		return goerr.New("job id is empty")
	}
	if e.RunID == "" {
		return goerr.New("run id is empty")
	}
	if e.TraceID == "" {
		return goerr.New("trace id is empty")
	}
	if e.Sequence == 0 {
		return goerr.New("sequence is zero")
	}
	if e.OccurredAt.IsZero() {
		return goerr.New("occurred at is zero")
	}
	if !e.Kind.IsValid() {
		return goerr.New("invalid kind", goerr.V("kind", string(e.Kind)))
	}
	if len(e.Phase) > phaseLabelMaxLen {
		return goerr.New("phase exceeds cap", goerr.V("len", len(e.Phase)))
	}
	if len(e.AgentLabel) > phaseLabelMaxLen {
		return goerr.New("agent label exceeds cap", goerr.V("len", len(e.AgentLabel)))
	}

	// Kind <-> payload pointer exclusivity. Exactly one payload must be
	// populated and it must match the declared Kind.
	populated := 0
	if e.LLMRequest != nil {
		populated++
	}
	if e.LLMResponse != nil {
		populated++
	}
	if e.ToolCall != nil {
		populated++
	}
	if e.RunError != nil {
		populated++
	}
	if populated != 1 {
		return goerr.New("exactly one payload pointer must be set",
			goerr.V("populated", populated),
			goerr.V("kind", string(e.Kind)))
	}
	switch e.Kind {
	case JobRunEventKindLLMRequest:
		if e.LLMRequest == nil {
			return goerr.New("LLMRequest payload required for kind LLM_REQUEST")
		}
	case JobRunEventKindLLMResponse:
		if e.LLMResponse == nil {
			return goerr.New("LLMResponse payload required for kind LLM_RESPONSE")
		}
	case JobRunEventKindToolCall:
		if e.ToolCall == nil {
			return goerr.New("ToolCall payload required for kind TOOL_CALL")
		}
		if e.ParentSequence == 0 {
			return goerr.New("ToolCall must reference a parent LLMResponse via ParentSequence")
		}
		if e.ParentSequence >= e.Sequence {
			return goerr.New("ParentSequence must be earlier than Sequence",
				goerr.V("parent", e.ParentSequence), goerr.V("seq", e.Sequence))
		}
	case JobRunEventKindRunError:
		if e.RunError == nil {
			return goerr.New("RunError payload required for kind RUN_ERROR")
		}
	}
	return nil
}

// LLMRequestPayload captures the input sent to the LLM API in one call,
// except the system prompt (which lives on JobRunLog because it doesn't
// change turn-to-turn). Maps to gollem trace.LLMRequest + LLMCallData.Model.
type LLMRequestPayload struct {
	Model    string        // LLMCallData.Model
	Messages []LLMMessage  // LLMRequest.Messages
	Tools    []LLMToolSpec // LLMRequest.Tools (name + description only)
}

// LLMResponsePayload captures one assistant response. gollem returns
// texts and function calls as separate arrays (no interleaving order
// info), so this payload mirrors that shape rather than fabricating an
// ordering we cannot observe.
type LLMResponsePayload struct {
	Model         string            // echoed back (LLMCallData.Model)
	Texts         []string          // LLMResponse.Texts
	FunctionCalls []LLMFunctionCall // LLMResponse.FunctionCalls
	InputTokens   int64             // LLMCallData.InputTokens
	OutputTokens  int64             // LLMCallData.OutputTokens
	DurationMs    int64             // wall-clock time between Start/End hook
}

// LLMFunctionCall mirrors gollem trace.FunctionCall.
type LLMFunctionCall struct {
	ID            string // provider-issued id
	Name          string
	ArgumentsJSON string // raw JSON of trace.FunctionCall.Arguments
}

// LLMMessage models one turn in the conversation history sent to the LLM.
// Mirrors gollem trace.Message.
type LLMMessage struct {
	Role     string
	Contents []LLMContentBlock
}

// LLMContentBlock is one chunk inside a message. Mirrors gollem
// trace.MessageContent — fields are populated only when relevant to Type.
type LLMContentBlock struct {
	Type string // "text" / "tool_call" / "tool_response" / "reasoning" / "image" / etc.

	Text string // text / reasoning

	// tool_call
	ID            string
	Name          string
	ArgumentsJSON string

	// tool_response
	ToolCallID string
	ResultJSON string

	// image / pdf / document / file
	MediaType string
	URL       string
	Title     string
}

// LLMToolSpec mirrors gollem trace.ToolSpec.
type LLMToolSpec struct {
	Name        string
	Description string
}

// ToolCallPayload is the full record of one tool execution. Maps to
// gollem trace.ToolExecData plus wall-clock timestamps captured by the
// handler at Start/End boundaries.
type ToolCallPayload struct {
	ToolName string

	ArgumentsJSON string // raw JSON of trace.ToolExecData.Args
	ResultJSON    string // raw JSON of trace.ToolExecData.Result; empty on pure error

	IsError      bool
	ErrorMessage string // trace.ToolExecData.Error when IsError

	StartedAt time.Time
	EndedAt   time.Time
}

// RunErrorPayload is the terminal log entry on a FAILED Run, emitted by
// JobRunner via the handler's EmitRunError API.
type RunErrorPayload struct {
	Stage   string // "prepare" / "execute" / "finish"
	Message string
}
