package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestFieldValidator_ValidateCaseFields(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "category",
				Name:     "Category",
				Type:     types.FieldTypeMultiSelect,
				Required: true,
				Options: []config.FieldOption{
					{ID: "data-breach", Name: "Data Breach"},
					{ID: "system-failure", Name: "System Failure"},
				},
			},
			{
				ID:       "likelihood",
				Name:     "Likelihood",
				Type:     types.FieldTypeSelect,
				Required: true,
				Options: []config.FieldOption{
					{ID: "low", Name: "Low"},
					{ID: "high", Name: "High"},
				},
			},
			{
				ID:       "description",
				Name:     "Description",
				Type:     types.FieldTypeText,
				Required: false,
			},
			{
				ID:       "score",
				Name:     "Score",
				Type:     types.FieldTypeNumber,
				Required: false,
			},
			{
				ID:       "assignee",
				Name:     "Assignee",
				Type:     types.FieldTypeUser,
				Required: false,
			},
			{
				ID:       "responders",
				Name:     "Responders",
				Type:     types.FieldTypeMultiUser,
				Required: false,
			},
			{
				ID:       "due-date",
				Name:     "Due Date",
				Type:     types.FieldTypeDate,
				Required: false,
			},
			{
				ID:       "reference-url",
				Name:     "Reference URL",
				Type:     types.FieldTypeURL,
				Required: false,
			},
		},
	}

	tests := []struct {
		name        string
		fieldValues map[string]model.FieldValue
		wantErr     error
	}{
		{
			name: "valid all field types",
			fieldValues: map[string]model.FieldValue{
				"category":      {FieldID: "category", Value: []string{"data-breach", "system-failure"}},
				"likelihood":    {FieldID: "likelihood", Value: "high"},
				"description":   {FieldID: "description", Value: "Test description"},
				"score":         {FieldID: "score", Value: float64(85)},
				"assignee":      {FieldID: "assignee", Value: "U123456"},
				"responders":    {FieldID: "responders", Value: []string{"U123456", "U789012"}},
				"due-date":      {FieldID: "due-date", Value: "2025-12-31T23:59:59Z"},
				"reference-url": {FieldID: "reference-url", Value: "https://example.com"},
			},
			wantErr: nil,
		},
		{
			name: "valid required fields only",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
			},
			wantErr: nil,
		},
		{
			name: "valid with interface slice for multi-select",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []interface{}{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
			},
			wantErr: nil,
		},
		{
			name: "valid with interface slice for multi-user",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"responders": {FieldID: "responders", Value: []interface{}{"U123456"}},
			},
			wantErr: nil,
		},
		{
			name: "valid with time.Time for date",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"due-date":   {FieldID: "due-date", Value: time.Now()},
			},
			wantErr: nil,
		},
		{
			name: "missing required field",
			fieldValues: map[string]model.FieldValue{
				"category": {FieldID: "category", Value: []string{"data-breach"}},
				// Missing "likelihood"
			},
			wantErr: model.ErrMissingRequired,
		},
		{
			name: "invalid select option",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "invalid-option"},
			},
			wantErr: model.ErrInvalidOptionID,
		},
		{
			name: "invalid multi-select option",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach", "invalid-option"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
			},
			wantErr: model.ErrInvalidOptionID,
		},
		{
			name: "invalid text type (number instead of string)",
			fieldValues: map[string]model.FieldValue{
				"category":    {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood":  {FieldID: "likelihood", Value: "low"},
				"description": {FieldID: "description", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid number type (string instead of number)",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"score":      {FieldID: "score", Value: "not a number"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid select type (array instead of string)",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: []string{"low"}},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid multi-select type (string instead of array)",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: "data-breach"},
				"likelihood": {FieldID: "likelihood", Value: "low"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid user type (number instead of string)",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"assignee":   {FieldID: "assignee", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid multi-user type (string instead of array)",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"responders": {FieldID: "responders", Value: "U123456"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid date format",
			fieldValues: map[string]model.FieldValue{
				"category":   {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood": {FieldID: "likelihood", Value: "low"},
				"due-date":   {FieldID: "due-date", Value: "invalid date"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid url type (number instead of string)",
			fieldValues: map[string]model.FieldValue{
				"category":      {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood":    {FieldID: "likelihood", Value: "low"},
				"reference-url": {FieldID: "reference-url", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "unknown field (should be ignored for forward compatibility)",
			fieldValues: map[string]model.FieldValue{
				"category":      {FieldID: "category", Value: []string{"data-breach"}},
				"likelihood":    {FieldID: "likelihood", Value: "low"},
				"unknown-field": {FieldID: "unknown-field", Value: "some value"},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := model.NewFieldValidator(schema)
			_, err := validator.ValidateCaseFields(tt.fieldValues)

			if tt.wantErr != nil {
				gt.Value(t, err).NotNil()
				gt.Error(t, err).Is(tt.wantErr)
				return
			}

			gt.NoError(t, err)
		})
	}
}

func TestFieldValidator_ValidateCaseFieldsPartial(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "stage",
				Name:     "Stage",
				Type:     types.FieldTypeSelect,
				Required: true,
				Options: []config.FieldOption{
					{ID: "screening", Name: "Screening"},
					{ID: "tech-interview", Name: "Tech Interview"},
				},
			},
			{
				ID:       "channel",
				Name:     "Channel",
				Type:     types.FieldTypeSelect,
				Required: false,
				Options: []config.FieldOption{
					{ID: "referral", Name: "Referral"},
					{ID: "agent", Name: "Agent"},
				},
			},
		},
	}
	validator := model.NewFieldValidator(schema)

	t.Run("missing required field is allowed in partial mode", func(t *testing.T) {
		got, err := validator.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"channel": {FieldID: "channel", Value: "referral"},
		})
		gt.NoError(t, err).Required()
		gt.Map(t, got).HasKey("channel")
	})

	t.Run("invalid option is still rejected in partial mode", func(t *testing.T) {
		_, err := validator.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"stage": {FieldID: "stage", Value: "no-such-stage"},
		})
		gt.Error(t, err).Is(model.ErrInvalidOptionID)
	})

	t.Run("empty input is fine in partial mode", func(t *testing.T) {
		got, err := validator.ValidateCaseFieldsPartial(map[string]model.FieldValue{})
		gt.NoError(t, err).Required()
		gt.Array(t, mapKeys(got)).Length(0)
	})
}

func TestFieldValidator_CaseRef(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:                 "ref",
				Name:               "Ref",
				Type:               types.FieldTypeCaseRef,
				ReferenceWorkspace: "other",
			},
			{
				ID:                 "refs",
				Name:               "Refs",
				Type:               types.FieldTypeMultiCaseRef,
				ReferenceWorkspace: "other",
			},
		},
	}
	v := model.NewFieldValidator(schema)

	t.Run("valid single case_ref accepted and Type injected", func(t *testing.T) {
		out, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"ref": {FieldID: "ref", Value: "42"},
		})
		gt.NoError(t, err).Required()
		gt.Map(t, out).HasKey("ref")
		gt.Value(t, out["ref"].Type).Equal(types.FieldTypeCaseRef)
	})

	t.Run("valid multi_case_ref accepted and Type injected", func(t *testing.T) {
		out, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"refs": {FieldID: "refs", Value: []string{"42", "57"}},
		})
		gt.NoError(t, err).Required()
		gt.Map(t, out).HasKey("refs")
		gt.Value(t, out["refs"].Type).Equal(types.FieldTypeMultiCaseRef)
	})

	t.Run("single case_ref with non-numeric string is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"ref": {FieldID: "ref", Value: "abc"},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("single case_ref with non-string value is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"ref": {FieldID: "ref", Value: 42},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("multi_case_ref with non-string element is rejected via []interface{}", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"refs": {FieldID: "refs", Value: []interface{}{"42", 99}},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("multi_case_ref with non-numeric string element is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"refs": {FieldID: "refs", Value: []string{"42", "not-a-number"}},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("multi_case_ref with non-slice value is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"refs": {FieldID: "refs", Value: "42"},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})
}

