package threadcase_test

import (
	"testing"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

func createWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "support", Name: "Support"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
				{ID: "summary", Name: "Summary", Type: types.FieldTypeText, Required: true},
			},
		},
	}
}

func TestValidateCreateDecision_AggregatesViolations(t *testing.T) {
	ws := createWorkspace()
	// empty title, invalid severity option, missing required summary, and an
	// unknown field id — all four must be reported in one error.
	dec := &threadcase.CreateDecisionForTest{
		Title: "",
		Fields: []threadcase.DecisionField{
			{FieldID: "severity", Value: "critical"},
			{FieldID: "bogus", Value: "x"},
		},
	}
	_, err := threadcase.ValidateCreateDecisionForTest(ws, dec)
	gt.Error(t, err).Required()
	msg := err.Error()
	gt.String(t, msg).Contains("title")
	gt.String(t, msg).Contains("severity")
	gt.String(t, msg).Contains("summary")
	gt.String(t, msg).Contains("bogus")
}

func TestValidateCreateDecision_Valid(t *testing.T) {
	ws := createWorkspace()
	dec := &threadcase.CreateDecisionForTest{
		Title:       "Login failure",
		Description: "A user cannot log in.",
		Fields: []threadcase.DecisionField{
			{FieldID: "severity", Value: "high"},
			{FieldID: "summary", Value: "login broken"},
		},
	}
	fields, err := threadcase.ValidateCreateDecisionForTest(ws, dec)
	gt.NoError(t, err).Required()
	gt.Value(t, fields["severity"].Type).Equal(types.FieldTypeSelect)
	gt.Value(t, fields["severity"].Value).Equal("high")
	gt.Value(t, fields["summary"].Type).Equal(types.FieldTypeText)
}

func TestValidateCreateDecision_OptionalEmptyNumberSkipped(t *testing.T) {
	ws := &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "support"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "title_ok", Name: "T", Type: types.FieldTypeText, Required: true},
				{ID: "score", Name: "Score", Type: types.FieldTypeNumber}, // optional
			},
		},
	}
	// An optional number field emitted with an empty value must not raise a
	// spurious "not a number" violation; it is simply dropped.
	dec := &threadcase.CreateDecisionForTest{
		Title:       "A title",
		Description: "desc",
		Fields: []threadcase.DecisionField{
			{FieldID: "title_ok", Value: "ok"},
			{FieldID: "score", Value: ""},
		},
	}
	fields, err := threadcase.ValidateCreateDecisionForTest(ws, dec)
	gt.NoError(t, err).Required()
	_, hasScore := fields["score"]
	gt.Bool(t, hasScore).False()

	// nil decision is rejected gracefully.
	_, err = threadcase.ValidateCreateDecisionForTest(ws, nil)
	gt.Error(t, err).Required()
}
