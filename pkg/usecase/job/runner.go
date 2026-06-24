package job

import (
	"context"
	"errors"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// DefaultLeaseDuration is the default lease length acquired by JobRunner
// before invoking the executor. Long enough to absorb LLM latency, short
// enough that a crashed instance does not lock the row out indefinitely.
const DefaultLeaseDuration = 10 * time.Minute

// recentMessageMaxCount and recentMessageWindow bound the thread's recent
// Slack messages embedded in a thread-mode Job's system prompt: at most the
// newest recentMessageMaxCount messages, and only those within recentMessageWindow
// of the run's start. Fixed feature parameters (not configurable) per the spec.
const (
	recentMessageMaxCount = 32
	recentMessageWindow   = 24 * time.Hour
)

// executorKindSingleLoop is the JobRunLog.ExecutorKind value emitted
// when JobStrategy=simple drives the run. executorKindPlanexec is the
// value emitted when JobStrategy=planexec runs through the shared
// planexec runtime.
const (
	executorKindSingleLoop = "single_loop"
	executorKindPlanexec   = "plan_execute"
)

// executorKindFor maps a Job.Strategy onto the ExecutorKind value
// persisted on JobRunLog. Unknown strategies fall back to single_loop —
// the caller (model.NormaliseJobStrategy + Job.Validate) is responsible
// for catching typos before they reach this point.
func executorKindFor(s model.JobStrategy) string {
	if s == model.JobStrategyPlanexec {
		return executorKindPlanexec
	}
	return executorKindSingleLoop
}

// runErrorStageExecute labels RUN_ERROR events emitted when the agent
// loop fails. Other stage labels (e.g. "prepare", "finish") are reserved
// for future expansion when those phases gain their own event trails.
const runErrorStageExecute = "execute"

// ToolBuilder lets the host customise the gollem tool set bound to each
// Job run. The JobRunner calls Build exactly once per invocation, after
// it has acquired the lease and loaded the Case. Implementations are
// expected to be pure (no I/O) and to use the *model.Case to pin
// channel-scoped tools.
//
// Source-aware tools (Slack search, GitHub query, Notion lookup, …)
// MUST honour the per-Case allowlist in c.AgentSourceIDs: when the
// slice is non-empty, expose only those Sources; an empty slice means
// "use every Workspace Source", preserving today's default behaviour.
type ToolBuilder interface {
	Build(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool
}

// SlackNotifier posts the operational, session-level messages a Job run
// emits around the agent loop: the "starting..." marker, the per-tool
// progress lines, and the completion / failure marker. It is intentionally
// the minimal surface the runner needs — a thin adapter over the broader
// Slack service is wired in at the CLI layer. A nil SlackNotifier disables
// all operational notifications (e.g. the scheduled-tick CLI, which has no
// Slack wiring); the agent's own slack__post_message tool is unaffected.
type SlackNotifier interface {
	// PostMessage posts a new root message to channelID and returns its
	// timestamp (used as the session thread parent in channel-mode Cases).
	PostMessage(ctx context.Context, channelID, text string) (string, error)
	// PostThreadReply posts a reply under threadTS and returns its timestamp.
	PostThreadReply(ctx context.Context, channelID, threadTS, text string) (string, error)
}

// ToolBuilderFunc is the function form of ToolBuilder for inline use.
type ToolBuilderFunc func(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool

func (f ToolBuilderFunc) Build(ctx context.Context, c *model.Case, ws *model.WorkspaceEntry) []gollem.Tool {
	return f(ctx, c, ws)
}

// RunnerDeps groups the dependencies a JobRunner needs.
type RunnerDeps struct {
	Repo      interfaces.Repository
	Registry  *model.WorkspaceRegistry
	LLMClient gollem.LLMClient

	// Executors maps a Job.Strategy onto the executor that drives it.
	// The runner looks up by the job's (normalised) Strategy at Run
	// time. Wiring code MUST populate at least JobStrategySimple;
	// JobStrategyPlanexec is required only when any workspace declares
	// `strategy = "planexec"`.
	Executors map[model.JobStrategy]job.JobExecutor

	ToolBuilder   ToolBuilder
	LeaseDuration time.Duration // 0 → DefaultLeaseDuration

	// Reflector runs the optional post-execution reflection pass (curating
	// workspace Knowledge from a successful run's history). Nil disables
	// reflection for every Job regardless of the Job's `reflection` flag.
	Reflector job.Reflector

	// HistoryRepo persists each run's conversation history (keyed by RunID) so
	// the reflection pass can replay it via gollem.WithHistory. The SAME
	// instance MUST be shared with the planexec runner so its planner/final
	// history is loadable here. Nil disables reflection (no history to carry).
	HistoryRepo gollem.HistoryRepository

	// SlackNotifier posts the run's operational session log (starting /
	// tool-progress / completion markers) to the Case's Slack channel. Nil
	// disables all such notifications; the run still executes and records
	// its trace as before.
	SlackNotifier SlackNotifier

	// InteractionPoster posts the interactive-Job question form (Block Kit)
	// into the run's Slack thread. Required for interactive Jobs; nil means
	// an interactive run that emits a question fails loudly (it has no
	// surface to ask on). The CLI wires the Slack service here. Distinct
	// from SlackNotifier because the question form is Block Kit, not a
	// text-only operational marker, and is NOT gated by the Job's quiet
	// flag (the question is a deliberate agent interaction, not a log).
	InteractionPoster jobQuestionPoster

	// UnansweredTimeout bounds how long a run may stay suspended awaiting
	// user input before Run treats the suspension as stale and recovers it
	// (so the Job is not blocked forever by an unanswered question). 0 →
	// DefaultUnansweredTimeout. The scheduled sweep uses the same bound.
	UnansweredTimeout time.Duration

	// NewRunID generates a fresh RunID for each Run. nil → UUIDv7.
	NewRunID func() string
	// NewTraceID generates a fresh TraceID for each Run. nil → UUIDv7.
	NewTraceID func() string
	// Clock returns the current wall-clock time. nil → time.Now().UTC().
	// Tests inject a fixed clock for deterministic timestamp assertions.
	Clock func() time.Time
}

// executorFor picks the JobExecutor matching strategy. Returns an error
// if no executor is registered for the strategy — callers handle this
// as a prepare-stage failure (RecordRun with FAILED status).
func (d *RunnerDeps) executorFor(strategy model.JobStrategy) (job.JobExecutor, error) {
	if len(d.Executors) == 0 {
		return nil, goerr.New("no executors registered")
	}
	exec, ok := d.Executors[strategy]
	if !ok {
		return nil, goerr.New("no executor for strategy",
			goerr.V("strategy", string(strategy)))
	}
	return exec, nil
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
		// before RecordRun. It only zeroes LeaseUntil and never touches
		// SuspendedRunID, so it is safe even when a run suspends.
		if relErr := r.deps.Repo.JobRun().ReleaseLease(context.Background(), key); relErr != nil {
			// Non-fatal; the lease will expire on its own.
			_ = relErr
		}
	}()

	// Do not start a fresh run while a prior run is genuinely suspended
	// awaiting user input for this (workspace, case, job). The lease is
	// released while suspended, so TryAcquireLease alone would let a trigger
	// start a duplicate; SuspendedRunID is the durable guard.
	//
	// Fail CLOSED on a Get error (other than not-found): a transient read
	// failure must not let a new run clobber a suspended one. Only a clean
	// "never ran" (ErrJobRunNotFound) proceeds.
	existing, getErr := r.deps.Repo.JobRun().Get(ctx, key)
	if getErr != nil && !errors.Is(getErr, interfaces.ErrJobRunNotFound) {
		return goerr.Wrap(getErr, "check job run suspension state before run",
			goerr.V("job_id", j.ID), goerr.V("case_id", ev.CaseID))
	}
	if existing != nil && existing.IsSuspended() {
		// A suspension is recorded. If it is genuinely active (the user still
		// has an open question within the timeout), skip this trigger so the
		// pending run keeps the slot. If it is stale (unanswered past the
		// timeout) or inconsistent (its run log is no longer AWAITING_INPUT —
		// e.g. a resume crashed leaving a RUNNING log + dangling marker),
		// finalize the orphan here so this trigger can start fresh rather than
		// being blocked forever. The orphan's SuspendedRunID is cleared by
		// this run's terminal RecordRun (or overwritten if it suspends again).
		if r.suspensionIsActive(ctx, existing, leaseAt) {
			return nil
		}
		r.finalizeOrphanedSuspension(ctx, existing)
	}

	// Mark the context so any mutations the executor performs do not
	// re-publish events.
	ctx = WithJobActor(ctx, JobActorMarker{JobID: j.ID})

	// Carry the Job's quiet flag for the whole run. Each operational-log
	// post site (starting marker, tool-progress handler, completion marker)
	// self-gates on isQuiet(ctx) rather than threading the bool through
	// every constructor — mirroring WithJobActor above.
	ctx = withQuiet(ctx, j.Quiet)

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

	sources, narrowed, srcErr := r.resolveSources(ctx, ev.WorkspaceID, c)
	if srcErr != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(srcErr, "load sources for system prompt"))
	}

	// Active memos are the agent's working memory; load them only when the
	// workspace enabled memos so non-memo workspaces incur no extra read.
	var memos []*model.Memo
	if ws != nil && ws.MemoConfig.Enabled() {
		ms, memoErr := r.deps.Repo.Memo().List(ctx, ev.WorkspaceID, ev.CaseID, interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeActiveOnly,
		})
		if memoErr != nil {
			return r.recordPrepareFailure(ctx, key, goerr.Wrap(memoErr, "load memos"))
		}
		memos = ms
	}

	startedAt := r.clock()

	// Thread-mode workspaces feed the Job agent the thread's recent Slack
	// messages so it can reason about the latest conversation. Channel-mode
	// Jobs skip this read entirely (their prompt has no such section). The
	// window is anchored to startedAt so it matches the prompt's "Current time".
	var recentMessages []*slack.Message
	if ws != nil && ws.IsThreadMode() {
		ms, msgErr := r.loadRecentMessages(ctx, ev.WorkspaceID, ev.CaseID, startedAt)
		if msgErr != nil {
			return r.recordPrepareFailure(ctx, key, goerr.Wrap(msgErr, "load recent thread messages",
				goerr.V("workspace_id", ev.WorkspaceID), goerr.V("case_id", ev.CaseID)))
		}
		recentMessages = ms
	}

	in := PromptInputs{
		Job:             j,
		Workspace:       ws,
		Case:            c,
		Actions:         actions,
		Memos:           memos,
		RecentMessages:  recentMessages,
		Event:           ev,
		Now:             startedAt,
		Sources:         sources,
		SourcesNarrowed: narrowed,
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

	strategy := model.NormaliseJobStrategy(j.Strategy)
	executor, execLookupErr := r.deps.executorFor(strategy)
	if execLookupErr != nil {
		return r.recordPrepareFailure(ctx, key, goerr.Wrap(execLookupErr, "select executor",
			goerr.V("job_id", j.ID),
			goerr.V("strategy", string(strategy))))
	}

	logRec := &model.JobRunLog{
		WorkspaceID:    key.WorkspaceID,
		CaseID:         key.CaseID,
		JobID:          key.JobID,
		RunID:          runID,
		TraceID:        traceID,
		Stage:          model.JobRunStageRunning,
		StartedAt:      startedAt,
		ExecutorKind:   executorKindFor(strategy),
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

	// Post the "starting..." marker and resolve the session thread the
	// run's operational log consolidates into. Channel-mode Cases root a
	// fresh thread at the marker; thread-mode Cases reuse the Case thread
	// (Slack only nests one level). Both no-op for quiet runs / nil
	// notifier; a failed marker post degrades to no session thread but the
	// run proceeds.
	channelID := ""
	if c != nil {
		channelID = c.SlackChannelID
	}
	sessionThreadTS := r.postStarting(ctx, j, c)

	var tools []gollem.Tool
	if r.deps.ToolBuilder != nil {
		tools = r.deps.ToolBuilder.Build(ctx, c, ws)
	}

	// The history key is the RunID: a fresh, unique key per run so each run's
	// conversation is persisted in isolation and the reflection pass loads
	// exactly this run's history. Always set (the planexec executor requires a
	// non-empty HistoryKey); HistoryRepository may be nil for the single-loop
	// path, which then simply does not persist.
	execReq := job.ExecuteRequest{
		JobID:             j.ID,
		SystemPrompt:      systemPrompt,
		Prompt:            userPrompt,
		Tools:             tools,
		LLMClient:         r.deps.LLMClient,
		TraceHandler:      handler,
		TraceID:           traceID,
		HistoryRepository: r.deps.HistoryRepo,
		HistoryKey:        runID,
	}
	// Interactive Jobs (planexec-only, enforced by Job.Validate) may suspend
	// the run to ask the user. Build the per-run Interactor and wire it; the
	// question posts into the run's session thread (channel-mode) or the Case
	// thread (thread-mode). The question is NOT gated by quiet — it is an
	// agent interaction, not an operational log.
	if j.Interactive {
		questionThreadTS := sessionThreadTS
		if questionThreadTS == "" && c != nil {
			questionThreadTS = c.SlackThreadTS
		}
		requesterUserID := ev.ActorUserID
		if requesterUserID == "" && c != nil {
			requesterUserID = c.ReporterID
		}
		execReq.Interactive = true
		execReq.Interactor = newJobInteractor(
			r.deps.Repo, r.deps.InteractionPoster, key, runID,
			channelID, questionThreadTS, requesterUserID, logRec, r.clock,
		)
	}
	res, execErr := executor.Execute(ctx, execReq)

	// An interactive run that suspended to ask the user has already
	// transitioned its log to AWAITING_INPUT and marked the JobRun suspended
	// (via the Interactor). Leave it paused: do NOT Finish the log or
	// RecordRun (which would clear the suspension marker). Resume arrives
	// out-of-band when the user answers.
	if execErr == nil && res != nil && res.Status == job.ExecuteStatusAwaitingInput {
		return nil
	}

	// --- finish stage ----------------------------------------------
	return r.finishRun(ctx, j, c, key, logRec, handler, channelID, sessionThreadTS, runID, traceID, execErr)
}

// finishRun transitions the run log to its terminal stage, emits the
// completion / failure session-log marker, runs the optional reflection
// pass, and records the outcome on the JobRun lock doc. Shared by the
// fresh-run and resume paths so both terminate identically. RecordRun also
// clears any suspension marker (a terminal run is no longer awaiting input).
func (r *JobRunner) finishRun(
	ctx context.Context,
	j *model.Job,
	c *model.Case,
	key model.JobRunKey,
	logRec *model.JobRunLog,
	handler *jobRunTraceHandler,
	channelID, sessionThreadTS, runID, traceID string,
	execErr error,
) error {
	endedAt := r.clock()
	logRec.EndedAt = endedAt
	// A resumed log may still carry the pending interaction in memory; the
	// terminal stages forbid it, so clear it before persisting.
	logRec.PendingInteraction = nil
	if execErr != nil {
		logRec.Stage = model.JobRunStageFailed
		logRec.Error = execErr.Error()
		// handler is nil on the resume prepare-failure path (no event stream
		// was set up); the RUN_ERROR event is then skipped but the failure is
		// still recorded on the log + lock doc below.
		if handler != nil {
			if emitErr := handler.EmitRunError(ctx, runErrorStageExecute, execErr.Error()); emitErr != nil {
				errutil.Handle(ctx, emitErr, "job: append run_error event")
			}
		}
		r.postSessionLog(ctx, channelID, sessionThreadTS,
			i18n.T(ctx, i18n.MsgJobRunFailed, j.ID, truncateString(execErr.Error(), model.MaxInlineBytes)))
	} else {
		logRec.Stage = model.JobRunStageSuccess
		r.postSessionLog(ctx, channelID, sessionThreadTS,
			i18n.T(ctx, i18n.MsgJobRunCompleted, j.ID))
		// Reflection runs only on success, as a best-effort tail that never
		// affects the run's outcome (failures are reported via errutil and the
		// Job stays SUCCESS). Gated on the Job opting in, a non-private case,
		// and the reflection deps being wired.
		r.maybeReflect(ctx, j, c, key, runID, handler)
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

// preparedRun bundles the workspace/case context, rendered prompts, and tool
// set shared by a fresh Run and a Resume.
type preparedRun struct {
	ws           *model.WorkspaceEntry
	c            *model.Case
	systemPrompt string
	userPrompt   string
	channelID    string
	tools        []gollem.Tool
	startedAt    time.Time
}

// prepareRun loads the workspace/case/actions/sources/memos, renders the
// prompts, and builds the tool set. It returns a wrapped error rather than
// recording a prepare-stage failure so each caller (Run / Resume) maps it
// onto its own failure path.
func (r *JobRunner) prepareRun(ctx context.Context, j *model.Job, ev Event) (*preparedRun, error) {
	ws, wsErr := r.deps.Registry.Get(ev.WorkspaceID)
	if wsErr != nil {
		return nil, goerr.Wrap(wsErr, "load workspace", goerr.V("workspace_id", ev.WorkspaceID))
	}
	c, caseErr := r.deps.Repo.Case().Get(ctx, ev.WorkspaceID, ev.CaseID)
	if caseErr != nil {
		return nil, goerr.Wrap(caseErr, "load case",
			goerr.V("workspace_id", ev.WorkspaceID), goerr.V("case_id", ev.CaseID))
	}
	actions, actErr := r.deps.Repo.Action().GetByCase(ctx, ev.WorkspaceID, ev.CaseID, interfaces.ActionListOptions{
		ArchiveScope: interfaces.ActionArchiveScopeActiveOnly,
	})
	if actErr != nil {
		return nil, goerr.Wrap(actErr, "load actions")
	}
	sources, narrowed, srcErr := r.resolveSources(ctx, ev.WorkspaceID, c)
	if srcErr != nil {
		return nil, goerr.Wrap(srcErr, "load sources for system prompt")
	}
	var memos []*model.Memo
	if ws != nil && ws.MemoConfig.Enabled() {
		ms, memoErr := r.deps.Repo.Memo().List(ctx, ev.WorkspaceID, ev.CaseID, interfaces.MemoListOptions{
			ArchiveScope: interfaces.MemoArchiveScopeActiveOnly,
		})
		if memoErr != nil {
			return nil, goerr.Wrap(memoErr, "load memos")
		}
		memos = ms
	}
	startedAt := r.clock()
	var recentMessages []*slack.Message
	if ws != nil && ws.IsThreadMode() {
		ms, msgErr := r.loadRecentMessages(ctx, ev.WorkspaceID, ev.CaseID, startedAt)
		if msgErr != nil {
			return nil, goerr.Wrap(msgErr, "load recent thread messages",
				goerr.V("workspace_id", ev.WorkspaceID), goerr.V("case_id", ev.CaseID))
		}
		recentMessages = ms
	}
	in := PromptInputs{
		Job:             j,
		Workspace:       ws,
		Case:            c,
		Actions:         actions,
		Memos:           memos,
		RecentMessages:  recentMessages,
		Event:           ev,
		Now:             startedAt,
		Sources:         sources,
		SourcesNarrowed: narrowed,
	}
	systemPrompt, err := BuildSystemPrompt(in)
	if err != nil {
		return nil, goerr.Wrap(err, "build system prompt")
	}
	userPrompt, err := RenderUserPrompt(in)
	if err != nil {
		return nil, goerr.Wrap(err, "render user prompt")
	}
	channelID := ""
	if c != nil {
		channelID = c.SlackChannelID
	}
	var tools []gollem.Tool
	if r.deps.ToolBuilder != nil {
		tools = r.deps.ToolBuilder.Build(ctx, c, ws)
	}
	return &preparedRun{
		ws: ws, c: c, systemPrompt: systemPrompt, userPrompt: userPrompt,
		channelID: channelID, tools: tools, startedAt: startedAt,
	}, nil
}

// Resume continues a run that suspended awaiting user input. It is the
// out-of-band counterpart to Run, invoked by the Slack question-submit
// handler with the decoded run identity and the user's answers. It
// re-acquires the lease (the exclusion gate against concurrent submits and
// the unanswered sweep), transitions the suspended log back to RUNNING, and
// drives the planexec executor's Resume, which re-enters at a replan round
// with the answers folded in and the same HistoryKey (so the conversation
// continues). A resumed turn may itself suspend again.
func (r *JobRunner) Resume(ctx context.Context, key model.JobRunKey, runID string, answers []interaction.Answer) error {
	if err := key.Validate(); err != nil {
		return goerr.Wrap(err, "invalid job-run key for resume")
	}
	if runID == "" {
		return goerr.New("run id is empty for resume")
	}
	if len(answers) == 0 {
		return goerr.New("resume requires at least one answer", goerr.V("run_id", runID))
	}

	ws, wsErr := r.deps.Registry.Get(key.WorkspaceID)
	if wsErr != nil {
		return goerr.Wrap(wsErr, "load workspace for resume", goerr.V("workspace_id", key.WorkspaceID))
	}
	var j *model.Job
	for _, candidate := range ws.Jobs {
		if candidate.ID == key.JobID {
			j = candidate
			break
		}
	}
	if j == nil {
		return goerr.New("job not found for resume", goerr.V("job_id", key.JobID))
	}

	lease := r.deps.LeaseDuration
	if lease <= 0 {
		lease = DefaultLeaseDuration
	}
	acquired, err := r.deps.Repo.JobRun().TryAcquireLease(ctx, key, r.clock(), lease)
	if err != nil {
		return goerr.Wrap(err, "acquire lease for resume",
			goerr.V("job_id", key.JobID), goerr.V("run_id", runID))
	}
	if !acquired {
		// Another resume / sweep is in flight — no-op (the submit surface
		// degrades to a stale form via the handler).
		return nil
	}
	defer func() {
		if relErr := r.deps.Repo.JobRun().ReleaseLease(context.Background(), key); relErr != nil {
			_ = relErr
		}
	}()

	logRec, err := r.deps.Repo.JobRunLog().Get(ctx, key, runID)
	if err != nil {
		if errors.Is(err, interfaces.ErrJobRunLogNotFound) {
			return nil
		}
		return goerr.Wrap(err, "load run log for resume", goerr.V("run_id", runID))
	}
	if logRec.Stage != model.JobRunStageAwaitingInput || logRec.PendingInteraction == nil {
		// Already resumed, completed, or expired — stale, no-op.
		return nil
	}
	pending := *logRec.PendingInteraction

	strategy := model.NormaliseJobStrategy(j.Strategy)
	executor, execLookupErr := r.deps.executorFor(strategy)
	if execLookupErr != nil {
		return goerr.Wrap(execLookupErr, "select executor for resume",
			goerr.V("job_id", j.ID), goerr.V("strategy", string(strategy)))
	}
	resumable, ok := executor.(job.ResumableJobExecutor)
	if !ok {
		return goerr.New("executor does not support resume",
			goerr.V("job_id", j.ID), goerr.V("strategy", string(strategy)))
	}

	ctx = WithJobActor(ctx, JobActorMarker{JobID: j.ID})
	ctx = withQuiet(ctx, j.Quiet)

	// Reconstruct the triggering Event from the run log's provenance so the
	// prompts render with the same framing as the original turn.
	ev := Event{
		Domain:      model.JobEventDomain(logRec.EventType),
		WorkspaceID: key.WorkspaceID,
		CaseID:      key.CaseID,
		Timestamp:   logRec.EventTriggerAt,
	}
	prep, prepErr := r.prepareRun(ctx, j, ev)
	if prepErr != nil {
		// Mark the run failed: we cannot rebuild the context to continue.
		return r.finishRun(ctx, j, nil, key, logRec, nil, "", "", runID, logRec.TraceID,
			goerr.Wrap(prepErr, "prepare resume"))
	}

	// Continue the run's event Sequence past the suspended turn's events so
	// the resumed turn's events do not collide on Sequence (same RunID space).
	startSeq := int64(1)
	if existing, listErr := r.deps.Repo.JobRunEvent().List(ctx, key, runID); listErr == nil {
		for _, e := range existing {
			if e.Sequence >= startSeq {
				startSeq = e.Sequence + 1
			}
		}
	}
	handler := newJobRunTraceHandler(
		r.deps.Repo.JobRunEvent(),
		jobRunRouting{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     logRec.TraceID,
		},
		newRunSequencerStartingAt(startSeq),
		r.clock,
		nil,
	)

	// Transition the log back to RUNNING (clears the pending interaction)
	// before executing so a crash mid-resume leaves a RUNNING log, not a
	// stuck AWAITING_INPUT one.
	logRec.Stage = model.JobRunStageRunning
	logRec.PendingInteraction = nil
	logRec.EndedAt = time.Time{}
	if resumeErr := r.deps.Repo.JobRunLog().Resume(ctx, logRec); resumeErr != nil {
		return goerr.Wrap(resumeErr, "transition run log to running for resume",
			goerr.V("run_id", runID))
	}

	// The re-question thread (if the resumed turn asks again) is the Case
	// thread for thread-mode Cases; channel-mode resume cannot recreate the
	// original session thread, so a re-question there fails loudly.
	questionThreadTS := ""
	requesterUserID := ""
	if prep.c != nil {
		questionThreadTS = prep.c.SlackThreadTS
		requesterUserID = prep.c.ReporterID
	}
	interactor := newJobInteractor(
		r.deps.Repo, r.deps.InteractionPoster, key, runID,
		prep.channelID, questionThreadTS, requesterUserID, logRec, r.clock,
	)

	execReq := job.ExecuteRequest{
		JobID:             j.ID,
		SystemPrompt:      prep.systemPrompt,
		Prompt:            prep.userPrompt,
		Tools:             prep.tools,
		LLMClient:         r.deps.LLMClient,
		TraceHandler:      handler,
		TraceID:           logRec.TraceID,
		HistoryRepository: r.deps.HistoryRepo,
		HistoryKey:        runID,
		Interactive:       true,
		Interactor:        interactor,
	}
	res, execErr := resumable.Resume(ctx, execReq, pending, answers)
	if execErr == nil && res != nil && res.Status == job.ExecuteStatusAwaitingInput {
		// Re-suspended on a follow-up question; leave paused.
		return nil
	}

	sessionThreadTS := questionThreadTS
	return r.finishRun(ctx, j, prep.c, key, logRec, handler, prep.channelID, sessionThreadTS, runID, logRec.TraceID, execErr)
}

// unansweredTimeout returns the configured suspension timeout, or the
// default when unset.
func (r *JobRunner) unansweredTimeout() time.Duration {
	if r.deps.UnansweredTimeout > 0 {
		return r.deps.UnansweredTimeout
	}
	return DefaultUnansweredTimeout
}

// suspensionIsActive reports whether the recorded suspension is a genuine,
// still-open question (so a new trigger must step aside) versus a stale or
// inconsistent leftover that should be recovered. It is "active" only when
// the suspended run's log is AWAITING_INPUT and the suspension is within the
// timeout. A missing log, a non-AWAITING_INPUT stage (e.g. a RUNNING log left
// by a crashed resume), or an elapsed timeout all count as NOT active.
func (r *JobRunner) suspensionIsActive(ctx context.Context, run *model.JobRun, now time.Time) bool {
	if run == nil || run.SuspendedRunID == "" {
		return false
	}
	if run.SuspendedAt.IsZero() || now.Sub(run.SuspendedAt) >= r.unansweredTimeout() {
		return false
	}
	log, err := r.deps.Repo.JobRunLog().Get(ctx, run.Key(), run.SuspendedRunID)
	if err != nil {
		// Cannot confirm an open question — treat as recoverable rather than
		// blocking the Job forever on an unverifiable marker.
		return false
	}
	return log.Stage == model.JobRunStageAwaitingInput
}

// finalizeOrphanedSuspension fails the orphaned run log behind a stale or
// inconsistent suspension so it does not linger as a perpetual
// AWAITING_INPUT / RUNNING record. Best-effort: the suspension marker itself
// is cleared by the fresh run's terminal RecordRun (or overwritten if the
// fresh run suspends again).
func (r *JobRunner) finalizeOrphanedSuspension(ctx context.Context, run *model.JobRun) {
	if run == nil || run.SuspendedRunID == "" {
		return
	}
	log, err := r.deps.Repo.JobRunLog().Get(ctx, run.Key(), run.SuspendedRunID)
	if err != nil {
		// Nothing to finalize (already gone / unreadable); the marker is
		// cleared by the fresh run's RecordRun.
		return
	}
	if log.Stage == model.JobRunStageSuccess || log.Stage == model.JobRunStageFailed {
		return
	}
	log.Stage = model.JobRunStageFailed
	log.EndedAt = r.clock()
	log.Error = "superseded: suspended run recovered by a new trigger before it was answered"
	log.PendingInteraction = nil
	if finErr := r.deps.Repo.JobRunLog().Finish(ctx, log); finErr != nil {
		errutil.Handle(ctx, finErr, "job: finalize orphaned suspended run log")
	}
}

// postStarting posts the "starting..." marker and returns the timestamp
// that roots the run's session-log thread. The contract:
//   - no notifier / quiet run / Case without a Slack channel → "" (no
//     session log at all).
//   - thread-mode Case (SlackThreadTS set) → reply into the Case thread and
//     return that thread_ts; the Case thread doubles as the session thread
//     (Slack nests only one level).
//   - channel-mode Case → post a root message; its timestamp roots a fresh
//     session thread. A failed post degrades to "" (run still proceeds).
//
// All failures are non-fatal (errutil.Handle); the marker is observability,
// not part of the run's success contract.
func (r *JobRunner) postStarting(ctx context.Context, j *model.Job, c *model.Case) string {
	notifier := r.deps.SlackNotifier
	if notifier == nil || isQuiet(ctx) || c == nil || c.SlackChannelID == "" {
		return ""
	}
	text := i18n.T(ctx, i18n.MsgJobRunStarting, j.ID)

	if c.SlackThreadTS != "" {
		if _, err := notifier.PostThreadReply(ctx, c.SlackChannelID, c.SlackThreadTS, text); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "post job run starting marker",
				goerr.V("channel_id", c.SlackChannelID),
				goerr.V("thread_ts", c.SlackThreadTS),
				goerr.V("job_id", j.ID)), "job: post starting marker to slack")
		}
		return c.SlackThreadTS
	}

	ts, err := notifier.PostMessage(ctx, c.SlackChannelID, text)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post job run starting marker",
			goerr.V("channel_id", c.SlackChannelID),
			goerr.V("job_id", j.ID)), "job: post starting marker to slack")
		return ""
	}
	return ts
}