func TestFieldValidator_Markdown(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "body",
				Name:     "Body",
				Type:     types.FieldTypeMarkdown,
				Required: true,
			},
		},
	}
	v := model.NewFieldValidator(schema)

	t.Run("valid markdown string accepted and Type injected", func(t *testing.T) {
		out, err := v.ValidateCaseFieldsAll(map[string]model.FieldValue{
			"body": {FieldID: "body", Value: "# Heading\n\n- item"},
		})
		gt.NoError(t, err).Required()
		gt.Map(t, out).HasKey("body")
		gt.Value(t, out["body"].Type).Equal(types.FieldTypeMarkdown)
		gt.Value(t, out["body"].Value).Equal("# Heading\n\n- item")
	})

	t.Run("empty string is valid (explicit empty, same as text)", func(t *testing.T) {
		out, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"body": {FieldID: "body", Value: ""},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, out["body"].Value).Equal("")
	})

	t.Run("non-string value is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"body": {FieldID: "body", Value: 42},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("slice value is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartial(map[string]model.FieldValue{
			"body": {FieldID: "body", Value: []string{"a"}},
		})
		gt.Error(t, err).Is(model.ErrInvalidFieldType)
	})

	t.Run("required markdown field missing is rejected", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsAll(map[string]model.FieldValue{})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
	})
}

