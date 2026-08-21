package planexec

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/direct.md
var directPromptTmpl string

var directPromptTemplate = template.Must(template.New("planexec_direct").Parse(directPromptTmpl))

// directPromptInput is the data fed into prompts/direct.md.
type directPromptInput struct {
	// HostPrompt is the host's base persona prompt (Input.SystemPrompt), NOT the
	// rendered planner prompt: the planner prompt mandates JSON-only output and
	// forbids prose, which contradicts the plain-text reply this path produces.
	HostPrompt string
	// Language is the LanguageLabel from Input. Empty omits the user-facing-language
	// directive. It matters more here than anywhere else in the run: this text is
	// the only thing the user sees, and nothing downstream translates it.
	Language string
	// Context mirrors Input.TaskContext — the identifiers the run's tools are pinned
	// to. The direct agent may be granted tools (DirectPlan.Tools), so it needs them
	// for the same reason a sub-agent does.
	Context string
}

// buildDirectSystemPrompt renders prompts/direct.md into the system prompt of the
// single child that answers on the round-1 direct fast path.
//
// It exists because that child's text is published to the user verbatim
// (stepCollect), so it must be prompted as the author of a user-facing message.
// Prompting it as an investigation sub-agent instead — which is what happened
// between #261 and this function, when launchDirect reused buildSubAgentSystemPrompt
// — makes it write to the sub-agent's output rules: a one-line conclusion, a
// supporting-evidence section, and prose addressed to "the parent planner". That
// whole report was then posted into the Slack thread as though it were the answer.
func buildDirectSystemPrompt(in directPromptInput) (string, error) {
	if in.HostPrompt == "" {
		return "", goerr.New("host prompt is required")
	}
	var buf bytes.Buffer
	if err := directPromptTemplate.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render the direct system prompt")
	}
	return buf.String(), nil
}
