package proposal

import (
	"context"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
)

// plannerPromptWorkspace is the identity tier of one workspace as the case-draft
// prompt renders it. Field schemas and source lists are deliberately absent: the
// agent pulls those per turn through get_workspace, so the prompt stays short
// however many workspaces are registered.
type plannerPromptWorkspace struct {
	ID          string
	Name        string
	Description string
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

// plannerLanguageLabel resolves the label planexec embeds in its
// user-facing-language directive. The mapping is shared with the other planexec
// hosts (i18n.LanguageLabel) so a host cannot drift into a different answer.
func plannerLanguageLabel(ctx context.Context) string {
	return i18n.LanguageLabel(ctx)
}
