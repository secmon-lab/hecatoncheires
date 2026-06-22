package planexec

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
)

//go:embed prompts/planner.md
var plannerBaseTmpl string

var plannerBaseTemplate = template.Must(template.New("planexec_planner").Parse(plannerBaseTmpl))

// plannerPromptInput is the template data fed into prompts/planner.md.
// Every dynamic value is exposed as a typed field so the template can
// switch sections (`{{ if .AllowQuestion }}` etc.) without resorting to
// Go-side string assembly.
type plannerPromptInput struct {
	// HostPrompt is the upstream caller's base system prompt (proposal
	// puts its workspace / mention guidance here; job puts its case-
	// scoped instructions here).
	HostPrompt string

	// Language is the LanguageLabel from RunRequest. Empty disables the
	// user-facing-language directive.
	Language string

	// KnownToolIDs is enumerated in the prompt so the planner knows
	// which ToolResolver IDs are valid without re-reading the JSON
	// schema. Mirrors RunRequest.KnownToolIDs.
	KnownToolIDs []string

	// AllowQuestion toggles the `question` shape description.
	AllowQuestion bool

	// AllowDirect toggles the round-1 `direct` (answer-without-investigation)
	// shape description. Mirrors RunRequest.AllowDirect.
	AllowDirect bool

	// StructuredFinal is true iff RunRequest.FinalOutputSchema != nil;
	// the planner is told the final-response phase will be JSON-shaped.
	StructuredFinal bool
}

// renderPlannerSystemPrompt builds the planner system prompt by piping
// the host's base prompt + planexec-side loop / schema rules through
// prompts/planner.md.
func renderPlannerSystemPrompt(in plannerPromptInput) (string, error) {
	if in.HostPrompt == "" {
		return "", goerr.New("host prompt is required")
	}
	if len(in.KnownToolIDs) == 0 {
		return "", goerr.New("known tool ids must not be empty")
	}
	var buf bytes.Buffer
	if err := plannerBaseTemplate.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render planner system prompt")
	}
	return buf.String(), nil
}
