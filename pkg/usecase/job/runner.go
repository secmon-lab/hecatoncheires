package job

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// DefaultLeaseDuration is the default lease length acquired by JobRunner
// before invoking the executor. Long enough to absorb LLM latency, short
// enough that a crashed instance does not lock the row out indefinitely.
const DefaultLeaseDuration = 10 * time.Minute

// executorKindSingleLoop is the JobRunLog.ExecutorKind value emitted by
// v1 SingleLoopJobExecutor. Future plan-execute runtimes will write a
// different value (e.g. "plan_execute") into the same field.
const executorKindSingleLoop = "single_loop"

// runErrorStageExecute labels RUN_ERROR events emitted when the agent
// loop fails. Other stage labels (e.g. "prepare", "finish") are reserved
// for future expansion when those phases gain their own event trails.
const runErrorStageExecute = "execute"

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

	// NewRunID generates a fresh RunID for each Run. nil → UUIDv7.
	NewRunID func() string
	// NewTraceID generates a fresh TraceID for each Run. nil → UUIDv7.
	NewTraceID func() string
	// Clock returns the current wall-clock time. nil → time.Now().UTC().
	// Tests inject a fixed clock for deterministic timestamp assertions.
	Clock func() time.Time
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

func (r *JobRunner) newRunID() string {
	if r.deps.NewRunID != nil {
		return r.deps.NewRunID()
	}
	return uuid.Must(uuid.NewV7()).String()
}

func (r *JobRunner) newTraceID() string {
	if r.deps.NewTraceID != nil {
		return r.deps.NewTraceID()
	}
	return uuid.Must(uuid.NewV7()).String()
}

func (r *JobRunner) clock() time.Time {
	if r.deps.Clock != nil {
		return r.deps.Clock()
	}
	return time.Now().UTC()
}