func mapKeys(m map[string]model.FieldValue) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestFieldValidator_ValidateNumber_MultipleTypes(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "score",
				Name:     "Score",
				Type:     types.FieldTypeNumber,
				Required: true,
			},
		},
	}

	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{
			name:    "float64",
			value:   float64(3.14),
			wantErr: false,
		},
		{
			name:    "int",
			value:   int(42),
			wantErr: false,
		},
		{
			name:    "int64",
			value:   int64(42),
			wantErr: false,
		},
		{
			name:    "int32",
			value:   int32(42),
			wantErr: false,
		},
		{
			name:    "string (invalid)",
			value:   "42",
			wantErr: true,
		},
		{
			name:    "json.Number integer (gqlgen Any input)",
			value:   json.Number("42"),
			wantErr: false,
		},
		{
			name:    "json.Number float (gqlgen Any input)",
			value:   json.Number("3.14"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := model.NewFieldValidator(schema)
			fieldValues := map[string]model.FieldValue{
				"score": {FieldID: "score", Value: tt.value},
			}

			_, err := validator.ValidateCaseFields(fieldValues)
			if tt.wantErr {
				gt.Value(t, err).NotNil()
			} else {
				gt.NoError(t, err)
			}
		})
	}
}

func TestFieldValidator_ValidateCaseFieldsAll(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "severity",
				Name:     "Severity",
				Type:     types.FieldTypeSelect,
				Required: true,
				Options: []config.FieldOption{
					{ID: "low", Name: "Low"},
					{ID: "high", Name: "High"},
				},
			},
			{
				ID:       "summary",
				Name:     "Summary",
				Type:     types.FieldTypeText,
				Required: true,
			},
		},
	}
	v := model.NewFieldValidator(schema)

	t.Run("aggregates all violations without fail-fast", func(t *testing.T) {
		// severity has an invalid option, summary (required text) is missing,
		// and an unknown field id is supplied. All three must be reported.
		_, err := v.ValidateCaseFieldsAll(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Value: "critical"},
			"bogus":    {FieldID: "bogus", Value: "x"},
		})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
		msg := err.Error()
		gt.String(t, msg).Contains("severity")
		gt.String(t, msg).Contains("summary")
		gt.String(t, msg).Contains("bogus")
	})

	t.Run("returns enriched values when all valid", func(t *testing.T) {
		out, err := v.ValidateCaseFieldsAll(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Value: "high"},
			"summary":  {FieldID: "summary", Value: "a clear summary"},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, out["severity"].Type).Equal(types.FieldTypeSelect)
		gt.Value(t, out["summary"].Type).Equal(types.FieldTypeText)
	})
}

