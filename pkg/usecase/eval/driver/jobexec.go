package driver

import (
	"context"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/env"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// JobExecution drives a workspace Job against a seeded case and judges what the
// job produced: its run outcome (success/failure + summary), the case state
// after the run, the actions present, and the tool-call trajectory. The job is
// run through the real JobRunner, so this faithfully exercises the production
// executor (simple / planexec), source resolution, and tool wiring.
type JobExecution struct{}

// NewJobExecution builds the driver.
func NewJobExecution() *JobExecution { return &JobExecution{} }

// Kind implements WorkflowDriver.
func (*JobExecution) Kind() string { return scenario.WorkflowJob }

// Run implements WorkflowDriver. The simulator is unused (jobs run unattended,
// without user questions).
func (*JobExecution) Run(ctx context.Context, e *env.Env, sc *scenario.Scenario, _ evaltype.Simulator) (evaltype.Artifact, error) {
	logger := logging.From(ctx)
	if lang, err := i18n.ParseLang(sc.Meta.Language); err == nil && sc.Meta.Language != "" {
		ctx = i18n.ContextWithLang(ctx, lang)
	}
	wsID := e.Entry.Workspace.ID

	target, err := resolveTargetCase(e.SeededCases, sc.Job.TargetCase)
	if err != nil {
		return nil, err
	}
	jobModel, err := resolveJob(e.Entry.Jobs, sc.Job.ID)
	if err != nil {
		return nil, err
	}

	ev := job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        target.ID,
		Timestamp:     time.Now().UTC(),
		ActorUserID:   "U-EVAL",
		CaseLifecycle: model.CaseLifecycleCreated,
	}

	logger.Info("eval: running job", "scenario", sc.Meta.ID, "job", jobModel.ID, "case_id", target.ID, "strategy", string(jobModel.Strategy))
	runErr := e.JobRunner.Run(ctx, jobModel, ev)
	async.Wait()

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: target.ID, JobID: jobModel.ID}
	logs, err := e.Repo.JobRunLog().List(ctx, key, 0)
	if err != nil {
		return nil, goerr.Wrap(err, "list job run logs")
	}
	if len(logs) == 0 {
		// No run record was created — the run failed before logging (e.g. a
		// prepare-stage error). Surface that as a driver error.
		if runErr != nil {
			return nil, goerr.Wrap(runErr, "job run failed before producing a log")
		}
		return nil, errNoCase
	}
	latest := logs[0]

	art := &evaltype.JobArtifact{
		JobID: jobModel.ID,
		Outcome: evaltype.JobOutcome{
			Stage: string(latest.Stage),
			Error: latest.Error,
		},
	}
	art.Outcome.Summary, art.ToolCalls = readTimeline(ctx, e.Repo, key, latest.RunID)

	if c, err := e.Repo.Case().Get(ctx, wsID, target.ID); err == nil {
		art.Case = c
	}
	art.Actions = listCaseActions(ctx, e.Repo, wsID, target.ID)

	logger.Info("eval: job done", "scenario", sc.Meta.ID, "job", jobModel.ID, "stage", art.Outcome.Stage, "actions", len(art.Actions))
	return art, nil
}

func resolveTargetCase(seeded []*model.Case, title string) (*model.Case, error) {
	if len(seeded) == 0 {
		return nil, goerr.New("job workflow: no seeded case to run the job against")
	}
	if title == "" {
		return seeded[0], nil
	}
	for _, c := range seeded {
		if c.Title == title {
			return c, nil
		}
	}
	return nil, goerr.New("job workflow: target case not found among seeded cases", goerr.V("target_case", title))
}

func resolveJob(jobs []*model.Job, id string) (*model.Job, error) {
	for _, j := range jobs {
		if j.ID == id {
			return j, nil
		}
	}
	return nil, goerr.New("job workflow: job id not found in workspace", goerr.V("job_id", id))
}

// readTimeline extracts the final LLM summary text and the ordered tool calls
// from the job run event timeline.
func readTimeline(ctx context.Context, repo interfaces.Repository, key model.JobRunKey, runID string) (summary string, toolCalls []evaltype.ToolCallRecord) {
	events, err := repo.JobRunEvent().List(ctx, key, runID)
	if err != nil {
		return "", nil
	}
	seq := 0
	for _, ev := range events {
		switch ev.Kind {
		case model.JobRunEventKindLLMResponse:
			if ev.LLMResponse != nil {
				if joined := strings.TrimSpace(strings.Join(ev.LLMResponse.Texts, "\n")); joined != "" {
					summary = joined // keep the latest non-empty response
				}
			}
		case model.JobRunEventKindToolCall:
			if ev.ToolCall != nil {
				seq++
				toolCalls = append(toolCalls, evaltype.ToolCallRecord{
					Seq:  seq,
					Tool: ev.ToolCall.ToolName,
					Args: ev.ToolCall.ArgumentsJSON,
					Mode: "job",
				})
			}
		}
	}
	return summary, toolCalls
}

func listCaseActions(ctx context.Context, repo interfaces.Repository, wsID string, caseID int64) []*model.Action {
	all, err := repo.Action().List(ctx, wsID, interfaces.ActionListOptions{})
	if err != nil {
		return nil
	}
	out := make([]*model.Action, 0)
	for _, a := range all {
		if a.CaseID == caseID {
			out = append(out, a)
		}
	}
	return out
}
