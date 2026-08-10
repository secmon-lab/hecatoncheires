package job

import (
	"context"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	agentjob "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// jobSimpleAgentVersion is the strategy state version stamped on every Process
// the simple-strategy Job agent creates. Bump it only alongside a DecodeState
// that still reads the older shape — a running deployment always has in-flight
// Processes on the old one.
const jobSimpleAgentVersion = 1

// DurableRuntime is the agent runtime a Job run is Spawned onto, plus the pieces
// a finished run needs to close itself out.
//
// It is a struct rather than a bag of fields on RunnerDeps because all of it is
// required together: a Kernel with no history store cannot reflect, and an agent
// handle with no Kernel cannot spawn.
type DurableRuntime struct {
	// Kernel is the runtime the agents run on. Filled by Bind after the Kernel is
	// built, because registration has to happen first.
	Kernel *agentkit.Kernel
	// History is where each Process's conversation is stored. The reflection pass
	// reads a finished run's transcript from here, by the ref recorded on the
	// Process — a durable run's history is not in the by-run-id store the
	// in-process executor used.
	History agentkit.HistoryStore

	simple agentkit.Agent[react.Input]
	runner *JobRunner
}

// Register registers the Job agents. Call it before building the Kernel, then
// Bind and AttachRunner afterwards.
//
// The three steps are separate because their dependencies form a cycle if they
// are not: building the Kernel needs the registry registration fills, and the
// JobRunner needs this runtime handed to it at construction. Registration itself
// needs neither, so it goes first and the other two are wired in after.
func (d *DurableRuntime) Register(reg *agentkit.Registry, limiter agentkit.Limiter) error {
	if d == nil {
		return goerr.New("durable runtime is nil")
	}
	if d.History == nil {
		return goerr.New("history store is required")
	}
	handle, err := react.Register(reg, agentkernel.AgentJobSimple, jobSimpleAgentVersion, limiter,
		agentkit.WithHistoryStore[react.Output](d.History),
		agentkit.WithOnFinish(d.onSimpleFinish),
	)
	if err != nil {
		return goerr.Wrap(err, "register the simple-strategy job agent")
	}
	d.simple = handle
	return nil
}

// Bind hands over the Kernel the registered agents run on. Until it is called
// agentFor reports no agent, so a Job started before the runtime is ready takes
// the in-process path rather than failing.
func (d *DurableRuntime) Bind(k *agentkit.Kernel) {
	if d != nil {
		d.Kernel = k
	}
}

// AttachRunner hands over the runner a finished run closes itself out through.
// It is separate from Register because the runner is built with this runtime as
// one of its dependencies; nothing can spawn before it exists, so a completion
// handler cannot fire in between.
func (d *DurableRuntime) AttachRunner(r *JobRunner) {
	if d != nil {
		d.runner = r
	}
}

// agentFor returns the agent that drives strategy, and whether there is one.
// A strategy with no agent stays on the in-process executor.
func (d *DurableRuntime) agentFor(strategy model.JobStrategy) (agentkit.Agent[react.Input], bool) {
	if d == nil || d.Kernel == nil {
		return agentkit.Agent[react.Input]{}, false
	}
	if strategy == model.JobStrategySimple && d.simple.Name() != "" {
		return d.simple, true
	}
	return agentkit.Agent[react.Input]{}, false
}

// spawnParams is what starting a durable Job run needs beyond the prompts.
//
// It carries no Slack coordinates. The completion handler reloads the case and
// posts from that, because a run can finish hours later on another instance and
// the channel or thread it should report into may have changed in between —
// snapshotting them here would post into wherever they pointed at spawn.
type spawnParams struct {
	job          *model.Job
	event        Event
	key          model.JobRunKey
	runID        string
	systemPrompt string
	userPrompt   string
}

// spawn starts one durable Job run and returns as soon as it is recorded.
//
// The subject is the (workspace, case, job) triple, which is what now serialises
// two runs of the same Job on the same Case. The lease the caller holds is
// released when Run returns, so the subject — not the lease — is the durable
// guard from here on.
func (d *DurableRuntime) spawn(ctx context.Context, agent agentkit.Agent[react.Input], p spawnParams) (agentkit.ProcessID, error) {
	scope := agentkernel.Scope{
		WorkspaceID: p.key.WorkspaceID,
		CaseID:      p.key.CaseID,
		ToolSets:    []string{agentkernel.ToolSetsAll},
		JobID:       p.key.JobID,
		JobRunID:    p.runID,
		EventType:   string(p.event.Domain),
		// Only the scheduled domain is rate-limited. One tick can make hundreds
		// of (job, case) pairs due at once and a skipped scheduled run costs
		// nothing, whereas a lifecycle event, a manual Run or an interactive
		// resume is a single user-visible action with no such retry.
		SlotGated: p.event.Domain == model.JobEventDomainScheduled,
	}
	if err := agentkernel.ValidateSpawn(agentkernel.AgentJobSimple, scope); err != nil {
		return "", goerr.Wrap(err, "validate the job run scope", goerr.V("job_id", p.key.JobID))
	}

	pid, err := agent.Spawn(ctx, d.Kernel,
		react.Input{SystemPrompt: p.systemPrompt, Prompt: p.userPrompt},
		agentkit.WithSubject(agentkernel.JobRunSubject(p.key.WorkspaceID, p.key.CaseID, p.key.JobID)),
		agentkit.WithMetadata(scope.Metadata()),
	)
	if err != nil {
		return "", goerr.Wrap(err, "spawn the job agent",
			goerr.V("job_id", p.key.JobID), goerr.V("run_id", p.runID))
	}
	return pid, nil
}

// onSimpleFinish closes out a finished simple-strategy run: the run log, the
// Slack completion marker, and the optional reflection pass.
//
// It reloads everything it needs from the Process scope rather than capturing it
// at spawn, because a durable run finishes on whichever instance committed its
// terminal transition — which need not be the one that started it.
func (d *DurableRuntime) onSimpleFinish(ctx context.Context, pid agentkit.ProcessID, res agentkit.FinishResult[react.Output]) error {
	r := d.runner
	if r == nil {
		return goerr.New("durable runtime has no job runner")
	}
	proc, err := d.Kernel.GetProcess(ctx, pid)
	if err != nil {
		return goerr.Wrap(err, "read the finished job run", goerr.V("process", pid))
	}
	sc := agentkernel.ScopeFrom(proc.Metadata)
	key := model.JobRunKey{WorkspaceID: sc.WorkspaceID, CaseID: sc.CaseID, JobID: sc.JobID}

	var runErr error
	switch res.Status {
	case agentkit.ProcessSucceeded:
	case agentkit.ProcessFailed:
		runErr = failureError(res.Failure)
	default:
		runErr = goerr.New("job run did not complete", goerr.V("status", string(proc.Status)))
	}

	// The run record first: it is what the case agent page lists, so it must be
	// closed even if the Slack marker or the reflection below fails.
	runtrace.FinishRun(ctx, r.deps.Repo, key, sc.JobRunID, runtrace.Usage{
		InputTokens:              proc.Metrics.InputTokens,
		OutputTokens:             proc.Metrics.OutputTokens,
		CacheCreationInputTokens: proc.Metrics.CacheCreationInputTokens,
		CacheReadInputTokens:     proc.Metrics.CacheReadInputTokens,
		LLMCalls:                 proc.Metrics.LLMCalls,
		ToolCalls:                proc.Metrics.ToolCalls,
	}, runErr, r.clock())

	j, c := d.reloadRunContext(ctx, sc)
	d.postCompletionMarker(ctx, sc, j, runErr)
	if runErr == nil {
		d.reflect(ctx, proc, sc, j, c)
	}
	return nil
}

// reloadRunContext fetches the Job definition and the Case a finished run was
// about. Either may come back nil: the workspace could have been reconfigured
// while the run was in flight, and that must not stop the run being closed out.
func (d *DurableRuntime) reloadRunContext(ctx context.Context, sc agentkernel.Scope) (*model.Job, *model.Case) {
	r := d.runner
	entry, err := r.deps.Registry.Get(sc.WorkspaceID)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "load the workspace of a finished job run",
			goerr.V("workspace_id", sc.WorkspaceID), goerr.T(errutil.TagBenign)),
			"job: load workspace after run")
		return nil, nil
	}
	var j *model.Job
	for _, candidate := range entry.Jobs {
		if candidate != nil && candidate.ID == sc.JobID {
			j = candidate
			break
		}
	}
	c, err := r.deps.Repo.Case().Get(ctx, sc.WorkspaceID, sc.CaseID)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "load the case of a finished job run",
			goerr.V("case_id", sc.CaseID)), "job: load case after run")
		return j, nil
	}
	return j, c
}

