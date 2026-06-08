// Package report aggregates scenario results and renders them: a human-readable
// stdout summary, a machine-readable JSON file, and per-scenario diagnostic
// dumps (see dump.go). There is deliberately no overall pass/fail field — the
// final OK/NG decision is a human's; the report only presents per-check
// verdicts and an informational pass ratio.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
)

// ComputeScore counts passed checks over total.
func ComputeScore(verdicts []evaltype.CheckVerdict) evaltype.Score {
	s := evaltype.Score{Total: len(verdicts)}
	for _, v := range verdicts {
		if v.Passed {
			s.Passed++
		}
	}
	return s
}

// Totals is the suite-level roll-up.
type Totals struct {
	Scenarios    int `json:"scenarios"`
	RunOK        int `json:"run_ok"`
	Errors       int `json:"errors"`
	ChecksPassed int `json:"checks_passed"`
	ChecksTotal  int `json:"checks_total"`
}

func aggregate(results []evaltype.ScenarioResult) Totals {
	var t Totals
	t.Scenarios = len(results)
	for _, r := range results {
		if r.Status == evaltype.StatusError {
			t.Errors++
			continue
		}
		t.RunOK++
		t.ChecksPassed += r.Score.Passed
		t.ChecksTotal += r.Score.Total
	}
	return t
}

// SummaryOptions controls stdout rendering verbosity.
type SummaryOptions struct {
	Quiet   bool
	Verbose bool
}

// lineWriter accumulates the first write error so the many formatted writes in
// Summary don't each need an explicit error check.
type lineWriter struct {
	w   io.Writer
	err error
}

func (l *lineWriter) printf(format string, a ...any) {
	if l.err != nil {
		return
	}
	_, l.err = fmt.Fprintf(l.w, format, a...)
}

func (l *lineWriter) println(s string) {
	if l.err != nil {
		return
	}
	_, l.err = fmt.Fprintln(l.w, s)
}

// Summary writes the human-readable summary to w.
func Summary(w io.Writer, results []evaltype.ScenarioResult, opts SummaryOptions) error {
	lw := &lineWriter{w: w}
	for _, r := range results {
		if opts.Quiet {
			lw.printf("%-40s %-22s %s checks %d/%d\n", r.ScenarioID, r.Workflow, r.Status, r.Score.Passed, r.Score.Total)
			continue
		}
		lw.println("════════════════════════════════════════════════════════════════")
		lw.printf(" %s        [%s]\n", r.ScenarioID, r.Workflow)
		lw.println("────────────────────────────────────────────────────────────────")
		if r.Status == evaltype.StatusError {
			lw.printf(" run: ERROR   %s\n", r.Err)
			lw.println("════════════════════════════════════════════════════════════════")
			continue
		}
		lw.printf(" run: ok      checks: %d/%d passed\n", r.Score.Passed, r.Score.Total)
		if r.Artifact != nil {
			lw.println("\n produced artifact")
			for _, line := range indent(r.Artifact.Render()) {
				lw.println(line)
			}
		}
		lw.println("\n checks  (judge assessment — final OK/NG is yours)")
		for _, v := range r.Checks {
			mark := "✓"
			if !v.Passed {
				mark = "✗"
			}
			lw.printf("   %s %s\n       %s\n", mark, v.ID, v.Reason)
		}
		if r.DumpDir != "" {
			lw.printf("\n dumped: %s\n", r.DumpDir)
		}
		lw.println("════════════════════════════════════════════════════════════════")
	}

	t := aggregate(results)
	lw.println("\n ── suite summary ──────────────────────────────")
	lw.printf(" scenarios : %d   (ok %d, error %d)\n", t.Scenarios, t.RunOK, t.Errors)
	if t.ChecksTotal > 0 {
		lw.printf(" checks    : %d/%d passed (%d%%)\n", t.ChecksPassed, t.ChecksTotal, t.ChecksPassed*100/t.ChecksTotal)
	}
	lw.println(" not passed:")
	any := false
	for _, r := range results {
		for _, v := range r.Checks {
			if !v.Passed {
				any = true
				lw.printf("   ✗ %s / %s\n", r.ScenarioID, v.ID)
			}
		}
	}
	if !any {
		lw.println("   (none)")
	}
	lw.println(" note: judge's assessments; confirm OK/NG by review.")
	lw.println(" ────────────────────────────────────────────────")

	if lw.err != nil {
		return goerr.Wrap(lw.err, "write summary")
	}
	return nil
}

func indent(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, "   "+s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, "   "+s[start:])
	}
	return out
}

// jsonReport is the on-disk machine-readable shape. No overall pass/fail field.
type jsonReport struct {
	Totals    Totals         `json:"totals"`
	Scenarios []jsonScenario `json:"scenarios"`
}

type jsonScenario struct {
	ID               string                  `json:"id"`
	EvalID           string                  `json:"eval_id"`
	Workflow         string                  `json:"workflow"`
	Status           string                  `json:"status"`
	Err              string                  `json:"error,omitempty"`
	DumpDir          string                  `json:"dump_dir,omitempty"`
	Score            evaltype.Score          `json:"score"`
	Checks           []evaltype.CheckVerdict `json:"checks"`
	ArtifactSnapshot string                  `json:"artifact_snapshot,omitempty"`
}

// WriteJSON writes the machine-readable report to path.
func WriteJSON(path string, results []evaltype.ScenarioResult) error {
	rep := jsonReport{Totals: aggregate(results)}
	for _, r := range results {
		js := jsonScenario{
			ID:       r.ScenarioID,
			EvalID:   r.EvalID,
			Workflow: r.Workflow,
			Status:   r.Status,
			Err:      r.Err,
			DumpDir:  r.DumpDir,
			Score:    r.Score,
			Checks:   r.Checks,
		}
		if r.Artifact != nil {
			js.ArtifactSnapshot = r.Artifact.Render()
		}
		rep.Scenarios = append(rep.Scenarios, js)
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return goerr.Wrap(err, "marshal json report")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return goerr.Wrap(err, "write json report", goerr.V("path", path))
	}
	return nil
}
