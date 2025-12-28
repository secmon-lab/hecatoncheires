package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

type RiskUseCase struct {
	repo interfaces.Repository
}

func NewRiskUseCase(repo interfaces.Repository) *RiskUseCase {
	return &RiskUseCase{
		repo: repo,
	}
}

func (uc *RiskUseCase) CreateRisk(ctx context.Context, name, description string) (*model.Risk, error) {
	if name == "" {
		return nil, goerr.New("risk name is required")
	}

	risk := &model.Risk{
		Name:        name,
		Description: description,
	}

	created, err := uc.repo.Risk().Create(ctx, risk)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create risk")
	}

	return created, nil
}

func (uc *RiskUseCase) UpdateRisk(ctx context.Context, id int64, name, description string) (*model.Risk, error) {
	if name == "" {
		return nil, goerr.New("risk name is required")
	}

	risk := &model.Risk{
		ID:          id,
		Name:        name,
		Description: description,
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
