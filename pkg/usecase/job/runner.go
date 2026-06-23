package job

import (
	"context"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// DefaultLeaseDuration is the default lease length acquired by JobRunner
// before invoking the executor. Long enough to absorb LLM latency, short
// enough that a crashed instance does not lock the row out indefinitely.
const DefaultLeaseDuration = 10 * time.Minute

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
		// before RecordRun.
		if relErr := r.deps.Repo.JobRun().ReleaseLease(context.Background(), key); relErr != nil {
			// Non-fatal; the lease will expire on its own.
			_ = relErr
		}
	}()

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
	in := PromptInputs{
		Job:             j,
		Workspace:       ws,
		Case:            c,
		Actions:         actions,
		Memos:           memos,
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

	// Compose the Slack progress handler with the Firestore trace handler so
	// tool executions surface (deduped, minimal) into the session thread.
	// Only wire it when there is actually a session thread to post into and
	// the run is not quiet — otherwise the handler would no-op on every trace
	// event, so skip the trace.Multi fan-out entirely and keep the bare
	// handler (the type the cast-based tests assert against).
	var traceHandler trace.Handler = handler
	if r.deps.SlackNotifier != nil && channelID != "" && sessionThreadTS != "" && !isQuiet(ctx) {
		traceHandler = trace.Multi(handler, newSlackProgressHandler(r.deps.SlackNotifier, channelID, sessionThreadTS))
	}

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
		TraceHandler:      traceHandler,
		TraceID:           traceID,
		HistoryRepository: r.deps.HistoryRepo,
		HistoryKey:        runID,
	}
	_, execErr := executor.Execute(ctx, execReq)

	// --- finish stage ----------------------------------------------
	endedAt := r.clock()
	logRec.EndedAt = endedAt
	if execErr != nil {
		logRec.Stage = model.JobRunStageFailed
		logRec.Error = execErr.Error()
		if emitErr := handler.EmitRunError(ctx, runErrorStageExecute, execErr.Error()); emitErr != nil {
			errutil.Handle(ctx, emitErr, "job: append run_error event")
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
