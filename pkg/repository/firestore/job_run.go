package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const jobRunsCollection = "jobRuns"

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
	// Key is reconstructed from path rather than persisted (avoid drift).
	run.Key = key
	return &run, nil
}

func (r *jobRunRepository) List(ctx context.Context, workspaceID string) ([]*model.JobRun, error) {
	if workspaceID == "" {
		return nil, goerr.New("workspace id is empty")
	}
	// We walk only jobRuns under this workspace by anchoring at the
	// workspace document path, but Firestore's CollectionGroup is the
	// least-index path for this many-cases-per-workspace shape.
	// To stay within existing single-field indexes we filter by parent
	// workspace path via Where on the implicit __name__ prefix is not
	// directly supported; instead we list cases first and fan out.
	wsCases := r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("cases").
		DocumentRefs(ctx)

	var runs []*model.JobRun
	for {
		caseRef, err := wsCases.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate cases for job runs",
				goerr.V("workspace_id", workspaceID))
		}
		jobIter := caseRef.Collection(jobRunsCollection).Documents(ctx)
		for {
			snap, err := jobIter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				jobIter.Stop()
				return nil, goerr.Wrap(err, "failed to iterate job runs",
					goerr.V("workspace_id", workspaceID),
					goerr.V("case_id", caseRef.ID))
			}
			var run model.JobRun
			if err := snap.DataTo(&run); err != nil {
				jobIter.Stop()
				return nil, goerr.Wrap(err, "failed to decode job run",
					goerr.V("doc_path", snap.Ref.Path))
			}
			// Reconstruct key from parent refs.
			caseID, parseErr := parseInt64DocID(caseRef.ID)
			if parseErr != nil {
				jobIter.Stop()
				return nil, goerr.Wrap(parseErr, "invalid case doc id",
					goerr.V("case_doc_id", caseRef.ID))
			}
			run.Key = model.JobRunKey{
				WorkspaceID: workspaceID,
				CaseID:      caseID,
				JobID:       snap.Ref.ID,
			}
			runs = append(runs, &run)
		}
		jobIter.Stop()
	}
	return runs, nil
}

func parseInt64DocID(id string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(id, "%d", &n)
	if err != nil {
		return 0, goerr.Wrap(err, "parse int64 doc id")
	}
	return n, nil
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
		run.LeaseUntil = time.Time{}
		if err := tx.Set(docRef, &run); err != nil {
			return goerr.Wrap(err, "tx set job run")
		}
		return nil
	})
}

func (r *jobRunRepository) RecordRun(ctx context.Context, key model.JobRunKey, status_ model.JobRunStatus, lastRunAt time.Time, traceID, errMsg string) error {
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
		run.LastRunAt = lastRunAt
		run.LastStatus = status_
		run.LastError = errMsg
		run.LastTraceID = traceID
		run.LeaseUntil = time.Time{}
		if err := tx.Set(docRef, &run); err != nil {
			return goerr.Wrap(err, "tx set job run")
		}
		return nil
	})
}
