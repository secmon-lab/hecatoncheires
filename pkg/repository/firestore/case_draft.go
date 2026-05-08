package firestore

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Drafts are stored under the subcollection drafts/case/items/{id}.
// The parent doc drafts/case carries no payload; it exists solely to host
// the items subcollection so future kinds (e.g. drafts/action/items) can
// coexist without underscore-joined collection names.
const (
	draftsCollection      = "drafts"
	caseDraftKindDocID    = "case"
	caseDraftItemsSubColl = "items"
)

type caseDraftRepository struct {
	client *firestore.Client
}

func newCaseDraftRepository(client *firestore.Client) *caseDraftRepository {
	return &caseDraftRepository{client: client}
}

func (r *caseDraftRepository) docRef(id model.CaseDraftID) *firestore.DocumentRef {
	return r.client.
		Collection(draftsCollection).
		Doc(caseDraftKindDocID).
		Collection(caseDraftItemsSubColl).
		Doc(id.String())
}

func (r *caseDraftRepository) Save(ctx context.Context, draft *model.CaseDraft) error {
	if draft == nil {
		return goerr.New("draft is nil")
	}
	if draft.ID == "" {
		return goerr.New("draft ID is empty")
	}

	if _, err := r.docRef(draft.ID).Set(ctx, draft); err != nil {
		return goerr.Wrap(err, "failed to save case draft", goerr.V("draftID", draft.ID))
	}
	return nil
}

func (r *caseDraftRepository) Get(ctx context.Context, id model.CaseDraftID) (*model.CaseDraft, error) {
	doc, err := r.docRef(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
		}
		return nil, goerr.Wrap(err, "failed to get case draft", goerr.V("draftID", id))
	}

	var d model.CaseDraft
	if err := doc.DataTo(&d); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case draft", goerr.V("draftID", id))
	}
	if d.IsExpired(time.Now().UTC()) {
		return nil, goerr.Wrap(ErrNotFound, "case draft expired", goerr.V("draftID", id))
	}
	return &d, nil
}

func (r *caseDraftRepository) SetMaterialization(
	ctx context.Context,
	id model.CaseDraftID,
	workspaceID string,
	m *model.WorkspaceMaterialization,
	inProgress bool,
) error {
	updates := []firestore.Update{
		{Path: "SelectedWorkspaceID", Value: workspaceID},
		{Path: "Materialization", Value: m},
		{Path: "InferenceInProgress", Value: inProgress},
	}
	if _, err := r.docRef(id).Update(ctx, updates); err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
		}
		return goerr.Wrap(err, "failed to update materialization", goerr.V("draftID", id))
	}
	return nil
}

func (r *caseDraftRepository) Delete(ctx context.Context, id model.CaseDraftID) error {
	docRef := r.docRef(id)
	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
		}
		return goerr.Wrap(err, "failed to get case draft for delete", goerr.V("draftID", id))
	}
	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete case draft", goerr.V("draftID", id))
	}
	return nil
}