// postSessionLog appends one operational line (completion / failure marker)
// to the run's session thread. No-op when notifications are disabled or no
// session thread was established. Non-fatal on error.
func (r *JobRunner) postSessionLog(ctx context.Context, channelID, sessionThreadTS, text string) {
	notifier := r.deps.SlackNotifier
	if notifier == nil || isQuiet(ctx) || channelID == "" || sessionThreadTS == "" {
		return
	}
	if _, err := notifier.PostThreadReply(ctx, channelID, sessionThreadTS, text); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post job run session log",
			goerr.V("channel_id", channelID),
			goerr.V("thread_ts", sessionThreadTS)), "job: post session log to slack")
	}
}

// resolveSources turns Case.AgentSourceIDs into the list of Sources that
// will appear in the system prompt. The contract is:
//   - operator narrowed (Case.AgentSourceIDs non-empty)
//     → fetch each by ID, drop any that no longer exist or that are
//     disabled (silent skip: a Source toggled off after selection
//     must not invalidate the Case settings or fail the Job), return
//     narrowed=true so the prompt phrases the list as a preference.
//   - no selection (empty)
//     → list every ENABLED Workspace Source so the agent sees the full
//     catalogue, return narrowed=false so the prompt phrases the
//     list as "no narrowing in effect".
//
// Either way the agent is never *forced* to restrict itself — the
// Sources section is a hint, not a filter. See `prompts/system.md`
// `# Sources`.
func (r *JobRunner) resolveSources(ctx context.Context, workspaceID string, c *model.Case) ([]*model.Source, bool, error) {
	if c == nil {
		return nil, false, nil
	}

	// One List call covers both branches: the empty-selection branch
	// returns every enabled Source as-is, the narrowed branch filters
	// the same list by the operator's allowlist. Workspace Source
	// catalogues are small (handful per workspace), so a single list
	// query is cheaper than N parallel Gets and avoids any per-ID
	// "not found" handling — IDs missing from the catalogue (Source
	// deleted after selection) simply don't appear in the filter
	// output, which is exactly the silent-skip semantics we want.
	all, err := r.deps.Repo.Source().List(ctx, workspaceID)
	if err != nil {
		return nil, len(c.AgentSourceIDs) > 0, goerr.Wrap(err, "list workspace sources",
			goerr.V("workspace_id", workspaceID),
			goerr.V("case_id", c.ID))
	}

	if len(c.AgentSourceIDs) == 0 {
		out := make([]*model.Source, 0, len(all))
		for _, s := range all {
			if s == nil || !s.Enabled {
				continue
			}
			out = append(out, s)
		}
		return out, false, nil
	}

	known := make(map[model.SourceID]*model.Source, len(all))
	for _, s := range all {
		if s == nil || !s.Enabled {
			continue
		}
		known[s.ID] = s
	}
	out := make([]*model.Source, 0, len(c.AgentSourceIDs))
	for _, id := range c.AgentSourceIDs {
		if s, ok := known[id]; ok {
			out = append(out, s)
		}
	}
	return out, true, nil
}

