package wsagent

import (
	"context"
	"errors"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// wsAgentVersion is the strategy state version stamped on every Process this
// agent creates. Bump it only alongside a DecodeState that still reads the older
// shape — a running deployment always has in-flight Processes on the old one.
const wsAgentVersion = 1

// Host is the Slack-facing surface a finished turn needs.
//
// It exists because a durable turn no longer ends where it started: the run
// outlives StartTurn, so its reply is posted from the completion handler, on
// whichever instance committed the terminal transition.
type Host interface {
	// Reply posts the agent's answer to the thread.
	Reply(ctx context.Context, channelID, threadTS, text string) error
	// ReportFailure tells the user the turn could not finish. reason is the
	// technical cause; the host decides how much of it to show.
	ReportFailure(ctx context.Context, channelID, threadTS, reason string) error
}

// Durable is the workspace agent running on the agentkit runtime.
//
// It coexists with the in-process planexec runner: a deployment that has not
// wired this yet keeps taking RunTurn's synchronous path, which is what lets the
// two runtimes overlap while the hosts move over one at a time.
// It keeps no JobRunLog: those records are case-scoped and this agent is
// workspace-scoped (CaseID is 0), so there is no case whose run history it would
// appear in. Its trace is the per-claim Cloud Storage archive the claim
// middleware opens.
type Durable struct {
	host    Host
	locator agentkernel.Locator

	agent  agentkit.Agent[planexec.Input]
	kernel *agentkit.Kernel
	// probe filters the planner palette down to the toolset ids that resolve to a
	// tool for this run. nil leaves the palette unfiltered.
	probe *agentkernel.ToolSetProbe
}

// NewDurable builds the durable workspace-agent host. locator is used only to
// tell a re-delivered Slack event from a busy thread; a nil locator makes every
// delivery look fresh, which the idempotency key still covers.
func NewDurable(host Host, locator agentkernel.Locator) (*Durable, error) {
	if host == nil {
		return nil, goerr.New("host is required")
	}
	return &Durable{host: host, locator: locator}, nil
}

// Register registers the workspace agent and wires this host as its completion
// handler. Call it before building the Kernel, and Bind after.
//
// taskAgent is the agent each planned task runs as; progress may be nil.
func (d *Durable) Register(
	reg *agentkit.Registry, taskAgent agentkit.Agent[react.Input],
	progress planexec.Progress, limiter agentkit.Limiter, store agentkit.HistoryStore,
) error {
	if d == nil {
		return goerr.New("durable workspace agent is nil")
	}
	handle, err := planexec.Register(reg, agentkernel.AgentWorkspace, wsAgentVersion,
		taskAgent, progress, limiter,
		// The workspace agent answers in prose: its reply goes straight to a
		// Slack thread, so there is no structured object for a host to apply.
		planexec.Config[planexec.TextResult]{TextOnly: true},
		agentkit.WithHistoryStore[planexec.Output[planexec.TextResult]](store),
		agentkit.WithOnFinish(d.onFinish),
	)
	if err != nil {
		return goerr.Wrap(err, "register the workspace agent")
	}
	d.agent = handle
	return nil
}

// Bind hands over the Kernel the registered agent runs on, and the probe that
// tells this host which toolset ids actually resolve to a tool for a given run.
func (d *Durable) Bind(k *agentkit.Kernel, probe *agentkernel.ToolSetProbe) {
	if d != nil {
		d.kernel = k
		d.probe = probe
	}
}

// ready reports whether a turn can be spawned.
func (d *Durable) ready() bool {
	return d != nil && d.kernel != nil && d.agent.Name() != ""
}

// StartTurn spawns one workspace-agent turn and returns as soon as the run is
// recorded. The planner rounds, the sub-agents and the reply all happen
// afterwards on the agent worker.
func (d *Durable) StartTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if !d.ready() {
		return nil, goerr.New("durable workspace agent is not bound to an agent runtime")
	}
	if err := validateRequest(&req); err != nil {
		return nil, err
	}
	if req.MentionText == "" {
		return nil, goerr.New("MentionText is required (it is the planner's first user message)")
	}

	systemPrompt, err := buildSystemPrompt(req.Workspace)
	if err != nil {
		return nil, goerr.Wrap(err, "build workspace-agent system prompt",
			goerr.V("workspace_id", req.Workspace.Workspace.ID))
	}

	scope := agentkernel.Scope{
		WorkspaceID: req.Workspace.Workspace.ID,
		ChannelID:   req.Session.ChannelID,
		ThreadTS:    req.Session.ThreadTS,
		SessionID:   req.Session.ID,
		// The mentioning user is the access actor for the whole run. Without one
		// the usecase layer reads the run as a system context and BYPASSES
		// private-case access control, so every cross-case read would see cases
		// this person is not a member of.
		ActorUserID: req.ActorID,
		Lang:        string(i18n.LangFromContext(ctx)),
		ToolSets:    []string{agentkernel.ToolSetsAll},
	}
	if err := agentkernel.ValidateSpawn(agentkernel.AgentWorkspace, scope); err != nil {
		return nil, goerr.Wrap(err, "validate the workspace-agent turn scope")
	}

	// Asking the store first is what tells a re-delivery apart from a busy
	// thread: Spawn resolves an idempotency key silently, returning the existing
	// id without saying that it is existing.
	if existing := d.existingRun(ctx, req); existing != "" {
		return &Result{Status: StatusIdempotent}, nil
	}

	// The workspace agent spans every case, so there is no case id to give — but
	// the channel and thread it was mentioned in are what a sub-agent asked to
	// read the request's own conversation needs.
	taskContext, err := agent.TaskContext{
		WorkspaceID:    req.Workspace.Workspace.ID,
		SlackChannelID: req.Session.ChannelID,
		SlackThreadTS:  req.Session.ThreadTS,
	}.Render()
	if err != nil {
		return nil, err
	}

	knownToolIDs, err := d.probe.Available(ctx, scope, agent.KnownToolSetIDsWorkspaceChannel)
	if err != nil {
		return nil, goerr.Wrap(err, "resolve the workspace agent tool palette",
			goerr.V("session_id", req.Session.ID))
	}

	_, err = d.agent.Spawn(ctx, d.kernel, planexec.Input{
		SystemPrompt:  systemPrompt,
		UserInput:     req.MentionText,
		KnownToolIDs:  knownToolIDs,
		TaskContext:   taskContext,
		Progress:      planexec.ProgressTarget{ChannelID: req.Session.ChannelID, ThreadTS: req.Session.ThreadTS},
		AllowDirect:   true,
		AllowQuestion: false,
		// Case mutations happen as sub-agent tool calls inside the loop, gated by
		// the safety-rule prompt. planexec itself performs no side effects.
		AllowSubAgentWrites: true,
	},
		agentkit.WithSubject(agentkernel.ThreadSubject(req.Session.ID)),
		agentkit.WithIdempotencyKey(agentkernel.TriggerKey(
			req.Session.ChannelID, req.Session.ThreadTS, req.TriggerTS)),
		agentkit.WithMetadata(scope.Metadata()),
	)
	switch {
	case errors.Is(err, agentkit.ErrSubjectBusy):
		return &Result{Status: StatusBusy, BusyOwner: d.busyDescription(ctx, req.Session.ID)}, nil
	case err != nil:
		return nil, goerr.Wrap(err, "spawn the workspace agent",
			goerr.V("session_id", req.Session.ID))
	}
	return &Result{Status: StatusStarted}, nil
}

