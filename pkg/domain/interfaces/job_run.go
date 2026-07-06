package interfaces

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ErrJobRunNotFound is returned when a JobRunRepository operation expects
// an existing record for the key but none exists. Callers that treat
// "no prior run" as a normal idle case must check for this with
// errors.Is(err, ErrJobRunNotFound) rather than parsing strings.
var ErrJobRunNotFound = goerr.New("job run not found")

// ErrJobRunLogNotFound is returned when JobRunLogRepository.Get does not
// find a log for the given (key, runID).
var ErrJobRunLogNotFound = goerr.New("job run log not found")

// ErrJobRunLogExists is returned when JobRunLogRepository.Create is
// called with a (key, runID) that already exists. Hard error rather than
// silent overwrite so a duplicate RunID generation surfaces immediately.
var ErrJobRunLogExists = goerr.New("job run log already exists")

// ErrJobRunEventExists is returned when JobRunEventRepository.Append is
// called with a Sequence that already exists for the same (key, runID).
// This signals a sequencer bug (e.g. two emitters not sharing a counter)
// rather than a transient collision; the caller should fail loudly.
var ErrJobRunEventExists = goerr.New("job run event sequence already exists")

// JobRunRepository persists per-(workspace, case, job) execution metadata
// and provides atomic lease primitives for serialising concurrent runs.
//
// The same document doubles as the run-history record and the lock holder:
// LeaseUntil represents "in flight" and the rest of the fields represent
// the most recently completed run. Storage backends must serialise lease
// transitions (Firestore RunTransaction, in-memory mutex) so two competing
// acquirers see a consistent view.
type JobRunRepository interface {
	// Get returns the JobRun for the given key, or (nil, ErrJobRunNotFound)
	// when no prior run exists. Callers use this for scheduled due-checks
	// and for surface observability; both treat absence as "never run".
	Get(ctx context.Context, key model.JobRunKey) (*model.JobRun, error)

	// ListByCase returns every JobRun stored under the given (workspace,
	// case) tuple. Implemented as a single Firestore subcollection scan
	// per call (no cross-case work), which matches the underlying
	// storage layout. The scanner calls this once per OPEN case during a
	// tick — typical workspaces have a small number of jobs per case
	// (~handful), so a single subcollection query returns the entire
	// per-case index that the due-check needs.
	ListByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.JobRun, error)

	// TryAcquireLease attempts to take the lock for the given key, valid
	// until now+leaseDuration. Returns true if the caller now owns the
	// lease, false if a live lease is held by someone else. The first
	// acquirer on a previously-absent key also creates the record.
	//
	// Lease ownership is implicit in the LeaseUntil timestamp — there is
	// no separate owner ID because at most one process should be
	// scheduling the same (workspace, case, job) at any given moment, and
	// a stuck holder simply has its lease reclaimed once LeaseUntil
	// elapses.
	TryAcquireLease(ctx context.Context, key model.JobRunKey, now time.Time, leaseDuration time.Duration) (acquired bool, err error)

	// ReleaseLease clears LeaseUntil (sets it to the zero value) so the
	// next acquirer can take the lock immediately. Idempotent: calling
	// it without a prior acquisition is a no-op.
	ReleaseLease(ctx context.Context, key model.JobRunKey) error

	// RecordRun persists the terminal outcome of a Job run. It also
	// clears any lease that may still be active (treat RecordRun as
	// implying release) AND clears any suspension marker (a terminal run
	// is, by definition, no longer awaiting input). lastRunAt is the
	// caller's clock at the moment the run completed — repositories do not
	// stamp it themselves. runID identifies the specific JobRunLog produced
	// by this run and is mirrored into JobRun.LastRunID for cross-reference.
	RecordRun(ctx context.Context, key model.JobRunKey, status model.JobRunStatus, lastRunAt time.Time, runID, traceID, errMsg string) error

	// Suspend marks the (workspace, case, job) as awaiting user input for
	// the given runID and releases any active lease in the same atomic
	// step. While SuspendedRunID is set, the scheduler/dispatcher MUST NOT
	// start a new run for this tuple (see model.JobRun.IsSuspended). The
	// lease is released because a human wait can outlast any lease; the
	// suspension marker is the durable "do not double-start" signal.
	// suspendedAt is the caller's clock, used later by the unanswered-run
	// sweep to expire stale suspensions.
	Suspend(ctx context.Context, key model.JobRunKey, runID string, suspendedAt time.Time) error
}

