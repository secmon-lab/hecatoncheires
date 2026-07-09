// Job runtime wiring shared between `hecatoncheires serve` (which hosts
// the Case lifecycle publisher) and `hecatoncheires scheduled` (which
// fires only the time-driven sweep). Both ultimately drive the same
// JobUseCase / JobRunner.
package cli

import (
	"context"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/actionwriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slackpost"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	modelconfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// jobReflectionLoopMax bounds the reflection agent's internal tool-calling
// loop. Set here at the wiring layer (not inside the reflector) so the budget
// stays configurable from the caller per project convention.
const jobReflectionLoopMax = 20

// tickRuntime bundles the dependencies the tick CLI / HTTP endpoint
// need to fire a sweep.
type tickRuntime struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	scanner  *job.ScheduledScanner
}

// buildTickRuntime wires the minimal dependency graph for a scheduled-Job
// sweep. This includes the Job runner (so dispatched Jobs can actually
// execute), but excludes the full HTTP / Slack worker stack the serve
// command needs.
func buildTickRuntime(
	ctx context.Context,
	appCfg *config.AppConfig,
	repoCfg *config.Repository,
	llmCfg *config.LLM,
	c *cli.Command,
) (*tickRuntime, error) {
	_, registry, err := appCfg.Configure(c)
	if err != nil {
		return nil, goerr.Wrap(err, "load workspace configs")
	}
	repo, err := repoCfg.Configure(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "init repository")
	}

	llmClient, err := llmCfg.NewClient(ctx)
	if err != nil {
		return nil, goerr.Wrap(err, "init LLM client")
	}

	uc := usecase.New(repo, registry)

	jobUC, _ := buildJobRuntime(jobRuntimeDeps{
		Repo:      repo,
		Registry:  registry,
		LLMClient: llmClient,
		UC:        uc,
		WebFetch:  uc.WebFetchClient(),
		// Mirror the read-tool wiring done in serve.go so every Job host
		// resolves the same tool set. The tick CLI builds `uc` without the
		// Slack / Notion options, so these accessors return nil today and the
		// tools stay disabled; passing them keeps the host-coverage rule honest
		// and lights the tools up automatically if the tick uc ever configures
		// them.
		SlackSearch:    uc.SlackSearchService(),
		SlackRetriever: uc.SlackMessageRetriever(),
		NotionTool:     uc.NotionToolClient(),
	})
	uc.Case.SetEventPublisher(jobUC)

	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      repo,
		Registry:  registry,
		Publisher: jobUC,
	})

	return &tickRuntime{
		repo:     repo,
		registry: registry,
		scanner:  scanner,
	}, nil
}

// jobRuntimeDeps groups everything the JobUseCase / JobRunner need at
// construction time.
type jobRuntimeDeps struct {
	Repo         interfaces.Repository
	Registry     *model.WorkspaceRegistry
	LLMClient    gollem.LLMClient
	UC           *usecase.UseCases
	SlackService slacksvc.Service // may be nil; slack_post tool then no-ops
	WebFetch     *webfetch.Client // may be nil; webfetch tool then not bound

	// Read-only tools the Job agent uses to read its case thread and do
	// corroboration. Each is nil-safe: the corresponding constructor binds no
	// tool when its dependency is nil, so an unconfigured deployment simply
	// runs without that tool (and the prompt's "do nothing if you can't read"
	// guard takes over).
	SlackSearch    slacktool.SearchService    // slack__search_messages
	SlackRetriever slacktool.MessageRetriever // slack__get_messages via User token
	NotionTool     notiontool.Client          // notion__search / notion__get_page

	// JiraTools carries the already-expanded Jira read tools (see
	// pkg/agent/tool/jira). Unlike NotionTool this is not a client type:
	// gollem exposes no exported helper to turn a gollem.ToolSet into
	// []gollem.Tool, so config.Jira.Configure expands it once at startup
	// and hands the result through as a plain tool slice. nil/empty means
	// Jira is not configured.
	JiraTools []gollem.Tool

	// HistoryRepo / TraceRepo are required when wiring the planexec
	// executor (it needs persistent storage to replay sub-agent
	// reasoning). Nil falls back to in-memory implementations so the
	// scheduled-tick CLI command (which does not configure storage)
	// still gets a fully wired runtime.
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository
}

// registryHasInteractiveJob reports whether any enabled Job in any workspace
// is interactive. Used at serve startup to enforce that interactive Jobs —
// which suspend and resume across requests / instances — have a persistent
// (shared) agent history backend.
func registryHasInteractiveJob(registry *model.WorkspaceRegistry) bool {
	if registry == nil {
		return false
	}
	for _, ws := range registry.List() {
		if ws == nil {
			continue
		}
		for _, j := range ws.Jobs {
			if j != nil && !j.Disabled && j.Interactive {
				return true
			}
		}
	}
	return false
}

