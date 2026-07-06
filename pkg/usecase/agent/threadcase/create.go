package threadcase

import (
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// CreateDecision is the structured final output of a ModeCreate turn
// (Run[CreateDecision]): the title / description / custom fields the planner
// wants the new case to carry. It reuses DecisionField (shared with the
// materialize decision). The planner schema is derived from these struct tags
// via gollem.ToSchema; Validate enforces the shape invariants. Workspace-schema
// field validation (required / options / types) is applied by the host when it
// commits the case (validateCreateDecision), not here — the value type has no
// access to the field schema.
type CreateDecision struct {
	Title       string          `json:"title" description:"A concise case title summarising the thread." required:"true"`
	Description string          `json:"description" description:"A clear case description derived from the thread and your investigation." required:"true"`
	Fields      []DecisionField `json:"fields,omitempty" description:"Custom field assignments. You MUST satisfy every required field and only use allowed option ids."`
}

// Validate enforces the create decision's shape invariants so a title-less or
// description-less proposal is rejected inside planexec's Run[CreateDecision]
// regeneration loop. It satisfies planexec.Validatable. Field-value validity is
// checked by the host against the workspace schema (validateCreateDecision).
func (d CreateDecision) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return goerr.New("create decision requires a non-empty title")
	}
	if strings.TrimSpace(d.Description) == "" {
		return goerr.New("create decision requires a non-empty description")
	}
	return nil
}

// validateCreateDecision turns a CreateDecision into typed, fully-validated
// field values. It does NOT fail fast: title emptiness, per-field coercion
// problems, and the schema validation (required / options / types) are all
// accumulated and returned as one error so the planner can fix everything in a
// single re-emit. On success it returns the enriched field-value map.
func validateCreateDecision(ws *model.WorkspaceEntry, d *CreateDecision) (map[string]model.FieldValue, error) {
	if d == nil {
		return nil, goerr.New("create decision is nil")
	}
	var violations []string

	if strings.TrimSpace(d.Title) == "" {
		violations = append(violations, "title: must not be empty")
	}

	raw, coerceViolations := coerceCreateFields(ws, d.Fields)
	violations = append(violations, coerceViolations...)

	var validated map[string]model.FieldValue
	if ws != nil && ws.FieldSchema != nil {
		v := model.NewFieldValidator(ws.FieldSchema)
		out, err := v.ValidateCaseFieldsAll(raw)
		if err != nil {
			// ValidateCaseFieldsAll already aggregates; surface its message
			// lines alongside our own coercion / title violations.
			violations = append(violations, err.Error())
		} else {
			validated = out
		}
	} else {
		validated = raw
	}

	if len(violations) > 0 {
		return nil, goerr.New("the case you proposed cannot be created yet:\n- " + strings.Join(violations, "\n- "))
	}
	return validated, nil
}

// coerceCreateFields maps the planner's DecisionField list into raw
// model.FieldValue entries keyed by field id, coercing multi-select and number
// shapes. Type-resolution problems (unknown field id, unparseable number) are
// returned as violation strings rather than silently dropped — the create path
// must surface them so the planner can correct the value. It delegates to the
// shared model.CoerceFieldInputs so the create path and the casewriter tool
// coerce identically.
func coerceCreateFields(ws *model.WorkspaceEntry, fields []DecisionField) (map[string]model.FieldValue, []string) {
	var schema *config.FieldSchema
	if ws != nil {
		schema = ws.FieldSchema
	}
	inputs := make([]model.FieldInput, 0, len(fields))
	for _, df := range fields {
		inputs = append(inputs, model.FieldInput{FieldID: df.FieldID, Value: df.Value, Values: df.Values})
	}
	return model.CoerceFieldInputs(schema, inputs)
}
