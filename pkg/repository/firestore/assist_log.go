package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
)

type firestoreAssistLogRepository struct {
	client *firestore.Client
}

func newFirestoreAssistLogRepository(client *firestore.Client) *firestoreAssistLogRepository {
	return &firestoreAssistLogRepository{client: client}
}

// assistsCollection returns the subcollection path:
// workspaces/{workspaceID}/cases/{caseID}/assists
func (r *firestoreAssistLogRepository) assistsCollection(workspaceID string, caseID int64) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).
		Collection("cases").Doc(fmt.Sprintf("%d", caseID)).
		Collection("assists")
}

func (r *firestoreAssistLogRepository) Create(ctx context.Context, workspaceID string, caseID int64, log *model.AssistLog) (*model.AssistLog, error) {
	// No struct-tag mirror, no converter — model.AssistLog is
	// persisted verbatim, the only way to guarantee that fields
	// added later to the domain model don't silently drop on write.
	//
	// Guard nil up front: CaseID is assigned (and Validate called) only after
	// log is dereferenced below, so a nil log would panic before the
	// post-assignment Validate could reject it.
	if log == nil {
		return nil, goerr.Wrap(model.ErrAssistLogValidation, "assist log is nil")
	}
	if log.ID == "" {
		log.ID = model.NewAssistLogID()
	}
	log.CaseID = caseID

	// Validate after CaseID is assigned from the parameter (the caller struct
	// may carry a zero CaseID and rely on the repository to set it).
	if err := log.Validate(); err != nil {
		return nil, goerr.Wrap(err, "assist log validation failed before create")
	}

	docRef := r.assistsCollection(workspaceID, caseID).Doc(string(log.ID))
	if _, err := docRef.Set(ctx, log); err != nil {
		return nil, goerr.Wrap(err, "failed to create assist log")
	}

	return log, nil
}

func (r *firestoreAssistLogRepository) List(ctx context.Context, workspaceID string, caseID int64, limit, offset int) ([]*model.AssistLog, int, error) {
	// Get total count using aggregation query for efficiency
	countResult, err := r.assistsCollection(workspaceID, caseID).NewAggregationQuery().WithCount("count").Get(ctx)
	if err != nil {
		return nil, 0, goerr.Wrap(err, "failed to count assist logs")
	}
	countVal, ok := countResult["count"]
	if !ok {
		return nil, 0, goerr.New("missing count in aggregation result")
	}
	countPB, ok := countVal.(*pb.Value)
	if !ok {
		return nil, 0, goerr.New("unexpected count type in aggregation result",
			goerr.V("type", fmt.Sprintf("%T", countVal)))
	}
	totalCount := int(countPB.GetIntegerValue())

	// Get paginated results ordered by CreatedAt descending
	query := r.assistsCollection(workspaceID, caseID).
		OrderBy("CreatedAt", firestore.Desc).
		Offset(offset).
		Limit(limit)

	iter := query.Documents(ctx)
	defer iter.Stop()

	logs := make([]*model.AssistLog, 0)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, 0, goerr.Wrap(err, "failed to iterate assist logs")
		}

		var log model.AssistLog
		if err := doc.DataTo(&log); err != nil {
			return nil, 0, goerr.Wrap(err, "failed to unmarshal assist log")
		}

		logs = append(logs, &log)
	}

	return logs, totalCount, nil
}
