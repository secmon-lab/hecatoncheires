package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func validationSchema() *config.FieldSchema {
	return &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "severity", Type: types.FieldTypeSelect, Required: true,
				Options: []config.FieldOption{{ID: "low"}, {ID: "high"}}},
			{ID: "tags", Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{{ID: "okta"}, {ID: "abuse"}}},
			{ID: "evidence_url", Type: types.FieldTypeURL},
			{ID: "detected_at", Type: types.FieldTypeDate},
			{ID: "count", Type: types.FieldTypeNumber},
			{ID: "owner", Type: types.FieldTypeUser},
		},
	}
}

func fv(id string, t types.FieldType, v any) model.FieldValue {
	return model.FieldValue{FieldID: types.FieldID(id), Type: t, Value: v}
}

func TestWorkspaceMaterialization_Validate_HappyPath(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title:       "ok",
		Description: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity":     fv("severity", types.FieldTypeSelect, "high"),
			"tags":         fv("tags", types.FieldTypeMultiSelect, []string{"okta", "abuse"}),
			"evidence_url": fv("evidence_url", types.FieldTypeURL, "https://example.com/x"),
			"detected_at":  fv("detected_at", types.FieldTypeDate, "2026-05-02"),
			"count":        fv("count", types.FieldTypeNumber, 5.0),
			"owner":        fv("owner", types.FieldTypeUser, "U1"),
		},
	}
	issues, fatal := mat.Validate(validationSchema())
	gt.Bool(t, fatal).False()
	gt.Array(t, issues).Length(0)
}

func TestWorkspaceMaterialization_Validate_MissingTitleIsFatal(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "  ",
		CustomFieldValues: map[string]model.FieldValue{
			"severity": fv("severity", types.FieldTypeSelect, "low"),
		},
	}
	_, fatal := mat.Validate(validationSchema())
	gt.Bool(t, fatal).True()
}

func TestWorkspaceMaterialization_Validate_MissingRequiredField(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title:             "ok",
		CustomFieldValues: map[string]model.FieldValue{}, // severity (required) missing
	}
	issues, fatal := mat.Validate(validationSchema())
	gt.Bool(t, fatal).False() // missing required is non-fatal — Edit modal can fill it
	hasMissing := false
	for _, i := range issues {
		if i.Code == "missing_required" && i.FieldID == "severity" {
			hasMissing = true
		}
	}
	gt.Bool(t, hasMissing).True()
}

func TestWorkspaceMaterialization_Validate_BadEnum(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity": fv("severity", types.FieldTypeSelect, "nope"),
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "severity", "bad_enum")).True()
}

func TestWorkspaceMaterialization_Validate_BadMultiSelectEnum(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity": fv("severity", types.FieldTypeSelect, "low"),
			"tags":     fv("tags", types.FieldTypeMultiSelect, []string{"okta", "ghost"}),
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "tags", "bad_enum")).True()
}

func TestWorkspaceMaterialization_Validate_BadURL(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity":     fv("severity", types.FieldTypeSelect, "low"),
			"evidence_url": fv("evidence_url", types.FieldTypeURL, "not a url"),
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "evidence_url", "bad_url")).True()
}

func TestWorkspaceMaterialization_Validate_BadDate(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity":    fv("severity", types.FieldTypeSelect, "low"),
			"detected_at": fv("detected_at", types.FieldTypeDate, "yesterday"),
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "detected_at", "bad_date")).True()
}

func TestWorkspaceMaterialization_Validate_TypeMismatch(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity": fv("severity", types.FieldTypeText, "low"), // wrong type marker
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "severity", "type_mismatch")).True()
}

func TestWorkspaceMaterialization_Validate_UnknownField(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity":     fv("severity", types.FieldTypeSelect, "low"),
			"hallucinated": fv("hallucinated", types.FieldTypeText, "x"),
		},
	}
	issues, _ := mat.Validate(validationSchema())
	gt.Bool(t, hasIssue(issues, "hallucinated", "unknown_field")).True()
}

func TestWorkspaceMaterialization_FilterToValid_StripsBadFields(t *testing.T) {
	mat := &model.WorkspaceMaterialization{
		Title: "ok",
		CustomFieldValues: map[string]model.FieldValue{
			"severity":     fv("severity", types.FieldTypeSelect, "low"),
			"tags":         fv("tags", types.FieldTypeMultiSelect, []string{"ghost"}),
			"evidence_url": fv("evidence_url", types.FieldTypeURL, "not a url"),
			"hallucinated": fv("hallucinated", types.FieldTypeText, "x"),
		},
	}
	out, err := mat.FilterToValid(validationSchema())
	gt.NoError(t, err).Required()
	gt.Map(t, out.CustomFieldValues).HasKey("severity")
	_, hasTags := out.CustomFieldValues["tags"]
	gt.Bool(t, hasTags).False()
	_, hasURL := out.CustomFieldValues["evidence_url"]
	gt.Bool(t, hasURL).False()
	_, hasH := out.CustomFieldValues["hallucinated"]
	gt.Bool(t, hasH).False()
}

func TestWorkspaceMaterialization_FilterToValid_FailsOnFatal(t *testing.T) {
	mat := &model.WorkspaceMaterialization{Title: ""} // missing title is fatal
	out, err := mat.FilterToValid(validationSchema())
	gt.Value(t, err).NotNil()
	gt.Value(t, out).Nil()
}

func hasIssue(issues []model.MaterializationValidationIssue, fieldID, code string) bool {
	for _, i := range issues {
		if string(i.FieldID) == fieldID && i.Code == code {
			return true
		}
	}
	return false
}

func TestNewCaseDraft(t *testing.T) {
	now := time.Now().UTC()
	d := model.NewCaseDraft(now, "U123")

	gt.Value(t, d.ID).NotEqual(model.CaseDraftID(""))
	gt.Value(t, d.CreatedBy).Equal("U123")
	gt.Bool(t, d.CreatedAt.Equal(now)).True()
	gt.Bool(t, d.ExpiresAt.Equal(now.Add(model.CaseDraftTTL))).True()
	gt.Value(t, d.Materialization).Nil()
	gt.Bool(t, d.InferenceInProgress).False()
}

func TestCaseDraftIsExpired(t *testing.T) {
	now := time.Now().UTC()
	d := model.NewCaseDraft(now, "U1")

	gt.Bool(t, d.IsExpired(now)).False()
	gt.Bool(t, d.IsExpired(now.Add(model.CaseDraftTTL-time.Second))).False()
	gt.Bool(t, d.IsExpired(now.Add(model.CaseDraftTTL))).True()
	gt.Bool(t, d.IsExpired(now.Add(model.CaseDraftTTL+time.Hour))).True()
}

func TestNewCaseDraftIDUnique(t *testing.T) {
	seen := make(map[model.CaseDraftID]struct{})
	for range 1000 {
		id := model.NewCaseDraftID()
		gt.Value(t, id).NotEqual(model.CaseDraftID(""))
		_, dup := seen[id]
		gt.Bool(t, dup).False()
		seen[id] = struct{}{}
	}
}
