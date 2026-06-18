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
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
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

	// HistoryRepo / TraceRepo are required when wiring the planexec
	// executor (it needs persistent storage to replay sub-agent
	// reasoning). Nil falls back to in-memory implementations so the
	// scheduled-tick CLI command (which does not configure storage)
	// still gets a fully wired runtime.
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository
}

// buildJobRuntime constructs the JobRunner + JobUseCase pair, with a
// ToolBuilder that binds every read-only and writer tool the spec calls
// for to each invocation.
func buildJobRuntime(deps jobRuntimeDeps) (*job.UseCase, *job.JobRunner) {
	actionAdapter := usecase.NewActionToolAdapter(deps.UC.Action)
	stepAdapter := usecase.NewActionStepToolAdapter(deps.UC.ActionStep)
	caseAdapter := usecase.NewCaseToolAdapter(deps.UC.Case)
	memoAdapter := usecase.NewMemoToolAdapter(deps.UC.Memo)

	toolBuilder := job.ToolBuilderFunc(func(_ context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
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
			ActionUC:     actionAdapter,
			ActionStepUC: stepAdapter,
		}

		out := make([]gollem.Tool, 0, 16)
		out = append(out, core.NewReadOnly(coreDeps)...)
		out = append(out, actionwriter.New(coreDeps)...)
		out = append(out, casewriter.New(casewriter.Deps{
			CaseUC:      caseAdapter,
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
		out = append(out, webfetch.New(deps.WebFetch)...)
		// Case-scoped memo tools, wired only when the workspace enabled memos.
		if ws != nil && ws.MemoConfig.Enabled() {
			out = append(out, memotool.New(memotool.Deps{
				Repo:        deps.Repo,
				WorkspaceID: wsID,
				CaseID:      caseID,
				MemoUC:      memoAdapter,
				Schema:      ws.MemoConfig.FieldSchema,
			})...)
		}
		return out
	})

	executors := map[model.JobStrategy]jobagent.JobExecutor{
		model.JobStrategySimple: jobagent.NewSingleLoopJobExecutor(),
	}

	// Wire the planexec executor when an LLM client is available. We
	// only need it for workspaces that declared `strategy = "planexec"`
	// in TOML; constructing it unconditionally is safe because the map
	// lookup at Run time picks the right one. Falls back to in-memory
	// history / trace repos when the caller did not pre-configure them
	// (e.g. the `tick` CLI command in test environments).
	if deps.LLMClient != nil {
		historyRepo := deps.HistoryRepo
		if historyRepo == nil {
			historyRepo = agentarchive.NewMemoryHistoryRepository()
		}
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
	}

	// Wire the operational session-log notifier only when a Slack service is
	// present. Leaving it nil (e.g. the scheduled-tick CLI) disables the
	// starting / progress / completion markers without affecting the run.
	var slackNotifier job.SlackNotifier
	if deps.SlackService != nil {
		slackNotifier = slackNotifierAdapter{svc: deps.SlackService}
	}

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:          deps.Repo,
		Registry:      deps.Registry,
		LLMClient:     deps.LLMClient,
		Executors:     executors,
		ToolBuilder:   toolBuilder,
		SlackNotifier: slackNotifier,
	})
	jobUC := job.NewUseCase(deps.Registry, runner)
	return jobUC, runner
}
