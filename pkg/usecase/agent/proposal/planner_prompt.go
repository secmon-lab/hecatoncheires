package proposal

import (
	"bytes"
	"context"
	_ "embed"
	"sync"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
)

//go:embed prompts/planner.md
var plannerPromptSrc string

var (
	plannerPromptOnce sync.Once
	plannerPromptTmpl *template.Template
	plannerPromptErr  error
)

// plannerPromptTemplate parses the embedded planner prompt template lazily
// and caches the parse result. Callers should pass a plannerPromptInput to
// Execute.
func plannerPromptTemplate() (*template.Template, error) {
	plannerPromptOnce.Do(func() {
		plannerPromptTmpl, plannerPromptErr = template.New("planner").
			Parse(plannerPromptSrc)
	})
	return plannerPromptTmpl, plannerPromptErr
}

// plannerPromptInput is the typed input rendered into the planner prompt
// template. The system prompt only carries the workspace identity tier
// (id / name / description). Field schemas and source lists are not inlined
// — the planner pulls them per-turn via the `get_workspace` tool.
type plannerPromptInput struct {
	// Language is the human-readable label of the locale the user-facing copy
	// (`question.text`, `question.items[].text`, `materialize.title`,
	// `materialize.description`) must be written in. Examples: "English",
	// "Japanese". Empty string disables the directive (the planner is free to
	// pick a language) — used in tests that don't care.
	Language   string
	Workspaces []plannerPromptWorkspace
}

type plannerPromptWorkspace struct {
	ID          string
	Name        string
	Description string
}

// renderPlannerPrompt builds the system prompt string. registry may be nil
// (in which case the prompt advertises no workspaces — the planner can
// still ask questions or post messages). language is the human-readable
// locale label for user-facing copy ("English", "Japanese", ...); pass
// empty to suppress the directive.
func renderPlannerPrompt(registry *model.WorkspaceRegistry, language string) (string, error) {
	tmpl, err := plannerPromptTemplate()
	if err != nil {
		return "", goerr.Wrap(err, "parse planner prompt template")
	}
	in := plannerPromptInput{
		Language:   language,
		Workspaces: workspacePromptEntries(registry),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, in); err != nil {
		return "", goerr.Wrap(err, "render planner prompt")
	}
	return buf.String(), nil
}

// workspacePromptEntries flattens registry into the prompt-template-friendly
// shape — only id / name / description. Returns nil when registry is nil or
// empty so the template's "no workspaces" branch fires.
func workspacePromptEntries(registry *model.WorkspaceRegistry) []plannerPromptWorkspace {
	if registry == nil {
		return nil
	}
	entries := registry.List()
	if len(entries) == 0 {
		return nil
	}
	out := make([]plannerPromptWorkspace, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		out = append(out, plannerPromptWorkspace{
			ID:          e.Workspace.ID,
			Name:        e.Workspace.Name,
			Description: e.Workspace.Description,
		})
	}
	return out
}

// workspaceRegistryCount returns the number of registered workspaces, or 0 if
// the registry is nil. Used by debug logging at planner-prompt render time.
func workspaceRegistryCount(registry *model.WorkspaceRegistry) int {
	if registry == nil {
		return 0
	}
	return len(registry.List())
}

// plannerLanguageLabel resolves the active language from ctx (falling back to
// the package-level default) and returns the human-readable label the prompt
// template embeds in the "Language" directive. Returns empty when neither the
// ctx nor the default is set.
func plannerLanguageLabel(ctx context.Context) string {
	lang := i18n.LangFromContext(ctx)
	if lang == "" {
		lang = i18n.DefaultLang()
	}
	switch lang {
	case i18n.LangJA:
		return "Japanese"
	case i18n.LangEN:
		return "English"
	default:
		return ""
	}
}
