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

const actionStepsCollection = "steps"

type actionStepRepository struct {
	client *firestore.Client
}

var _ interfaces.ActionStepRepository = &actionStepRepository{}

func newActionStepRepository(client *firestore.Client) *actionStepRepository {
	return &actionStepRepository{client: client}
}

func (r *actionStepRepository) stepsCollection(workspaceID string, actionID int64) *firestore.CollectionRef {
	return r.client.
		Collection("workspaces").Doc(workspaceID).
		Collection("actions").Doc(fmt.Sprintf("%d", actionID)).
		Collection(actionStepsCollection)
}

func (r *actionStepRepository) Put(ctx context.Context, workspaceID string, step *model.ActionStep) error {
	if step == nil {
		return goerr.New("action step is nil")
	}
	if step.ID == "" {
		return goerr.New("action step id is empty")
	}

	ref := r.stepsCollection(workspaceID, step.ActionID).Doc(step.ID)
	if _, err := ref.Set(ctx, step); err != nil {
		return goerr.Wrap(err, "failed to save action step",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", step.ActionID),
			goerr.V("step_id", step.ID))
	}
	return nil
}

func (r *actionStepRepository) Get(ctx context.Context, workspaceID string, actionID int64, stepID string) (*model.ActionStep, error) {
	docSnap, err := r.stepsCollection(workspaceID, actionID).Doc(stepID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "action step not found",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID),
				goerr.V("step_id", stepID))
		}
		return nil, goerr.Wrap(err, "failed to get action step",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("step_id", stepID))
	}

	var s model.ActionStep
	if err := docSnap.DataTo(&s); err != nil {
		return nil, goerr.Wrap(err, "failed to decode action step",
			goerr.V("doc_id", docSnap.Ref.ID))
	}
	return &s, nil
}

func (r *actionStepRepository) List(ctx context.Context, workspaceID string, actionID int64) ([]*model.ActionStep, error) {
	query := r.stepsCollection(workspaceID, actionID).
		OrderBy("CreatedAt", firestore.Asc)

	iter := query.Documents(ctx)
	defer iter.Stop()

	steps := []*model.ActionStep{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, goerr.Wrap(err, "failed to iterate action steps",
				goerr.V("workspace_id", workspaceID),
				goerr.V("action_id", actionID))
		}
		var s model.ActionStep
		if err := doc.DataTo(&s); err != nil {
			return nil, goerr.Wrap(err, "failed to decode action step",
				goerr.V("doc_id", doc.Ref.ID))
		}
		steps = append(steps, &s)
	}
	return steps, nil
}

func (r *actionStepRepository) Delete(ctx context.Context, workspaceID string, actionID int64, stepID string) error {
	ref := r.stepsCollection(workspaceID, actionID).Doc(stepID)
	if _, err := ref.Delete(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return goerr.Wrap(err, "failed to delete action step",
			goerr.V("workspace_id", workspaceID),
			goerr.V("action_id", actionID),
			goerr.V("step_id", stepID))
	}
	return nil
}