// postCompletionMarker posts the run's terminal operational-log line.
func (d *DurableRuntime) postCompletionMarker(ctx context.Context, sc agentkernel.Scope, j *model.Job, runErr error) {
	if j == nil {
		return
	}
	r := d.runner
	// The marker follows the Job's quiet flag, exactly as the in-process path
	// does; withQuiet is what every post site self-gates on.
	ctx = withQuiet(ctx, j.Quiet)
	text := i18n.T(ctx, i18n.MsgJobRunCompleted, j.ID)
	if runErr != nil {
		text = i18n.T(ctx, i18n.MsgJobRunFailed, j.ID,
			runtrace.Truncate(runErr.Error(), model.MaxInlineBytes))
	}
	r.postSessionLog(ctx, sc.ChannelID, sc.ThreadTS, text)
}

// reflect runs the optional post-run reflection pass over the run's transcript.
//
// The transcript comes from the Process's own history version, not from the
// by-run-id history repository the in-process executor wrote: a durable run's
// conversation is stored per Process, and the ref naming its final version is on
// the Process row.
func (d *DurableRuntime) reflect(ctx context.Context, proc *agentkit.Process, sc agentkernel.Scope, j *model.Job, c *model.Case) {
	r := d.runner
	if j == nil || !j.Reflection || r.deps.Reflector == nil {
		return
	}
	if c == nil || c.IsPrivate {
		// A private case's contents must not reach shared workspace knowledge.
		return
	}
	if proc.HistoryRef == "" {
		// A run that committed no conversation has nothing to reflect on.
		return
	}
	history, err := d.History.Load(ctx, proc.ID, proc.HistoryRef)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "load a finished job run's history for reflection",
			goerr.V("process", proc.ID), goerr.V("job_id", sc.JobID)),
			"job: load reflection history")
		return
	}
	if history == nil {
		return
	}
	if err := r.deps.Reflector.Reflect(ctx, agentjob.ReflectRequest{
		WorkspaceID:    sc.WorkspaceID,
		CaseID:         sc.CaseID,
		JobID:          j.ID,
		JobName:        j.Name,
		JobDescription: j.Description,
		History:        history,
	}); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "job reflection",
			goerr.V("job_id", j.ID), goerr.V("case_id", sc.CaseID)), "job: reflection")
	}
}

// failureError turns a recorded Failure into an error the run log can carry.
func failureError(f *agentkit.Failure) error {
	if f == nil {
		return goerr.New("job run failed")
	}
	return goerr.New(f.Message, goerr.V("code", string(f.Code)))
}
