package runtrace

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// runErrorStage labels the RUN_ERROR event a failed run emits. Mention runs
// have a single execution stage, so the value is fixed.
const runErrorStage = "execute"

// Recorder manages the JobRunLog lifecycle for a case-scoped agent run that
// does NOT go through the Job runner — a Slack mention handled by the
// casebound / threadcase hosts. It is the mention-side counterpart to the
// lifecycle the Job runner drives inline: Open creates the RUNNING JobRunLog
// and the per-event Handler, and Finish transitions the log to its terminal
// stage and materialises the JobRun summary doc (so ListByCase surfaces the
// run on the case agent page).
//
// The parent JobRun doc is materialised at Finish (via RecordRun), not at Open:
// the mention hosts serialise concurrent turns through their own per-thread
// session lock, so Recorder must not take the Job lease (that would falsely
// exclude a concurrent mention on a different thread of the same case). A run
// therefore surfaces in the list at Finish.
//
// Because each mention turn uses its OWN fresh JobID, its parent JobRun doc
// exists only once that turn Finishes. A hard process kill between Open and
// Finish therefore leaves a RUNNING log whose parent JobRun doc was never
// written, so ListByCase never returns it and the orphan stays invisible — no
// perpetual-RUNNING row pollutes the list. This is an accepted edge case: the
// deferred Finish runs on normal returns and on panics recovered by
// async.Dispatch, so only an abrupt process death (SIGKILL / OOM) can reach it.
type Recorder struct {
	repo    interfaces.Repository
	key     model.JobRunKey
	log     *model.JobRunLog
	handler *Handler
	clock   func() time.Time
}

// OpenParams collects the inputs Open needs. The caller owns RunID / TraceID
// so it can align the TraceID with its own durable trace recorder (the
// Cloud Storage archive), keeping both trace sinks correlated.
type OpenParams struct {
	Repo         interfaces.Repository
	WorkspaceID  string
	CaseID       int64
	JobID        string // fresh per-turn id for a mention run; a config id for a Job
	RunID        string
	TraceID      string
	EventType    string // provenance, e.g. model.EventTypeMention
	ExecutorKind string // model.ExecutorKindSingleLoop / ExecutorKindPlanexec
	SystemPrompt string
	StartedAt    time.Time
	// Clock supplies wall-clock time for the per-event handler and the Close
	// timestamp. nil → time.Now().UTC(). Tests inject a fixed clock.
	Clock func() time.Time
}

func (p OpenParams) validate() error {
	if p.Repo == nil {
		return goerr.New("repository is required")
	}
	if p.WorkspaceID == "" {
		return goerr.New("workspace id is empty")
	}
	if p.CaseID == 0 {
		return goerr.New("case id is zero")
	}
	if p.JobID == "" {
		return goerr.New("job id is empty")
	}
	if p.RunID == "" {
		return goerr.New("run id is empty")
	}
	if p.TraceID == "" {
		return goerr.New("trace id is empty")
	}
	if p.EventType == "" {
		return goerr.New("event type is empty")
	}
	if p.ExecutorKind == "" {
		return goerr.New("executor kind is empty")
	}
	if p.StartedAt.IsZero() {
		return goerr.New("started at is zero")
	}
	return nil
}

// Open creates the RUNNING JobRunLog and returns a Recorder whose Handler the
// caller wires into gollem (or planexec's RunRequest.TraceHandler). A failure
// here is returned to the caller, which treats it as non-fatal (the turn still
// runs, just untraced) — creating the log is observability, not part of the
// turn's success contract.
func Open(ctx context.Context, p OpenParams) (*Recorder, error) {
	if err := p.validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid runtrace open params")
	}
	clock := p.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	key := model.JobRunKey{WorkspaceID: p.WorkspaceID, CaseID: p.CaseID, JobID: p.JobID}

	logRec := &model.JobRunLog{
		WorkspaceID:    p.WorkspaceID,
		CaseID:         p.CaseID,
		JobID:          p.JobID,
		RunID:          p.RunID,
		TraceID:        p.TraceID,
		Stage:          model.JobRunStageRunning,
		StartedAt:      p.StartedAt,
		ExecutorKind:   p.ExecutorKind,
		EventType:      p.EventType,
		EventTriggerAt: p.StartedAt,
		SystemPrompt:   Truncate(p.SystemPrompt, model.MaxInlineBytes),
	}
	if err := p.Repo.JobRunLog().Create(ctx, logRec); err != nil {
		return nil, goerr.Wrap(err, "create mention run log",
			goerr.V("workspace_id", p.WorkspaceID),
			goerr.V("case_id", p.CaseID),
			goerr.V("run_id", p.RunID))
	}

	handler := NewHandler(
		p.Repo.JobRunEvent(),
		Routing{
			WorkspaceID: p.WorkspaceID,
			CaseID:      p.CaseID,
			JobID:       p.JobID,
			RunID:       p.RunID,
			TraceID:     p.TraceID,
		},
		NewSequencer(),
		clock,
	)

	return &Recorder{repo: p.Repo, key: key, log: logRec, handler: handler, clock: clock}, nil
}

// Handler returns the per-event trace handler. The caller wires it into the
// gollem agent (casebound) or planexec (threadcase) so LLM / tool calls stream
// into the JobRunEvent timeline.
func (r *Recorder) Handler() *Handler {
	if r == nil {
		return nil
	}
	return r.handler
}

// Finish transitions the run's JobRunLog to its terminal stage and materialises
// the JobRun summary doc so the run surfaces in ListByCase. execErr nil →
// SUCCESS; non-nil → FAILED (and a RUN_ERROR event is appended). Every
// persistence failure is non-fatal (errutil.Handle): the trace is
// observability, so a trace write failing must not fail the caller's turn.
//
// It is deliberately named Finish, not Close: this ends a run record, it does
// not close an io.Closer resource (safe.Close is for those).
func (r *Recorder) Finish(ctx context.Context, execErr error) {
	if r == nil {
		return
	}
	endedAt := r.clock()
	r.log.EndedAt = endedAt

	status := model.JobRunStatusSuccess
	if execErr != nil {
		status = model.JobRunStatusFailed
		r.log.Stage = model.JobRunStageFailed
		r.log.Error = Truncate(execErr.Error(), model.MaxInlineBytes)
		if emitErr := r.handler.EmitRunError(ctx, runErrorStage, execErr.Error()); emitErr != nil {
			errutil.Handle(ctx, emitErr, "runtrace: append run_error event")
		}
	} else {
		r.log.Stage = model.JobRunStageSuccess
	}

	if finErr := r.repo.JobRunLog().Finish(ctx, r.log); finErr != nil {
		errutil.Handle(ctx, finErr, "runtrace: finish run log")
	}
	if recErr := r.repo.JobRun().RecordRun(ctx, r.key, status, endedAt, r.log.RunID, r.log.TraceID, r.log.Error); recErr != nil {
		errutil.Handle(ctx, recErr, "runtrace: record run summary")
	}
}
