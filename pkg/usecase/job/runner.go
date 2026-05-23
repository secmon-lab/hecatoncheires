package job

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
)

// DefaultLeaseDuration is the default lease length acquired by JobRunner
// before invoking the executor. Long enough to absorb LLM latency, short
// enough that a crashed instance does not lock the row out indefinitely.
const DefaultLeaseDuration = 10 * time.Minute

// ToolBuilder lets the host customise the gollem tool set bound to each
// Job run. The JobRunner calls Build exactly once per invocation, after
// it has acquired the lease and loaded the Case. Implementations are
// expected to be pure (no I/O) and to use the *model.Case to pin
// channel-scoped tools.
type ToolBuilder interface {
	Build(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool
}

// ToolBuilderFunc is the function form of ToolBuilder for inline use.
type ToolBuilderFunc func(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool

func (f ToolBuilderFunc) Build(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	return f(ctx, c, ws)
}

// RunnerDeps groups the dependencies a JobRunner needs.
type RunnerDeps struct {
	Repo          interfaces.Repository
	Registry      *model.WorkspaceRegistry
	LLMClient     gollem.LLMClient
	Executor      job.JobExecutor
	ToolBuilder   ToolBuilder
	LeaseDuration time.Duration // 0 → DefaultLeaseDuration
}

// JobRunner orchestrates a single (Job, Case) invocation: acquire lease,
// load entity, build prompts, hand off to the executor, record the
// outcome.
type JobRunner struct {
	deps RunnerDeps
}

// NewJobRunner builds a JobRunner. The caller retains ownership of the
// deps fields; nil checks happen at Run time so DI is easier to wire.
func NewJobRunner(deps RunnerDeps) *JobRunner {
	return &JobRunner{deps: deps}
}

// Run executes the Job for the given (Job, Case, Event) tuple. It is
// always called from a goroutine — typically launched via
// JobUseCase.Publish → async.Dispatch — so the caller does not block on
// the LLM round-trip.
//
// Errors are routed both as the return value and through errutil.Handle
// inside async.Dispatch's wrapper, so callers that fire-and-forget do
// not need to inspect the return value.
func (r *JobRunner) Run(ctx context.Context, j *model.Job, ev Event) error {
	if j == nil {
		return goerr.New("job is nil")
	}
	if err := j.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job at runner entry")
	}

	key := model.JobRunKey{
		WorkspaceID: ev.WorkspaceID,
		CaseID:      ev.CaseID,
		JobID:       j.ID,
	}
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job-run key")
	}

	lease := r.deps.LeaseDuration
	if lease <= 0 {
		lease = DefaultLeaseDuration
	}
	now := time.Now().UTC()

	acquired, err := r.deps.Repo.JobRun().TryAcquireLease(ctx, key, now, lease)
	if err != nil {
		return goerr.Wrap(err, "acquire job lease",
			goerr.V("job_id", j.ID), goerr.V("case_id", ev.CaseID))
	}
	if !acquired {
		// Another runner holds the lease. This is the expected silent
		// idempotent skip — F8.
		return nil
	}
	defer func() {
		// Release on exit. RecordRun also clears the lease, so this is
		// belt-and-braces for the very early error paths that fail
		// before RecordRun.
		if relErr := r.deps.Repo.JobRun().ReleaseLease(context.Background(), key); relErr != nil {
			// Non-fatal; the lease will expire on its own.
			_ = relErr
		}
	}()

	// Mark the context so any mutations the executor performs do not
	// re-publish events.
	ctx = WithJobActor(ctx, JobActorMarker{JobID: j.ID})

	ws, wsErr := r.deps.Registry.Get(ev.WorkspaceID)
	if wsErr != nil {
		return r.recordFailure(ctx, key, now, "", goerr.Wrap(wsErr, "load workspace",
			goerr.V("workspace_id", ev.WorkspaceID)))
	}

	c, caseErr := r.deps.Repo.Case().Get(ctx, ev.WorkspaceID, ev.CaseID)
	if caseErr != nil {
		return r.recordFailure(ctx, key, now, "", goerr.Wrap(caseErr, "load case",
			goerr.V("workspace_id", ev.WorkspaceID), goerr.V("case_id", ev.CaseID)))
	}

	actions, actErr := r.deps.Repo.Action().GetByCase(ctx, ev.WorkspaceID, ev.CaseID, interfaces.ActionListOptions{
		ArchiveScope: interfaces.ActionArchiveScopeActiveOnly,
	})
	if actErr != nil {
		return r.recordFailure(ctx, key, now, "", goerr.Wrap(actErr, "load actions"))
	}

	in := PromptInputs{
		Job:       j,
		Workspace: ws,
		Case:      c,
		Actions:   actions,
		Event:     ev,
		Now:       now,
	}
	systemPrompt, err := BuildSystemPrompt(in)
	if err != nil {
		return r.recordFailure(ctx, key, now, "", err)
	}
	userPrompt, err := RenderUserPrompt(in)
	if err != nil {
		return r.recordFailure(ctx, key, now, "", err)
	}

	var tools []gollem.Tool
	if r.deps.ToolBuilder != nil {
		tools = r.deps.ToolBuilder.Build(ctx, c, ws)
	}

	execReq := job.ExecuteRequest{
		JobID:        j.ID,
		SystemPrompt: systemPrompt,
		Prompt:       userPrompt,
		Tools:        tools,
		LLMClient:    r.deps.LLMClient,
	}
	result, execErr := r.deps.Executor.Execute(ctx, execReq)
	if execErr != nil {
		return r.recordFailure(ctx, key, time.Now().UTC(), "", execErr)
	}

	if recErr := r.deps.Repo.JobRun().RecordRun(ctx, key, model.JobRunStatusSuccess, time.Now().UTC(), "", ""); recErr != nil {
		return goerr.Wrap(recErr, "record successful run")
	}
	_ = result
	return nil
}

func (r *JobRunner) recordFailure(ctx context.Context, key model.JobRunKey, when time.Time, traceID string, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	if recErr := r.deps.Repo.JobRun().RecordRun(ctx, key, model.JobRunStatusFailed, when, traceID, msg); recErr != nil {
		// Don't lose the original error; the record-side failure is
		// secondary.
		return goerr.Wrap(cause, "job failed (and record-run also failed)",
			goerr.V("record_run_error", recErr.Error()))
	}
	return cause
}