func TestFieldValidator_ValidateCaseFieldsPartialStrict(t *testing.T) {
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:       "severity",
				Name:     "Severity",
				Type:     types.FieldTypeSelect,
				Required: true,
				Options: []config.FieldOption{
					{ID: "low", Name: "Low"},
					{ID: "high", Name: "High"},
				},
			},
			{
				ID:       "summary",
				Name:     "Summary",
				Type:     types.FieldTypeText,
				Required: true,
			},
		},
	}
	v := model.NewFieldValidator(schema)

	t.Run("missing required field is NOT a violation", func(t *testing.T) {
		// Only severity is submitted; the required summary is absent. Unlike
		// ValidateCaseFieldsAll, a partial strict update must not fail on it.
		out, err := v.ValidateCaseFieldsPartialStrict(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Value: "high"},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, out["severity"].Type).Equal(types.FieldTypeSelect)
		gt.Map(t, out).Length(1)
	})

	t.Run("unknown field id IS a violation", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartialStrict(map[string]model.FieldValue{
			"ghost": {FieldID: "ghost", Value: "x"},
		})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
		gt.String(t, err.Error()).Contains("ghost")
	})

	t.Run("type / option violations are accumulated", func(t *testing.T) {
		_, err := v.ValidateCaseFieldsPartialStrict(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Value: "critical"},
			"ghost":    {FieldID: "ghost", Value: "x"},
		})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
		msg := err.Error()
		gt.String(t, msg).Contains("severity")
		gt.String(t, msg).Contains("ghost")
	})
}

func buildStoredValidatorSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "note", Name: "Note", Type: types.FieldTypeText},
			{ID: "body", Name: "Body", Type: types.FieldTypeMarkdown},
			{ID: "score", Name: "Score", Type: types.FieldTypeNumber},
			{
				ID: "severity", Name: "Severity", Type: types.FieldTypeSelect,
				Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}},
			},
			{
				ID: "tags", Name: "Tags", Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{{ID: "net", Name: "Net"}},
			},
			{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
			{ID: "watchers", Name: "Watchers", Type: types.FieldTypeMultiUser},
			{ID: "due", Name: "Due", Type: types.FieldTypeDate},
			{ID: "link", Name: "Link", Type: types.FieldTypeURL},
			{ID: "parent", Name: "Parent", Type: types.FieldTypeCaseRef, ReferenceWorkspace: "ws-other"},
			{ID: "related", Name: "Related", Type: types.FieldTypeMultiCaseRef, ReferenceWorkspace: "ws-other"},
			{ID: "mandatory", Name: "Mandatory", Type: types.FieldTypeText, Required: true},
		},
		Labels: config.EntityLabels{Case: "Case"},
	}
}

