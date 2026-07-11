package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
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
	if err := action.Validate(); err != nil {
		return nil, goerr.Wrap(err, "action validation failed before create")
	}

	nextID, err := r.getNextID(ctx, workspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get next ID")
	}

	// Mutate the storage-side ID directly on the caller's struct so the
	// model is persisted verbatim. NEVER rebuild via a field-by-field
	// struct literal — that pattern silently drops fields added later
	// to model.Action and was the root cause of the empty-reporter bug
	// on the Case side.
	action.ID = nextID

	docID := fmt.Sprintf("%d", action.ID)
	if _, err := r.actionsCollection(workspaceID).Doc(docID).Set(ctx, action); err != nil {
		return nil, goerr.Wrap(err, "failed to create action", goerr.V("id", action.ID))
	}

	return action, nil
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

func (r *actionRepository) GetByIDs(ctx context.Context, workspaceID string, ids []int64) (map[int64]*model.Action, error) {
	result := make(map[int64]*model.Action, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	col := r.actionsCollection(workspaceID)
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = col.Doc(fmt.Sprintf("%d", id))
	}

	snaps, err := r.client.GetAll(ctx, refs)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to batch get actions", goerr.V("ids", ids))
	}

	for _, snap := range snaps {
		if !snap.Exists() {
			continue
		}
		var a model.Action
		if err := snap.DataTo(&a); err != nil {
			return nil, goerr.Wrap(err, "failed to decode action", goerr.V("doc_id", snap.Ref.ID))
		}
		result[a.ID] = &a
	}

	return result, nil
}

func (r *actionRepository) List(ctx context.Context, workspaceID string, opts interfaces.ActionListOptions) ([]*model.Action, error) {
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

		if !opts.ArchiveScope.Allows(a.IsArchived()) {
			continue
		}

		actions = append(actions, &a)
	}

	return actions, nil
}

func (r *actionRepository) Update(ctx context.Context, workspaceID string, action *model.Action) (*model.Action, error) {
	if err := action.Validate(); err != nil {
		return nil, goerr.Wrap(err, "action validation failed before update")
	}

	docID := fmt.Sprintf("%d", action.ID)
	docRef := r.actionsCollection(workspaceID).Doc(docID)

	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("id", action.ID))
		}
		return nil, goerr.Wrap(err, "failed to check action existence", goerr.V("id", action.ID))
	}

	if _, err := docRef.Set(ctx, action); err != nil {
		return nil, goerr.Wrap(err, "failed to update action", goerr.V("id", action.ID))
	}

	return action, nil
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

func (r *actionRepository) GetByCase(ctx context.Context, workspaceID string, caseID int64, opts interfaces.ActionListOptions) ([]*model.Action, error) {
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

		if !opts.ArchiveScope.Allows(a.IsArchived()) {
			continue
		}

		actions = append(actions, &a)
	}

	return actions, nil
}

func (r *actionRepository) GetBySlackMessageTS(ctx context.Context, workspaceID string, ts string) (*model.Action, error) {
	if ts == "" {
		return nil, goerr.Wrap(ErrNotFound, "slack message ts is empty")
	}

	iter := r.actionsCollection(workspaceID).
		Where("SlackMessageTS", "==", ts).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	docSnap, err := iter.Next()
	if err == iterator.Done {
		return nil, goerr.Wrap(ErrNotFound, "action not found", goerr.V("slack_message_ts", ts))
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to query action by slack message ts", goerr.V("slack_message_ts", ts))
	}

	var a model.Action
	if err := docSnap.DataTo(&a); err != nil {
		return nil, goerr.Wrap(err, "failed to decode action", goerr.V("doc_id", docSnap.Ref.ID))
	}

	return &a, nil
}

func (r *actionRepository) GetByCases(ctx context.Context, workspaceID string, caseIDs []int64, opts interfaces.ActionListOptions) (map[int64][]*model.Action, error) {
	// Initialize result map
	result := make(map[int64][]*model.Action)
	for _, caseID := range caseIDs {
		result[caseID] = make([]*model.Action, 0)
	}

	// Execute parallel queries for each case ID
	// (avoids creating new composite index)
	for _, caseID := range caseIDs {
		actions, err := r.GetByCase(ctx, workspaceID, caseID, opts)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to get actions by case", goerr.V("case_id", caseID))
		}
		result[caseID] = actions
	}

	return result, nil
}
