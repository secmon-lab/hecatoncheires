package firestore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type actionRepository struct {
	client *firestore.Client
}

func newActionRepository(client *firestore.Client) *actionRepository {
	return &actionRepository{
		client: client,
	}
}

func (r *actionRepository) actionsCollection(workspaceID string) *firestore.CollectionRef {
	return r.client.Collection("workspaces").Doc(workspaceID).Collection("actions")
}

func (r *actionRepository) actionCounterRef(workspaceID string) *firestore.DocumentRef {
	return r.client.Collection("counters").Doc("action").Collection("workspaces").Doc(workspaceID)
}

func (r *actionRepository) getNextID(ctx context.Context, workspaceID string) (int64, error) {
	counterRef := r.actionCounterRef(workspaceID)

	var nextID int64
	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(counterRef)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				nextID = 1
				return tx.Set(counterRef, map[string]interface{}{
					"value": nextID,
				})
			}
			return goerr.Wrap(err, "failed to get counter")
		}

		currentValue, err := doc.DataAt("value")
		if err != nil {
			return goerr.Wrap(err, "failed to get counter value")
		}

		val, ok := currentValue.(int64)
		if !ok {
			return goerr.New("counter value is not of type int64", goerr.V("value", currentValue))
		}
		nextID = val + 1
		return tx.Update(counterRef, []firestore.Update{
			{Path: "value", Value: nextID},
		})
	})

	if err != nil {
		return 0, goerr.Wrap(err, "failed to get next ID")
	}

	return nextID, nil
}

func (r *actionRepository) Create(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	nextID, err := r.getNextID(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}

	now := time.Now().UTC()
	created := &model.Action{
		ID:             nextID,
		CaseID:         action.CaseID,
		Title:          action.Title,
		Description:    action.Description,
		AssigneeIDs:    action.AssigneeIDs,
		SlackMessageTS: action.SlackMessageTS,
		Status:         action.Status,
		DueDate:        action.DueDate,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	docID := fmt.Sprintf("%d", created.ID)

	_, err = r.actionsCollection(workspaceID).Doc(docID).Set(ctx, created)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create action", goerr.V("id", created.ID))
	}

	return created, nil
}

func (r *actionRepository) Get(ctx context.Context, workspaceID string, id int64) (*model.Action, error) {
	docID := fmt.Sprintf("%d", id)
	docSnap, err := r.actionsCollection(workspaceID).Doc(docID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
		}
		return nil, goerr.Wrap(err, "failed to get action", goerr.V("id", id))
	}

	var a model.Action
	if err := docSnap.DataTo(&a); err != nil {
		return nil, goerr.Wrap(err, "failed to decode action", goerr.V("id", id))
	}

	return &a, nil
}

func (r *actionRepository) List(ctx context.Context, workspaceID string) ([]*model.Action, error) {
	iter := r.actionsCollection(workspaceID).Documents(ctx)
	defer iter.Stop()

	actions := make([]*model.Action, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate actions")
		}

		var a model.Action
		if err := docSnap.DataTo(&a); err != nil {
			return nil, goerr.Wrap(err, "failed to decode action", goerr.V("doc_id", docSnap.Ref.ID))
		}

		actions = append(actions, &a)
	}

	return actions, nil
}

func (r *actionRepository) Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	docID := fmt.Sprintf("%d", action.ID)
	docRef := r.actionsCollection(workspaceID).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", action.ID))
		}
		return nil, goerr.Wrap(err, "failed to check action existence", goerr.V("id", action.ID))
	}

	// Update with new timestamp
	updated := &model.Action{
		ID:             action.ID,
		CaseID:         action.CaseID,
		Title:          action.Title,
		Description:    action.Description,
		AssigneeIDs:    action.AssigneeIDs,
		SlackMessageTS: action.SlackMessageTS,
		Status:         action.Status,
		DueDate:        action.DueDate,
		CreatedAt:      action.CreatedAt,
		UpdatedAt:      time.Now().UTC(),
	}

	_, err = docRef.Set(ctx, updated)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update action", goerr.V("id", action.ID))
	}

	return updated, nil
}

func (r *actionRepository) Delete(ctx context.Context, workspaceID string, id int64) error {
	docID := fmt.Sprintf("%d", id)
	docRef := r.actionsCollection(workspaceID).Doc(docID)

	// Check if document exists
	_, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", id))
		}
		return goerr.Wrap(err, "failed to check action existence", goerr.V("id", id))
	}

	_, err = docRef.Delete(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to delete action", goerr.V("id", id))
	}

	return nil
}

func (r *actionRepository) GetByCase(ctx context.Context, workspaceID string, caseID int64) ([]*model.Action, error) {
	iter := r.actionsCollection(workspaceID).
		Where("CaseID", "==", caseID).
		Documents(ctx)
	defer iter.Stop()

	actions := make([]*model.Action, 0)
	for {
		docSnap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate actions", goerr.V("case_id", caseID))
		}

		var a model.Action
		if err := docSnap.DataTo(&a); err != nil {
			return nil, goerr.Wrap(err, "failed to decode action", goerr.V("doc_id", docSnap.Ref.ID))
		}

		actions = append(actions, &a)
	}

	return actions, nil
}

func (r *actionRepository) GetByCases(ctx context.Context, workspaceID string, caseIDs []int64) (map[int64][]*model.Action, error) {
	// Initialize result map
	result := make(map[int64][]*model.Action)
	for _, caseID := range caseIDs {
		result[caseID] = make([]*model.Action, 0)
	}

	// Execute parallel queries for each case ID
	// (avoids creating new composite index)
	for _, caseID := range caseIDs {
		actions, err := r.GetByCase(ctx, workspaceID, caseID)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to get actions by case", goerr.V("case_id", caseID))
		}
		result[caseID] = actions
	}

	return result, nil
}
