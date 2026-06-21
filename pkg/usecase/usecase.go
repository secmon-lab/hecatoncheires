package usecase

import (
	"context"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/service/notion"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

type UseCases struct {
	repo                     interfaces.Repository
	workspaceRegistry        *model.WorkspaceRegistry
	notion                   notion.Service
	notionTool               notiontool.Client
	slackService             slack.Service
	slackAdminService        slack.AdminService
	slackSearch              slacktool.SearchService
	slackRetriever           slacktool.MessageRetriever
	githubClient             *github.Client
	webfetchSettings         *webfetch.ClientConfig
	webfetchClient           *webfetch.Client
	llmClient                gollem.LLMClient
	embedClient              interfaces.EmbedClient
	historyRepo              gollem.HistoryRepository
	traceRepo                trace.Repository
	baseURL                  string
	notificationSlotDuration time.Duration
	Case                     *CaseUseCase
	Action                   *ActionUseCase
	Memo                     *MemoUseCase
	Knowledge                *KnowledgeUseCase
	ActionStep               *ActionStepUseCase
	Agent                    *AgentUseCase
	Auth                     AuthUseCaseInterface
	Slack                    *SlackUseCases
	Source                   *SourceUseCase
	Assist                   *AssistUseCase
	MentionProposal          *MentionProposalUseCase
	JobRun                   *JobRunUseCase
	Import                   *ImportUseCase
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

func WithBaseURL(url string) Option {
	return func(uc *UseCases) {
		uc.baseURL = url
	}
}

func WithGitHubService(c *github.Client) Option {
	return func(uc *UseCases) {
		uc.githubClient = c
	}
}

// WithWebFetch configures the agent webfetch tool's HTTP-side settings. The
// shared LLM client (used for injection screening) is injected in New, so the
// tool is built only when both these settings and an LLM client are present.
func WithWebFetch(cfg webfetch.ClientConfig) Option {
	return func(uc *UseCases) {
		uc.webfetchSettings = &cfg
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

// WithSlackMessageRetriever configures the Slack User-token-backed message
// retriever used by slack__get_messages. When set, conversations.replies /
// conversations.history are called with the User token, which allows reading
// public channels without bot membership. Requires the underlying User OAuth
// Token to have the channels:history scope. nil keeps the existing Bot-token
// path (which returns not_in_channel when the bot is not a channel member).
func WithSlackMessageRetriever(svc slacktool.MessageRetriever) Option {
	return func(uc *UseCases) {
		uc.slackRetriever = svc
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

// WithEmbedClient sets the embedding client. The Memory / Knowledge similarity
// search consumers were demolished pending redesign, so the client currently
// has no production reader; the option is preserved so the upcoming redesign
// can drop similarity-search features back in without rewiring the CLI /
// usecase boundary. Configured separately from the chat completion LLM so it
// can target Gemini regardless of provider.
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

// WithNotificationSlotDuration sets the rolling window length used to
// aggregate Slack channel-side change notifications into a single editable
// message. Pass 0 (the default) to disable aggregation, restoring the legacy
// per-event reply_broadcast path.
func WithNotificationSlotDuration(d time.Duration) Option {
	return func(uc *UseCases) {
		uc.notificationSlotDuration = d
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

	// Build the webfetch client only when both the HTTP settings (WithWebFetch)
	// and an LLM client (WithLLMClient) are present. The LLM screen is the only
	// injection defense in this codebase, so webfetch fails closed (stays nil)
	// without an LLM client.
	if uc.webfetchSettings != nil && uc.llmClient != nil {
		cfg := *uc.webfetchSettings
		cfg.LLM = uc.llmClient
		uc.webfetchClient = webfetch.NewClient(cfg)
	}

	uc.Case = NewCaseUseCase(repo, registry, uc.slackService, uc.slackAdminService, uc.baseURL)
	slotCoord := newNotificationSlotCoordinator(repo.NotificationSlot(), uc.slackService, uc.notificationSlotDuration, nil)
	uc.Action = NewActionUseCase(repo, registry, uc.slackService, uc.baseURL, slotCoord)
	uc.Memo = NewMemoUseCase(repo, registry)
	uc.Knowledge = NewKnowledgeUseCase(repo, uc.embedClient)
	uc.ActionStep = NewActionStepUseCase(repo, uc.slackService, slotCoord)

	// Convert *github.Client to githubAPI interface, preserving nil-ness:
	// passing a typed nil pointer through an interface parameter would make
	// `iface == nil` evaluate false at the receiver, so we explicitly leave
	// the interface untyped when no client is configured.
	var githubSvc githubAPI
	if uc.githubClient != nil {
		githubSvc = uc.githubClient
	}
	uc.Source = NewSourceUseCase(repo, uc.notion, uc.slackService, githubSvc)
	uc.JobRun = NewJobRunUseCase(repo, registry)
	uc.Import = NewImportUseCase(repo, registry, uc.Case, uc.Action)

	// Whenever Slack is wired, the LLM client must also be wired — Slack-driven
	// flows (agent mention, mention-draft, assist) all require LLM by design.
	// The embed client is intentionally NOT enforced here: its only consumers
	// (Memory / Knowledge similarity search) were demolished pending redesign,
	// so requiring it would block minimal local-dev configurations without any
	// functional benefit. The wiring is preserved (field + WithEmbedClient
	// option) so the redesign can plug new consumers back in without changes
	// at the CLI / usecase boundary.
	if uc.slackService != nil {
		if uc.llmClient == nil {
			panic("usecase.New: LLM client is required when Slack service is configured (use WithLLMClient)")
		}
		// Agent depends on the persistent History/Trace archive. Callers
		// that drive Slack events (the serve CLI) MUST wire both; callers
		// that only use Assist (the assist CLI) may omit them, in which
		// case the Agent usecase is simply not constructed.
		if uc.historyRepo != nil && uc.traceRepo != nil {
			uc.Agent = NewAgentUseCase(AgentDeps{
				Repo:           repo,
				Registry:       registry,
				LLM:            uc.llmClient,
				HistoryRepo:    uc.historyRepo,
				TraceRepo:      uc.traceRepo,
				ActionUC:       uc.Action,
				ActionStepUC:   uc.ActionStep,
				CaseUC:         uc.Case,
				MemoUC:         uc.Memo,
				KnowledgeUC:    uc.Knowledge,
				SlackService:   uc.slackService,
				SlackSearch:    uc.slackSearch,
				SlackRetriever: uc.slackRetriever,
				NotionTool:     uc.notionTool,
				GitHubClient:   uc.githubClient,
				WebFetchClient: uc.webfetchClient,
				EmbedClient:    uc.embedClient,
			})
		} else if uc.historyRepo != nil || uc.traceRepo != nil {
			panic("usecase.New: WithHistoryRepository and WithTraceRepository must be paired")
		}
		uc.Assist = NewAssistUseCase(AssistDeps{
			Repo:           repo,
			Registry:       registry,
			LLM:            uc.llmClient,
			ActionUC:       uc.Action,
			CaseUC:         uc.Case,
			SlackService:   uc.slackService,
			SlackSearch:    uc.slackSearch,
			SlackRetriever: uc.slackRetriever,
			NotionTool:     uc.notionTool,
			GitHubClient:   uc.githubClient,
			WebFetchClient: uc.webfetchClient,
			EmbedClient:    uc.embedClient,
		})

		// MentionProposal is wired only when the persistent History/Trace archive
		// is configured — the planner runtime depends on both. Without them,
		// the open-mode path is simply not constructed (the dispatcher will
		// no-op for app_mention in unbound channels).
		if uc.historyRepo != nil && uc.traceRepo != nil {
			deps := &agent.CommonDeps{
				Repo:                repo,
				Registry:            registry,
				LLMClient:           uc.llmClient,
				HistoryRepo:         uc.historyRepo,
				TraceRepo:           uc.traceRepo,
				SlackBot:            uc.slackService,
				SlackSearch:         uc.slackSearch,
				SlackRetriever:      uc.slackRetriever,
				NotionClient:        uc.notionTool,
				GitHubClient:        uc.githubClient,
				WebFetchClient:      uc.webfetchClient,
				ActionUC:            NewActionToolAdapter(uc.Action),
				ActionStepUC:        NewActionStepToolAdapter(uc.ActionStep),
				KnowledgeAccessor:   NewKnowledgeToolAccessor(uc.Knowledge),
				KnowledgeMutator:    NewKnowledgeToolMutator(uc.Knowledge),
				HeartbeatInterval:   agent.DefaultHeartbeatInterval,
				HeartbeatStaleAfter: agent.DefaultHeartbeatStaleAfter,
			}
			draftUC, err := proposal.New(deps, 0, 0)
			if err != nil {
				errutil.Handle(context.Background(), goerr.Wrap(err, "failed to build draft usecase"), "failed to build draft usecase")
			} else {
				uc.MentionProposal = NewMentionProposalUseCase(repo, registry, uc.slackService, draftUC)
			}
		}
	}
	uc.Slack = NewSlackUseCases(repo, registry, uc.Agent, uc.MentionProposal, uc.slackService)

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

// WebFetchClient returns the agent webfetch client (nil when the tool is
// disabled or no LLM client is configured). Exposed so the Job runtime wiring
// can bind the same client into the Job tool set.
func (uc *UseCases) WebFetchClient() *webfetch.Client {
	return uc.webfetchClient
}
