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

// CaseProposals are stored under the subcollection drafts/case/items/{id}.
// The "drafts" collection name is legacy: this entity was previously named
// CaseDraft, and the path is kept to preserve any in-flight documents
// across the rename. The parent doc drafts/case carries no payload; it
// exists solely to host the items subcollection.
const (
	proposalsCollection      = "drafts"
	caseProposalKindDocID    = "case"
	caseProposalItemsSubColl = "items"
)

type caseProposalRepository struct {
	client *firestore.Client
}

func newCaseProposalRepository(client *firestore.Client) *caseProposalRepository {
	return &caseProposalRepository{client: client}
}

func (r *caseProposalRepository) docRef(id model.CaseProposalID) *firestore.DocumentRef {
	return r.client.
		Collection(proposalsCollection).
		Doc(caseProposalKindDocID).
		Collection(caseProposalItemsSubColl).
		Doc(id.String())
}

func (r *caseProposalRepository) Save(ctx context.Context, proposal *model.CaseProposal) error {
	if proposal == nil {
		return goerr.New("proposal is nil")
	}
	if proposal.ID == "" {
		return goerr.New("proposal ID is empty")
	}

	if _, err := r.docRef(proposal.ID).Set(ctx, proposal); err != nil {
		return goerr.Wrap(err, "failed to save case proposal", goerr.V("proposalID", proposal.ID))
	}
	return nil
}

func (r *caseProposalRepository) Get(ctx context.Context, id model.CaseProposalID) (*model.CaseProposal, error) {
	doc, err := r.docRef(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
		}
		return nil, goerr.Wrap(err, "failed to get case proposal", goerr.V("proposalID", id))
	}

	var d model.CaseProposal
	if err := doc.DataTo(&d); err != nil {
		return nil, goerr.Wrap(err, "failed to decode case proposal", goerr.V("proposalID", id))
	}
	if d.IsExpired(time.Now().UTC()) {
		return nil, goerr.Wrap(ErrNotFound, "case proposal expired", goerr.V("proposalID", id))
	}
	return &d, nil
}

func (r *caseProposalRepository) SetMaterialization(
	ctx context.Context,
	id model.CaseProposalID,
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
			return goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
		}
		return goerr.Wrap(err, "failed to update materialization", goerr.V("proposalID", id))
	}
	return nil
}

func (r *caseProposalRepository) Delete(ctx context.Context, id model.CaseProposalID) error {
	docRef := r.docRef(id)
	if _, err := docRef.Get(ctx); err != nil {
		if status.Code(err) == codes.NotFound {
			return goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
		}
		return goerr.Wrap(err, "failed to get case proposal for delete", goerr.V("proposalID", id))
	}
	if _, err := docRef.Delete(ctx); err != nil {
		return goerr.Wrap(err, "failed to delete case proposal", goerr.V("proposalID", id))
	}
	return nil
}
