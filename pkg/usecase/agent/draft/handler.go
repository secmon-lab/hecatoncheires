package draft

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
	// while the prior turn ended on post_question.
	TriggerThreadReply
	// TriggerWSSwitch — the user switched the active workspace via the
	// preview UI, requiring a re-materialise on the existing draft.
	TriggerWSSwitch
)

// Handler is the host-side surface the draft runtime calls into for all
// Slack-side side effects. The runtime never touches the Slack service
// directly; the host implements this interface and renders the
// terminal action / busy / trace into Slack messages.
type Handler interface {
	// PostMessage renders the planner's terminal post_message text into a
	// thread reply.
	PostMessage(ctx context.Context, ssn *model.Session, text string) error
	// PostQuestion renders the planner's terminal question; options are
	// optional and render however the host prefers (buttons / list / plain).
	PostQuestion(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	// Materialize persists / refreshes the CaseDraft for ssn with the given
	// payload. The host validates against the workspace's FieldSchema.
	Materialize(ctx context.Context, ssn *model.Session, m MaterializePayload) error
	// Trace appends one progress line to the host's per-turn trace UI.
	Trace(ctx context.Context, line string)
	// PostBusy notifies the user that another turn is running on this
	// session and the new trigger is being dropped.
	PostBusy(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error
}

// QuestionPayload is the pure-domain shape passed to Handler.PostQuestion.
type QuestionPayload struct {
	Text    string
	Options []string
	Reason  string
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
	PostMessageFn  func(ctx context.Context, ssn *model.Session, text string) error
	PostQuestionFn func(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	MaterializeFn  func(ctx context.Context, ssn *model.Session, m MaterializePayload) error
	TraceFn        func(ctx context.Context, line string)
	PostBusyFn     func(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error
}

func (h HandlerFuncs) PostMessage(ctx context.Context, ssn *model.Session, text string) error {
	if h.PostMessageFn == nil {
		return nil
	}
	return h.PostMessageFn(ctx, ssn, text)
}

func (h HandlerFuncs) PostQuestion(ctx context.Context, ssn *model.Session, q QuestionPayload) error {
	if h.PostQuestionFn == nil {
		return nil
	}
	return h.PostQuestionFn(ctx, ssn, q)
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

func (h HandlerFuncs) PostBusy(ctx context.Context, ssn *model.Session, info agent.BusyInfo) error {
	if h.PostBusyFn == nil {
		return nil
	}
	return h.PostBusyFn(ctx, ssn, info)
}

// Compile-time assertion.
var _ Handler = HandlerFuncs{}
