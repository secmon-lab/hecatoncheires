package memory

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type caseDraftRepository struct {
	mu     sync.RWMutex
	drafts map[model.CaseDraftID]*model.CaseDraft
}

func newCaseDraftRepository() *caseDraftRepository {
	return &caseDraftRepository{
		drafts: make(map[model.CaseDraftID]*model.CaseDraft),
	}
}

func cloneCaseDraft(d *model.CaseDraft) *model.CaseDraft {
	if d == nil {
		return nil
	}
	cp := *d
	if d.RawMessages != nil {
		cp.RawMessages = make([]model.DraftMessage, len(d.RawMessages))
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

func (r *caseDraftRepository) Save(ctx context.Context, draft *model.CaseDraft) error {
	if draft == nil {
		return goerr.New("draft is nil")
	}
	if draft.ID == "" {
		return goerr.New("draft ID is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.drafts[draft.ID] = cloneCaseDraft(draft)
	return nil
}

func (r *caseDraftRepository) Get(ctx context.Context, id model.CaseDraftID) (*model.CaseDraft, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.drafts[id]
	if !ok {
		return nil, goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
	}
	if d.IsExpired(time.Now().UTC()) {
		return nil, goerr.Wrap(ErrNotFound, "case draft expired", goerr.V("draftID", id))
	}
	return cloneCaseDraft(d), nil
}

func (r *caseDraftRepository) SetMaterialization(
	ctx context.Context,
	id model.CaseDraftID,
	workspaceID string,
	m *model.WorkspaceMaterialization,
	inProgress bool,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.drafts[id]
	if !ok {
		return goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
	}

	d.SelectedWorkspaceID = workspaceID
	d.Materialization = cloneMaterialization(m)
	d.InferenceInProgress = inProgress
	return nil
}

func (r *caseDraftRepository) Delete(ctx context.Context, id model.CaseDraftID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.drafts[id]; !ok {
		return goerr.Wrap(ErrNotFound, "case draft not found", goerr.V("draftID", id))
	}
	delete(r.drafts, id)
	return nil
}