func TestFieldValidator_ValidateStored(t *testing.T) {
	v := model.NewFieldValidator(buildStoredValidatorSchema())

	t.Run("every field type accepts a well-formed stored value", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"note":      {FieldID: "note", Type: types.FieldTypeText, Value: "text"},
			"body":      {FieldID: "body", Type: types.FieldTypeMarkdown, Value: "# heading"},
			"score":     {FieldID: "score", Type: types.FieldTypeNumber, Value: float64(3)},
			"severity":  {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"tags":      {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"net"}},
			"owner":     {FieldID: "owner", Type: types.FieldTypeUser, Value: "U1"},
			"watchers":  {FieldID: "watchers", Type: types.FieldTypeMultiUser, Value: []string{"U1", "U2"}},
			"due":       {FieldID: "due", Type: types.FieldTypeDate, Value: "2026-08-07T00:00:00Z"},
			"link":      {FieldID: "link", Type: types.FieldTypeURL, Value: "https://example.com"},
			"parent":    {FieldID: "parent", Type: types.FieldTypeCaseRef, Value: "12"},
			"related":   {FieldID: "related", Type: types.FieldTypeMultiCaseRef, Value: []string{"12", "13"}},
			"mandatory": {FieldID: "mandatory", Type: types.FieldTypeText, Value: "set"},
		})
		gt.Array(t, violations).Length(0)
	})

	t.Run("a field id the schema no longer defines is skipped", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"removed-field": {FieldID: "removed-field", Type: types.FieldTypeSelect, Value: "anything"},
		})
		gt.Array(t, violations).Length(0)
	})

	t.Run("a required field with no stored value is not reported", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"note": {FieldID: "note", Type: types.FieldTypeText, Value: "text"},
		})
		gt.Array(t, violations).Length(0)
	})

	t.Run("number holding a string is an invalid field type", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"score": {FieldID: "score", Type: types.FieldTypeNumber, Value: "three"},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.Value(t, violations[0].FieldID).Equal("score")
		gt.Error(t, violations[0].Err).Is(model.ErrInvalidFieldType)
	})

	t.Run("date outside RFC3339 is an invalid field type", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"due": {FieldID: "due", Type: types.FieldTypeDate, Value: "2026/08/07"},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.Value(t, violations[0].FieldID).Equal("due")
		gt.Error(t, violations[0].Err).Is(model.ErrInvalidFieldType)
	})

	t.Run("select value outside the option list is an invalid option id", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "medium"},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.Value(t, violations[0].FieldID).Equal("severity")
		gt.Error(t, violations[0].Err).Is(model.ErrInvalidOptionID)
	})

	t.Run("multi_case_ref holding a non-numeric id is an invalid field type", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"related": {FieldID: "related", Type: types.FieldTypeMultiCaseRef, Value: []string{"12", "not-a-number"}},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.Value(t, violations[0].FieldID).Equal("related")
		gt.Error(t, violations[0].Err).Is(model.ErrInvalidFieldType)
	})

	t.Run("stored type that drifted from the schema is reported even when the shape fits", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"body": {FieldID: "body", Type: types.FieldTypeText, Value: "still a string"},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.Value(t, violations[0].FieldID).Equal("body")
		gt.Error(t, violations[0].Err).Is(model.ErrStoredFieldTypeMismatch)
	})

	t.Run("an empty stored type is not treated as drift", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"body": {FieldID: "body", Value: "still a string"},
		})
		gt.Array(t, violations).Length(0)
	})

	t.Run("drift and a bad value on one field yield two violations", func(t *testing.T) {
		violations := v.ValidateStored(map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeText, Value: "medium"},
		})
		gt.Array(t, violations).Length(2).Required()
		gt.Value(t, violations[0].FieldID).Equal("severity")
		gt.Value(t, violations[1].FieldID).Equal("severity")
		gt.Error(t, violations[0].Err).Is(model.ErrStoredFieldTypeMismatch)
		gt.Error(t, violations[1].Err).Is(model.ErrInvalidOptionID)
	})

	t.Run("violations are ordered by field id", func(t *testing.T) {
		values := map[string]model.FieldValue{
			"score":    {FieldID: "score", Type: types.FieldTypeNumber, Value: "three"},
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "medium"},
			"due":      {FieldID: "due", Type: types.FieldTypeDate, Value: "2026/08/07"},
		}
		violations := v.ValidateStored(values)
		gt.Array(t, violations).Length(3).Required()
		gt.Value(t, violations[0].FieldID).Equal("due")
		gt.Value(t, violations[1].FieldID).Equal("score")
		gt.Value(t, violations[2].FieldID).Equal("severity")

		// Same input, same order: the report must not depend on map iteration.
		// Compare the field ids rather than the violations themselves — each
		// goerr value carries its own stack trace and never compares equal.
		second := v.ValidateStored(values)
		gt.Array(t, second).Length(len(violations)).Required()
		for i := range violations {
			gt.Value(t, second[i].FieldID).Equal(violations[i].FieldID)
		}
	})
}