// buildJobRuntime constructs the JobRunner + JobUseCase pair, with a
// ToolBuilder that binds every read-only and writer tool the spec calls
// for to each invocation.
func buildJobRuntime(deps jobRuntimeDeps) (*job.UseCase, *job.JobRunner) {
	adapters := jobToolAdapters{
		action:            usecase.NewActionToolAdapter(deps.UC.Action),
		step:              usecase.NewActionStepToolAdapter(deps.UC.ActionStep),
		caseUC:            usecase.NewCaseToolAdapter(deps.UC.Case),
		caseRef:           deps.UC.Case,
		memo:              usecase.NewMemoToolAdapter(deps.UC.Memo),
		knowledgeAccessor: usecase.NewKnowledgeToolAccessor(deps.UC.Knowledge, deps.UC.Tag),
		knowledgeMutator:  usecase.NewKnowledgeToolMutator(deps.UC.Knowledge, deps.UC.Tag),
	}

	toolBuilder := job.ToolBuilderFunc(func(_ context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
		return buildJobTools(deps, adapters, c, ws)
	})

	executors := map[model.JobStrategy]jobagent.JobExecutor{
		model.JobStrategySimple: jobagent.NewSingleLoopJobExecutor(),
	}

	// Shared history repository: the JobRunner persists each run's conversation
	// under the RunID and the reflection pass loads it back. The SAME instance
	// is handed to the planexec runner so planexec runs are reflectable too.
	historyRepo := deps.HistoryRepo
	if historyRepo == nil {
		historyRepo = agentarchive.NewMemoryHistoryRepository()
	}

	// Wire the planexec executor and the reflection agent when an LLM client is
	// available. We only need planexec for workspaces that declared
	// `strategy = "planexec"`; constructing it unconditionally is safe because
	// the map lookup at Run time picks the right one. Falls back to in-memory
	// trace repo when the caller did not pre-configure one (e.g. the `tick` CLI).
	var reflector jobagent.Reflector
	if deps.LLMClient != nil {
		traceRepo := deps.TraceRepo
		if traceRepo == nil {
			traceRepo = agentarchive.NewMemoryTraceRepository()
		}
		planexecRunner, err := planexec.NewRunner(planexec.RunnerDeps{
			LLMClient:   deps.LLMClient,
			HistoryRepo: historyRepo,
			TraceRepo:   traceRepo,
			Budget: planexec.BudgetConfig{
				PlannerLoopMax:  8,
				SubAgentLoopMax: 20,
			},
		})
		if err == nil {
			planexecExec, peErr := jobagent.NewPlanexecJobExecutor(planexecRunner)
			if peErr == nil {
				executors[model.JobStrategyPlanexec] = planexecExec
			}
		}

		// Reflection agent: knowledge/tag tools only, sharing the same knowledge
		// use cases as the Job tools. Disabled (nil reflector) if knowledge is
		// not configured.
		if refl, rErr := jobagent.NewLLMReflector(jobagent.ReflectorDeps{
			LLMClient:         deps.LLMClient,
			KnowledgeAccessor: adapters.knowledgeAccessor,
			KnowledgeMutator:  adapters.knowledgeMutator,
			LoopMax:           jobReflectionLoopMax,
		}); rErr == nil {
			reflector = refl
		}
	}

	// Wire the operational session-log notifier only when a Slack service is
	// present. Leaving it nil (e.g. the scheduled-tick CLI) disables the
	// starting / progress / completion markers without affecting the run.
	var slackNotifier job.SlackNotifier
	if deps.SlackService != nil {
		slackNotifier = slackNotifierAdapter{svc: deps.SlackService}
	}

	deps2 := job.RunnerDeps{
		Repo:          deps.Repo,
		Registry:      deps.Registry,
		LLMClient:     deps.LLMClient,
		Executors:     executors,
		ToolBuilder:   toolBuilder,
		SlackNotifier: slackNotifier,
		Reflector:     reflector,
		HistoryRepo:   historyRepo,
	}
	// The interactive-Job question form is Block Kit posted/updated directly
	// via the Slack service (the narrow SlackNotifier cannot carry blocks).
	// Wired only when Slack is present; without it an interactive Job that
	// emits a question fails loudly at the Interactor (it has no surface).
	if deps.SlackService != nil {
		deps2.InteractionPoster = deps.SlackService
	}
	runner := job.NewJobRunner(deps2)
	jobUC := job.NewUseCase(deps.Registry, runner)
	return jobUC, runner
}

// jobToolAdapters groups the usecase-to-tool adapters once so buildJobTools
// can be called per Job invocation without rebuilding them each time.
type jobToolAdapters struct {
	action            core.ActionMutator
	step              core.ActionStepMutator
	caseUC            casewriter.CaseMutator
	caseRef           core.CaseRefReader
	memo              memotool.MemoMutator
	knowledgeAccessor knowledgetool.KnowledgeAccessor
	knowledgeMutator  knowledgetool.KnowledgeMutator
}