// existingRun returns the run a previous delivery of this same Slack event
// already started, or "" when this is the first delivery.
func (d *Durable) existingRun(ctx context.Context, req TurnRequest) agentkit.ProcessID {
	if d.locator == nil {
		return ""
	}
	key := agentkernel.TriggerKey(req.Session.ChannelID, req.Session.ThreadTS, req.TriggerTS)
	pid, err := d.locator.ByTrigger(ctx, key)
	if err != nil {
		// Not knowing means treating it as a fresh delivery: dropping a real
		// mention is worse than the duplicate a wrong guess would cause, and the
		// idempotency key still stops a second run from being created.
		errutil.Handle(ctx, goerr.Wrap(err, "look up the run for this trigger"),
			"look up the run for this trigger")
		return ""
	}
	return pid
}

// busyDescription names the run holding the thread, for the "already working on
// this" message.
func (d *Durable) busyDescription(ctx context.Context, sessionID string) string {
	if d.locator == nil {
		return ""
	}
	busy, err := d.locator.Busy(ctx, agentkernel.ThreadSubject(sessionID))
	if err != nil {
		errutil.Handle(ctx, err, "read the run holding this thread")
		return ""
	}
	if busy == nil {
		return ""
	}
	return string(busy.ProcessID)
}

// onFinish posts the turn's answer. agentkit calls it once, after the terminal
// transition committed, on whichever instance committed it.
func (d *Durable) onFinish(ctx context.Context, pid agentkit.ProcessID,
	res agentkit.FinishResult[planexec.Output[planexec.TextResult]],
) error {
	proc, err := d.kernel.GetProcess(ctx, pid)
	if err != nil {
		return goerr.Wrap(err, "read the finished run", goerr.V("process", pid))
	}
	sc := agentkernel.ScopeFrom(proc.Metadata)

	switch {
	case res.Status == agentkit.ProcessSucceeded && res.Output != nil:
		out := *res.Output
		switch out.Kind {
		case planexec.OutputFinal, planexec.OutputDirect:
			if perr := d.host.Reply(ctx, sc.ChannelID, sc.ThreadTS, out.Text); perr != nil {
				errutil.Handle(ctx, goerr.Wrap(perr, "post the workspace-agent reply"),
					"post the workspace-agent reply")
			}
		default:
			// A fallback (or a question, which this host does not enable) has no
			// answer to post; the user still needs to be told the turn ended.
			reason := out.FallbackReason
			if reason == "" {
				reason = "the turn ended without a conclusion"
			}
			if perr := d.host.ReportFailure(ctx, sc.ChannelID, sc.ThreadTS, reason); perr != nil {
				errutil.Handle(ctx, perr, "report the workspace-agent fallback")
			}
		}
	case res.Status == agentkit.ProcessFailed:
		reason := "the run failed"
		if res.Failure != nil && res.Failure.Message != "" {
			reason = res.Failure.Message
		}
		if perr := d.host.ReportFailure(ctx, sc.ChannelID, sc.ThreadTS, reason); perr != nil {
			errutil.Handle(ctx, perr, "report the workspace-agent failure")
		}
	}
	return nil
}
