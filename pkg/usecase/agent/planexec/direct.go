package planexec

import (
	"bytes"
	"context"
	_ "embed"
	"strings"
	"text/template"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/direct.md
var directUserPromptTmpl string

var directUserPromptTemplate = template.Must(template.New("planexec_direct_user").Parse(directUserPromptTmpl))

// directPromptInput is the typed data passed into prompts/direct.md.
type directPromptInput struct {
	UserInput string
	Language  string
}

// generateDirectResponse runs the round-1 direct path: a single tool-enabled
// ReAct loop that answers the user's request without any investigation
// phase. Unlike generateFinalResponse it is plain-text only — no response
// schema, no structured output — because the direct path is reserved for
// trivial requests that need no structured terminal action (see
// prompts/planner.md "Direct answer").
//
// systemPrompt is the host's base persona prompt (RunRequest.SystemPrompt),
// NOT the rendered planner prompt: the planner prompt mandates JSON-only
// output and forbids prose, which would contradict the plain-text reply this
// path produces. historyKey is the shared gollem.WithHistoryRepository key so
// the direct agent sees the prior conversation. tools is the resolved
// DirectPlan.Tools set (may be empty for a pure conversational reply).
// loopLimit bounds the ReAct loop so a tool-using direct reply has room to
// call a tool and then answer.
func generateDirectResponse(
	ctx context.Context,
	llm gollem.LLMClient,
	historyRepo gollem.HistoryRepository,
	traceHandler trace.Handler,
	systemPrompt string,
	historyKey string,
	language string,
	userInput string,
	tools []gollem.Tool,
	loopLimit int,
) (string, error) {
	userPrompt, err := renderDirectUserPrompt(directPromptInput{
		UserInput: userInput,
		Language:  language,
	})
	if err != nil {
		return "", goerr.Wrap(err, "render direct user prompt")
	}

	opts := []gollem.Option{
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithHistoryRepository(historyRepo, historyKey),
		gollem.WithTools(tools...),
		gollem.WithLoopLimit(loopLimit),
		gollem.WithPromptCache(true),
	}
	if traceHandler != nil {
		opts = append(opts, gollem.WithTrace(traceHandler))
	}

	agent := gollem.New(llm, opts...)
	resp, execErr := agent.Execute(ctx, gollem.Text(userPrompt))
	if execErr != nil {
		return "", goerr.Wrap(execErr, "execute direct response")
	}
	if resp == nil || resp.IsEmpty() {
		return "", goerr.New("direct response is empty")
	}
	return strings.Join(resp.Texts, "\n"), nil
}

// renderDirectUserPrompt executes prompts/direct.md.
func renderDirectUserPrompt(in directPromptInput) (string, error) {
	var buf bytes.Buffer
	if err := directUserPromptTemplate.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render direct user prompt")
	}
	return buf.String(), nil
}