// buildJobTools assembles the tool slice for a single Job invocation. Action
// tools (read-only list/get plus the writer set) are bound only for
// channel-mode workspaces: a thread-mode workspace manages no Actions, so the
// Job agent must not be able to read or mutate them. Case-editing
// (casewriter, incl. thread-mode board status), Slack post, web fetch and memo
// tools are bound in both modes.
func buildJobTools(deps jobRuntimeDeps, adapters jobToolAdapters, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	var statusSet *model.ActionStatusSet
	var caseStatusSet *model.ActionStatusSet
	var fieldSchema *modelconfig.FieldSchema
	if ws != nil {
		statusSet = ws.ActionStatusSet
		caseStatusSet = ws.CaseStatusSet
		fieldSchema = ws.FieldSchema
	}
	caseID := int64(0)
	channelID := ""
	threadTS := ""
	if c != nil {
		caseID = c.ID
		channelID = c.SlackChannelID
		// Thread-mode cases post Job output into the case thread rather
		// than the monitored channel's root.
		threadTS = c.SlackThreadTS
	}
	wsID := ""
	if ws != nil {
		wsID = ws.Workspace.ID
	}

	coreDeps := core.Deps{
		Repo:         deps.Repo,
		WorkspaceID:  wsID,
		CaseID:       caseID,
		StatusSet:    statusSet,
		ActionUC:     adapters.action,
		ActionStepUC: adapters.step,
		CaseRefUC:    adapters.caseRef,
	}

	out := make([]gollem.Tool, 0, 16)
	// Action tools exist only where Actions exist: channel-mode workspaces.
	// core.NewReadOnly also wires the case_ref read tools (CaseRefUC); those
	// are case-reference lookups, not Actions, but they live in the core
	// toolset, so thread-mode forgoes them along with the action tools.
	if ws == nil || !ws.IsThreadMode() {
		out = append(out, core.NewReadOnly(coreDeps)...)
		out = append(out, actionwriter.New(coreDeps)...)
	}
	out = append(out, casewriter.New(casewriter.Deps{
		CaseUC:      adapters.caseUC,
		WorkspaceID: wsID,
		CaseID:      caseID,
		Schema:      fieldSchema,
		StatusSet:   caseStatusSet,
	})...)
	if deps.SlackService != nil && channelID != "" {
		out = append(out, slackpost.New(slackpost.Deps{
			Poster:          slackPosterAdapter{svc: deps.SlackService},
			ChannelID:       channelID,
			DefaultThreadTS: threadTS,
		})...)
	}
	// Slack read-only tools (slack__get_messages / slack__search_messages). Not
	// Action tools, so wired in both channel- and thread-mode. NewReadOnly does
	// NOT include the post tool (posting stays on slackpost above); ChannelID is
	// intentionally omitted here. get_messages binds on a non-nil Bot and reads
	// via the User-token Retriever when present, else via the Bot if it is a
	// channel member; search_messages binds only on a non-nil Search.
	out = append(out, slacktool.NewReadOnly(slacktool.Deps{
		Bot:       deps.SlackService,
		Search:    deps.SlackSearch,
		Retriever: deps.SlackRetriever,
	})...)
	// Notion read-only tools (notion__search / notion__get_page). New returns no
	// tool when the client is nil, so this is safe in deployments without Notion.
	out = append(out, notiontool.New(notiontool.Deps{Client: deps.NotionTool})...)
	out = append(out, webfetch.New(deps.WebFetch)...)
	// Jira read tools (jira_list_projects / jira_search_issues / jira_get_issues).
	// Already expanded at startup (see JiraTools doc comment); appended
	// unconditionally, same as the other integration tool sets above — an
	// empty/nil slice is a safe no-op.
	out = append(out, deps.JiraTools...)
	// Case-scoped memo tools, wired only when the workspace enabled memos.
	if ws != nil && ws.MemoConfig.Enabled() {
		out = append(out, memotool.New(memotool.Deps{
			Repo:        deps.Repo,
			WorkspaceID: wsID,
			CaseID:      caseID,
			MemoUC:      adapters.memo,
			Schema:      ws.MemoConfig.FieldSchema,
		})...)
	}
	// Workspace-wide knowledge tools (not Actions, so available in both modes).
	// Read is always offered; write is withheld while the Job runs against a
	// PRIVATE case (its contents must not leak into shared knowledge).
	if adapters.knowledgeAccessor != nil {
		kdeps := knowledgetool.Deps{WorkspaceID: wsID, Accessor: adapters.knowledgeAccessor}
		if adapters.knowledgeMutator != nil && c != nil && !c.IsPrivate {
			kdeps.Mutator = adapters.knowledgeMutator
			out = append(out, knowledgetool.New(kdeps)...)
		} else {
			out = append(out, knowledgetool.NewReadOnly(kdeps)...)
		}
	}
	return out
}
