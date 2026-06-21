// Package env builds the in-memory environment a scenario runs against: a
// memory repository (seeded with the scenario's prior cases), in-memory history
// and trace repositories, the system-under-test wired via usecase.New, a
// recording fake Slack service, and per-tool clients that are either simulated
// (ToolSimulator) or live (recorded). It exposes the AgentUseCase entrypoints
// the driver drives, plus the fake Slack, the tool-call recorder, and the trace
// repository for diagnostic dumps.
package env

import (
	"context"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/actionwriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	modelconfig "github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/toolsim"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// seedReporterID is the synthesized reporter for injected prior cases.
const seedReporterID = "U-EVALSEED"

// Options carries the externally-provided dependencies needed to build an env.
type Options struct {
	// LLM is the system-under-test agent's LLM client (required).
	LLM gollem.LLMClient
	// Completer drives the simulated tools (required when any tool is sim).
	Completer evaltype.Completer
	// Live* are real tool clients, used for tools marked live=true. Nil unless
	// the corresponding live tool is requested.
	LiveSlackSearch slacktool.SearchService
	LiveNotion      notiontool.Client
	GitHub          *githubtool.Client
	// WebFetch holds the live-only webfetch HTTP settings; the eval LLM is
	// injected as the screening client when the tool is built.
	WebFetch *webfetch.ClientConfig
}

// Env is a prepared single-scenario environment.
type Env struct {
	AgentUC        *usecase.AgentUseCase
	JobRunner      *job.JobRunner
	Repo           interfaces.Repository
	Registry       *model.WorkspaceRegistry
	Entry          *model.WorkspaceEntry
	Slack          *fakeSlack
	Recorder       *toolsim.Recorder
	Trace          *agentarchive.MemoryTraceRepository
	SeededCases    []*model.Case
	MonitorChannel string
	Language       string
}

// Build assembles the environment for one scenario.
func Build(ctx context.Context, sc *scenario.Scenario, opts Options) (*Env, error) {
	if opts.LLM == nil {
		return nil, goerr.New("env: LLM client is required")
	}
	if sc.Workspace == nil {
		return nil, goerr.New("env: scenario has no workspace config")
	}

	repo := memory.New()
	registry, entry := buildRegistry(sc.Workspace)

	historyRepo := agentarchive.NewMemoryHistoryRepository()
	traceRepo := agentarchive.NewMemoryTraceRepository()
	recorder := toolsim.NewRecorder()

	slackSearch, err := resolveSlackSearch(sc, opts, recorder)
	if err != nil {
		return nil, err
	}
	notionClient, err := resolveNotion(sc, opts, recorder)
	if err != nil {
		return nil, err
	}

	fake := newFakeSlack()
	ucOpts := []usecase.Option{
		usecase.WithLLMClient(opts.LLM),
		usecase.WithSlackService(fake),
		usecase.WithSlackSearchService(slackSearch),
		usecase.WithSlackMessageRetriever(toolsim.SlackRetriever(recorder)),
		usecase.WithNotionToolClient(notionClient),
		usecase.WithHistoryRepository(historyRepo),
		usecase.WithTraceRepository(traceRepo),
	}
	if opts.GitHub != nil {
		ucOpts = append(ucOpts, usecase.WithGitHubService(opts.GitHub))
	}
	if opts.WebFetch != nil {
		ucOpts = append(ucOpts, usecase.WithWebFetch(*opts.WebFetch))
	}

	uc := usecase.New(repo, registry, ucOpts...)
	if uc.Agent == nil {
		return nil, goerr.New("env: agent use case was not constructed (LLM/history/trace wiring incomplete)")
	}

	seeded, err := seedCases(ctx, repo, entry.Workspace.ID, sc)
	if err != nil {
		return nil, err
	}
	if err := seedSources(ctx, repo, entry.Workspace.ID, sc); err != nil {
		return nil, err
	}

	jobRunner := buildJobRunner(repo, registry, uc, opts.LLM, historyRepo, traceRepo)

	return &Env{
		AgentUC:        uc.Agent,
		JobRunner:      jobRunner,
		Repo:           repo,
		Registry:       registry,
		Entry:          entry,
		Slack:          fake,
		Recorder:       recorder,
		Trace:          traceRepo,
		SeededCases:    seeded,
		MonitorChannel: sc.Workspace.SlackMonitorChannel,
		Language:       sc.Meta.Language,
	}, nil
}

func resolveSlackSearch(sc *scenario.Scenario, opts Options, rec *toolsim.Recorder) (slacktool.SearchService, error) {
	t := sc.Tools[toolsim.ToolSlackSearch]
	if t.Live {
		if opts.LiveSlackSearch == nil {
			return nil, goerr.New("env: slack_search marked live but no live client provided")
		}
		return toolsim.RecordingSlackSearch(opts.LiveSlackSearch, rec), nil
	}
	return toolsim.SlackSearch(opts.Completer, t.Background, rec), nil
}

func resolveNotion(sc *scenario.Scenario, opts Options, rec *toolsim.Recorder) (notiontool.Client, error) {
	t := sc.Tools[toolsim.ToolNotionSearch]
	if t.Live {
		if opts.LiveNotion == nil {
			return nil, goerr.New("env: notion_search marked live but no live client provided")
		}
		return toolsim.RecordingNotion(opts.LiveNotion, rec), nil
	}
	return toolsim.NotionSearch(opts.Completer, t.Background, rec), nil
}

// buildRegistry constructs a single-entry registry from the workspace config,
// mirroring config.AppConfig.Configure's mapping.
func buildRegistry(wc *config.WorkspaceConfig) (*model.WorkspaceRegistry, *model.WorkspaceEntry) {
	entry := &model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:          wc.ID,
			Name:        wc.Name,
			Description: wc.Description,
			Emoji:       wc.Emoji,
			Color:       wc.Color,
		},
		FieldSchema:           wc.FieldSchema,
		ActionStatusSet:       wc.ActionStatusSet,
		SlackChannelPrefix:    wc.SlackChannelPrefix,
		SlackTeamID:           wc.SlackTeamID,
		SlackInviteUsers:      wc.SlackInviteUsers,
		SlackInviteGroups:     wc.SlackInviteGroups,
		SlackWelcomeMessages:  wc.SlackWelcomeMessages,
		CompilePrompt:         wc.CompilePrompt,
		AssistPrompt:          wc.AssistPrompt,
		AssistLanguage:        wc.AssistLanguage,
		CaseCreatePrompt:      wc.CaseCreatePrompt,
		Jobs:                  wc.Jobs,
		CaseMode:              wc.CaseMode,
		SlackMonitorChannelID: wc.SlackMonitorChannel,
		CaseStatusSet:         wc.CaseStatusSet,
	}
	registry := model.NewWorkspaceRegistry()
	registry.Register(entry)
	return registry, entry
}

