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
//
// Identifiers are flat top-level fields (no nested JobRunKey) so a
// Firestore doc viewed in isolation surfaces "which (workspace, case,
// job) is this?" without having to parse the document path, and a
// BigQuery export yields rows that JOIN directly on WorkspaceID /
// CaseID / JobID with JobRunLog / JobRunEvent.
type JobRun struct {
	WorkspaceID string
	CaseID      int64
	JobID       string

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

	// SuspendedRunID, when non-empty, names a Run that is currently
	// suspended awaiting user input (Stage=AWAITING_INPUT in its
	// JobRunLog). While set, the scheduler/dispatcher MUST NOT start a new
	// Run for this (workspace, case, job) — the in-flight interactive Run
	// owns the slot until the user answers (resume clears it) or the
	// unanswered-run sweep expires it. The lease is released while
	// suspended (a human wait can outlast any lease), so SuspendedRunID is
	// the durable "do not double-start" signal, not the lease.
	SuspendedRunID string

	// SuspendedAt is when the current suspension began; the unanswered-run
	// sweep uses it to expire stale AWAITING_INPUT runs. Zero when not
	// suspended.
	SuspendedAt time.Time
}

// Key returns the composite JobRunKey for this run, reconstructed from
// the flat identifier fields. Kept for callers that still want to
// thread the composite around (scanner / runner helpers); the storage
// layer relies on the flat fields directly.
func (r *JobRun) Key() JobRunKey {
	if r == nil {
		return JobRunKey{}
	}
	return JobRunKey{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, JobID: r.JobID}
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

// IsSuspended reports whether a Run is currently suspended awaiting user
// input for this (workspace, case, job). New triggers must step aside
// while this is true.
func (r *JobRun) IsSuspended() bool {
	return r != nil && r.SuspendedRunID != ""
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
	// JobRunStageAwaitingInput marks an interactive Run that suspended to
	// ask the user a question. It is a non-terminal stage: EndedAt stays
	// zero and PendingInteraction carries the question so the resume path
	// can parse the answer and continue. Only interactive (planexec) Jobs
	// ever reach it.
	JobRunStageAwaitingInput JobRunStage = "AWAITING_INPUT"
)

// IsValid reports whether the stage is a known enum member.
func (s JobRunStage) IsValid() bool {
	switch s {
	case JobRunStageRunning, JobRunStageSuccess, JobRunStageFailed, JobRunStageAwaitingInput:
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

	// PendingInteraction is set ONLY while Stage == AWAITING_INPUT: it holds
	// the question put to the user and the Slack message coordinates needed
	// to update the form in place on resume. It MUST be nil in every other
	// stage (the resume path clears it). It is the entire persisted state a
	// suspend needs — no planexec plan snapshot is stored, because the
	// gollem conversation history (keyed by RunID) already carries the
	// sub-agent observations and the resume re-enters planexec at the
	// replan branch with a fresh budget.
	PendingInteraction *PendingInteraction
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
	case JobRunStageAwaitingInput:
		if !l.EndedAt.IsZero() {
			return goerr.New("ended at must be zero while awaiting input")
		}
		if l.PendingInteraction == nil {
			return goerr.New("pending interaction is required while awaiting input")
		}
		if err := l.PendingInteraction.Validate(); err != nil {
			return goerr.Wrap(err, "pending interaction invalid")
		}
	}
	// PendingInteraction is meaningful only while suspended; carrying it in
	// any other stage signals a resume that forgot to clear it.
	if l.Stage != JobRunStageAwaitingInput && l.PendingInteraction != nil {
		return goerr.New("pending interaction must be nil unless awaiting input",
			goerr.V("stage", string(l.Stage)))
	}
	if len(l.SystemPrompt) > MaxInlineBytes {
		return goerr.New("system prompt exceeds MaxInlineBytes (truncate before save)",
			goerr.V("len", len(l.SystemPrompt)))
	}
	return nil
}

// pendingInteractionItemType enumerates the answer-control types a
// PendingInteractionItem may carry. They mirror the host-neutral
// interaction.ItemType values; the persisted form is a plain string so the
// domain layer does not depend on pkg/agent/interaction.
const (
	pendingInteractionSelect      = "select"
	pendingInteractionMultiSelect = "multi_select"
	pendingInteractionFreeText    = "free_text"
)

// PendingInteraction is the persisted snapshot of a question put to the
// user while an interactive Run is suspended. It lives on the run's
// JobRunLog (Stage=AWAITING_INPUT). The shape mirrors the host-neutral
// interaction.Request, but is defined here (domain layer) so persistence
// does not depend on pkg/agent/interaction; the Job host converts at the
// boundary.
type PendingInteraction struct {
	// PostedChannelID / PostedMessageTS locate the Slack question form so
	// the resume path can update it in place to an "answered" view.
	PostedChannelID string
	PostedMessageTS string

	// Reason is the shared rationale rendered once above the items.
	Reason string

	// Items is the ordered list of questions (1..5).
	Items []PendingInteractionItem
}

// PendingInteractionItem is one question within a PendingInteraction.
type PendingInteractionItem struct {
	ID      string
	Text    string
	Type    string // select | multi_select | free_text
	Options []string
}

// Validate enforces the same invariants the host-neutral interaction.Request
// enforces, so a snapshot that round-trips through Firestore stays well
// formed. Slack coordinates are required because the resume path must locate
// the form to update it.
func (p *PendingInteraction) Validate() error {
	if p == nil {
		return goerr.New("pending interaction is nil")
	}
	if p.PostedChannelID == "" {
		return goerr.New("pending interaction posted channel id is empty")
	}
	if p.PostedMessageTS == "" {
		return goerr.New("pending interaction posted message ts is empty")
	}
	if len(p.Items) == 0 {
		return goerr.New("pending interaction has no items")
	}
	if len(p.Items) > 5 {
		return goerr.New("pending interaction has too many items",
			goerr.V("items", len(p.Items)))
	}
	seen := make(map[string]struct{}, len(p.Items))
	for i := range p.Items {
		it := p.Items[i]
		if it.ID == "" {
			return goerr.New("pending interaction item id is empty", goerr.V("index", i))
		}
		if _, dup := seen[it.ID]; dup {
			return goerr.New("duplicate pending interaction item id", goerr.V("id", it.ID))
		}
		seen[it.ID] = struct{}{}
		if it.Text == "" {
			return goerr.New("pending interaction item text is empty", goerr.V("id", it.ID))
		}
		switch it.Type {
		case pendingInteractionSelect, pendingInteractionMultiSelect:
			if len(it.Options) < 2 {
				return goerr.New("select item needs at least two options", goerr.V("id", it.ID))
			}
		case pendingInteractionFreeText:
			// Options ignored for free_text.
		default:
			return goerr.New("invalid pending interaction item type",
				goerr.V("id", it.ID), goerr.V("type", it.Type))
		}
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

	// EventID is the doc ID of this event. It is a UUIDv7 string,
	// chosen for Firestore-console readability (the doc ID surfaces a
	// millisecond timestamp prefix) and global uniqueness. doc IDs are
	// NOT relied on for strict ordering — `Sequence` is the
	// authoritative monotonic order.
	EventID string

	// Position.
	// Sequence is the authoritative monotonic order within a Run. It is
	// int64 (not uint64) because the Firestore SDK rejects uint64. The
	// value space is still effectively unbounded for this workload
	// (max int64 = 9.2e18 events per Run). List queries MUST OrderBy
	// "Sequence" — doc ID order may diverge under clock skew.
	Sequence   int64
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
	if e.EventID == "" {
		return goerr.New("event id is empty")
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
