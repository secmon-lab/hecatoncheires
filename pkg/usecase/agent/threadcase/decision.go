package threadcase

import (
	"encoding/json"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
)

// DecisionKind discriminates the terminal action the planner chose for a
// thread-mode turn. It is the `kind` field of the final structured output.
type DecisionKind string

const (
	// DecisionRespond posts Message as a reply in the case thread.
	DecisionRespond DecisionKind = "respond"
	// DecisionMaterialize writes Title / Description / Fields onto the Case.
	DecisionMaterialize DecisionKind = "materialize"
	// DecisionClose transitions the Case board status to CloseStatus
	// (a closed status id) and, via lifecycle sync, closes the Case.
	DecisionClose DecisionKind = "close"
)

// DecisionField is one custom-field assignment emitted by a materialize
// decision. Value carries the scalar form (text / number / url / single
// select); Values carries the multi-select form. The host maps field_id to
// the workspace field schema to build the typed FieldValue.
type DecisionField struct {
	FieldID string   `json:"field_id"`
	Value   string   `json:"value,omitempty"`
	Values  []string `json:"values,omitempty"`
}

// Decision is the parsed final structured output of a thread-mode turn.
type Decision struct {
	Kind        DecisionKind    `json:"kind"`
	Message     string          `json:"message,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Fields      []DecisionField `json:"fields,omitempty"`
	CloseStatus string          `json:"close_status,omitempty"`
}

// decisionSchema is the gollem response schema for the final-response phase.
// The planner emits exactly one of the three kinds; the other fields are
// populated according to the chosen kind.
func decisionSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Terminal decision for this thread-mode turn.",
		Properties: map[string]*gollem.Parameter{
			"kind": {
				Type:        gollem.TypeString,
				Description: "The terminal action: respond (post a reply), materialize (fill the case fields), or close (mark the case done).",
				Enum: []string{
					string(DecisionRespond),
					string(DecisionMaterialize),
					string(DecisionClose),
				},
				Required: true,
			},
			"message": {
				Type:        gollem.TypeString,
				Description: "For respond: the reply text shown to the user. For close: a short closing note. Omit for materialize.",
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "For materialize: a concise case title summarising the thread.",
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "For materialize: a clear case description derived from the thread.",
			},
			"fields": {
				Type:        gollem.TypeArray,
				Description: "For materialize: custom field assignments. Only include fields you are confident about.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"field_id": {Type: gollem.TypeString, Description: "The field id from the workspace schema.", Required: true},
						"value":    {Type: gollem.TypeString, Description: "Scalar value (text / number / url / single select option id)."},
						"values":   {Type: gollem.TypeArray, Description: "Multi-select option ids.", Items: &gollem.Parameter{Type: gollem.TypeString}},
					},
				},
			},
			"close_status": {
				Type:        gollem.TypeString,
				Description: "For close: the closed status id to transition the case to.",
			},
		},
	}
}

// parseDecision unmarshals the final structured output into a Decision and
// validates the kind.
func parseDecision(raw []byte) (*Decision, error) {
	if len(raw) == 0 {
		return nil, goerr.New("empty decision payload")
	}
	var d Decision
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, goerr.Wrap(err, "decode thread-case decision", goerr.V("raw_len", len(raw)))
	}
	switch d.Kind {
	case DecisionRespond, DecisionMaterialize, DecisionClose:
		return &d, nil
	default:
		return nil, goerr.New("unknown thread-case decision kind", goerr.V("kind", string(d.Kind)))
	}
}
