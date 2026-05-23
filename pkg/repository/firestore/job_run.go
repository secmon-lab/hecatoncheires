package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

const (
	jobRunsCollection      = "jobRuns"
	jobRunLogsCollection   = "logs"
	jobRunEventsCollection = "events"
)

type jobRunRepository struct {
	client *firestore.Client
}

var _ interfaces.JobRunRepository = &jobRunRepository{}

func newJobRunRepository(client *firestore.Client) *jobRunRepository {
	return &jobRunRepository{client: client}
}

func (r *jobRunRepository) doc(key model.JobRunKey) *firestore.DocumentRef {
	return r.client.
		Collection("workspaces").Doc(key.WorkspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", key.CaseID)).
		Collection(jobRunsCollection).Doc(key.JobID)
}

func (r *jobRunRepository) Get(ctx context.Context, key model.JobRunKey) (*model.JobRun, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	snap, err := r.doc(key).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrJobRunNotFound, "job run not found",
				goerr.V("workspace_id", key.WorkspaceID),
				goerr.V("case_id", key.CaseID),
				goerr.V("job_id", key.JobID))
		}
		return nil, goerr.Wrap(err, "failed to get job run",
			goerr.V("workspace_id", key.WorkspaceID),
			goerr.V("case_id", key.CaseID),
			goerr.V("job_id", key.JobID))
	}
	var run model.JobRun
	if err := snap.DataTo(&run); err != nil {
		return nil, goerr.Wrap(err, "failed to decode job run")
	}
	// Identity fields are restored from path so they survive even if a
	// historical doc was written before the model was flattened.
	run.WorkspaceID = key.WorkspaceID
	run.CaseID = key.CaseID
	run.JobID = key.JobID
	return &run, nil
}

func (r *jobRunRepository) ListByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.JobRun, error) {
	if workspaceID == "" {
		return nil, goerr.New("workspace id is empty")
	}
	if caseID == 0 {
		return nil, goerr.New("case id is zero")
	}
	// A single Firestore subcollection query — the storage layout
	// (workspaces/{ws}/cases/{c}/jobRuns) already gives us a natural
	// per-case scope. The scanner calls this once per OPEN case; cross-
	// case access patterns simply do not exist for JobRun.
	caseDoc := r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID))
	iter := caseDoc.Collection(jobRunsCollection).Documents(ctx)
	defer iter.Stop()

	var runs []*model.JobRun
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate job runs",
				goerr.V("workspace_id", workspaceID),
				goerr.V("case_id", caseID))
		}
		var run model.JobRun
		if err := snap.DataTo(&run); err != nil {
			return nil, goerr.Wrap(err, "failed to decode job run",
				goerr.V("doc_path", snap.Ref.Path))
		}
		run.WorkspaceID = workspaceID
		run.CaseID = caseID
		run.JobID = snap.Ref.ID
		runs = append(runs, &run)
	}
	return runs, nil
}

func (r *jobRunRepository) TryAcquireLease(ctx context.Context, key model.JobRunKey, now time.Time, leaseDuration time.Duration) (bool, error) {
	if err := key.Validate(); err != nil {
		return false, goerr.Wrap(err, "invalid job run key")
	}
	if leaseDuration <= 0 {
		return false, goerr.New("lease duration must be positive",
			goerr.V("lease_duration", leaseDuration))
	}
	docRef := r.doc(key)
	var acquired bool
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		acquired = false
		snap, err := tx.Get(docRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return goerr.Wrap(err, "tx get job run")
		}
		var run model.JobRun
		if err == nil {
			if err := snap.DataTo(&run); err != nil {
				return goerr.Wrap(err, "decode existing job run")
			}
			if run.IsLeased(now) {
				return nil
			}
		}
		// Always stamp identity onto the doc — covers both fresh creates
		// and re-acquires of historical docs that were written before
		// the model was flattened.
		run.WorkspaceID = key.WorkspaceID
		run.CaseID = key.CaseID
		run.JobID = key.JobID
		run.LeaseUntil = now.Add(leaseDuration)
		if err := tx.Set(docRef, &run); err != nil {
			return goerr.Wrap(err, "tx set job run")
		}
		acquired = true
		return nil
	})
	if err != nil {
		return false, goerr.Wrap(err, "TryAcquireLease",
			goerr.V("workspace_id", key.WorkspaceID),
			goerr.V("case_id", key.CaseID),
			goerr.V("job_id", key.JobID))
	}
	return acquired, nil
}

