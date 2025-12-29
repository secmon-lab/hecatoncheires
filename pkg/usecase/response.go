package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

type ResponseUseCase struct {
	repo interfaces.Repository
}

func NewResponseUseCase(repo interfaces.Repository) *ResponseUseCase {
	return &ResponseUseCase{
		repo: repo,
	}
}

func (uc *ResponseUseCase) CreateResponse(ctx context.Context, title, description string, responderIDs []string, url string, status types.ResponseStatus, riskIDs []int64) (*model.Response, error) {
	if title == "" {
		return nil, goerr.New("response title is required")
	}

	// Default status to backlog if not provided
	if status == "" {
		status = types.ResponseStatusBacklog
	}

	// Validate status
	if !status.IsValid() {
		return nil, goerr.New("invalid response status", goerr.V("status", status))
	}

	// Ensure responderIDs is not nil
	if responderIDs == nil {
		responderIDs = []string{}
	}

	response := &model.Response{
		Title:        title,
		Description:  description,
		ResponderIDs: responderIDs,
		URL:          url,
		Status:       status,
	}

	created, err := uc.repo.Response().Create(ctx, response)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create response")
	}

	// Link to risks if provided
	for _, riskID := range riskIDs {
		if err := uc.repo.RiskResponse().Link(ctx, riskID, created.ID); err != nil {
			// Rollback: delete the created response to maintain atomicity
			if delErr := uc.repo.Response().Delete(ctx, created.ID); delErr != nil {
				// Log cleanup error but return the primary linking error
				return nil, goerr.Wrap(err, "failed to link response to risk and rollback failed",
					goerr.V("responseID", created.ID),
					goerr.V("riskID", riskID),
					goerr.V("rollbackError", delErr))
			}
			return nil, goerr.Wrap(err, "failed to link response to risk, response creation rolled back",
				goerr.V("responseID", created.ID),
				goerr.V("riskID", riskID))
		}
	}

	return created, nil
}

func (uc *ResponseUseCase) UpdateResponse(ctx context.Context, id int64, title *string, description *string, responderIDs []string, url *string, status *types.ResponseStatus, riskIDs []int64) (*model.Response, error) {
	// Get existing response
	existing, err := uc.repo.Response().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get existing response", goerr.V("id", id))
	}

	// Build response with only updated fields
	response := &model.Response{
		ID:           id,
		Title:        existing.Title,
		Description:  existing.Description,
		ResponderIDs: existing.ResponderIDs,
		URL:          existing.URL,
		Status:       existing.Status,
		CreatedAt:    existing.CreatedAt,
	}

	// Update only provided fields
	if title != nil {
		if *title == "" {
			return nil, goerr.New("response title cannot be empty")
		}
		response.Title = *title
	}

	if description != nil {
		response.Description = *description
	}

	if responderIDs != nil {
		response.ResponderIDs = responderIDs
	}

	if url != nil {
		response.URL = *url
	}

	if status != nil {
		if !status.IsValid() {
			return nil, goerr.New("invalid response status", goerr.V("status", *status))
		}
		response.Status = *status
	}

	updated, err := uc.repo.Response().Update(ctx, response)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update response", goerr.V("id", id))
	}

	// Update risk associations if riskIDs provided
	if riskIDs != nil {
		// Get current risk associations
		currentRisks, err := uc.repo.RiskResponse().GetRisksByResponse(ctx, id)
		if err != nil {
			return updated, goerr.Wrap(err, "failed to get current risk associations")
		}

		// Build sets for comparison
		currentRiskIDSet := make(map[int64]bool)
		for _, risk := range currentRisks {
			currentRiskIDSet[risk.ID] = true
		}

		newRiskIDSet := make(map[int64]bool)
		for _, riskID := range riskIDs {
			newRiskIDSet[riskID] = true
		}

		// Remove links that are no longer needed
		for riskID := range currentRiskIDSet {
			if !newRiskIDSet[riskID] {
				if err := uc.repo.RiskResponse().Unlink(ctx, riskID, id); err != nil {
					return updated, goerr.Wrap(err, "failed to unlink response from risk",
						goerr.V("responseID", id),
						goerr.V("riskID", riskID))
				}
			}
		}

		// Add new links
		for riskID := range newRiskIDSet {
			if !currentRiskIDSet[riskID] {
				if err := uc.repo.RiskResponse().Link(ctx, riskID, id); err != nil {
					return updated, goerr.Wrap(err, "failed to link response to risk",
						goerr.V("responseID", id),
						goerr.V("riskID", riskID))
				}
			}
		}
	}

	return updated, nil
}

func (uc *ResponseUseCase) DeleteResponse(ctx context.Context, id int64) error {
	// Delete all risk-response links first
	if err := uc.repo.RiskResponse().DeleteByResponse(ctx, id); err != nil {
		return goerr.Wrap(err, "failed to delete risk-response links", goerr.V("id", id))
	}

	// Delete the response
	if err := uc.repo.Response().Delete(ctx, id); err != nil {
		return goerr.Wrap(err, "failed to delete response", goerr.V("id", id))
	}

	return nil
}

func (uc *ResponseUseCase) GetResponse(ctx context.Context, id int64) (*model.Response, error) {
	response, err := uc.repo.Response().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get response", goerr.V("id", id))
	}

	return response, nil
}

func (uc *ResponseUseCase) ListResponses(ctx context.Context) ([]*model.Response, error) {
	responses, err := uc.repo.Response().List(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list responses")
	}

	return responses, nil
}

func (uc *ResponseUseCase) LinkResponseToRisk(ctx context.Context, responseID, riskID int64) error {
	// Verify response exists
	if _, err := uc.repo.Response().Get(ctx, responseID); err != nil {
		return goerr.Wrap(err, "response not found", goerr.V("responseID", responseID))
	}

	// Verify risk exists
	if _, err := uc.repo.Risk().Get(ctx, riskID); err != nil {
		return goerr.Wrap(err, "risk not found", goerr.V("riskID", riskID))
	}

	if err := uc.repo.RiskResponse().Link(ctx, riskID, responseID); err != nil {
		return goerr.Wrap(err, "failed to link response to risk",
			goerr.V("responseID", responseID),
			goerr.V("riskID", riskID))
	}

	return nil
}

func (uc *ResponseUseCase) UnlinkResponseFromRisk(ctx context.Context, responseID, riskID int64) error {
	if err := uc.repo.RiskResponse().Unlink(ctx, riskID, responseID); err != nil {
		return goerr.Wrap(err, "failed to unlink response from risk",
			goerr.V("responseID", responseID),
			goerr.V("riskID", riskID))
	}

	return nil
}

func (uc *ResponseUseCase) GetResponsesByRisk(ctx context.Context, riskID int64) ([]*model.Response, error) {
	responses, err := uc.repo.RiskResponse().GetResponsesByRisk(ctx, riskID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get responses by risk", goerr.V("riskID", riskID))
	}

	return responses, nil
}

func (uc *ResponseUseCase) GetRisksByResponse(ctx context.Context, responseID int64) ([]*model.Risk, error) {
	risks, err := uc.repo.RiskResponse().GetRisksByResponse(ctx, responseID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get risks by response", goerr.V("responseID", responseID))
	}

	return risks, nil
}
