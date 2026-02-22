package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
)

// assistLogDoc is the Firestore document representation of model.AssistLog.
type assistLogDoc struct {
	ID        model.AssistLogID `firestore:"ID"`
	CaseID    int64             `firestore:"CaseID"`
	Summary   string            `firestore:"Summary"`
	Actions   string            `firestore:"Actions"`
	Reasoning string            `firestore:"Reasoning"`
	NextSteps string            `firestore:"NextSteps"`
	CreatedAt time.Time         `firestore:"CreatedAt"`
}

func toAssistLogDoc(l *model.AssistLog) *assistLogDoc {
	return &assistLogDoc{
		ID:        l.ID,
		CaseID:    l.CaseID,
		Summary:   l.Summary,
		Actions:   l.Actions,
		Reasoning: l.Reasoning,
		NextSteps: l.NextSteps,
		CreatedAt: l.CreatedAt,
	}
}

func fromAssistLogDoc(d *assistLogDoc) *model.AssistLog {
	return &model.AssistLog{
		ID:        d.ID,
		CaseID:    d.CaseID,
		Summary:   d.Summary,
		Actions:   d.Actions,
		Reasoning: d.Reasoning,
		NextSteps: d.NextSteps,
		CreatedAt: d.CreatedAt,
	}
}

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
	if log.ID == "" {
		log.ID = model.NewAssistLogID()
	}
	log.CaseID = caseID
	log.CreatedAt = time.Now().UTC()

	docRef := r.assistsCollection(workspaceID, caseID).Doc(string(log.ID))
	if _, err := docRef.Set(ctx, toAssistLogDoc(log)); err != nil {
		return nil, goerr.Wrap(err, "failed to create assist log")
	}

	return log, nil
}

func (r *firestoreAssistLogRepository) List(ctx context.Context, workspaceID string, caseID int64, limit, offset int) ([]*model.AssistLog, int, error) {
	// Get total count first
	allDocs, err := r.assistsCollection(workspaceID, caseID).Documents(ctx).GetAll()
	if err != nil {
		return nil, 0, goerr.Wrap(err, "failed to count assist logs")
	}
	totalCount := len(allDocs)

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

		var d assistLogDoc
		if err := doc.DataTo(&d); err != nil {
			return nil, 0, goerr.Wrap(err, "failed to unmarshal assist log")
		}

		logs = append(logs, fromAssistLogDoc(&d))
	}

	return logs, totalCount, nil
}
