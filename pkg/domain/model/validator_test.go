package model_test

import (
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
		fieldValues []model.FieldValue
		wantErr     error
	}{
		{
			name: "valid all field types",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach", "system-failure"}},
				{FieldID: "likelihood", Value: "high"},
				{FieldID: "description", Value: "Test description"},
				{FieldID: "score", Value: float64(85)},
				{FieldID: "assignee", Value: "U123456"},
				{FieldID: "responders", Value: []string{"U123456", "U789012"}},
				{FieldID: "due-date", Value: "2025-12-31T23:59:59Z"},
				{FieldID: "reference-url", Value: "https://example.com"},
			},
			wantErr: nil,
		},
		{
			name: "valid required fields only",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
			},
			wantErr: nil,
		},
		{
			name: "valid with interface slice for multi-select",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []interface{}{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
			},
			wantErr: nil,
		},
		{
			name: "valid with interface slice for multi-user",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "responders", Value: []interface{}{"U123456"}},
			},
			wantErr: nil,
		},
		{
			name: "valid with time.Time for date",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "due-date", Value: time.Now()},
			},
			wantErr: nil,
		},
		{
			name: "missing required field",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				// Missing "likelihood"
			},
			wantErr: model.ErrMissingRequired,
		},
		{
			name: "invalid select option",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "invalid-option"},
			},
			wantErr: model.ErrInvalidOptionID,
		},
		{
			name: "invalid multi-select option",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach", "invalid-option"}},
				{FieldID: "likelihood", Value: "low"},
			},
			wantErr: model.ErrInvalidOptionID,
		},
		{
			name: "invalid text type (number instead of string)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "description", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid number type (string instead of number)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "score", Value: "not a number"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid select type (array instead of string)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: []string{"low"}},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid multi-select type (string instead of array)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: "data-breach"},
				{FieldID: "likelihood", Value: "low"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid user type (number instead of string)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "assignee", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid multi-user type (string instead of array)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "responders", Value: "U123456"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid date format",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "due-date", Value: "invalid date"},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "invalid url type (number instead of string)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "reference-url", Value: 123},
			},
			wantErr: model.ErrInvalidFieldType,
		},
		{
			name: "unknown field (should be ignored for forward compatibility)",
			fieldValues: []model.FieldValue{
				{FieldID: "category", Value: []string{"data-breach"}},
				{FieldID: "likelihood", Value: "low"},
				{FieldID: "unknown-field", Value: "some value"},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := model.NewFieldValidator(schema)
			err := validator.ValidateCaseFields(tt.fieldValues)

			if tt.wantErr != nil {
				gt.Value(t, err).NotNil()
				gt.Error(t, err).Is(tt.wantErr)
				return
			}

			gt.NoError(t, err)
		})
	}
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := model.NewFieldValidator(schema)
			fieldValues := []model.FieldValue{
				{FieldID: "score", Value: tt.value},
			}

			err := validator.ValidateCaseFields(fieldValues)
			if tt.wantErr {
				gt.Value(t, err).NotNil()
			} else {
				gt.NoError(t, err)
			}
		})
	}
}
