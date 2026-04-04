package usecase

import (
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/github"
	"github.com/secmon-lab/hecatoncheires/pkg/service/knowledge"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

type UseCases struct {
	repo              interfaces.Repository
	workspaceRegistry *model.WorkspaceRegistry
	notion            notion.Service
	slackService      slack.Service
	githubService     github.Service
	knowledgeService  knowledge.Service
	llmClient         gollem.LLMClient
	baseURL           string
	Case              *CaseUseCase
	Action            *ActionUseCase
	Agent             *AgentUseCase
	Auth              AuthUseCaseInterface
	Slack             *SlackUseCases
	Source            *SourceUseCase
	Compile           *CompileUseCase
	Assist            *AssistUseCase
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

func WithGitHubService(svc github.Service) Option {
	return func(uc *UseCases) {
		uc.githubService = svc
	}
}

func WithLLMClient(client gollem.LLMClient) Option {
	return func(uc *UseCases) {
		uc.llmClient = client
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
	uc.Source = NewSourceUseCase(repo, uc.notion, uc.slackService, uc.githubService)
	uc.Compile = NewCompileUseCase(repo, registry, uc.notion, uc.knowledgeService, uc.slackService, uc.githubService, uc.baseURL)

	// Create AgentUseCase and AssistUseCase only if LLM client and Slack service are both available
	if uc.llmClient != nil && uc.slackService != nil {
		uc.Agent = NewAgentUseCase(repo, registry, uc.slackService, uc.llmClient)
		uc.Assist = NewAssistUseCase(repo, registry, uc.slackService, uc.llmClient)
	}
	uc.Slack = NewSlackUseCases(repo, registry, uc.Agent, uc.slackService)

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
