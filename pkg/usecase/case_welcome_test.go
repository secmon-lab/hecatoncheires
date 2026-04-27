package usecase_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
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

func TestWelcomeRenderer_AccessesSelectFieldByIDAndName(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Severity: {{.Fields.severity.name}} ({{.Fields.severity.id}}) / Risk: {{.Fields.risk_level.name}}",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{
		Title: "Test",
		FieldValues: map[string]model.FieldValue{
			"severity":   {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
			"risk_level": {FieldID: "risk_level", Type: types.FieldTypeSelect, Value: "critical"},
		},
	}
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:   "severity",
				Type: types.FieldTypeSelect,
				Options: []config.FieldOption{
					{ID: "high", Name: "High"},
					{ID: "low", Name: "Low"},
				},
			},
			{
				ID:   "risk_level",
				Type: types.FieldTypeSelect,
				Options: []config.FieldOption{
					{ID: "critical", Name: "Critical"},
				},
			},
		},
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:   c,
		Fields: usecase.BuildWelcomeFieldsForTest(c, schema),
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(1)
	gt.Value(t, out[0]).Equal("Severity: High (high) / Risk: Critical")
}

func TestWelcomeRenderer_AccessesMultiSelectItems(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Tags: {{range $i, $t := .Fields.tags.items}}{{if $i}}, {{end}}{{$t.name}}{{end}}",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{
		FieldValues: map[string]model.FieldValue{
			"tags": {FieldID: "tags", Type: types.FieldTypeMultiSelect, Value: []string{"urgent", "review_needed"}},
		},
	}
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{
				ID:   "tags",
				Type: types.FieldTypeMultiSelect,
				Options: []config.FieldOption{
					{ID: "urgent", Name: "Urgent"},
					{ID: "review_needed", Name: "Review Needed"},
				},
			},
		},
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:   c,
		Fields: usecase.BuildWelcomeFieldsForTest(c, schema),
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(1)
	gt.Value(t, out[0]).Equal("Tags: Urgent, Review Needed")
}

func TestWelcomeRenderer_TextFieldExposesIDAndName(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Note: {{.Fields.note.id}} / {{.Fields.note.name}}",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{
		FieldValues: map[string]model.FieldValue{
			"note": {FieldID: "note", Type: types.FieldTypeText, Value: "free text"},
		},
	}
	schema := &config.FieldSchema{
		Fields: []config.FieldDefinition{
			{ID: "note", Type: types.FieldTypeText},
		},
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:   c,
		Fields: usecase.BuildWelcomeFieldsForTest(c, schema),
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(1)
	gt.Value(t, out[0]).Equal("Note: free text / free text")
}

func TestWelcomeRenderer_FallsBackWhenSchemaMissing(t *testing.T) {
	r, err := usecase.NewWelcomeRendererForTest([]string{
		"Severity: {{.Fields.severity.id}} / {{.Fields.severity.name}}",
	})
	gt.NoError(t, err).Required()

	c := &model.Case{
		FieldValues: map[string]model.FieldValue{
			"severity": {FieldID: "severity", Type: types.FieldTypeSelect, Value: "high"},
		},
	}
	out, err := usecase.WelcomeRendererRenderForTest(r, usecase.WelcomeContextForTest{
		Case:   c,
		Fields: usecase.BuildWelcomeFieldsForTest(c, nil),
	})
	gt.NoError(t, err).Required()
	gt.Array(t, out).Length(1)
	// With no schema, both id and name fall back to the raw value.
	gt.Value(t, out[0]).Equal("Severity: high / high")
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
