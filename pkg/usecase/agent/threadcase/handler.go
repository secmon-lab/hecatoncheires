// Package threadcase hosts the thread-mode agent: a plan-and-execute turn
// (planexec.Runner) that runs when a Case is created from a monitored channel
// post (materialize the Case fields) or when the bot is mentioned in a Case
// thread (investigate and respond / update fields / close). Slack SDK imports
// are forbidden here; the host communicates via the Handler interface and the
// returned Decision, exactly like casebound / proposal.
package threadcase

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// ConversationMessage is a single pre-fetched thread message handed to the
// runtime. The host resolves user display names; the runtime only formats.
type ConversationMessage struct {
	Timestamp string
	UserID    string
	UserName  string
	Text      string
}

// QuestionItemType discriminates how the host renders a question's answer
// control. Mirrors planexec.QuestionItemType values.
type QuestionItemType string

const (
	QuestionItemSelect      QuestionItemType = "select"
	QuestionItemMultiSelect QuestionItemType = "multi_select"
	QuestionItemFreeText    QuestionItemType = "free_text"
)

// QuestionItem is one question within a QuestionPayload.
type QuestionItem struct {
	ID      string
	Text    string
	Type    QuestionItemType
	Options []string
}

// QuestionPayload is forwarded to the host when the planner needs human
// input. The host posts it to the Slack thread; the turn then ends and the
// user resumes by mentioning the bot again (the conversation history is keyed
// on Session.ID so the next turn continues seamlessly).
type QuestionPayload struct {
	Reason string
	Items  []QuestionItem
}

// Handler is the host-side surface for one thread-mode turn. Trace renders
// progress lines; Question posts a question to the thread.
type Handler interface {
	// Trace appends a progressive status line to the thread-side trace block.
	Trace(ctx context.Context, line string)
	// Question posts a question to the user. Returning an error aborts the turn.
	Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error
}

// HandlerFuncs is a struct-of-funcs adapter for tests and minimal hosts.
// Missing entries are treated as no-ops.
type HandlerFuncs struct {
	TraceFn    func(ctx context.Context, line string)
	QuestionFn func(ctx context.Context, ssn *model.Session, q QuestionPayload) error
}

func (h HandlerFuncs) Trace(ctx context.Context, line string) {
	if h.TraceFn == nil {
		return
	}
	h.TraceFn(ctx, line)
}

func (h HandlerFuncs) Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error {
	if h.QuestionFn == nil {
		return nil
	}
	return h.QuestionFn(ctx, ssn, q)
}

// Compile-time assertion that HandlerFuncs satisfies Handler.
var _ Handler = HandlerFuncs{}
