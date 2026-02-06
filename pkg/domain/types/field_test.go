package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestFieldType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		fieldType types.FieldType
		want      bool
	}{
		{
			name:      "valid text",
			fieldType: types.FieldTypeText,
			want:      true,
		},
		{
			name:      "valid number",
			fieldType: types.FieldTypeNumber,
			want:      true,
		},
		{
			name:      "valid select",
			fieldType: types.FieldTypeSelect,
			want:      true,
		},
		{
			name:      "valid multi-select",
			fieldType: types.FieldTypeMultiSelect,
			want:      true,
		},
		{
			name:      "valid user",
			fieldType: types.FieldTypeUser,
			want:      true,
		},
		{
			name:      "valid multi-user",
			fieldType: types.FieldTypeMultiUser,
			want:      true,
		},
		{
			name:      "valid date",
			fieldType: types.FieldTypeDate,
			want:      true,
		},
		{
			name:      "valid url",
			fieldType: types.FieldTypeURL,
			want:      true,
		},
		{
			name:      "invalid type",
			fieldType: types.FieldType("invalid"),
			want:      false,
		},
		{
			name:      "empty type",
			fieldType: types.FieldType(""),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want {
				gt.B(t, tt.fieldType.IsValid()).True()
			} else {
				gt.B(t, tt.fieldType.IsValid()).False()
			}
		})
	}
}

func TestAllFieldTypes(t *testing.T) {
	fieldTypes := types.AllFieldTypes()
	expectedCount := 8

	gt.A(t, fieldTypes).Length(expectedCount)

	// Verify all returned types are valid
	for _, fieldType := range fieldTypes {
		gt.B(t, fieldType.IsValid()).
			Describef("Field type %s should be valid", fieldType).
			True()
	}

	// Verify all expected types are present
	expectedTypes := []types.FieldType{
		types.FieldTypeText,
		types.FieldTypeNumber,
		types.FieldTypeSelect,
		types.FieldTypeMultiSelect,
		types.FieldTypeUser,
		types.FieldTypeMultiUser,
		types.FieldTypeDate,
		types.FieldTypeURL,
	}

	typeMap := make(map[types.FieldType]bool)
	for _, fieldType := range fieldTypes {
		typeMap[fieldType] = true
	}

	for _, expected := range expectedTypes {
		gt.B(t, typeMap[expected]).
			Describef("Expected field type %s should be present", expected).
			True()
	}
}

func TestFieldType_String(t *testing.T) {
	tests := []struct {
		name      string
		fieldType types.FieldType
		want      string
	}{
		{
			name:      "text",
			fieldType: types.FieldTypeText,
			want:      "text",
		},
		{
			name:      "number",
			fieldType: types.FieldTypeNumber,
			want:      "number",
		},
		{
			name:      "select",
			fieldType: types.FieldTypeSelect,
			want:      "select",
		},
		{
			name:      "multi-select",
			fieldType: types.FieldTypeMultiSelect,
			want:      "multi-select",
		},
		{
			name:      "user",
			fieldType: types.FieldTypeUser,
			want:      "user",
		},
		{
			name:      "multi-user",
			fieldType: types.FieldTypeMultiUser,
			want:      "multi-user",
		},
		{
			name:      "date",
			fieldType: types.FieldTypeDate,
			want:      "date",
		},
		{
			name:      "url",
			fieldType: types.FieldTypeURL,
			want:      "url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.S(t, tt.fieldType.String()).Equal(tt.want)
		})
	}
}