func (r *jobRunRepository) ReleaseLease(ctx context.Context, key model.JobRunKey) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run key")
	}
	docRef := r.doc(key)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil
			}
			return goerr.Wrap(err, "tx get job run")
		}
		var run model.JobRun
		if err := snap.DataTo(&run); err != nil {
			return goerr.Wrap(err, "decode existing job run")
		}
		run.WorkspaceID = key.WorkspaceID
		run.CaseID = key.CaseID
		run.JobID = key.JobID
		run.LeaseUntil = time.Time{}
		if err := tx.Set(docRef, &run); err != nil {
			return goerr.Wrap(err, "tx set job run")
		}
		return nil
	})
}

func (r *jobRunRepository) RecordRun(ctx context.Context, key model.JobRunKey, status_ model.JobRunStatus, lastRunAt time.Time, runID, traceID, errMsg string) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run key")
	}
	if !status_.IsValid() {
		return goerr.New("invalid job run status",
			goerr.V("status", string(status_)))
	}
	docRef := r.doc(key)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(docRef)
		var run model.JobRun
		switch {
		case err == nil:
			if decErr := snap.DataTo(&run); decErr != nil {
				return goerr.Wrap(decErr, "decode existing job run")
			}
		case status.Code(err) == codes.NotFound:
			// fresh record
		default:
			return goerr.Wrap(err, "tx get job run")
		}
		run.WorkspaceID = key.WorkspaceID
		run.CaseID = key.CaseID
		run.JobID = key.JobID
		run.LastRunAt = lastRunAt
		run.LastStatus = status_
		run.LastError = errMsg
		run.LastRunID = runID
		run.LastTraceID = traceID
		run.LeaseUntil = time.Time{}
		if err := tx.Set(docRef, &run); err != nil {
			return goerr.Wrap(err, "tx set job run")
		}
		return nil
	})
}

// --- JobRunLog ---------------------------------------------------------

type jobRunLogRepository struct {
	client *firestore.Client
}

var _ interfaces.JobRunLogRepository = &jobRunLogRepository{}

func newJobRunLogRepository(client *firestore.Client) *jobRunLogRepository {
	return &jobRunLogRepository{client: client}
}

func (r *jobRunLogRepository) doc(key model.JobRunKey, runID string) *firestore.DocumentRef {
	return r.client.
		Collection("workspaces").Doc(key.WorkspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", key.CaseID)).
		Collection(jobRunsCollection).Doc(key.JobID).
		Collection(jobRunLogsCollection).Doc(runID)
}

func (r *jobRunLogRepository) Create(ctx context.Context, log *model.JobRunLog) error {
	if err := log.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run log")
	}
	key := model.JobRunKey{WorkspaceID: log.WorkspaceID, CaseID: log.CaseID, JobID: log.JobID}
	if _, err := r.doc(key, log.RunID).Create(ctx, log); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return goerr.Wrap(interfaces.ErrJobRunLogExists, "job run log already exists",
				goerr.V("workspace_id", log.WorkspaceID),
				goerr.V("case_id", log.CaseID),
				goerr.V("job_id", log.JobID),
				goerr.V("run_id", log.RunID))
		}
		return goerr.Wrap(err, "create job run log",
			goerr.V("run_id", log.RunID))
	}
	return nil
}

func (r *jobRunLogRepository) Finish(ctx context.Context, log *model.JobRunLog) error {
	if err := log.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run log")
	}
	if log.Stage == model.JobRunStageRunning {
		return goerr.New("Finish must transition out of RUNNING")
	}
	key := model.JobRunKey{WorkspaceID: log.WorkspaceID, CaseID: log.CaseID, JobID: log.JobID}
	if _, err := r.doc(key, log.RunID).Set(ctx, log); err != nil {
		return goerr.Wrap(err, "finish job run log",
			goerr.V("run_id", log.RunID))
	}
	return nil
}

func (r *jobRunLogRepository) Get(ctx context.Context, key model.JobRunKey, runID string) (*model.JobRunLog, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	if runID == "" {
		return nil, goerr.New("run id is empty")
	}
	snap, err := r.doc(key, runID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(interfaces.ErrJobRunLogNotFound, "job run log not found",
				goerr.V("workspace_id", key.WorkspaceID),
				goerr.V("case_id", key.CaseID),
				goerr.V("job_id", key.JobID),
				goerr.V("run_id", runID))
		}
		return nil, goerr.Wrap(err, "get job run log",
			goerr.V("run_id", runID))
	}
	var log model.JobRunLog
	if err := snap.DataTo(&log); err != nil {
		return nil, goerr.Wrap(err, "decode job run log")
	}
	// Identity from path (defensive; the doc itself should carry them too).
	log.WorkspaceID = key.WorkspaceID
	log.CaseID = key.CaseID
	log.JobID = key.JobID
	log.RunID = runID
	return &log, nil
}

