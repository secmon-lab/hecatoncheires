package memory

import (
	"context"
	"sort"
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
		existing = &model.JobRun{WorkspaceID: key.WorkspaceID, CaseID: key.CaseID, JobID: key.JobID}
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

func (r *jobRunRepository) RecordRun(ctx context.Context, key model.JobRunKey, status model.JobRunStatus, lastRunAt time.Time, runID, traceID, errMsg string) error {
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
		existing = &model.JobRun{WorkspaceID: key.WorkspaceID, CaseID: key.CaseID, JobID: key.JobID}
	}
	existing.LastRunAt = lastRunAt
	existing.LastStatus = status
	existing.LastError = errMsg
	existing.LastRunID = runID
	existing.LastTraceID = traceID
	existing.LeaseUntil = time.Time{}
	r.runs[key] = existing
	return nil
}

// jobRunLogKey identifies a single JobRunLog inside the memory store.
type jobRunLogKey struct {
	K     model.JobRunKey
	RunID string
}

type jobRunLogRepository struct {
	mu   sync.Mutex
	logs map[jobRunLogKey]*model.JobRunLog
}

var _ interfaces.JobRunLogRepository = &jobRunLogRepository{}

func newJobRunLogRepository() *jobRunLogRepository {
	return &jobRunLogRepository{logs: make(map[jobRunLogKey]*model.JobRunLog)}
}

func copyJobRunLog(l *model.JobRunLog) *model.JobRunLog {
	if l == nil {
		return nil
	}
	cp := *l
	return &cp
}

func (r *jobRunLogRepository) Create(ctx context.Context, log *model.JobRunLog) error {
	if err := log.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run log")
	}
	key := jobRunLogKey{
		K:     model.JobRunKey{WorkspaceID: log.WorkspaceID, CaseID: log.CaseID, JobID: log.JobID},
		RunID: log.RunID,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.logs[key]; ok {
		return goerr.Wrap(interfaces.ErrJobRunLogExists, "job run log already exists",
			goerr.V("workspace_id", log.WorkspaceID),
			goerr.V("case_id", log.CaseID),
			goerr.V("job_id", log.JobID),
			goerr.V("run_id", log.RunID))
	}
	r.logs[key] = copyJobRunLog(log)
	return nil
}

func (r *jobRunLogRepository) Finish(ctx context.Context, log *model.JobRunLog) error {
	if err := log.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run log")
	}
	if log.Stage == model.JobRunStageRunning {
		return goerr.New("Finish must transition out of RUNNING")
	}
	key := jobRunLogKey{
		K:     model.JobRunKey{WorkspaceID: log.WorkspaceID, CaseID: log.CaseID, JobID: log.JobID},
		RunID: log.RunID,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.logs[key]; !ok {
		return goerr.Wrap(interfaces.ErrJobRunLogNotFound, "job run log not found for Finish",
			goerr.V("run_id", log.RunID))
	}
	r.logs[key] = copyJobRunLog(log)
	return nil
}

func (r *jobRunLogRepository) Get(ctx context.Context, key model.JobRunKey, runID string) (*model.JobRunLog, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	if runID == "" {
		return nil, goerr.New("run id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	log, ok := r.logs[jobRunLogKey{K: key, RunID: runID}]
	if !ok {
		return nil, goerr.Wrap(interfaces.ErrJobRunLogNotFound, "job run log not found",
			goerr.V("workspace_id", key.WorkspaceID),
			goerr.V("case_id", key.CaseID),
			goerr.V("job_id", key.JobID),
			goerr.V("run_id", runID))
	}
	return copyJobRunLog(log), nil
}

func (r *jobRunLogRepository) List(ctx context.Context, key model.JobRunKey, limit int) ([]*model.JobRunLog, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.JobRunLog
	for k, v := range r.logs {
		if k.K != key {
			continue
		}
		out = append(out, copyJobRunLog(v))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// jobRunEventKey identifies a single JobRunEvent inside the memory
// store. The map key mirrors the Firestore doc key — (Run, EventID) —
// so collisions surface in the same way across both backends.
type jobRunEventKey struct {
	K       model.JobRunKey
	RunID   string
	EventID string
}

type jobRunEventRepository struct {
	mu     sync.Mutex
	events map[jobRunEventKey]*model.JobRunEvent
}

var _ interfaces.JobRunEventRepository = &jobRunEventRepository{}

func newJobRunEventRepository() *jobRunEventRepository {
	return &jobRunEventRepository{events: make(map[jobRunEventKey]*model.JobRunEvent)}
}

func copyJobRunEvent(e *model.JobRunEvent) *model.JobRunEvent {
	if e == nil {
		return nil
	}
	cp := *e
	if e.LLMRequest != nil {
		req := *e.LLMRequest
		if e.LLMRequest.Messages != nil {
			req.Messages = append([]model.LLMMessage(nil), e.LLMRequest.Messages...)
			for i := range req.Messages {
				if e.LLMRequest.Messages[i].Contents != nil {
					req.Messages[i].Contents = append([]model.LLMContentBlock(nil), e.LLMRequest.Messages[i].Contents...)
				}
			}
		}
		if e.LLMRequest.Tools != nil {
			req.Tools = append([]model.LLMToolSpec(nil), e.LLMRequest.Tools...)
		}
		cp.LLMRequest = &req
	}
	if e.LLMResponse != nil {
		resp := *e.LLMResponse
		if e.LLMResponse.Texts != nil {
			resp.Texts = append([]string(nil), e.LLMResponse.Texts...)
		}
		if e.LLMResponse.FunctionCalls != nil {
			resp.FunctionCalls = append([]model.LLMFunctionCall(nil), e.LLMResponse.FunctionCalls...)
		}
		cp.LLMResponse = &resp
	}
	if e.ToolCall != nil {
		tc := *e.ToolCall
		cp.ToolCall = &tc
	}
	if e.RunError != nil {
		re := *e.RunError
		cp.RunError = &re
	}
	return &cp
}

func (r *jobRunEventRepository) Append(ctx context.Context, ev *model.JobRunEvent) error {
	if err := ev.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run event")
	}
	key := jobRunEventKey{
		K:       model.JobRunKey{WorkspaceID: ev.WorkspaceID, CaseID: ev.CaseID, JobID: ev.JobID},
		RunID:   ev.RunID,
		EventID: ev.EventID,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.events[key]; ok {
		return goerr.Wrap(interfaces.ErrJobRunEventExists, "job run event already exists",
			goerr.V("run_id", ev.RunID),
			goerr.V("event_id", ev.EventID))
	}
	r.events[key] = copyJobRunEvent(ev)
	return nil
}

func (r *jobRunEventRepository) List(ctx context.Context, key model.JobRunKey, runID string) ([]*model.JobRunEvent, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	if runID == "" {
		return nil, goerr.New("run id is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.JobRunEvent
	for k, v := range r.events {
		if k.K != key || k.RunID != runID {
			continue
		}
		out = append(out, copyJobRunEvent(v))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Sequence < out[j].Sequence
	})
	return out, nil
}
