package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
)

type UseCases struct {
	repo       interfaces.Repository
	riskConfig *config.RiskConfig
	notion     notion.Service
	Risk       *RiskUseCase
	Response   *ResponseUseCase
	Auth       AuthUseCaseInterface
	Slack      *SlackUseCases
	Source     *SourceUseCase
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

func WithNotion(svc notion.Service) Option {
	return func(uc *UseCases) {
		uc.notion = svc
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
	uc.Slack = NewSlackUseCases(repo)
	uc.Source = NewSourceUseCase(repo, uc.notion)

	return uc
}