func (r *jobRunLogRepository) List(ctx context.Context, key model.JobRunKey, limit int) ([]*model.JobRunLog, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	q := r.client.
		Collection("workspaces").Doc(key.WorkspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", key.CaseID)).
		Collection(jobRunsCollection).Doc(key.JobID).
		Collection(jobRunLogsCollection).
		OrderBy("StartedAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(ctx)
	defer iter.Stop()

	var out []*model.JobRunLog
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "iterate job run logs",
				goerr.V("workspace_id", key.WorkspaceID),
				goerr.V("case_id", key.CaseID),
				goerr.V("job_id", key.JobID))
		}
		var log model.JobRunLog
		if err := snap.DataTo(&log); err != nil {
			return nil, goerr.Wrap(err, "decode job run log",
				goerr.V("doc_path", snap.Ref.Path))
		}
		log.WorkspaceID = key.WorkspaceID
		log.CaseID = key.CaseID
		log.JobID = key.JobID
		log.RunID = snap.Ref.ID
		out = append(out, &log)
	}
	return out, nil
}

// --- JobRunEvent -------------------------------------------------------

type jobRunEventRepository struct {
	client *firestore.Client
}

var _ interfaces.JobRunEventRepository = &jobRunEventRepository{}

func newJobRunEventRepository(client *firestore.Client) *jobRunEventRepository {
	return &jobRunEventRepository{client: client}
}

// doc returns the DocumentRef for one JobRunEvent, keyed by eventID
// (a UUIDv7 supplied by the caller via JobRunEvent.EventID). doc IDs
// are NOT used for ordering — List queries OrderBy("Sequence").
func (r *jobRunEventRepository) doc(key model.JobRunKey, runID, eventID string) *firestore.DocumentRef {
	return r.client.
		Collection("workspaces").Doc(key.WorkspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", key.CaseID)).
		Collection(jobRunsCollection).Doc(key.JobID).
		Collection(jobRunLogsCollection).Doc(runID).
		Collection(jobRunEventsCollection).Doc(eventID)
}

func (r *jobRunEventRepository) Append(ctx context.Context, ev *model.JobRunEvent) error {
	if err := ev.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job run event")
	}
	key := model.JobRunKey{WorkspaceID: ev.WorkspaceID, CaseID: ev.CaseID, JobID: ev.JobID}
	if _, err := r.doc(key, ev.RunID, ev.EventID).Create(ctx, ev); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return goerr.Wrap(interfaces.ErrJobRunEventExists, "job run event already exists",
				goerr.V("run_id", ev.RunID),
				goerr.V("event_id", ev.EventID))
		}
		return goerr.Wrap(err, "append job run event",
			goerr.V("run_id", ev.RunID),
			goerr.V("event_id", ev.EventID),
			goerr.V("sequence", ev.Sequence))
	}
	return nil
}

func (r *jobRunEventRepository) List(ctx context.Context, key model.JobRunKey, runID string) ([]*model.JobRunEvent, error) {
	if err := key.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job run key")
	}
	if runID == "" {
		return nil, goerr.New("run id is empty")
	}
	// Authoritative monotonic order is the Sequence field, NOT the doc
	// ID. doc IDs are UUIDv7 and can diverge from append order under
	// clock skew or sub-millisecond races; Sequence is allocated by a
	// shared runSequencer and is guaranteed strict-monotonic.
	q := r.client.
		Collection("workspaces").Doc(key.WorkspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", key.CaseID)).
		Collection(jobRunsCollection).Doc(key.JobID).
		Collection(jobRunLogsCollection).Doc(runID).
		Collection(jobRunEventsCollection).
		OrderBy("Sequence", firestore.Asc)

	iter := q.Documents(ctx)
	defer iter.Stop()

	var out []*model.JobRunEvent
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "iterate job run events",
				goerr.V("run_id", runID))
		}
		var ev model.JobRunEvent
		if err := snap.DataTo(&ev); err != nil {
			return nil, goerr.Wrap(err, "decode job run event",
				goerr.V("doc_path", snap.Ref.Path))
		}
		ev.WorkspaceID = key.WorkspaceID
		ev.CaseID = key.CaseID
		ev.JobID = key.JobID
		ev.RunID = runID
		ev.EventID = snap.Ref.ID
		out = append(out, &ev)
	}
	return out, nil
}
