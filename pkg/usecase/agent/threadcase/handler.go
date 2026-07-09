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

// Handler is the host-side surface for one thread-mode turn. TraceAppend /
// TraceReplace render progress lines; Question posts a question to the thread;
// Create commits a new case for the ModeCreate flow.
type Handler interface {
	// TraceAppend records a milestone line that must stay visible (planner
	// rounds, task results) in the thread-side trace block.
	TraceAppend(ctx context.Context, line string)
	// TraceReplace overwrites the single transient activity line in place, so
	// per-tool chatter ("Searching…", "Fetching…") does not accumulate.
	TraceReplace(ctx context.Context, line string)
	// Question posts a question to the user. Returning an error aborts the turn.
	Question(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	// Create persists a new case for the ModeCreate flow and returns it. The host
	// invokes it once, AFTER the turn completes, with the field values the
	// in-loop finalizer already validated against the workspace schema. A returned
	// error is a persistence failure the model cannot repair by re-emitting JSON,
	// so it is NOT fed back for regeneration; the host surfaces it and falls back.
	// Only called in ModeCreate turns.
	Create(ctx context.Context, ssn *model.Session, p CreatePayload) (*model.Case, error)
}

// HandlerFuncs is a struct-of-funcs adapter for tests and minimal hosts.
// Missing entries are treated as no-ops (Create errors when unset, since a
// ModeCreate turn cannot commit without it).
type HandlerFuncs struct {
	TraceAppendFn  func(ctx context.Context, line string)
	TraceReplaceFn func(ctx context.Context, line string)
	QuestionFn     func(ctx context.Context, ssn *model.Session, q QuestionPayload) error
	CreateFn       func(ctx context.Context, ssn *model.Session, p CreatePayload) (*model.Case, error)
}

func (h HandlerFuncs) TraceAppend(ctx context.Context, line string) {
	if h.TraceAppendFn == nil {
		return
	}
	h.TraceAppendFn(ctx, line)
}

func (h HandlerFuncs) TraceReplace(ctx context.Context, line string) {
	if h.TraceReplaceFn == nil {
		return
	}
	h.TraceReplaceFn(ctx, line)
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
