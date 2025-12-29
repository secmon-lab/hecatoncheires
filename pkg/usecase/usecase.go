package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

type UseCases struct {
	repo       interfaces.Repository
	riskConfig *config.RiskConfig
	Risk       *RiskUseCase
	Response   *ResponseUseCase
	Auth       AuthUseCaseInterface
}

type Option func(*UseCases)

func WithRiskConfig(cfg *config.RiskConfig) Option {
	return func(uc *UseCases) {
		uc.riskConfig = cfg
	}
}

func WithAuth(auth AuthUseCaseInterface) Option {
	return func(uc *UseCases) {
		uc.Auth = auth
	}
}

func New(repo interfaces.Repository, opts ...Option) *UseCases {
	uc := &UseCases{
		repo: repo,
	}

	for _, opt := range opts {
		opt(uc)
	}

	uc.Risk = NewRiskUseCase(repo, uc.riskConfig)
	uc.Response = NewResponseUseCase(repo)

	return uc
}
