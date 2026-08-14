package planexec

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/final.md
var finalUserPromptTmpl string

var finalUserPromptTemplate = template.Must(template.New("planexec_final_user").Parse(finalUserPromptTmpl))

// finalPromptInput is the typed data passed into prompts/final.md.
type finalPromptInput struct {
	Observations    string
	StructuredFinal bool
	Language        string
}

// finalOutputMaxRetry bounds how many times the terminal output is re-asked of
// the LLM after a decode / Validate / finalizer failure before the run gives up.
// Mirrors gollem's defaultMaxRetry (3 attempts total). These retries regenerate
// the terminal output only; they do NOT re-enter the planner loop (that is what
// the step budget is for).
const finalOutputMaxRetry = 2

// finalRetryInput is the correction message sent to the final-output LLM after
// a decode / Validate failure.
func finalRetryInput(cause error) string {
	return "Your previous final output was rejected: " + cause.Error() +
		". Please re-emit a JSON object that matches the schema and satisfies every requirement."
}

// renderFinalUserPrompt executes prompts/final.md.
func renderFinalUserPrompt(in finalPromptInput) (string, error) {
	var buf bytes.Buffer
	if err := finalUserPromptTemplate.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render final user prompt")
	}
	return buf.String(), nil
}

// renderObservationsForFinal collapses all phases into one observation
// trail string for the final LLM call. We fold every phase's results so
// the final-response LLM has the full picture in one prompt, regardless
// of how many planner rounds the loop took.
func renderObservationsForFinal(allResults []PhaseSummary) string {
	if len(allResults) == 0 {
		return "(no investigations were run before the loop exited)"
	}
	var b strings.Builder
	for _, ps := range allResults {
		fmt.Fprintf(&b, "## Phase %d\n\n", ps.Phase)
		b.WriteString(formatObservationsAsUserTurn(ps.Tasks, ps.Results))
		b.WriteString("\n")
	}
	return b.String()
}
