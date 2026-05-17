package memory

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type caseProposalRepository struct {
	mu        sync.RWMutex
	proposals map[model.CaseProposalID]*model.CaseProposal
}

func newCaseProposalRepository() *caseProposalRepository {
	return &caseProposalRepository{
		proposals: make(map[model.CaseProposalID]*model.CaseProposal),
	}
}

func cloneCaseProposal(d *model.CaseProposal) *model.CaseProposal {
	if d == nil {
		return nil
	}
	cp := *d
	if d.RawMessages != nil {
		cp.RawMessages = make([]model.ProposalMessage, len(d.RawMessages))
		copy(cp.RawMessages, d.RawMessages)
	}
	cp.Materialization = cloneMaterialization(d.Materialization)
	return &cp
}

func cloneMaterialization(m *model.WorkspaceMaterialization) *model.WorkspaceMaterialization {
	if m == nil {
		return nil
	}
	cp := *m
	if m.CustomFieldValues != nil {
		cp.CustomFieldValues = make(map[string]model.FieldValue, len(m.CustomFieldValues))
		maps.Copy(cp.CustomFieldValues, m.CustomFieldValues)
	}
	return &cp
}

func (r *caseProposalRepository) Save(ctx context.Context, proposal *model.CaseProposal) error {
	if proposal == nil {
		return goerr.New("proposal is nil")
	}
	if proposal.ID == "" {
		return goerr.New("proposal ID is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.proposals[proposal.ID] = cloneCaseProposal(proposal)
	return nil
}

func (r *caseProposalRepository) Get(ctx context.Context, id model.CaseProposalID) (*model.CaseProposal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.proposals[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
	}
	if d.IsExpired(time.Now().UTC()) {
		return nil, goerr.Wrap(ErrNotFound, "case proposal expired", goerr.V("proposalID", id))
	}
	return cloneCaseProposal(d), nil
}

func (r *caseProposalRepository) SetMaterialization(
	ctx context.Context,
	id model.CaseProposalID,
	workspaceID string,
	m *model.WorkspaceMaterialization,
	inProgress bool,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.proposals[id]
	if !ok {
		return goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
	}

	d.SelectedWorkspaceID = workspaceID
	d.Materialization = cloneMaterialization(m)
	d.InferenceInProgress = inProgress
	return nil
}

func (r *caseProposalRepository) Delete(ctx context.Context, id model.CaseProposalID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.proposals[id]; !ok {
		return goerr.Wrap(ErrNotFound, "case proposal not found", goerr.V("proposalID", id))
	}
	delete(r.proposals, id)
	return nil
}
