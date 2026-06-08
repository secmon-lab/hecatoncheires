package report_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/report"
)

func sampleResult() evaltype.ScenarioResult {
	return evaltype.ScenarioResult{
		ScenarioID: "thread-initial-login",
		EvalID:     "0192f0c1-7a3b-7e10-aaaa-bbbbbbbbbbbb",
		Workflow:   "thread_mode_initial",
		Status:     evaltype.StatusOK,
		Score:      evaltype.Score{Passed: 1, Total: 2},
		Checks: []evaltype.CheckVerdict{
			{ID: "c1", Question: "title?", Passed: true, Reason: "ok"},
			{ID: "c2", Question: "severity?", Passed: false, Reason: "missing"},
		},
		Artifact: &evaltype.CaseArtifact{Case: &model.Case{Title: "Portal 503"}},
	}
}

func TestComputeScore(t *testing.T) {
	s := report.ComputeScore([]evaltype.CheckVerdict{{Passed: true}, {Passed: false}, {Passed: true}})
	gt.Number(t, s.Passed).Equal(2)
	gt.Number(t, s.Total).Equal(3)
}

func TestSummary_ContainsVerdictsAndFooter(t *testing.T) {
	var buf bytes.Buffer
	gt.NoError(t, report.Summary(&buf, []evaltype.ScenarioResult{sampleResult()}, report.SummaryOptions{}))
	out := buf.String()
	for _, want := range []string{"thread-initial-login", "✓ c1", "✗ c2", "checks: 1/2", "suite summary", "1/2 passed"} {
		gt.String(t, out).Contains(want)
	}
}

func TestSummary_Quiet(t *testing.T) {
	var buf bytes.Buffer
	gt.NoError(t, report.Summary(&buf, []evaltype.ScenarioResult{sampleResult()}, report.SummaryOptions{Quiet: true}))
	out := buf.String()
	gt.B(t, strings.Contains(out, "thread-initial-login")).True()
	gt.B(t, strings.Contains(out, "produced artifact")).False() // quiet omits artifact
}

func TestSummary_ErrorScenario(t *testing.T) {
	var buf bytes.Buffer
	r := sampleResult()
	r.Status = evaltype.StatusError
	r.Err = "judge failed"
	gt.NoError(t, report.Summary(&buf, []evaltype.ScenarioResult{r}, report.SummaryOptions{}))
	gt.B(t, strings.Contains(buf.String(), "run: ERROR")).True()
}

func TestWriteJSON_NoOverallPassFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	gt.NoError(t, report.WriteJSON(path, []evaltype.ScenarioResult{sampleResult()}))

	data, err := os.ReadFile(path)
	gt.NoError(t, err)
	var m map[string]any
	gt.NoError(t, json.Unmarshal(data, &m))
	// No top-level overall pass/fail key.
	_, hasPass := m["passed"]
	gt.B(t, hasPass).False()
	_, hasTotals := m["totals"]
	gt.B(t, hasTotals).True()
}
