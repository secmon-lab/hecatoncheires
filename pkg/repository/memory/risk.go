package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
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

func (r *riskRepository) Create(ctx context.Context, risk *model.Risk) (*model.Risk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	created := &model.Risk{
		ID:          r.nextID,
		Name:        risk.Name,
		Description: risk.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	r.nextID++

	r.risks[created.ID] = created
	return created, nil
}

func (r *riskRepository) Get(ctx context.Context, id int64) (*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	risk, exists := r.risks[id]
	if !exists {
		return nil, goerr.Wrap(ErrNotFound, "risk not found", goerr.V("id", id))
	}

	// Return a copy to prevent external modification
	return &model.Risk{
		ID:          risk.ID,
		Name:        risk.Name,
		Description: risk.Description,
		CreatedAt:   risk.CreatedAt,
		UpdatedAt:   risk.UpdatedAt,
	}, nil
}

func (r *riskRepository) List(ctx context.Context) ([]*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	risks := make([]*model.Risk, 0, len(r.risks))
	for _, risk := range r.risks {
		risks = append(risks, &model.Risk{
			ID:          risk.ID,
			Name:        risk.Name,
			Description: risk.Description,
			CreatedAt:   risk.CreatedAt,
			UpdatedAt:   risk.UpdatedAt,
		})
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

	updated := &model.Risk{
		ID:          existing.ID,
		Name:        risk.Name,
		Description: risk.Description,
		CreatedAt:   existing.CreatedAt,
		UpdatedAt:   time.Now().UTC(),
	}

	r.risks[updated.ID] = updated
	return updated, nil
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
