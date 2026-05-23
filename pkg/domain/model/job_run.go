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
