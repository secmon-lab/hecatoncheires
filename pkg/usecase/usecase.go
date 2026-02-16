package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

type UseCases struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	notion            notion.Service
	slackService      slack.Service
	knowledgeService  knowledge.Service
	baseURL           string
	Case              *CaseUseCase
	Action            *ActionUseCase
	Auth              AuthUseCaseInterface
	Slack             *SlackUseCases
	Source            *SourceUseCase
	Compile           *CompileUseCase
}

type Option func(*UseCases)

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

func WithBaseURL(url string) Option {
	return func(uc *UseCases) {
		uc.baseURL = url
	}
}

func New(repo interfaces.Repository, registry *model.WorkspaceRegistry, opts ...Option) *UseCases {
	uc := &UseCases{
		repo:              repo,
		workspaceRegistry: registry,
	}

	for _, opt := range opts {
		opt(uc)
	}

	uc.Case = NewCaseUseCase(repo, registry, uc.slackService, uc.baseURL)
	uc.Action = NewActionUseCase(repo, uc.slackService, uc.baseURL)
	uc.Slack = NewSlackUseCases(repo, registry)
	uc.Source = NewSourceUseCase(repo, uc.notion, uc.slackService)
	uc.Compile = NewCompileUseCase(repo, registry, uc.notion, uc.knowledgeService, uc.slackService, uc.baseURL)

	return uc
}

// WorkspaceRegistry returns the workspace registry
func (uc *UseCases) WorkspaceRegistry() *model.WorkspaceRegistry {
	return uc.workspaceRegistry
}

// SlackService returns the Slack service (may be nil if not configured)
func (uc *UseCases) SlackService() slack.Service {
	return uc.slackService
}
