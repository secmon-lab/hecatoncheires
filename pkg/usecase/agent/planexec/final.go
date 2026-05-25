package planexec

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
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

// generateFinalResponse makes one additional LLM call after the planner
// loop exits, producing the user-visible terminal output. The shape is
// governed by the host:
//   - schema == nil → plain text in the returned string; rawJSON is nil
//   - schema != nil → raw JSON bytes; the text return is empty
//
// systemPrompt is the planner's system prompt; it is reused so the final
// call inherits the same persona / guidance the planner had. historyKey
// is the gollem.WithHistoryRepository key — passing the same key the
// planner used lets the final call see every observation it gathered.
func generateFinalResponse(
	ctx context.Context,
	llm gollem.LLMClient,
	historyRepo gollem.HistoryRepository,
	traceHandler trace.Handler,
	systemPrompt string,
	historyKey string,
	language string,
	allResults []PhaseSummary,
	schema *gollem.Parameter,
) (text string, rawJSON json.RawMessage, err error) {
	userPrompt, err := renderFinalUserPrompt(finalPromptInput{
		Observations:    renderObservationsForFinal(allResults),
		StructuredFinal: schema != nil,
		Language:        language,
	})
	if err != nil {
		return "", nil, goerr.Wrap(err, "render final user prompt")
	}

	opts := []gollem.Option{
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithHistoryRepository(historyRepo, historyKey),
		// The final-response phase is a single user-prompt → model-reply
		// exchange, but gollem's loop accounting needs one extra slot to
		// detect "no more tool calls" before terminating. Two is the
		// minimum that lets a structured-output round actually return.
		gollem.WithLoopLimit(2),
	}
	if traceHandler != nil {
		opts = append(opts, gollem.WithTrace(traceHandler))
	}
	if schema != nil {
		opts = append(opts,
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(schema),
		)
	}

	agent := gollem.New(llm, opts...)
	resp, execErr := agent.Execute(ctx, gollem.Text(userPrompt))
	if execErr != nil {
		return "", nil, goerr.Wrap(execErr, "execute final response")
	}
	if resp == nil || resp.IsEmpty() {
		return "", nil, goerr.New("final response is empty")
	}

	combined := strings.Join(resp.Texts, "\n")
	if schema != nil {
		body := extractJSONObject([]byte(combined))
		return "", json.RawMessage(body), nil
	}
	return combined, nil, nil
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
