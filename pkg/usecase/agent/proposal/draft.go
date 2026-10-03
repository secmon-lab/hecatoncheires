package proposal

import (
	"fmt"
	"strings"

	"github.com/m-mizutani/goerr/v2"

	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
)

// Draft is the terminal output of a case-draft turn: the case the agent proposes.
// The schema handed to the model is derived from these struct tags via
// gollem.ToSchema; Validate enforces what a plain JSON schema cannot.
//
// It is a proposal, not a case: the host renders it into a preview a human
// reviews, edits and submits.
type Draft struct {
	WorkspaceID string `json:"workspace_id" description:"The id of the workspace this case belongs to, taken from the registered list." required:"true"`
	Title       string `json:"title" description:"A concise case title, about 80 characters or fewer. A noun phrase that fits one line." required:"true"`
	Description string `json:"description" description:"A clear case description, never more than 2000 characters. Summarise; do not paste raw logs or whole transcripts." required:"true"`
	// Fields is a list rather than a map keyed by field id: Claude structured
	// outputs and OpenAI strict mode cannot express an object with free keys, so a
	// map here makes every terminal call fail before it is sent.
	//
	// A required field the agent could not determine is left out on purpose — the
	// review UI blocks submit until the human fills it, which is better than a
	// fabricated value.
	Fields []draftField `json:"fields,omitempty" description:"Custom field values, one entry per field id from get_workspace. Omit a field you cannot determine rather than guessing."`
	IsTest bool         `json:"is_test,omitempty" description:"True only when the case exists to verify the system itself or as a drill, never for a real case."`
}

// draftField is one custom-field value the draft proposes. Value carries the
// scalar form and Values the multi-value form; the host coerces both to the
// field's type through model.CoerceFieldInputs.
type draftField struct {
	FieldID string   `json:"field_id" description:"A field id from get_workspace." required:"true"`
	Value   string   `json:"value,omitempty" description:"Scalar value: text, number, URL, RFC3339 date, a single select option id, or a user id."`
	Values  []string `json:"values,omitempty" description:"Multi-value: multi-select option ids or user ids."`
}

// Validate enforces the draft's shape invariants so a workspace-less or
// title-less proposal is rejected inside planexec's regeneration loop rather than
// reaching the human as a broken preview. It satisfies planexec.Validatable.
//
// The field VALUES are not checked against the workspace's schema, here or in
// the host's finalizer: the host coerces each to its field's type when it stores
// the preview and drops what it cannot place, and the human fixes the rest in
// the review modal.
func (d Draft) Validate() error {
	if strings.TrimSpace(d.WorkspaceID) == "" {
		return goerr.New("the draft must name the workspace it belongs to")
	}
	if strings.TrimSpace(d.Title) == "" {
		return goerr.New("the draft requires a non-empty title")
	}
	if strings.TrimSpace(d.Description) == "" {
		return goerr.New("the draft requires a non-empty description")
	}
	seen := make(map[string]struct{}, len(d.Fields))
	for i, f := range d.Fields {
		id := strings.TrimSpace(f.FieldID)
		if id == "" {
			return goerr.New(fmt.Sprintf("draft field %d has no field_id", i), goerr.V("index", i))
		}
		// The host keys values by field id, so a second entry would silently
		// overwrite the first.
		if _, dup := seen[id]; dup {
			return goerr.New(fmt.Sprintf("draft field %q is given more than once", id), goerr.V("field_id", id))
		}
		seen[id] = struct{}{}
	}
	return nil
}

// fieldInputs converts the draft's fields to the shape model.CoerceFieldInputs
// takes, in the order the draft gives them.
func (d Draft) fieldInputs() []model.FieldInput {
	out := make([]model.FieldInput, 0, len(d.Fields))
	for _, f := range d.Fields {
		out = append(out, model.FieldInput{FieldID: f.FieldID, Value: f.Value, Values: f.Values})
	}
	return out
}
