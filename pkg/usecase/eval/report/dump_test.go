package report_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/report"
)

func TestDump_WritesBundle(t *testing.T) {
	dir := t.TempDir()
	r := sampleResult()
	traces := map[string]json.RawMessage{"sess/trace": json.RawMessage(`{"trace":1}`)}

	out, err := report.Dump(dir, r, traces, "en")
	gt.NoError(t, err)
	gt.String(t, out).Contains(r.EvalID)

	for _, f := range []string{"trace.json", "run.json", "analysis.md"} {
		_, statErr := os.Stat(filepath.Join(out, f))
		gt.NoError(t, statErr)
	}

	analysis, err := os.ReadFile(filepath.Join(out, "analysis.md"))
	gt.NoError(t, err)
	gt.String(t, string(analysis)).Contains("FAIL] c2")
}

func TestDump_LanguageDirective(t *testing.T) {
	dir := t.TempDir()
	out, err := report.Dump(dir, sampleResult(), nil, "ja")
	gt.NoError(t, err)
	analysis, err := os.ReadFile(filepath.Join(out, "analysis.md"))
	gt.NoError(t, err)
	gt.String(t, string(analysis)).Contains("Respond in Japanese")
}
