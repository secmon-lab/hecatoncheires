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
	// implying release). lastRunAt is the caller's clock at the moment
	// the run completed — repositories do not stamp it themselves.
	RecordRun(ctx context.Context, key model.JobRunKey, status model.JobRunStatus, lastRunAt time.Time, traceID, errMsg string) error
}
