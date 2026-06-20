package model_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func coerceTestSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "summary", Name: "Summary", Type: types.FieldTypeText},
			{ID: "score", Name: "Score", Type: types.FieldTypeNumber},
			{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
			{ID: "tags", Name: "Tags", Type: types.FieldTypeMultiSelect, Options: []config.FieldOption{{ID: "a"}, {ID: "b"}}},
			{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
			{ID: "watchers", Name: "Watchers", Type: types.FieldTypeMultiUser},
			{ID: "due", Name: "Due", Type: types.FieldTypeDate},
			{ID: "link", Name: "Link", Type: types.FieldTypeURL},
		},
	}
}

func TestCoerceFieldInputs(t *testing.T) {
	schema := coerceTestSchema()

	t.Run("empty input returns empty map and no violations", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, nil)
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).Length(0)
	})

	t.Run("scalar types pass through as strings", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "summary", Value: "hello"},
			{FieldID: "severity", Value: "high"},
			{FieldID: "owner", Value: "U001"},
			{FieldID: "due", Value: "2026-01-01T00:00:00Z"},
			{FieldID: "link", Value: "https://example.com"},
		})
		gt.Array(t, violations).Length(0)
		gt.Value(t, out["summary"].Value).Equal("hello")
		gt.Value(t, out["summary"].Type).Equal(types.FieldTypeText)
		gt.Value(t, out["severity"].Value).Equal("high")
		gt.Value(t, out["owner"].Value).Equal("U001")
		gt.Value(t, out["due"].Value).Equal("2026-01-01T00:00:00Z")
		gt.Value(t, out["link"].Value).Equal("https://example.com")
	})

	t.Run("number is parsed to float64", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "score", Value: "42.5"},
		})
		gt.Array(t, violations).Length(0)
		gt.Value(t, out["score"].Value).Equal(float64(42.5))
		gt.Value(t, out["score"].Type).Equal(types.FieldTypeNumber)
	})

	t.Run("empty number is skipped, not a violation", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "score", Value: "  "},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).Length(0)
	})

	t.Run("unparseable number is reported as a violation", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "score", Value: "not-a-number"},
		})
		gt.Array(t, violations).Length(1).Required()
		gt.String(t, violations[0]).Contains("score")
		gt.String(t, violations[0]).Contains("number")
		gt.Map(t, out).Length(0)
	})

	t.Run("multi types use Values, falling back to single Value", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "tags", Values: []string{"a", "b"}},
			{FieldID: "watchers", Value: "U001"},
		})
		gt.Array(t, violations).Length(0)
		gt.Value(t, out["tags"].Value).Equal([]string{"a", "b"})
		gt.Value(t, out["tags"].Type).Equal(types.FieldTypeMultiSelect)
		gt.Value(t, out["watchers"].Value).Equal([]string{"U001"})
		gt.Value(t, out["watchers"].Type).Equal(types.FieldTypeMultiUser)
	})

	t.Run("multi with neither Value nor Values yields empty slice", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "tags"},
		})
		gt.Array(t, violations).Length(0)
		gt.Value(t, out["tags"].Value).Equal([]string{})
	})

	t.Run("unknown field id passes through with raw value and unset type", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "ghost", Value: "x"},
		})
		// No coercion violation: the downstream validator owns the
		// "not defined in the workspace schema" report.
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("ghost")
		gt.Value(t, out["ghost"].Value).Equal("x")
		gt.Value(t, out["ghost"].Type).Equal(types.FieldType(""))
	})

	t.Run("empty scalar is preserved as an explicit empty value", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "summary", Value: ""},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("summary")
		gt.Value(t, out["summary"].Value).Equal("")
	})

	t.Run("nil schema treats every id as unknown", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(nil, []model.FieldInput{
			{FieldID: "summary", Value: "hello"},
		})
		gt.Array(t, violations).Length(0)
		gt.Value(t, out["summary"].Value).Equal("hello")
		gt.Value(t, out["summary"].Type).Equal(types.FieldType(""))
	})
}

func coerceTestSchemaWithCaseRef() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "ref", Name: "Ref", Type: types.FieldTypeCaseRef, ReferenceWorkspace: "other"},
			{ID: "refs", Name: "Refs", Type: types.FieldTypeMultiCaseRef, ReferenceWorkspace: "other"},
		},
	}
}

func TestCoerceFieldInputs_CaseRef(t *testing.T) {
	schema := coerceTestSchemaWithCaseRef()

	t.Run("single case_ref coerces scalar Value to string", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "ref", Value: "42"},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("ref")
		gt.Value(t, out["ref"].Type).Equal(types.FieldTypeCaseRef)
		gt.Value(t, out["ref"].Value).Equal("42")
	})

	t.Run("multi_case_ref coerces Values slice to []string", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "refs", Values: []string{"42", "57"}},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("refs")
		gt.Value(t, out["refs"].Type).Equal(types.FieldTypeMultiCaseRef)
		gt.Value(t, out["refs"].Value).Equal([]string{"42", "57"})
	})

	t.Run("multi_case_ref falls back to scalar Value as single-element slice", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "refs", Value: "42"},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("refs")
		gt.Value(t, out["refs"].Type).Equal(types.FieldTypeMultiCaseRef)
		gt.Value(t, out["refs"].Value).Equal([]string{"42"})
	})

	t.Run("multi_case_ref with neither Value nor Values yields empty slice", func(t *testing.T) {
		out, violations := model.CoerceFieldInputs(schema, []model.FieldInput{
			{FieldID: "refs"},
		})
		gt.Array(t, violations).Length(0)
		gt.Map(t, out).HasKey("refs")
		gt.Value(t, out["refs"].Value).Equal([]string{})
	})
}
