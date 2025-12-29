package memory

import (
	"context"
	"sync"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type riskResponseRepository struct {
	mu            sync.RWMutex
	riskResponses []model.RiskResponse
	// References to other repositories for join operations
	responseRepo *responseRepository
	riskRepo     *riskRepository
}

func newRiskResponseRepository(responseRepo *responseRepository, riskRepo *riskRepository) *riskResponseRepository {
	return &riskResponseRepository{
		riskResponses: make([]model.RiskResponse, 0),
		responseRepo:  responseRepo,
		riskRepo:      riskRepo,
	}
}

func (r *riskResponseRepository) Link(ctx context.Context, riskID, responseID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if the link already exists
	for _, rr := range r.riskResponses {
		if rr.RiskID == riskID && rr.ResponseID == responseID {
			return nil // Already linked, not an error
		}
	}

	// Verify that both risk and response exist
	if _, err := r.riskRepo.Get(ctx, riskID); err != nil {
		return goerr.Wrap(err, "risk not found", goerr.V("riskID", riskID))
	}

	if _, err := r.responseRepo.Get(ctx, responseID); err != nil {
		return goerr.Wrap(err, "response not found", goerr.V("responseID", responseID))
	}

	r.riskResponses = append(r.riskResponses, model.RiskResponse{
		RiskID:     riskID,
		ResponseID: responseID,
		CreatedAt:  time.Now().UTC(),
	})

	return nil
}

func (r *riskResponseRepository) Unlink(ctx context.Context, riskID, responseID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, rr := range r.riskResponses {
		if rr.RiskID == riskID && rr.ResponseID == responseID {
			// Remove the link
			r.riskResponses = append(r.riskResponses[:i], r.riskResponses[i+1:]...)
			return nil
		}
	}

	return goerr.Wrap(ErrNotFound, "risk-response link not found",
		goerr.V("riskID", riskID),
		goerr.V("responseID", responseID))
}

func (r *riskResponseRepository) GetResponsesByRisk(ctx context.Context, riskID int64) ([]*model.Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	responseIDs := make([]int64, 0)
	for _, rr := range r.riskResponses {
		if rr.RiskID == riskID {
			responseIDs = append(responseIDs, rr.ResponseID)
		}
	}

	responses := make([]*model.Response, 0, len(responseIDs))
	for _, id := range responseIDs {
		resp, err := r.responseRepo.Get(ctx, id)
		if err != nil {
			// Skip if response was deleted
			continue
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

func (r *riskResponseRepository) GetResponsesByRisks(ctx context.Context, riskIDs []int64) (map[int64][]*model.Response, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build a map of riskID to responseIDs
	riskToResponseIDs := make(map[int64][]int64)
	for _, riskID := range riskIDs {
		riskToResponseIDs[riskID] = make([]int64, 0)
	}

	for _, rr := range r.riskResponses {
		if _, exists := riskToResponseIDs[rr.RiskID]; exists {
			riskToResponseIDs[rr.RiskID] = append(riskToResponseIDs[rr.RiskID], rr.ResponseID)
		}
	}

	// Fetch all responses
	result := make(map[int64][]*model.Response)
	for riskID, responseIDs := range riskToResponseIDs {
		responses := make([]*model.Response, 0, len(responseIDs))
		for _, respID := range responseIDs {
			resp, err := r.responseRepo.Get(ctx, respID)
			if err != nil {
				// Skip if response was deleted
				continue
			}
			responses = append(responses, resp)
		}
		result[riskID] = responses
	}

	return result, nil
}

func (r *riskResponseRepository) GetRisksByResponse(ctx context.Context, responseID int64) ([]*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	riskIDs := make([]int64, 0)
	for _, rr := range r.riskResponses {
		if rr.ResponseID == responseID {
			riskIDs = append(riskIDs, rr.RiskID)
		}
	}

	risks := make([]*model.Risk, 0, len(riskIDs))
	for _, id := range riskIDs {
		risk, err := r.riskRepo.Get(ctx, id)
		if err != nil {
			// Skip if risk was deleted
			continue
		}
		risks = append(risks, risk)
	}

	return risks, nil
}

func (r *riskResponseRepository) GetRisksByResponses(ctx context.Context, responseIDs []int64) (map[int64][]*model.Risk, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build a map of responseID to riskIDs
	responseToRiskIDs := make(map[int64][]int64)
	for _, responseID := range responseIDs {
		responseToRiskIDs[responseID] = make([]int64, 0)
	}

	for _, rr := range r.riskResponses {
		if _, exists := responseToRiskIDs[rr.ResponseID]; exists {
			responseToRiskIDs[rr.ResponseID] = append(responseToRiskIDs[rr.ResponseID], rr.RiskID)
		}
	}

	// Fetch all risks
	result := make(map[int64][]*model.Risk)
	for responseID, riskIDs := range responseToRiskIDs {
		risks := make([]*model.Risk, 0, len(riskIDs))
		for _, riskID := range riskIDs {
			risk, err := r.riskRepo.Get(ctx, riskID)
			if err != nil {
				// Skip if risk was deleted
				continue
			}
			risks = append(risks, risk)
		}
		result[responseID] = risks
	}

	return result, nil
}

func (r *riskResponseRepository) DeleteByResponse(ctx context.Context, responseID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newRiskResponses := make([]model.RiskResponse, 0)
	for _, rr := range r.riskResponses {
		if rr.ResponseID != responseID {
			newRiskResponses = append(newRiskResponses, rr)
		}
	}

	r.riskResponses = newRiskResponses
	return nil
}

func (r *riskResponseRepository) DeleteByRisk(ctx context.Context, riskID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	newRiskResponses := make([]model.RiskResponse, 0)
	for _, rr := range r.riskResponses {
		if rr.RiskID != riskID {
			newRiskResponses = append(newRiskResponses, rr)
		}
	}

	r.riskResponses = newRiskResponses
	return nil
}
