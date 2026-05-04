package usecase

import (
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
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
	notionTool        notiontool.Client
	slackService      slack.Service
	slackAdminService slack.AdminService
	slackSearch       slacktool.SearchService
	githubService     github.Service
	knowledgeService  knowledge.Service
	llmClient         gollem.LLMClient
	embedClient       interfaces.EmbedClient
	historyRepo       gollem.HistoryRepository
	traceRepo         trace.Repository
	baseURL           string
	Case              *CaseUseCase
	Action            *ActionUseCase
	Agent             *AgentUseCase
	Auth              AuthUseCaseInterface
	Slack             *SlackUseCases
	Source            *SourceUseCase
	Compile           *CompileUseCase
	Assist            *AssistUseCase
	MentionDraft      *MentionDraftUseCase
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

func WithSlackAdminService(svc slack.AdminService) Option {
	return func(uc *UseCases) {
		uc.slackAdminService = svc
	}
}

// WithSlackSearchService configures the Slack User-token-backed search client.
// When set, the agent gains the slack__search_messages tool. Requires the
// underlying User OAuth Token to have the search:read scope.
func WithSlackSearchService(svc slacktool.SearchService) Option {
	return func(uc *UseCases) {
		uc.slackSearch = svc
	}
}

// WithNotionToolClient configures the agent-tool Notion client.
// When set, the agent gains the notion__search and notion__get_page tools.
func WithNotionToolClient(c notiontool.Client) Option {
	return func(uc *UseCases) {
		uc.notionTool = c
	}
}

func WithLLMClient(client gollem.LLMClient) Option {
	return func(uc *UseCases) {
		uc.llmClient = client
	}
}

// WithEmbedClient sets the embedding client used by core tools (memory /
// knowledge similarity search) and the knowledge extraction service. Always
// required when the LLM-driven flows are wired; configured separately so it
// can target Gemini regardless of the chat completion provider.
func WithEmbedClient(client interfaces.EmbedClient) Option {
	return func(uc *UseCases) {
		uc.embedClient = client
	}
}

// WithHistoryRepository sets the gollem.HistoryRepository used by the agent
// session flow to persist conversation history across mentions.
func WithHistoryRepository(repo gollem.HistoryRepository) Option {
	return func(uc *UseCases) {
		uc.historyRepo = repo
	}
}

// WithTraceRepository sets the trace.Repository used by the agent session
// flow to persist execution traces.
func WithTraceRepository(repo trace.Repository) Option {
	return func(uc *UseCases) {
		uc.traceRepo = repo
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

	uc.Case = NewCaseUseCase(repo, registry, uc.slackService, uc.slackAdminService, uc.baseURL)
	uc.Action = NewActionUseCase(repo, registry, uc.slackService, uc.baseURL)
	uc.Source = NewSourceUseCase(repo, uc.notion, uc.slackService, uc.githubService)
	uc.Compile = NewCompileUseCase(repo, registry, uc.notion, uc.knowledgeService, uc.slackService, uc.githubService, uc.baseURL)

	// Whenever Slack is wired, LLM and Embed clients must also be wired —
	// Slack-driven flows (agent mention, mention-draft, assist) all require
	// LLM by design, and the agent/assist core tools need an embedder for
	// memory / knowledge similarity search.
	if uc.slackService != nil {
		if uc.llmClient == nil {
			panic("usecase.New: LLM client is required when Slack service is configured (use WithLLMClient)")
		}
		if uc.embedClient == nil {
			panic("usecase.New: Embed client is required when Slack service is configured (use WithEmbedClient)")
		}
		// Agent depends on the persistent History/Trace archive. Callers
		// that drive Slack events (the serve CLI) MUST wire both; callers
		// that only use Assist (the assist CLI) may omit them, in which
		// case the Agent usecase is simply not constructed.
		if uc.historyRepo != nil && uc.traceRepo != nil {
			uc.Agent = NewAgentUseCase(repo, registry, uc.slackService, uc.slackSearch, uc.notionTool, uc.llmClient, uc.embedClient, uc.historyRepo, uc.traceRepo, uc.Action)
		} else if uc.historyRepo != nil || uc.traceRepo != nil {
			panic("usecase.New: WithHistoryRepository and WithTraceRepository must be paired")
		}
		uc.Assist = NewAssistUseCase(repo, registry, uc.slackService, uc.slackSearch, uc.notionTool, uc.llmClient, uc.embedClient, uc.Action)
		uc.MentionDraft = NewMentionDraftUseCase(repo, registry, uc.slackService, NewDraftMaterializer(uc.llmClient))
	}
	uc.Slack = NewSlackUseCases(repo, registry, uc.Agent, uc.MentionDraft, uc.slackService)

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