// JobRunLogRepository persists one *invocation* of a Job (= one Run)
// against a Case. Stored at:
//
//	workspaces/{WorkspaceID}/cases/{CaseID}/jobRuns/{JobID}/logs/{RunID}
//
// The Stage transitions RUNNING -> SUCCESS|FAILED. Callers Create the
// log in RUNNING state once prompts are ready, then Finish it when the
// agent loop terminates. A Run that crashes mid-flight leaves the
// RUNNING log in place; that is intentional so the events captured up
// to the crash remain attributable.
type JobRunLogRepository interface {
	// Create writes the RUNNING-stage log. Errors with
	// ErrJobRunLogExists if a doc for the same (key, runID) already
	// exists; backends MUST use Firestore Create (or equivalent) so the
	// duplicate is rejected by the storage layer.
	Create(ctx context.Context, log *model.JobRunLog) error

	// Finish transitions an existing log to its terminal stage
	// (SUCCESS or FAILED). The caller supplies the full *JobRunLog
	// with Stage / EndedAt / Error populated; the implementation just
	// persists it.
	Finish(ctx context.Context, log *model.JobRunLog) error

	// Suspend transitions an existing log to the non-terminal
	// AWAITING_INPUT stage. The caller supplies the full *JobRunLog with
	// Stage=AWAITING_INPUT and PendingInteraction populated; EndedAt stays
	// zero. Errors with ErrJobRunLogNotFound if the log does not exist.
	Suspend(ctx context.Context, log *model.JobRunLog) error

	// Resume transitions a suspended log back to RUNNING. The caller
	// supplies the full *JobRunLog with Stage=RUNNING and
	// PendingInteraction cleared (nil). Errors with ErrJobRunLogNotFound if
	// the log does not exist.
	Resume(ctx context.Context, log *model.JobRunLog) error

	// Get returns the log identified by (key, runID), or
	// (nil, ErrJobRunLogNotFound) when no such log exists.
	Get(ctx context.Context, key model.JobRunKey, runID string) (*model.JobRunLog, error)

	// List returns logs under (key) in descending StartedAt order, up
	// to limit. limit <= 0 means no limit. Implemented as a single
	// subcollection scan per call (no cross-Job aggregation here).
	List(ctx context.Context, key model.JobRunKey, limit int) ([]*model.JobRunLog, error)
}

// JobRunEventRepository persists the per-Run timeline of events
// (LLM_REQUEST / LLM_RESPONSE / TOOL_CALL / RUN_ERROR). Stored at:
//
//	workspaces/{WS}/cases/{Case}/jobRuns/{Job}/logs/{Run}/events/{EventID}
//
// EventID is a UUIDv7 (timestamp-prefixed) chosen for Firestore-console
// readability and global uniqueness. The authoritative monotonic order
// is the Sequence field, not the doc ID — List MUST OrderBy("Sequence").
//
// Within a single Run, exactly one runSequencer instance owns Sequence
// allocation — both the per-call appends from the trace.Handler AND any
// RUN_ERROR emits from JobRunner go through the same sequencer.
type JobRunEventRepository interface {
	// Append writes one event keyed by ev.EventID. Both EventID and
	// Sequence must be set by the caller; Sequence must be strictly
	// increasing across calls in the same Run. Backends use Create
	// (not Set) so a duplicate EventID surfaces as ErrJobRunEventExists.
	Append(ctx context.Context, ev *model.JobRunEvent) error

	// List returns events for (key, runID) in ascending Sequence order
	// (not doc-ID order — doc IDs are UUIDv7 and may diverge under
	// clock skew).
	List(ctx context.Context, key model.JobRunKey, runID string) ([]*model.JobRunEvent, error)
}
