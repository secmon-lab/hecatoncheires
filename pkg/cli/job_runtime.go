// Job runtime wiring shared between `hecatoncheires serve` (which hosts
// the Case lifecycle publisher) and `hecatoncheires scheduled` (which
// fires only the time-driven sweep). Both ultimately drive the same
// JobUseCase / JobRunner.
package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/actionwriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slackpost"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// scheduledRuntime bundles the dependencies the scheduled CLI / HTTP
// endpoint need to fire a sweep.
type scheduledRuntime struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	scanner  *job.ScheduledScanner
}

// buildScheduledRuntime wires the minimal dependency graph for a
// scheduled-Job sweep. This includes the Job runner (so dispatched Jobs
// can actually execute), but excludes the full HTTP / Slack worker
// stack the serve command needs.
func buildScheduledRuntime(
	ctx context.Context,
	appCfg *config.AppConfig,
	repoCfg *config.Repository,
	llmCfg *config.LLM,
	c *cli.Command,
) (*scheduledRuntime, error) {
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
	})
	uc.Case.SetEventPublisher(jobUC)

	scanner := job.NewScheduledScanner(job.ScannerDeps{
		Repo:      repo,
		Registry:  registry,
		Publisher: jobUC,
	})

	return &scheduledRuntime{
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
}

// buildJobRuntime constructs the JobRunner + JobUseCase pair, with a
// ToolBuilder that binds every read-only and writer tool the spec calls
// for to each invocation.
func buildJobRuntime(deps jobRuntimeDeps) (*job.UseCase, *job.JobRunner) {
	actionAdapter := usecase.NewActionToolAdapter(deps.UC.Action)
	stepAdapter := usecase.NewActionStepToolAdapter(deps.UC.ActionStep)
	caseAdapter := newJobCaseAdapter(deps.UC.Case)

	toolBuilder := job.ToolBuilderFunc(func(_ context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
		var statusSet *model.ActionStatusSet
		if ws != nil {
			statusSet = ws.ActionStatusSet
		}
		caseID := int64(0)
		channelID := ""
		if c != nil {
			caseID = c.ID
			channelID = c.SlackChannelID
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
		})...)
		if deps.SlackService != nil && channelID != "" {
			out = append(out, slackpost.New(slackpost.Deps{
				Poster:    slackPosterAdapter{svc: deps.SlackService},
				ChannelID: channelID,
			})...)
		}
		return out
	})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:        deps.Repo,
		Registry:    deps.Registry,
		LLMClient:   deps.LLMClient,
		Executor:    jobagent.NewSingleLoopJobExecutor(),
		ToolBuilder: toolBuilder,
	})
	jobUC := job.NewUseCase(deps.Registry, runner)
	return jobUC, runner
}
