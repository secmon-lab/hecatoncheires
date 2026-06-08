package evaltype_test

import (
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
)

func TestCaseArtifactRender(t *testing.T) {
	art := &evaltype.CaseArtifact{
		Case: &model.Case{
			Title:       "Portal login failing with 503",
			Description: "Users get 503 since this morning",
			Status:      types.CaseStatusOpen,
			BoardStatus: "triage",
			FieldValues: map[string]model.FieldValue{
				"severity": {FieldID: types.FieldID("severity"), Type: types.FieldTypeSelect, Value: "high"},
			},
		},
		Transcript: []evaltype.TurnRecord{
			{Turn: 1, Mode: "materialize", Input: "Cannot log in", Decision: "materialize"},
		},
		ToolCalls: []evaltype.ToolCallRecord{
			{Seq: 1, Tool: "slack_search", Args: map[string]any{"query": "portal login"}, Mode: "sim", Result: "found 2"},
		},
	}

	gt.V(t, art.Kind()).Equal("case")
	out := art.Render()

	for _, want := range []string{
		"Portal login failing with 503",
		"Board status: triage",
		"severity: high",
		"Turn 1 (materialize)",
		"decision: materialize",
		"slack_search",
		"[sim]",
	} {
		gt.String(t, out).Contains(want)
	}
}

func TestCaseArtifactRender_NoToolCalls(t *testing.T) {
	art := &evaltype.CaseArtifact{Case: &model.Case{Title: "t"}}
	out := art.Render()
	gt.B(t, strings.Contains(out, "# Tool calls\n(none)")).True()
}