// loadRecentMessages returns the case thread's recent Slack messages for the
// system prompt: at most recentMessageMaxCount, only those created within
// recentMessageWindow of now, ordered oldest-first.
//
// CaseMessage().List returns the newest recentMessageMaxCount messages
// newest-first; we then drop any older than the window (so the cap and the
// window compose to "the newest <=N within the window") and reverse into
// chronological order so the prompt reads as a conversation. The window is
// judged on CreatedAt — the same field the repository orders and prunes by —
// rather than the Slack EventTS string, keeping recency consistent with the
// stored order.
func (r *JobRunner) loadRecentMessages(ctx context.Context, workspaceID string, caseID int64, now time.Time) ([]*slack.Message, error) {
	msgs, _, err := r.deps.Repo.CaseMessage().List(ctx, workspaceID, caseID, recentMessageMaxCount, "")
	if err != nil {
		return nil, goerr.Wrap(err, "list case messages")
	}

	cutoff := now.Add(-recentMessageWindow)
	out := make([]*slack.Message, 0, len(msgs))
	// msgs is newest-first; iterate in reverse to emit oldest-first. A message
	// at or after the cutoff is in-window; older ones are dropped.
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m == nil || m.CreatedAt().Before(cutoff) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// maybeReflect runs the optional post-execution reflection pass. It is a no-op
// unless the Job opted in (j.Reflection), the case is non-private, and both the
// Reflector and HistoryRepo are wired. All failures are non-fatal: reflection
// is a learning tail, not part of the run's success contract.
func (r *JobRunner) maybeReflect(ctx context.Context, j *model.Job, c *model.Case, key model.JobRunKey, runID string, handler *jobRunTraceHandler) {
	if !j.Reflection || r.deps.Reflector == nil || r.deps.HistoryRepo == nil {
		return
	}
	if c == nil || c.IsPrivate {
		// Private-case contents must not leak into shared workspace knowledge.
		return
	}

	history, err := r.deps.HistoryRepo.Load(ctx, runID)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "load run history for reflection",
			goerr.V("job_id", j.ID), goerr.V("run_id", runID)), "job: load reflection history")
		return
	}
	if history == nil {
		// Nothing was persisted (e.g. the executor ran without a history repo),
		// so there is no conversation to reflect on.
		return
	}

	handler.enterReflectionPhase()
	if reflErr := r.deps.Reflector.Reflect(ctx, job.ReflectRequest{
		WorkspaceID:    key.WorkspaceID,
		CaseID:         key.CaseID,
		JobID:          j.ID,
		JobName:        j.Name,
		JobDescription: j.Description,
		History:        history,
		TraceHandler:   handler,
	}); reflErr != nil {
		errutil.Handle(ctx, goerr.Wrap(reflErr, "job reflection",
			goerr.V("job_id", j.ID), goerr.V("case_id", key.CaseID)), "job: reflection")
	}
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
