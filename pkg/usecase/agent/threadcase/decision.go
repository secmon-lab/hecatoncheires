package threadcase

import (
	"strings"

	"github.com/m-mizutani/goerr/v2"
)

// DecisionKind discriminates the terminal action a mention turn resolves to.
// It is the `kind` field of the structured final output (Run[Decision]).
//
// Closing / transitioning the case is NOT a Decision kind: that is a side
// effect the sub-agent performs during investigation via the
// case__update_case_status tool. The terminal Decision only covers the two
// host-applied outcomes — reply to the user, or materialize case content.
type DecisionKind string

const (
	// DecisionRespond posts Message as a reply in the case thread.
	DecisionRespond DecisionKind = "respond"
	// DecisionMaterialize writes Title / Description / Fields onto the Case
	// (the host applies it via CaseUC.MaterializeThreadCase).
	DecisionMaterialize DecisionKind = "materialize"
)

// DecisionField is one custom-field assignment emitted by a materialize
// decision. Value carries the scalar form (text / number / url / single
// select); Values carries the multi-select form. The host maps field_id to
// the workspace field schema to build the typed FieldValue.
type DecisionField struct {
	FieldID string   `json:"field_id" description:"The field id from the workspace schema." required:"true"`
	Value   string   `json:"value,omitempty" description:"Scalar value (text / number / url / single select option id)."`
	Values  []string `json:"values,omitempty" description:"Multi-select option ids."`
}

// Decision is the structured final output of a mention turn (Run[Decision]).
// The schema handed to the planner is derived from these struct tags via
// gollem.ToSchema; Validate enforces the per-kind invariants the schema cannot
// (a plain JSON schema cannot say "materialize requires title + description").
type Decision struct {
	Kind        DecisionKind    `json:"kind" description:"The terminal action: respond (post a reply) or materialize (fill the case title/description/fields). To close or change the case status, do NOT use a decision — call the case__update_case_status tool during investigation instead." enum:"respond,materialize" required:"true"`
	Message     string          `json:"message,omitempty" description:"For respond: the reply text shown to the user. Omit for materialize."`
	Title       string          `json:"title,omitempty" description:"For materialize: a concise case title summarising the thread."`
	Description string          `json:"description,omitempty" description:"For materialize: a clear case description derived from the thread."`
	Fields      []DecisionField `json:"fields,omitempty" description:"For materialize: custom field assignments. Only include fields you are confident about."`
}

// Validate enforces the mention decision's per-kind invariants so a malformed
// terminal output is rejected inside planexec's Run[Decision] regeneration loop
// rather than producing an empty reply or a blank materialize. It satisfies
// planexec.Validatable.
func (d Decision) Validate() error {
	switch d.Kind {
	case DecisionRespond:
		if strings.TrimSpace(d.Message) == "" {
			return goerr.New("respond decision requires a non-empty message")
		}
	case DecisionMaterialize:
		if strings.TrimSpace(d.Title) == "" {
			return goerr.New("materialize decision requires a non-empty title")
		}
		if strings.TrimSpace(d.Description) == "" {
			return goerr.New("materialize decision requires a non-empty description")
		}
	default:
		return goerr.New("unknown thread-case decision kind", goerr.V("kind", string(d.Kind)))
	}
	return nil
}
