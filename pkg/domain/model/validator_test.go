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
