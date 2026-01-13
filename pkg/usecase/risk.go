package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

type RiskUseCase struct {
	repo         interfaces.Repository
	riskConfig   *config.RiskConfig
	slackService slack.Service
}

func NewRiskUseCase(repo interfaces.Repository, cfg *config.RiskConfig, slackService slack.Service) *RiskUseCase {
	return &RiskUseCase{
		repo:         repo,
		riskConfig:   cfg,
		slackService: slackService,
	}
}

func (uc *RiskUseCase) CreateRisk(ctx context.Context, name, description string, categoryIDs []types.CategoryID, specificImpact string, likelihoodID types.LikelihoodID, impactID types.ImpactID, responseTeamIDs []types.TeamID, assigneeIDs []string, detectionIndicators string) (*model.Risk, error) {
	if name == "" {
		return nil, goerr.New("risk name is required")
	}

	// Validate category IDs
	for _, id := range categoryIDs {
		if err := uc.ValidateCategoryID(id); err != nil {
			return nil, goerr.Wrap(err, "invalid category ID")
		}
	}

	// Validate likelihood ID
	if err := uc.ValidateLikelihoodID(likelihoodID); err != nil {
		return nil, goerr.Wrap(err, "invalid likelihood ID")
	}

	// Validate impact ID
	if err := uc.ValidateImpactID(impactID); err != nil {
		return nil, goerr.Wrap(err, "invalid impact ID")
	}

	// Validate team IDs
	for _, id := range responseTeamIDs {
		if err := uc.ValidateTeamID(id); err != nil {
			return nil, goerr.Wrap(err, "invalid team ID")
		}
	}

	risk := &model.Risk{
		Name:                name,
		Description:         description,
		CategoryIDs:         categoryIDs,
		SpecificImpact:      specificImpact,
		LikelihoodID:        likelihoodID,
		ImpactID:            impactID,
		ResponseTeamIDs:     responseTeamIDs,
		AssigneeIDs:         assigneeIDs,
		DetectionIndicators: detectionIndicators,
	}

	// Create risk first to get ID
	created, err := uc.repo.Risk().Create(ctx, risk)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create risk")
	}

	// Create Slack channel if service is available
	if uc.slackService != nil {
		channelID, err := uc.slackService.CreateChannel(ctx, created.ID, created.Name)
		if err != nil {
			// Rollback: delete the created risk (best effort)
			_ = uc.repo.Risk().Delete(ctx, created.ID)
			return nil, goerr.Wrap(err, "failed to create Slack channel for risk")
		}

		// Update risk with channel ID
		created.SlackChannelID = channelID
		updated, err := uc.repo.Risk().Update(ctx, created)
		if err != nil {
			// Note: Channel is created but risk update failed
			// We don't attempt to delete the channel here
			return nil, goerr.Wrap(err, "failed to update risk with Slack channel ID")
		}
		return updated, nil
	}

	return created, nil
}

func (uc *RiskUseCase) UpdateRisk(ctx context.Context, id int64, name, description string, categoryIDs []types.CategoryID, specificImpact string, likelihoodID types.LikelihoodID, impactID types.ImpactID, responseTeamIDs []types.TeamID, assigneeIDs []string, detectionIndicators string) (*model.Risk, error) {
	if name == "" {
		return nil, goerr.New("risk name is required")
	}

	// Get existing risk to check if name changed
	existingRisk, err := uc.repo.Risk().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get risk")
	}

	// Validate category IDs
	for _, cid := range categoryIDs {
		if err := uc.ValidateCategoryID(cid); err != nil {
			return nil, goerr.Wrap(err, "invalid category ID")
		}
	}

	// Validate likelihood ID
	if err := uc.ValidateLikelihoodID(likelihoodID); err != nil {
		return nil, goerr.Wrap(err, "invalid likelihood ID")
	}

	// Validate impact ID
	if err := uc.ValidateImpactID(impactID); err != nil {
		return nil, goerr.Wrap(err, "invalid impact ID")
	}

	// Validate team IDs
	for _, tid := range responseTeamIDs {
		if err := uc.ValidateTeamID(tid); err != nil {
			return nil, goerr.Wrap(err, "invalid team ID")
		}
	}

	// Rename Slack channel if name changed and channel exists
	if uc.slackService != nil && existingRisk.SlackChannelID != "" && existingRisk.Name != name {
		if err := uc.slackService.RenameChannel(ctx, existingRisk.SlackChannelID, id, name); err != nil {
			return nil, goerr.Wrap(err, "failed to rename Slack channel")
		}
	}

	risk := &model.Risk{
		ID:                  id,
		Name:                name,
		Description:         description,
		CategoryIDs:         categoryIDs,
		SpecificImpact:      specificImpact,
		LikelihoodID:        likelihoodID,
		ImpactID:            impactID,
		ResponseTeamIDs:     responseTeamIDs,
		AssigneeIDs:         assigneeIDs,
		DetectionIndicators: detectionIndicators,
		SlackChannelID:      existingRisk.SlackChannelID, // Preserve channel ID
	}

	updated, err := uc.repo.Risk().Update(ctx, risk)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to update risk")
	}

	return updated, nil
}

func (uc *RiskUseCase) DeleteRisk(ctx context.Context, id int64) error {
	if err := uc.repo.Risk().Delete(ctx, id); err != nil {
		return goerr.Wrap(err, "failed to delete risk")
	}

	return nil
}

func (uc *RiskUseCase) GetRisk(ctx context.Context, id int64) (*model.Risk, error) {
	risk, err := uc.repo.Risk().Get(ctx, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get risk")
	}

	return risk, nil
}

func (uc *RiskUseCase) ListRisks(ctx context.Context) ([]*model.Risk, error) {
	risks, err := uc.repo.Risk().List(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list risks")
	}

	return risks, nil
}

func (uc *RiskUseCase) GetRiskConfiguration() (*config.RiskConfig, error) {
	if uc.riskConfig == nil {
		return &config.RiskConfig{}, nil
	}
	return uc.riskConfig, nil
}

func (uc *RiskUseCase) ValidateCategoryID(id types.CategoryID) error {
	if uc.riskConfig == nil {
		return nil
	}

	for _, cat := range uc.riskConfig.Categories {
		if types.CategoryID(cat.ID) == id {
			return nil
		}
	}

	return goerr.New("category ID not found in configuration", goerr.V("id", id))
}

func (uc *RiskUseCase) ValidateLikelihoodID(id types.LikelihoodID) error {
	if uc.riskConfig == nil {
		return nil
	}

	for _, level := range uc.riskConfig.Likelihood {
		if types.LikelihoodID(level.ID) == id {
			return nil
		}
	}

	return goerr.New("likelihood ID not found in configuration", goerr.V("id", id))
}

func (uc *RiskUseCase) ValidateImpactID(id types.ImpactID) error {
	if uc.riskConfig == nil {
		return nil
	}

	for _, level := range uc.riskConfig.Impact {
		if types.ImpactID(level.ID) == id {
			return nil
		}
	}

	return goerr.New("impact ID not found in configuration", goerr.V("id", id))
}

func (uc *RiskUseCase) ValidateTeamID(id types.TeamID) error {
	if uc.riskConfig == nil {
		return nil
	}

	for _, team := range uc.riskConfig.Teams {
		if types.TeamID(team.ID) == id {
			return nil
		}
	}

	return goerr.New("team ID not found in configuration", goerr.V("id", id))
}
