package planexec

import "context"

// Sink is the host-side output channel for progress, task state, and
// free-form notifications during a planexec run. The planexec package
// never touches Slack / Firestore directly — it pushes events here, and
// the host (proposal / job) renders them onto its transport. Sink is
// defined inside planexec on purpose: it describes what THIS package
// wants to emit; consumers implement the interface in their own packages
// rather than re-defining a parallel interface elsewhere.
type Sink interface {
	// Notify is a free-form line aimed at the user. Examples:
	// planner.message at the top of a phase, "couldn't reach a
	// conclusion" on fallback, etc.
	Notify(ctx context.Context, line string)

	// PlanProposed fires once per planner round AFTER the JSON parses.
	// The host typically renders the round's reasoning / message into a
	// progress card that it can later update in place.
	PlanProposed(ctx context.Context, info PlanInfo)

	// PhaseStarted reserves a progress block per TaskPlan before the
	// sub-agents start. Tasks come in the order they will run; the host
	// renders one row each and addresses them via TaskProgress later.
	PhaseStarted(ctx context.Context, phase int, tasks []TaskInfo)

	// TaskProgress overwrites the line of a previously-registered task
	// (running / tool-call / final result snippet). The host MUST
	// no-op gracefully if taskID is unknown — that path fires when a
	// progress middleware races a phase boundary.
	TaskProgress(ctx context.Context, taskID, line string)

	// TaskFinished marks a single task as completed or failed. The host
	// updates the corresponding row to its terminal display.
	TaskFinished(ctx context.Context, result TaskResult)
}

// PlanInfo is the per-round summary emitted via Sink.PlanProposed.
// The fields are deliberately minimal: the host renders a status card,
// not a full plan replay.
type PlanInfo struct {
	// Round is 1-based.
	Round int
	// Reasoning is the planner's `message` field (1-2 sentence rationale).
	Reasoning string
	// IsReplan is true when this round was a replan (i.e. Round > 1).
	IsReplan bool
	// Direct is true when this round chose the direct (no-investigation)
	// fast path instead of proposing tasks. Round is 1 in this case and no
	// PhaseStarted / TaskProgress events follow.
	Direct bool
}

// TaskInfo describes one investigation task at the moment its trace block
// is being reserved. Title is the short, ID-free label the host renders.
type TaskInfo struct {
	ID    string
	Title string
}

// SinkFuncs is a struct-of-funcs adapter for tests and minimal hosts that
// only care about a subset of the Sink methods. Missing entries are
// treated as no-ops, so an all-zero SinkFuncs is safe to pass as Sink.
type SinkFuncs struct {
	NotifyFn       func(ctx context.Context, line string)
	PlanProposedFn func(ctx context.Context, info PlanInfo)
	PhaseStartedFn func(ctx context.Context, phase int, tasks []TaskInfo)
	TaskProgressFn func(ctx context.Context, taskID, line string)
	TaskFinishedFn func(ctx context.Context, result TaskResult)
}

func (s SinkFuncs) Notify(ctx context.Context, line string) {
	if s.NotifyFn == nil {
		return
	}
	s.NotifyFn(ctx, line)
}

func (s SinkFuncs) PlanProposed(ctx context.Context, info PlanInfo) {
	if s.PlanProposedFn == nil {
		return
	}
	s.PlanProposedFn(ctx, info)
}

func (s SinkFuncs) PhaseStarted(ctx context.Context, phase int, tasks []TaskInfo) {
	if s.PhaseStartedFn == nil {
		return
	}
	s.PhaseStartedFn(ctx, phase, tasks)
}

func (s SinkFuncs) TaskProgress(ctx context.Context, taskID, line string) {
	if s.TaskProgressFn == nil {
		return
	}
	s.TaskProgressFn(ctx, taskID, line)
}

func (s SinkFuncs) TaskFinished(ctx context.Context, result TaskResult) {
	if s.TaskFinishedFn == nil {
		return
	}
	s.TaskFinishedFn(ctx, result)
}

// Compile-time assertion that SinkFuncs satisfies Sink.
var _ Sink = SinkFuncs{}
