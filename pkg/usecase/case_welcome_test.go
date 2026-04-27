package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

func TestWelcomeRenderer_NilForEmptyMessages(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest(nil)
	gt.NoError(t, err)
	gt.Value(t, r).Nil()

	r2, err := usecase.NewWelcomeRendererForTest([]string{})
	gt.NoError(t, err)
	gt.Value(t, r2).Nil()
}

func TestWelcomeRenderer_RendersSimpleTemplates(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Hello {{.Case.Title}}",
		"Reporter: <@{{.Case.ReporterID}}>",
	})
	gt.NoError(t, err).Required()
	gt.Value(t, r).NotNil().Required()

	c := &model.Case{
		Title:      "Investigate Phishing",
		ReporterID: "U999",
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:      c,
		Workspace: model.Workspace{ID: "risk", Name: "Risk"},
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(2)
	gt.Value(t, out[0]).Equal("Hello Investigate Phishing")
	gt.Value(t, out[1]).Equal("Reporter: <@U999>")
}

func TestWelcomeRenderer_AccessesCustomFields(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Severity: {{.Fields.severity}} / Risk: {{.Fields.risk_level}}",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{
		Title: "Test",
		FieldValues: map[string]model.FieldValue{
			"severity":   {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"risk_level": {FieldID: "risk_level", Type: types.FieldTypeSelect, Value: "critical"},
		},
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:   c,
		Fields: usecase.BuildWelcomeFieldsForTest(c),
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(1)
	gt.Value(t, out[0]).Equal("Severity: high / Risk: critical")
}

func TestWelcomeRenderer_DropsEmptyResults(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"first",
		"{{if .Case.IsPrivate}}private only{{end}}",
		"third",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{IsPrivate: false}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{Case: c})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(2)
	gt.Value(t, out[0]).Equal("first")
	gt.Value(t, out[1]).Equal("third")
}

func TestWelcomeRenderer_RejectsInvalidTemplate(t *testing.T) {
	_, err := usecase.NewWelcomeRendererForTest([]string{
		"good",
		"broken {{.Case.Title",
	})
	gt.Value(t, err).NotNil()
}

func TestWelcomeRenderer_ExecuteErrorPropagates(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"{{.Case.NonExistentField}}",
	})
	gt.NoError(t, err).Required()

	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case: &model.Case{},
	})
	gt.Value(t, err).NotNil()
	gt.Array(t, out).Length(0)
}
