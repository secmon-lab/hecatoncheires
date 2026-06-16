// Package judge evaluates a produced artifact against a scenario's checklist.
// All checks are scored in a single LLM call returning a per-check verdict
// (pass/fail + reason). The verdicts are the judge's assessment only; the final
// OK/NG decision is left to a human reviewer, so there is no automatic gate
// here — just per-check verdicts plus an informational pass ratio.
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/evaltype"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/eval/scenario"
)

// Judge scores checklists against artifacts.
type Judge struct {
	completer evaltype.Completer
	language  string
}

// New builds a Judge. language is the language the verdict reasons are written
// in (the eval output language, distinct from the agent's conversation
// language).
func New(completer evaltype.Completer, language string) *Judge {
	return &Judge{completer: completer, language: language}
}

// Evaluate scores every check against the artifact in one call and returns the
// verdicts in the same order as checks. A check id missing from the model's
// output is an error (the judge must address every check); unknown ids are
// ignored.
func (j *Judge) Evaluate(ctx context.Context, art evaltype.Artifact, checks []scenario.Check) ([]evaltype.CheckVerdict, error) {
	if len(checks) == 0 {
		return nil, goerr.New("no checks to evaluate")
	}

	raw, err := j.completer.Complete(ctx, j.systemPrompt(), buildUserPrompt(art, checks), verdictSchema())
	if err != nil {
		return nil, goerr.Wrap(err, "judge completion")
	}

	var parsed struct {
		Verdicts []struct {
			ID     string `json:"id"`
			Passed bool   `json:"passed"`
			Reason string `json:"reason"`
		} `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, goerr.Wrap(err, "decode judge verdicts", goerr.V("raw_len", len(raw)))
	}

	byID := make(map[string]struct {
		passed bool
		reason string
	}, len(parsed.Verdicts))
	for _, v := range parsed.Verdicts {
		byID[v.ID] = struct {
			passed bool
			reason string
		}{v.Passed, v.Reason}
	}

	out := make([]evaltype.CheckVerdict, 0, len(checks))
	for _, c := range checks {
		v, ok := byID[c.ID]
		if !ok {
			return nil, goerr.New("judge did not return a verdict for check", goerr.V("check_id", c.ID))
		}
		out = append(out, evaltype.CheckVerdict{
			ID:       c.ID,
			Question: c.Question,
			Passed:   v.passed,
			Reason:   v.reason,
		})
	}
	return out, nil
}

func (j *Judge) systemPrompt() string {
	var b strings.Builder
	b.WriteString("You are an evaluation judge. You are given a snapshot of an artifact an AI agent produced (a case with fields, the conversation transcript, and the tool calls it made), and a checklist of yes/no questions. Answer each check independently and honestly based ONLY on the snapshot.\n\n")
	b.WriteString("# Rules\n")
	b.WriteString("- Judge each check on its own merits; a question about a field/status/tool-call is decidable from the explicit snapshot state.\n")
	b.WriteString("- Do not reward verbosity or length, and do not credit unsupported claims or citations.\n")
	b.WriteString("- For each check return passed=true only if the snapshot clearly satisfies it; otherwise passed=false with a short reason.\n")
	b.WriteString("- Return exactly one verdict per check id provided.\n")
	if j.language != "" {
		fmt.Fprintf(&b, "- Write every reason in %s.\n", j.language)
	}
	return b.String()
}

func buildUserPrompt(art evaltype.Artifact, checks []scenario.Check) string {
	var b strings.Builder
	b.WriteString("# Artifact snapshot\n")
	b.WriteString(art.Render())
	b.WriteString("\n# Checks\n")
	for _, c := range checks {
		fmt.Fprintf(&b, "- id=%s: %s\n", c.ID, c.Question)
	}
	b.WriteString("\nReturn a verdict (passed + reason) for every check id above.")
	return b.String()
}

func verdictSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type:        gollem.TypeObject,
		Description: "Per-check verdicts.",
		Properties: map[string]*gollem.Parameter{
			"verdicts": {
				Type:        gollem.TypeArray,
				Description: "One entry per check id.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"id":     {Type: gollem.TypeString, Description: "The check id.", Required: true},
						"passed": {Type: gollem.TypeBoolean, Description: "Whether the artifact satisfies the check.", Required: true},
						"reason": {Type: gollem.TypeString, Description: "Short justification.", Required: true},
					},
				},
			},
		},
	}
}
