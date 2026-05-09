package draft

import (
	"bytes"
	"context"
	_ "embed"
	"strings"
	"sync"
	"text/template"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
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
// template. It captures the workspace registry + each workspace's custom
// field schema so the planner sees the exact set of valid field IDs and
// option IDs while choosing a `materialize` payload.
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
	ID             string
	Name           string
	Description    string
	RequiredFields []plannerPromptField
	OptionalFields []plannerPromptField
}

type plannerPromptField struct {
	ID          string
	Name        string
	Description string
	Type        string
	// OptionList is a pre-formatted comma-separated list of option IDs for
	// select / multi-select fields. Empty for free-form types.
	OptionList string
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
// shape, splitting required and optional fields and pre-rendering option
// lists. Returns nil when registry is nil or empty.
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
		ws := plannerPromptWorkspace{
			ID:          e.Workspace.ID,
			Name:        e.Workspace.Name,
			Description: e.Workspace.Description,
		}
		if e.FieldSchema != nil {
			for _, fd := range e.FieldSchema.Fields {
				pf := plannerPromptField{
					ID:          fd.ID,
					Name:        fd.Name,
					Description: fd.Description,
					Type:        string(fd.Type),
					OptionList:  formatOptionList(fd),
				}
				if fd.Required {
					ws.RequiredFields = append(ws.RequiredFields, pf)
				} else {
					ws.OptionalFields = append(ws.OptionalFields, pf)
				}
			}
		}
		out = append(out, ws)
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

// formatOptionList renders the field's allowed option IDs (with names when
// present) as a comma-separated string, e.g. `low (Low), high (High)`. Empty
// for free-form field types.
func formatOptionList(fd config.FieldDefinition) string {
	if len(fd.Options) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fd.Options))
	for _, opt := range fd.Options {
		if opt.Name != "" && opt.Name != opt.ID {
			parts = append(parts, opt.ID+" ("+opt.Name+")")
		} else {
			parts = append(parts, opt.ID)
		}
	}
	return strings.Join(parts, ", ")
}
