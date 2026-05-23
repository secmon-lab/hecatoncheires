package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type jobRunRepository struct {
	mu   sync.Mutex
	runs map[model.JobRunKey]*model.JobRun
}

var _ interfaces.JobRunRepository = &jobRunRepository{}

func newJobRunRepository() *jobRunRepository {
	return &jobRunRepository{
		runs: make(map[model.JobRunKey]*model.JobRun),
	}
}

func copyJobRun(r *model.JobRun) *model.JobRun {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

func (r *jobRunRepository) Get(ctx context.Context, key model.JobRunKey) (*model.JobRun, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[key]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrJobRunNotFound, "job run not found",
			goerr.V("workspace_id", key.WorkspaceID),
			goerr.V("case_id", key.CaseID),
			goerr.V("job_id", key.JobID))
	}
	return copyJobRun(run), nil
}

func (r *jobRunRepository) ListByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.JobRun, error) {
	if workspaceID == "" {
		return nil, goerr.New("workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.New("case id is zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.JobRun
	for k, v := range r.runs {
		if k.WorkspaceID != workspaceID || k.CaseID != caseID {
			continue
		}
		out = append(out, copyJobRun(v))
	}
	return out, nil
}

func (r *jobRunRepository) TryAcquireLease(ctx context.Context, key model.JobRunKey, now time.Time, leaseDuration time.Duration) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid job run key")
	}
	if leaseDuration <= 0 {
		return false, goerr.New("lease duration must be positive",
			goerr.V("lease_duration", leaseDuration))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.runs[key]
	if ok && existing.IsLeased(now) {
		return false, nil
	}

	if !ok {
		existing = &model.JobRun{Key: key}
	}
	existing.LeaseUntil = now.Add(leaseDuration)
	r.runs[key] = existing
	return true, nil
}

func (r *jobRunRepository) ReleaseLease(ctx context.Context, key model.JobRunKey) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.runs[key]
	if !ok {
		return nil
	}
	existing.LeaseUntil = time.Time{}
	r.runs[key] = existing
	return nil
}

func (r *jobRunRepository) RecordRun(ctx context.Context, key model.JobRunKey, status model.JobRunStatus, lastRunAt time.Time, traceID, errMsg string) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run key")
	}
	if !status.IsValid() {
		return goerr.New("invalid job run status",
			goerr.V("status", string(status)))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.runs[key]
	if !ok {
		existing = &model.JobRun{Key: key}
	}
	existing.LastRunAt = lastRunAt
	existing.LastStatus = status
	existing.LastError = errMsg
	existing.LastTraceID = traceID
	existing.LeaseUntil = time.Time{}
	r.runs[key] = existing
	return nil
}
