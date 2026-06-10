package threadcase

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

// CreateDecision is the structured final output of a ModeCreate turn: the
// title / description / custom fields the planner wants the new case to carry.
// It reuses DecisionField (shared with the materialize decision).
type CreateDecision struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Fields      []DecisionField `json:"fields,omitempty"`
}

// createDecisionSchema is the gollem response schema for the ModeCreate final
// phase. The planner emits a concrete case to create; the runtime validates it
// (required / options / types) and commits it via Handler.Create, folding any
// failure back into the loop.
func createDecisionSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "The case to create from this thread, once you are confident enough to create it.",
		Properties: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "A concise case title summarising the thread.",
				Required:    true,
			},
			"description": {
				Type:        gollem.TypeString,
				Description: "A clear case description derived from the thread and your investigation.",
				Required:    true,
			},
			"fields": {
				Type:        gollem.TypeArray,
				Description: "Custom field assignments. You MUST satisfy every required field and only use allowed option ids.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"field_id": {Type: gollem.TypeString, Description: "The field id from the workspace schema.", Required: true},
						"value":    {Type: gollem.TypeString, Description: "Scalar value (text / number / url / single select option id)."},
						"values":   {Type: gollem.TypeArray, Description: "Multi-select option ids.", Items: &gollem.Parameter{Type: gollem.TypeString}},
					},
				},
			},
		},
	}
}

// parseCreateDecision unmarshals the ModeCreate final structured output.
func parseCreateDecision(raw []byte) (*CreateDecision, error) {
	if len(raw) == 0 {
		return nil, goerr.New("empty create decision payload")
	}
	var d CreateDecision
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, goerr.Wrap(err, "decode thread-case create decision", goerr.V("raw_len", len(raw)))
	}
	return &d, nil
}

// validateCreateDecision turns a CreateDecision into typed, fully-validated
// field values. It does NOT fail fast: title emptiness, per-field coercion
// problems, and the schema validation (required / options / types) are all
// accumulated and returned as one error so the planner can fix everything in a
// single re-emit. On success it returns the enriched field-value map.
func validateCreateDecision(ws *model.WorkspaceEntry, d *CreateDecision) (map[string]model.FieldValue, error) {
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
// must surface them so the planner can correct the value.
func coerceCreateFields(ws *model.WorkspaceEntry, fields []DecisionField) (map[string]model.FieldValue, []string) {
	out := make(map[string]model.FieldValue, len(fields))
	if len(fields) == 0 {
		return out, nil
	}
	typeByID := make(map[string]types.FieldType)
	if ws != nil && ws.FieldSchema != nil {
		for _, f := range ws.FieldSchema.Fields {
			typeByID[f.ID] = types.FieldType(f.Type)
		}
	}

	var violations []string
	for _, df := range fields {
		ft, known := typeByID[df.FieldID]
		if !known {
			// Leave it in the map so ValidateCaseFieldsAll reports the
			// "not defined in the workspace schema" violation with a
			// consistent message.
			out[df.FieldID] = model.FieldValue{FieldID: types.FieldID(df.FieldID), Value: firstNonEmpty(df.Value, df.Values)}
			continue
		}
		var val any
		switch ft {
		case types.FieldTypeMultiSelect, types.FieldTypeMultiUser:
			switch {
			case len(df.Values) > 0:
				val = df.Values
			case df.Value != "":
				val = []string{df.Value}
			default:
				val = []string{}
			}
		case types.FieldTypeNumber:
			n, err := strconv.ParseFloat(strings.TrimSpace(df.Value), 64)
			if err != nil {
				violations = append(violations, "field "+strconv.Quote(df.FieldID)+": value must be a number, got "+strconv.Quote(df.Value))
				continue
			}
			val = n
		default:
			val = df.Value
		}
		out[df.FieldID] = model.FieldValue{FieldID: types.FieldID(df.FieldID), Type: ft, Value: val}
	}
	return out, violations
}

// firstNonEmpty returns the scalar when present, else the slice — used only to
// preserve an unknown field's value for the validator's error message.
func firstNonEmpty(scalar string, slice []string) any {
	if scalar != "" {
		return scalar
	}
	if len(slice) > 0 {
		return slice
	}
	return ""
}