// Run executes the Job for the given (Job, Case, Event) tuple. It is
// always called from a goroutine — typically launched via
// JobUseCase.Publish → async.Dispatch — so the caller does not block on
// the LLM round-trip.
//
// Lifecycle:
//   - acquire lease
//   - load workspace / case / actions
//   - generate RunID + TraceID, build prompts
//   - JobRunLogRepo.Create with Stage=RUNNING (SystemPrompt inlined)
//   - construct jobRunTraceHandler, hand it to the Executor
//   - Executor runs the gollem agent loop; per-call events stream into
//     Firestore via the handler as they happen
//   - on return (success or failure or panic recovery), append RUN_ERROR
//     if applicable, transition the JobRunLog to SUCCESS/FAILED, and
//     update the JobRun lock doc with LastRunID/LastTraceID/LastError
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
	leaseAt := r.clock()

	acquired, err := r.deps.Repo.JobRun().TryAcquireLease(ctx, key, leaseAt, lease)
	if err != nil {
		return goerr.Wrap(err, "acquire job lease",
			goerr.V("job_id", j.ID), goerr.V("case_id", ev.CaseID))
	}
	if !acquired {
		// Another runner holds the lease — silent idempotent skip.
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

	// --- prepare stage ----------------------------------------------
	// Workspace / case / actions / prompt construction all happen
	// BEFORE we create the JobRunLog. A failure here means the Run
	// never reached a point where we could attribute events to it; we
	// record the failure on the JobRun lock doc only.
	ws, wsErr := r.deps.Registry.Get(ev.WorkspaceID)
	if wsErr != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(wsErr, "load workspace",
			goerr.V("workspace_id", ev.WorkspaceID)))
	}

	c, caseErr := r.deps.Repo.Case().Get(ctx, ev.WorkspaceID, ev.CaseID)
	if caseErr != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(caseErr, "load case",
			goerr.V("workspace_id", ev.WorkspaceID), goerr.V("case_id", ev.CaseID)))
	}

	actions, actErr := r.deps.Repo.Action().GetByCase(ctx, ev.WorkspaceID, ev.CaseID, interfaces.ActionListOptions{
		ArchiveScope: interfaces.ActionArchiveScopeActiveOnly,
	})
	if actErr != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(actErr, "load actions"))
	}

	startedAt := r.clock()
	in := PromptInputs{
		Job:       j,
		Workspace: ws,
		Case:      c,
		Actions:   actions,
		Event:     ev,
		Now:       startedAt,
	}
	systemPrompt, err := BuildSystemPrompt(in)
	if err != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(err, "build system prompt"))
	}
	userPrompt, err := RenderUserPrompt(in)
	if err != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(err, "render user prompt"))
	}

	// --- execute stage ----------------------------------------------
	runID := r.newRunID()
	traceID := r.newTraceID()

	logRec := &model.JobRunLog{
		WorkspaceID:    key.WorkspaceID,
		CaseID:         key.CaseID,
		JobID:          key.JobID,
		RunID:          runID,
		TraceID:        traceID,
		Stage:          model.JobRunStageRunning,
		StartedAt:      startedAt,
		ExecutorKind:   executorKindSingleLoop,
		EventType:      string(ev.Domain),
		EventTriggerAt: ev.Timestamp.UTC(),
		SystemPrompt:   truncateString(systemPrompt, model.MaxInlineBytes),
	}
	if createErr := r.deps.Repo.JobRunLog().Create(ctx, logRec); createErr != nil {
		// We never reached the event-emitting stage; surface as a
		// prepare-stage failure on the lock doc and return.
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(createErr, "create job run log",
			goerr.V("run_id", runID)))
	}

	seq := newRunSequencer()
	handler := newJobRunTraceHandler(
		r.deps.Repo.JobRunEvent(),
		jobRunRouting{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     traceID,
		},
		seq,
		r.clock,
		nil, // default truncator
	)

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
		TraceHandler: handler,
	}
	_, execErr := r.deps.Executor.Execute(ctx, execReq)

	// --- finish stage ----------------------------------------------
	endedAt := r.clock()
	logRec.EndedAt = endedAt
	if execErr != nil {
		logRec.Stage = model.JobRunStageFailed
		logRec.Error = execErr.Error()
		if emitErr := handler.EmitRunError(ctx, runErrorStageExecute, execErr.Error()); emitErr != nil {
			errutil.Handle(ctx, emitErr, "job: append run_error event")
		}
	} else {
		logRec.Stage = model.JobRunStageSuccess
	}

	if finErr := r.deps.Repo.JobRunLog().Finish(ctx, logRec); finErr != nil {
		errutil.Handle(ctx, finErr, "job: finish job run log")
	}

	jobRunStatus := model.JobRunStatusSuccess
	if execErr != nil {
		jobRunStatus = model.JobRunStatusFailed
	}
	if recErr := r.deps.Repo.JobRun().RecordRun(ctx, key, jobRunStatus, endedAt, runID, traceID, logRec.Error); recErr != nil {
		if execErr != nil {
			return goerr.Wrap(execErr, "job failed (and record-run also failed)",
				goerr.V("record_run_error", recErr.Error()))
		}
		return goerr.Wrap(recErr, "record successful run")
	}
	return execErr
}

// recordPrepareFailure writes a FAILED outcome to the JobRun lock doc
// for failures that happened before the JobRunLog was created (workspace
// load, case load, action load, prompt assembly). There is no
// JobRunLog / event trail in this path because no RunID had been
// allocated yet.
func (r *JobRunner) recordPrepareFailure(ctx context.Context, key model.JobRunKey, cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	if recErr := r.deps.Repo.JobRun().RecordRun(ctx, key, model.JobRunStatusFailed, r.clock(), "", "", msg); recErr != nil {
		return goerr.Wrap(cause, "job failed in prepare stage (and record-run also failed)",
			goerr.V("record_run_error", recErr.Error()))
	}
	return cause
}