// seedCases injects the scenario's prior cases into the memory repository. Field
// values are not populated in v1 (the scenario `cases.fields` are reserved for
// future search use); core identity fields are set so the repository's
// create-time validation passes.
func seedCases(ctx context.Context, repo interfaces.Repository, wsID string, sc *scenario.Scenario) ([]*model.Case, error) {
	now := time.Now().UTC()
	out := make([]*model.Case, 0, len(sc.Cases))
	for i := range sc.Cases {
		cs := sc.Cases[i]
		c := &model.Case{
			Title:          cs.Title,
			Description:    cs.Description,
			Status:         types.CaseStatusOpen,
			ReporterID:     seedReporterID,
			SlackChannelID: sc.Workspace.SlackMonitorChannel,
			BoardStatus:    cs.BoardStatus,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		created, err := repo.Case().Create(ctx, wsID, c)
		if err != nil {
			return nil, goerr.Wrap(err, "seed case", goerr.V("title", cs.Title))
		}
		out = append(out, created)
	}
	return out, nil
}

// buildJobRunner wires a JobRunner mirroring the production job runtime
// (pkg/cli/job_runtime.go) with the in-memory env: read-only core tools plus
// action writer tools, and both the simple and planexec executors. Sources
// reach the job through the system prompt (resolveSources), not tools, so the
// seeded sources are surfaced here. (casewriter / slackpost tools are omitted
// in v1 — they require host adapters not exported from the usecase layer.)
func buildJobRunner(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	uc *usecase.UseCases,
	llm gollem.LLMClient,
	historyRepo gollem.HistoryRepository,
	traceRepo trace.Repository,
) *job.JobRunner {
	actionAdapter := usecase.NewActionToolAdapter(uc.Action)
	stepAdapter := usecase.NewActionStepToolAdapter(uc.ActionStep)
	caseAdapter := usecase.NewCaseToolAdapter(uc.Case)
	memoAdapter := usecase.NewMemoToolAdapter(uc.Memo)
	knowledgeAccessor := usecase.NewKnowledgeToolAccessor(uc.Knowledge)
	knowledgeMutator := usecase.NewKnowledgeToolMutator(uc.Knowledge)

	toolBuilder := job.ToolBuilderFunc(func(_ context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
		var statusSet *model.ActionStatusSet
		var caseStatusSet *model.ActionStatusSet
		var fieldSchema *modelconfig.FieldSchema
		wsID := ""
		if ws != nil {
			statusSet = ws.ActionStatusSet
			caseStatusSet = ws.CaseStatusSet
			fieldSchema = ws.FieldSchema
			wsID = ws.Workspace.ID
		}
		caseID := int64(0)
		if c != nil {
			caseID = c.ID
		}
		coreDeps := core.Deps{
			Repo:         repo,
			WorkspaceID:  wsID,
			CaseID:       caseID,
			StatusSet:    statusSet,
			ActionUC:     actionAdapter,
			ActionStepUC: stepAdapter,
			CaseRefUC:    uc.Case,
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
		if ws != nil && ws.MemoConfig.Enabled() {
			out = append(out, memotool.New(memotool.Deps{
				Repo:        repo,
				WorkspaceID: wsID,
				CaseID:      caseID,
				MemoUC:      memoAdapter,
				Schema:      ws.MemoConfig.FieldSchema,
			})...)
		}
		if knowledgeAccessor != nil {
			kdeps := knowledgetool.Deps{WorkspaceID: wsID, Accessor: knowledgeAccessor}
			if knowledgeMutator != nil && c != nil && !c.IsPrivate {
				kdeps.Mutator = knowledgeMutator
				out = append(out, knowledgetool.New(kdeps)...)
			} else {
				out = append(out, knowledgetool.NewReadOnly(kdeps)...)
			}
		}
		return out
	})

	executors := map[model.JobStrategy]jobagent.JobExecutor{
		model.JobStrategySimple: jobagent.NewSingleLoopJobExecutor(),
	}
	if planexecRunner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: historyRepo,
		TraceRepo:   traceRepo,
		Budget: planexec.BudgetConfig{
			PlannerLoopMax:  8,
			SubAgentLoopMax: 20,
		},
	}); err == nil {
		if exec, peErr := jobagent.NewPlanexecJobExecutor(planexecRunner); peErr == nil {
			executors[model.JobStrategyPlanexec] = exec
		}
	}

	return job.NewJobRunner(job.RunnerDeps{
		Repo:        repo,
		Registry:    registry,
		LLMClient:   llm,
		Executors:   executors,
		ToolBuilder: toolBuilder,
	})
}

