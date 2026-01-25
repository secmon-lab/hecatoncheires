package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

type UseCases struct {
	repo             interfaces.Repository
	riskConfig       *config.RiskConfig
	notion           notion.Service
	slackService     slack.Service
	knowledgeService knowledge.Service
	Risk             *RiskUseCase
	Response         *ResponseUseCase
	Auth             AuthUseCaseInterface
	Slack            *SlackUseCases
	Source           *SourceUseCase
	Compile          *CompileUseCase
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

func WithSlackService(svc slack.Service) Option {
	return func(uc *UseCases) {
		uc.slackService = svc
	}
}

func WithKnowledgeService(svc knowledge.Service) Option {
	return func(uc *UseCases) {
		uc.knowledgeService = svc
	}
}

func New(repo interfaces.Repository, opts ...Option) *UseCases {
	uc := &UseCases{
		repo: repo,
	}

	for _, opt := range opts {
		opt(uc)
	}

	uc.Risk = NewRiskUseCase(repo, uc.riskConfig, uc.slackService)
	uc.Response = NewResponseUseCase(repo)
	uc.Slack = NewSlackUseCases(repo)
	uc.Source = NewSourceUseCase(repo, uc.notion, uc.slackService)
	uc.Compile = NewCompileUseCase(repo, uc.notion, uc.knowledgeService)

	return uc
}

// SlackService returns the Slack service (may be nil if not configured)
func (uc *UseCases) SlackService() slack.Service {
	return uc.slackService
}
