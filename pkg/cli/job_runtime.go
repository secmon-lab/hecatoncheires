// Job runtime wiring shared between `hecatoncheires serve` (which hosts
// the Case lifecycle publisher) and `hecatoncheires scheduled` (which
// fires only the time-driven sweep). Both ultimately drive the same
// JobUseCase / JobRunner.
package cli

import (
	"context"
	"time"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
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
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// jobReflectionLoopMax bounds the reflection agent's internal tool-calling
// loop. Set here at the wiring layer (not inside the reflector) so the budget
// stays configurable from the caller per project convention.
const jobReflectionLoopMax = 20

// Timing of the deployment-wide Job concurrency slots. Fixed here at the
// wiring layer (only the limit itself is operator-configurable) so no default
// hides inside pkg/usecase/job.
const (
	// jobSlotTTL is the delay between a holder's last heartbeat and its slot
	// becoming reusable — i.e. how long a crashed instance blocks a slot. Kept
	// short because at a limit of 1 that delay is lost throughput.
	jobSlotTTL = 30 * time.Second
	// jobSlotRenewInterval is TTL/3, so two consecutive renew failures still
	// leave a third attempt before a live run's slot expires.
	jobSlotRenewInterval = 10 * time.Second
	// jobSlotMaxHold stops a leaked hold from renewing itself forever. It is
	// not a run timeout: only the renewal stops, the run continues.
	jobSlotMaxHold = 2 * time.Hour
)

// tickRuntime bundles the dependencies the tick CLI / HTTP endpoint
// need to fire a sweep.
type tickRuntime struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	scanner  *job.ScheduledScanner
	// durable is the agent runtime the dispatched runs execute on. The sweep owns
	// its worker: it spawns the runs and then drains them in the foreground, so a
	// scheduled sweep does not depend on a serve instance being up to execute what
	// it dispatched.
	durable *job.DurableRuntime
	// cleanup releases what the runtime opened (the agent process repository, the
	// Cloud Storage client). Always non-nil.
	cleanup func()
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
	jobCfg *config.JobConcurrency,
	agentCfg *config.Agent,
	storageCfg *config.Storage,
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

	// The sweep runs its Jobs on the same agent runtime serve does, so a scheduled
	// run gets the same step / token budget and the same one-transition-at-a-time
	// checkpointing. It differs only in who drives the worker: serve runs one
	// continuously, a sweep drains its own runs and exits.
	durable, cleanup, err := buildTickAgentRuntime(ctx, tickAgentDeps{
		repo: repo, registry: registry, llm: llmClient, uc: uc,
		agentCfg: agentCfg, storageCfg: storageCfg, repoCfg: repoCfg,
		slotLimit: jobCfg.Limit(),
	})
	if err != nil {
		return nil, err
	}

	jobUC, jobRunner, err := buildJobRuntime(jobRuntimeDeps{
		Repo:      repo,
		Registry:  registry,
		LLMClient: llmClient,
		UC:        uc,
		WebFetch:  uc.WebFetchClient(),
		// The tick CLI dispatches Job runs itself, so it must enforce the same
		// deployment-wide limit as serve — a sweep that skipped the gate would
		// defeat it entirely.
		SlotLimit:   jobCfg.Limit(),
		SlotLimiter: durable.SlotLimiter,
		// Mirror the read-tool wiring done in serve.go so every Job host
		// resolves the same tool set. The tick CLI builds `uc` without the
		// Slack / Notion options, so these accessors return nil today and the
		// tools stay disabled; passing them keeps the host-coverage rule honest
		// and lights the tools up automatically if the tick uc ever configures
		// them.
		SlackSearch:    uc.SlackSearchService(),
		SlackRetriever: uc.SlackMessageRetriever(),
		NotionTool:     uc.NotionToolClient(),
		Durable:        durable.Runtime,
	})
	if err != nil {
		cleanup()
		return nil, goerr.Wrap(err, "build job runtime")
	}
	durable.Runtime.AttachRunner(jobRunner)
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
		durable:  durable.Runtime,
		cleanup:  cleanup,
	}, nil
}

// tickAgentDeps is what building the sweep's agent runtime needs.
type tickAgentDeps struct {
	repo       interfaces.Repository
	registry   *model.WorkspaceRegistry
	llm        gollem.LLMClient
	uc         *usecase.UseCases
	agentCfg   *config.Agent
	storageCfg *config.Storage
	repoCfg    *config.Repository
	slotLimit  int
}

// tickAgentRuntime is the sweep's agent runtime plus the slot gate the Job runner
// must share with it.
type tickAgentRuntime struct {
	Runtime     *job.DurableRuntime
	SlotLimiter *job.ConcurrencyLimiter
}

