package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
)

const actionEventsCollection = "events"

type actionEventRepository struct {
	client *firestore.Client
}

var _ interfaces.ActionEventRepository = &actionEventRepository{}

func newActionEventRepository(client *firestore.Client) *actionEventRepository {
	return &actionEventRepository{client: client}
}

func (r *actionEventRepository) eventsCollection(workspaceID string, actionID int64) *firestore.CollectionRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("actions").Doc(fmt.Sprintf("%d", actionID)).
		Collection(actionEventsCollection)
}

func (r *actionEventRepository) Put(ctx context.Context, workspaceID string, actionID int64, event *model.ActionEvent) error {
	if event == nil {
		return goerr.New("action event is nil")
	}
	if event.ID == "" {
		return goerr.New("action event id is empty")
	}

	ref := r.eventsCollection(workspaceID, actionID).Doc(event.ID)
	if _, err := ref.Set(ctx, event); err != nil {
		return goerr.Wrap(err, "failed to save action event",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("event_id", event.ID))
	}
	return nil
}

func (r *actionEventRepository) List(ctx context.Context, workspaceID string, actionID int64, limit int, cursor string) ([]*model.ActionEvent, string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.eventsCollection(workspaceID, actionID).
		OrderBy("CreatedAt", firestore.Desc).
		Limit(limit + 1)

	if cursor != "" {
		cursorDoc := r.eventsCollection(workspaceID, actionID).Doc(cursor)
		docSnap, err := cursorDoc.Get(ctx)
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to get cursor document",
				goerr.V("cursor", cursor))
		}
		query = query.StartAfter(docSnap)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var events []*model.ActionEvent
	hasMore := false
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", goerr.Wrap(err, "failed to iterate action events")
		}
		if len(events) >= limit {
			hasMore = true
			break
		}
		var e model.ActionEvent
		if err := doc.DataTo(&e); err != nil {
			return nil, "", goerr.Wrap(err, "failed to unmarshal action event",
				goerr.V("doc_id", doc.Ref.ID))
		}
		events = append(events, &e)
	}

	var nextCursor string
	if hasMore && len(events) > 0 {
		nextCursor = events[len(events)-1].ID
	}
	return events, nextCursor, nil
}
