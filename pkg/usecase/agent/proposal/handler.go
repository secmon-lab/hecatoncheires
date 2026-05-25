package proposal

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// Trigger discriminates how the host invoked RunTurn for a given Session.
// The planner prompt may use it (e.g. WSSwitch should yield materialize
// without further investigation).
type Trigger int

const (
	// TriggerAppMention — the user @-mentioned the bot.
	TriggerAppMention Trigger = iota
	// TriggerThreadReply — the user replied in the thread without a mention,
	// while the prior turn ended on action=question.
	TriggerThreadReply
	// TriggerWSSwitch — the user switched the active workspace via the
	// preview UI, requiring a re-materialise on the existing proposal.
	TriggerWSSwitch
)

// Handler is the host-side surface the draft runtime calls into for all
// Slack-side side effects. The runtime never touches the Slack service
// directly; the host implements this interface and renders the
// terminal action / busy / trace into Slack messages.
//
// PostMessage was retired with the post_message planner action: when the
// planner needs user input, it always uses Question instead. Internal
// fallback (planner budget exhausted, internal errors) is signalled via
// Result.Status=StatusFallback so the host can render whatever copy fits.
type Handler interface {
	// Question renders the planner's terminal question payload. Items can
	// be 1-5 with each item carrying a select / multi-select control.
	Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	// Materialize persists / refreshes the CaseProposal for ssn with the given
	// payload. The host validates against the workspace's FieldSchema.
	Materialize(ctx context.Context, ssn *model.Session, m MaterializePayload) error
	// Trace appends one progress line to the host's per-turn trace UI.
	// Used for phase-level updates (planner round start, planner.message).
	// Per-task transitions go through TraceTask instead.
	Trace(ctx context.Context, line string)
	// TraceRound posts or updates a per-round phase line. The first call
	// for a given roundKey posts a fresh thread reply; subsequent calls
	// with the same key REPLACE the prior content of that round's
	// message in place (no append). This collapses the
	// "Planning… → retry → Planning… → action" sequence within one
	// planner round into a single self-updating message instead of a
	// stack of three Slack posts. A new roundKey opens a new message;
	// passing an empty roundKey or empty line is a no-op.
	TraceRound(ctx context.Context, roundKey, line string)
	// RegisterTasks reserves a stable trace block per investigation task
	// before the sub-agents start. The host renders one block per task in
	// the supplied order so subsequent TraceTask calls overwrite that
	// block in place. Block creation is deliberately the host's job —
	// sub-agents only update their line via TraceTask, never create new
	// blocks. Calling RegisterTasks with an empty slice is a no-op.
	RegisterTasks(ctx context.Context, tasks []TaskInfo)
	// TraceTask updates the line associated with a task that was previously
	// registered via RegisterTasks. Calling with an unknown taskID is a
	// no-op; calling before RegisterTasks is also a no-op (the host has no
	// block to address). The line replaces the prior content for that task
	// — there is no append.
	TraceTask(ctx context.Context, taskID, line string)
	// PostBusy notifies the user that another turn is running on this
	// session and the new trigger is being dropped.
	PostBusy(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error
}

// TaskInfo describes one investigation task at the moment its trace block
// is being reserved. Title is the short, ID-free label the host renders;
// it should already be human-readable (the planner is asked to keep it
// under ~40 characters in planner.md).
type TaskInfo struct {
	ID    string
	Title string
}

// QuestionPayload is the pure-domain shape passed to Handler.Question.
type QuestionPayload struct {
	// Reason explains the information gap (single rationale shared across
	// all items).
	Reason string
	// Items is the ordered list of questions to ask in this turn. Always
	// non-empty (validation guarantees ≥1).
	Items []QuestionItem
}

// QuestionItemType is the host-rendering hint for a question item.
type QuestionItemType string

const (
	// QuestionItemSelect is a single-choice picker.
	QuestionItemSelect QuestionItemType = "select"
	// QuestionItemMultiSelect is a multi-choice picker.
	QuestionItemMultiSelect QuestionItemType = "multi_select"
	// QuestionItemFreeText is a multiline plain-text input. Reserved
	// for the last-resort case where neither investigation nor a
	// closed-list classification can capture what we need from the
	// user. See prompts/planner.md for the policy.
	QuestionItemFreeText QuestionItemType = "free_text"
)

// QuestionItem is one question within QuestionPayload.Items.
type QuestionItem struct {
	// ID uniquely identifies the question within the payload; the host
	// uses it to correlate answers back when the user submits.
	ID string
	// Text is the prompt shown to the user.
	Text string
	// Type discriminates the answer control (select / multi_select /
	// free_text).
	Type QuestionItemType
	// Options is the allowed answer set for select / multi_select
	// (always ≥2 entries). Ignored for free_text.
	Options []string
}

// MaterializePayload is the pure-domain shape passed to Handler.Materialize.
type MaterializePayload struct {
	WorkspaceID       string
	Title             string
	Description       string
	CustomFieldValues map[string]any
}

// HandlerFuncs is a struct-of-funcs adapter for tests / one-off wiring,
// letting callers supply only the methods they care about. Missing entries
// are treated as no-ops (or returning nil for methods that return error).
type HandlerFuncs struct {
	QuestionFn      func(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	MaterializeFn   func(ctx context.Context, ssn *model.Session, m MaterializePayload) error
	TraceFn         func(ctx context.Context, line string)
	TraceRoundFn    func(ctx context.Context, roundKey, line string)
	RegisterTasksFn func(ctx context.Context, tasks []TaskInfo)
	TraceTaskFn     func(ctx context.Context, taskID, line string)
	PostBusyFn      func(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error
}

func (h HandlerFuncs) Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error {
	if h.QuestionFn == nil {
		return nil
	}
	return h.QuestionFn(ctx, ssn, q)
}

func (h HandlerFuncs) Materialize(ctx context.Context, ssn *model.Session, m MaterializePayload) error {
	if h.MaterializeFn == nil {
		return nil
	}
	return h.MaterializeFn(ctx, ssn, m)
}

func (h HandlerFuncs) Trace(ctx context.Context, line string) {
	if h.TraceFn == nil {
		return
	}
	h.TraceFn(ctx, line)
}

func (h HandlerFuncs) TraceRound(ctx context.Context, roundKey, line string) {
	if h.TraceRoundFn == nil {
		return
	}
	h.TraceRoundFn(ctx, roundKey, line)
}

func (h HandlerFuncs) RegisterTasks(ctx context.Context, tasks []TaskInfo) {
	if h.RegisterTasksFn == nil {
		return
	}
	h.RegisterTasksFn(ctx, tasks)
}

func (h HandlerFuncs) TraceTask(ctx context.Context, taskID, line string) {
	if h.TraceTaskFn == nil {
		return
	}
	h.TraceTaskFn(ctx, taskID, line)
}

func (h HandlerFuncs) PostBusy(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error {
	if h.PostBusyFn == nil {
		return nil
	}
	return h.PostBusyFn(ctx, ssn, info)
}

// Compile-time assertion.
var _ Handler = HandlerFuncs{}