// buildTickAgentRuntime builds the Kernel a sweep's Job runs execute on, in the
// order agentkit requires: register, build, bind.
//
// Without an LLM client there is nothing for an agent to run, so it returns a
// runtime the Job runner will find unusable (`handles` reports false) and the sweep
// dispatches nothing — the same shape serve has.
func buildTickAgentRuntime(ctx context.Context, d tickAgentDeps) (*tickAgentRuntime, func(), error) {
	noop := func() {}
	if d.llm == nil {
		return &tickAgentRuntime{Runtime: &job.DurableRuntime{}}, noop, nil
	}

	archive, err := d.storageCfg.Configure(ctx)
	if err != nil {
		return nil, noop, goerr.Wrap(err, "configure the agent session archive")
	}
	// Archive.Close is a plain cleanup func, not an io.Closer, so it is called
	// through a variable rather than as a method on the struct.
	closeArchive := archive.Close

	procRepo, procCleanup, err := d.repoCfg.ConfigureAgentProcess(ctx)
	if err != nil {
		closeArchive()
		return nil, noop, goerr.Wrap(err, "configure the agent process repository")
	}
	cleanup := func() {
		procCleanup()
		closeArchive()
	}

	budgets, err := d.agentCfg.Budgets()
	if err != nil {
		cleanup()
		return nil, noop, err
	}

	locator, err := agentkernel.NewLocator(procRepo)
	if err != nil {
		cleanup()
		return nil, noop, goerr.Wrap(err, "build the agent process locator")
	}

	var slots *job.ConcurrencyLimiter
	if d.slotLimit > 0 {
		slots, err = buildJobSlotLimiter(d.repo, d.slotLimit)
		if err != nil {
			cleanup()
			return nil, noop, err
		}
	}

	reg := agentkit.NewRegistry()
	taskAgent, err := agentkernel.RegisterTaskAgent(reg, budgets.Task.Limiter(), archive.ProcessHistory)
	if err != nil {
		cleanup()
		return nil, noop, goerr.Wrap(err, "register the task sub-agent")
	}
	durable := &job.DurableRuntime{History: archive.ProcessHistory, Locator: locator}
	if err := durable.Register(reg, budgets.Root.Limiter(), taskAgent); err != nil {
		cleanup()
		return nil, noop, goerr.Wrap(err, "register the job agents")
	}

	k, err := agentkernel.Build(agentkernel.Deps{
		Repo:    procRepo,
		History: archive.ProcessHistory,
		LLM:     d.llm,
		Trace:   archive.Trace,
		Budgets: budgets,
		Agents:  reg,
		Slots:   slots,
		// The same tool palette serve gives a Job run, so a scheduled run behaves
		// identically wherever it is dispatched from. The Slack / Notion / GitHub
		// clients are nil in a sweep (its usecase set is built without them) and each
		// tool's constructor degrades to no tool, which is the pre-existing shape of
		// the tick path.
		Tools: agentkernel.ToolDeps{
			Repo:              d.repo,
			Registry:          d.registry,
			SlackSearch:       d.uc.SlackSearchService(),
			SlackRetriever:    d.uc.SlackMessageRetriever(),
			NotionClient:      d.uc.NotionToolClient(),
			GitHubClient:      d.uc.GitHubToolClient(),
			WebFetchClient:    d.uc.WebFetchClient(),
			ActionUC:          usecase.NewActionToolAdapter(d.uc.Action),
			ActionStepUC:      usecase.NewActionStepToolAdapter(d.uc.ActionStep),
			CaseUC:            usecase.NewCaseToolAdapter(d.uc.Case),
			CaseRefUC:         d.uc.Case,
			CaseMultiUC:       usecase.NewCaseMultiCaseAdapter(d.uc.Case),
			CaseMultiActionUC: usecase.NewCaseMultiActionAdapter(d.uc.Action, d.uc.ActionStep),
			MemoUC:            usecase.NewMemoToolAdapter(d.uc.Memo),
			KnowledgeAccessor: usecase.NewKnowledgeToolAccessor(d.uc.Knowledge, d.uc.Tag),
			KnowledgeMutator:  usecase.NewKnowledgeToolMutator(d.uc.Knowledge, d.uc.Tag),
		},
	})
	if err != nil {
		cleanup()
		return nil, noop, goerr.Wrap(err, "build the agent runtime")
	}
	durable.Bind(k)
	// The sweep waits for exactly the runs it dispatched, so it must remember them.
	durable.TrackSpawns()

	logging.Default().Info("Agent runtime configured for the sweep",
		logAttrsToArgs(d.agentCfg.LogAttrs())...)
	return &tickAgentRuntime{Runtime: durable, SlotLimiter: slots}, cleanup, nil
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

	// SlotLimit caps how many scheduled Job runs execute concurrently across
	// the whole deployment (see config.JobConcurrency). 0 means no limit and
	// builds no limiter at all.
	SlotLimit int

	// SlotLimiter, when non-nil, is the limiter to use instead of building one.
	// `serve` must pass the same instance it gave the Kernel as its slot gate:
	// with a durable Job the claim is what occupies a slot for the length of the
	// run, so a second limiter here would count a different set of holds and the
	// deployment-wide limit would bound neither side correctly.
	SlotLimiter *job.ConcurrencyLimiter

	// HistoryRepo / TraceRepo are required when wiring the planexec
	// executor (it needs persistent storage to replay sub-agent
	// reasoning). Nil falls back to in-memory implementations so the
	// scheduled-tick CLI command (which does not configure storage)
	// still gets a fully wired runtime.
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository

	// Durable, when non-nil, is the agent runtime the simple-strategy Jobs run
	// on. The caller creates it, hands it here, then registers and binds it
	// around the Kernel build — the order agentkit requires. Nil keeps every
	// strategy on the in-process executors.
	Durable *job.DurableRuntime
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
//
// It returns an error when the concurrency limiter cannot be built: a
// deployment that asked for a limit must fail to start rather than silently
// dispatch Job runs unbounded.
func buildJobRuntime(deps jobRuntimeDeps) (*job.UseCase, *job.JobRunner, error) {
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

	// No in-process executors are registered: every strategy runs on the durable
	// runtime in both entry points — `serve` keeps a worker running, and the `tick`
	// CLI drains its own runs before it exits. The runner's executor path is
	// therefore unreachable from here; it survives only for the eval harness and
	// tests, which drive the runner directly. Retiring it is tracked in the spec.

	// Reflection agent: knowledge/tag tools only, sharing the same knowledge use
	// cases as the Job tools. Disabled (nil reflector) if knowledge is not
	// configured. It reads a finished run's transcript from the agent history store,
	// by the ref recorded on the Process.
	var reflector jobagent.Reflector
	if deps.LLMClient != nil {
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
		ToolBuilder:   toolBuilder,
		SlackNotifier: slackNotifier,
		Reflector:     reflector,
		Durable:       deps.Durable,
	}
	// The interactive-Job question form is Block Kit posted/updated directly
	// via the Slack service (the narrow SlackNotifier cannot carry blocks).
	// Wired only when Slack is present; without it an interactive Job that
	// emits a question fails loudly at the Interactor (it has no surface).
	if deps.SlackService != nil {
		deps2.InteractionPoster = deps.SlackService
	}
	// Deployment-wide concurrency gate on scheduled runs. `serve` builds it before
	// the Kernel (it is also the Kernel's slot gate) and hands the same instance
	// here; `tick` has no Kernel, so it builds one now.
	if deps.SlotLimiter != nil {
		deps2.SlotLimiter = deps.SlotLimiter
	} else if deps.SlotLimit > 0 {
		limiter, err := buildJobSlotLimiter(deps.Repo, deps.SlotLimit)
		if err != nil {
			return nil, nil, err
		}
		deps2.SlotLimiter = limiter
	}
	runner := job.NewJobRunner(deps2)
	jobUC := job.NewUseCase(deps.Registry, runner)
	return jobUC, runner, nil
}

// buildJobSlotLimiter builds the deployment-wide concurrency gate for scheduled
// Job runs. limit must be positive; 0 means ungated and the caller skips this.
//
// It is a standalone constructor because `serve` needs the limiter BEFORE the
// Kernel exists — the Kernel takes it as its claim-time slot gate, which is what
// bounds concurrent execution now that a durable run outlives the call that
// started it. Building a second one for the Runner would leave each counting only
// its own holds.
func buildJobSlotLimiter(repo interfaces.Repository, limit int) (*job.ConcurrencyLimiter, error) {
	limiter, err := job.NewConcurrencyLimiter(job.ConcurrencyLimiterDeps{
		Repo:          repo.JobSlot(),
		Limit:         limit,
		TTL:           jobSlotTTL,
		RenewInterval: jobSlotRenewInterval,
		MaxHold:       jobSlotMaxHold,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "build job concurrency limiter",
			goerr.V("job_max_concurrency", limit))
	}
	// The slot timings are compiled-in, so this is the only place they are
	// visible; without them a slot_hold_ms cannot be read against the TTL that
	// would have expired it.
	logging.Default().Info("job concurrency limiter enabled",
		"job_max_concurrency", limit,
		"slot_ttl", jobSlotTTL.String(),
		"slot_renew_interval", jobSlotRenewInterval.String(),
		"slot_max_hold", jobSlotMaxHold.String())
	return limiter, nil
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
