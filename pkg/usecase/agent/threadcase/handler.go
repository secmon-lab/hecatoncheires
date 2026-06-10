// Package threadcase hosts the thread-mode agent: a plan-and-execute turn
// (planexec.Runner) that runs when a Case is created from a monitored channel
// post (materialize the Case fields) or when the bot is mentioned in a Case
// thread (investigate and respond / update fields / close). Slack SDK imports
// are forbidden here; the host communicates via the Handler interface and the
// returned Decision, exactly like casebound / proposal.
package threadcase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
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

// CreatePayload is handed to Handler.Create when the ModeCreate planner
// commits a new case. Fields are the already type-validated custom field
// values (Type injected, options/required checked by the runtime). The host
// owns the case identity (workspace / channel / thread / reporter) — those are
// captured when the host builds the Handler, not carried here.
type CreatePayload struct {
	Title       string
	Description string
	Fields      map[string]model.FieldValue
}

// Handler is the host-side surface for one thread-mode turn. Trace renders
// progress lines; Question posts a question to the thread; Create commits a
// new case for the ModeCreate flow.
type Handler interface {
	// Trace appends a progressive status line to the thread-side trace block.
	Trace(ctx context.Context, line string)
	// Question posts a question to the user. Returning an error aborts the turn.
	Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	// Create persists a new case for the ModeCreate flow and returns it. It is
	// invoked from inside the planner loop (planexec OnFinalize): returning an
	// error makes the runtime fold the failure back into the next planner
	// round so the planner can investigate / ask / re-emit. Only called in
	// ModeCreate turns.
	Create(ctx context.Context, ssn *model.Session, p CreatePayload) (*model.Case, error)
}

// HandlerFuncs is a struct-of-funcs adapter for tests and minimal hosts.
// Missing entries are treated as no-ops (Create errors when unset, since a
// ModeCreate turn cannot commit without it).
type HandlerFuncs struct {
	TraceFn    func(ctx context.Context, line string)
	QuestionFn func(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	CreateFn   func(ctx context.Context, ssn *model.Session, p CreatePayload) (*model.Case, error)
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

func (h HandlerFuncs) Create(ctx context.Context, ssn *model.Session, p CreatePayload) (*model.Case, error) {
	if h.CreateFn == nil {
		return nil, goerr.New("threadcase: Create handler not configured")
	}
	return h.CreateFn(ctx, ssn, p)
}

// Compile-time assertion that HandlerFuncs satisfies Handler.
var _ Handler = HandlerFuncs{}
