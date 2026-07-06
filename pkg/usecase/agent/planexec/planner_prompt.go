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

	// StructuredFinal is true for a Run[T] turn (the final output is a
	// host-supplied JSON object) and false for RunText / ResumeText (plain
	// text). The entry point decides it, since the output mode is chosen by
	// which Run function the host called, not by a request field.
	StructuredFinal bool

	// AllowSubAgentWrites toggles the "Actions and writes" guidance:
	// when true, the planner is told tasks may perform writes/actions
	// (not only investigation), how to sequence them, and that the final
	// response cannot perform side effects. Mirrors
	// RunRequest.AllowSubAgentWrites.
	AllowSubAgentWrites bool
}

// buildPlannerSystemPrompt maps a RunRequest into the planner system prompt.
// Run / RunText / ResumeText configure the planner identically except for the
// final-output shape, which the entry point passes as structuredFinal (true for
// Run[T], false for the text variants). It uses no Runner state, hence a free
// function rather than a method.
func buildPlannerSystemPrompt(req RunRequest, structuredFinal bool) (string, error) {
	return renderPlannerSystemPrompt(plannerPromptInput{
		HostPrompt:          req.SystemPrompt,
		Language:            req.LanguageLabel,
		KnownToolIDs:        req.KnownToolIDs,
		AllowQuestion:       req.AllowQuestion,
		AllowDirect:         req.AllowDirect,
		StructuredFinal:     structuredFinal,
		AllowSubAgentWrites: req.AllowSubAgentWrites,
	})
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
