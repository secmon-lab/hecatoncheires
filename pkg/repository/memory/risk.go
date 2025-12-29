package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type riskRepository struct {
	mu     sync.RWMutex
	risks  map[int64]*model.Risk
	nextID int64
}

func newRiskRepository() *riskRepository {
	return &riskRepository{
		risks:  make(map[int64]*model.Risk),
		nextID: 1,
	}
}

// copyRisk creates a deep copy of a risk
func copyRisk(risk *model.Risk) *model.Risk {
	categoryIDs := make([]types.CategoryID, len(risk.CategoryIDs))
	copy(categoryIDs, risk.CategoryIDs)

	teamIDs := make([]types.TeamID, len(risk.ResponseTeamIDs))
	copy(teamIDs, risk.ResponseTeamIDs)

	assigneeIDs := make([]string, len(risk.AssigneeIDs))
	copy(assigneeIDs, risk.AssigneeIDs)

	return &model.Risk{
		ID:                  risk.ID,
		Name:                risk.Name,
		Description:         risk.Description,
		CategoryIDs:         categoryIDs,
		SpecificImpact:      risk.SpecificImpact,
		LikelihoodID:        risk.LikelihoodID,
		ImpactID:            risk.ImpactID,
		ResponseTeamIDs:     teamIDs,
		AssigneeIDs:         assigneeIDs,
		DetectionIndicators: risk.DetectionIndicators,
		CreatedAt:           risk.CreatedAt,
		UpdatedAt:           risk.UpdatedAt,
	}
}

func (r *riskRepository) Create(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := copyRisk(risk)
	created.ID = r.nextID
	created.CreatedAt = now
	created.UpdatedAt = now
	r.nextID++

	r.risks[created.ID] = created
	return copyRisk(created), nil
}

func (r *riskRepository) Get(ctx context.Context, id int64) (*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	risk, exists := r.risks[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", id))
	}

	// Return a copy to prevent external modification
	return copyRisk(risk), nil
}

func (r *riskRepository) List(ctx context.Context) ([]*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	risks := make([]*model.Risk, 0, len(r.risks))
	for _, risk := range r.risks {
		risks = append(risks, copyRisk(risk))
	}

	return risks, nil
}

func (r *riskRepository) Update(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.risks[risk.ID]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", risk.ID))
	}

	updated := copyRisk(risk)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	r.risks[updated.ID] = updated
	return copyRisk(updated), nil
}

func (r *riskRepository) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.risks[id]; !exists {
		return goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", id))
	}

	delete(r.risks, id)
	return nil
}