// seedSources injects the scenario's workspace data sources into the memory
// repository so source-aware tools / workflows read them from the same repo.
func seedSources(ctx context.Context, repo interfaces.Repository, wsID string, sc *scenario.Scenario) error {
	now := time.Now().UTC()
	for i := range sc.Sources {
		src := toModelSource(sc.Sources[i], now)
		if _, err := repo.Source().Create(ctx, wsID, src); err != nil {
			return goerr.Wrap(err, "seed source", goerr.V("name", src.Name), goerr.V("type", string(src.SourceType)))
		}
	}
	return nil
}

// toModelSource maps a scenario source (already validated) to a model.Source
// with a fresh id and the matching typed config.
func toModelSource(s scenario.Source, now time.Time) *model.Source {
	src := &model.Source{
		ID:          model.NewSourceID(),
		Name:        s.Name,
		SourceType:  model.SourceType(s.Type),
		Description: s.Description,
		Enabled:     s.IsEnabled(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	switch model.SourceType(s.Type) {
	case model.SourceTypeNotionDB:
		src.NotionDBConfig = &model.NotionDBConfig{
			DatabaseID:    s.NotionDB.DatabaseID,
			DatabaseTitle: s.NotionDB.DatabaseTitle,
			DatabaseURL:   s.NotionDB.DatabaseURL,
		}
	case model.SourceTypeNotionPage:
		src.NotionPageConfig = &model.NotionPageConfig{
			PageID:    s.NotionPage.PageID,
			PageTitle: s.NotionPage.PageTitle,
			PageURL:   s.NotionPage.PageURL,
			Recursive: s.NotionPage.Recursive,
			MaxDepth:  s.NotionPage.MaxDepth,
		}
	case model.SourceTypeSlack:
		channels := make([]model.SlackChannel, 0, len(s.Slack.Channels))
		for _, ch := range s.Slack.Channels {
			channels = append(channels, model.SlackChannel{ID: ch.ID, Name: ch.Name})
		}
		src.SlackConfig = &model.SlackConfig{Channels: channels}
	case model.SourceTypeGitHub:
		repos := make([]model.GitHubRepository, 0, len(s.GitHub.Repositories))
		for _, r := range s.GitHub.Repositories {
			repos = append(repos, model.GitHubRepository{Owner: r.Owner, Repo: r.Repo})
		}
		src.GitHubConfig = &model.GitHubConfig{Repositories: repos}
	}
	return src
}
