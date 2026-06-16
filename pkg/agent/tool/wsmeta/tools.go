// Package wsmeta exposes read-only tools that report on the WorkspaceRegistry
// — the registered workspaces, their custom field schemas, and their
// configured external sources. Used by the draft-mode planner so it can
// inspect a workspace's exact field IDs / option IDs (and the sources it can
// reach) before committing to a materialise payload, instead of having the
// entire registry inlined into the system prompt.
package wsmeta

import (
	"context"
	"fmt"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// Deps groups the read-only collaborators wsmeta tools need.
type Deps struct {
	// Registry is the active workspace registry. Required; the tools return
	// an empty list / a hard error when nil so a misconfigured wiring is
	// surfaced loudly rather than silently degrading the planner.
	Registry *model.WorkspaceRegistry
	// SourceRepo is consulted by get_workspace to enumerate the workspace's
	// external sources. Optional: when nil, get_workspace returns an empty
	// `sources` array (the planner can still drive question / materialize).
	SourceRepo interfaces.SourceRepository
}

// New builds the planner-side workspace metadata tools: list_workspaces and
// get_workspace. Both are read-only and never touch repository write methods.
func New(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&listWorkspacesTool{registry: deps.Registry},
		&getWorkspaceTool{registry: deps.Registry, sourceRepo: deps.SourceRepo},
	}
}

// listWorkspacesTool reports id / name / description for every registered
// workspace. The system prompt already advertises this list, so the planner
// usually does not need to call this tool — it exists as a backup when the
// prompt was truncated or the planner wants to verify the registry state.
type listWorkspacesTool struct {
	registry *model.WorkspaceRegistry
}

func (t *listWorkspacesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "list_workspaces",
		Description: "List the id, name and description of every registered workspace. The system prompt already advertises this list, so call this only when you need to re-check the registry.",
		Parameters:  map[string]*gollem.Parameter{},
	}
}

func (t *listWorkspacesTool) Run(ctx context.Context, _ map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing workspaces...")
	if t.registry == nil {
		return map[string]any{"workspaces": []map[string]any{}}, nil
	}
	entries := t.registry.List()
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e == nil {
			continue
		}
		items = append(items, map[string]any{
			"id":          e.Workspace.ID,
			"name":        e.Workspace.Name,
			"description": e.Workspace.Description,
		})
	}
	return map[string]any{"workspaces": items}, nil
}

// getWorkspaceTool returns the workspace identity, its complete custom field
// schema (with select / multi-select option descriptions and metadata), and
// its configured external sources in a single payload. The planner MUST call
// this before emitting a materialise action so it knows the exact field IDs
// and option IDs to use.
type getWorkspaceTool struct {
	registry   *model.WorkspaceRegistry
	sourceRepo interfaces.SourceRepository
}

func (t *getWorkspaceTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "get_workspace",
		Description: "Return a workspace's identity, its complete custom field schema (each select / multi-select option carries its description and any metadata), and its configured external sources. Call this before materialising so you fill custom_field_values with the correct field IDs and option IDs.",
		Parameters: map[string]*gollem.Parameter{
			"workspace_id": {
				Type:        gollem.TypeString,
				Description: "The id of the workspace to inspect (one of the workspaces advertised in the system prompt).",
				Required:    true,
			},
		},
	}
}

func (t *getWorkspaceTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	wsID, _ := args["workspace_id"].(string)
	if wsID == "" {
		return nil, goerr.New("workspace_id is required")
	}
	tool.Update(ctx, fmt.Sprintf("Fetching workspace %s...", wsID))
	if t.registry == nil {
		return nil, goerr.New("workspace registry is not configured")
	}
	entry, err := t.registry.Get(wsID)
	if err != nil {
		return nil, goerr.Wrap(err, "lookup workspace", goerr.V("workspace_id", wsID))
	}

	out := map[string]any{
		"id":          entry.Workspace.ID,
		"name":        entry.Workspace.Name,
		"description": entry.Workspace.Description,
		"fields":      fieldsToMaps(entry.FieldSchema),
	}

	var sources []*model.Source
	if t.sourceRepo != nil {
		listed, listErr := t.sourceRepo.List(ctx, wsID)
		if listErr != nil {
			return nil, goerr.Wrap(listErr, "list sources", goerr.V("workspace_id", wsID))
		}
		sources = listed
	}
	out["sources"] = sourcesToMaps(sources)
	return out, nil
}

// fieldsToMaps flattens a FieldSchema to the planner-facing JSON shape. Each
// option carries `description` (always present, even when empty) so the
// planner can pick select / multi-select values on substance, not on guessing
// from the option ID alone. `metadata` is included only when non-empty to
// keep the payload tight.
func fieldsToMaps(schema *config.FieldSchema) []map[string]any {
	if schema == nil || len(schema.Fields) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(schema.Fields))
	for _, fd := range schema.Fields {
		field := map[string]any{
			"id":          fd.ID,
			"name":        fd.Name,
			"type":        string(fd.Type),
			"required":    fd.Required,
			"description": fd.Description,
		}
		if len(fd.Options) > 0 {
			opts := make([]map[string]any, 0, len(fd.Options))
			for _, opt := range fd.Options {
				o := map[string]any{
					"id":          opt.ID,
					"name":        opt.Name,
					"description": opt.Description,
				}
				if len(opt.Metadata) > 0 {
					o["metadata"] = opt.Metadata
				}
				opts = append(opts, o)
			}
			field["options"] = opts
		}
		out = append(out, field)
	}
	return out
}

// sourcesToMaps flattens the workspace's source list to the planner-facing
// JSON shape. `config` is inlined per source type (notion_db / notion_page /
// slack / github) so the planner does not need a second round-trip to learn
// what each source actually points at.
func sourcesToMaps(sources []*model.Source) []map[string]any {
	if len(sources) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(sources))
	for _, s := range sources {
		if s == nil {
			continue
		}
		item := map[string]any{
			"id":          string(s.ID),
			"name":        s.Name,
			"type":        string(s.SourceType),
			"description": s.Description,
			"enabled":     s.Enabled,
		}
		if cfg := sourceConfigToMap(s); cfg != nil {
			item["config"] = cfg
		}
		out = append(out, item)
	}
	return out
}

func sourceConfigToMap(s *model.Source) map[string]any {
	switch s.SourceType {
	case model.SourceTypeNotionDB:
		if s.NotionDBConfig == nil {
			return nil
		}
		return map[string]any{
			"database_id":    s.NotionDBConfig.DatabaseID,
			"database_title": s.NotionDBConfig.DatabaseTitle,
			"database_url":   s.NotionDBConfig.DatabaseURL,
		}
	case model.SourceTypeNotionPage:
		if s.NotionPageConfig == nil {
			return nil
		}
		return map[string]any{
			"page_id":    s.NotionPageConfig.PageID,
			"page_title": s.NotionPageConfig.PageTitle,
			"page_url":   s.NotionPageConfig.PageURL,
			"recursive":  s.NotionPageConfig.Recursive,
			"max_depth":  s.NotionPageConfig.MaxDepth,
		}
	case model.SourceTypeSlack:
		if s.SlackConfig == nil {
			return nil
		}
		channels := make([]map[string]any, 0, len(s.SlackConfig.Channels))
		for _, ch := range s.SlackConfig.Channels {
			channels = append(channels, map[string]any{
				"id":   ch.ID,
				"name": ch.Name,
			})
		}
		return map[string]any{"channels": channels}
	case model.SourceTypeGitHub:
		if s.GitHubConfig == nil {
			return nil
		}
		repos := make([]map[string]any, 0, len(s.GitHubConfig.Repositories))
		for _, r := range s.GitHubConfig.Repositories {
			repos = append(repos, map[string]any{
				"owner": r.Owner,
				"repo":  r.Repo,
			})
		}
		return map[string]any{"repositories": repos}
	default:
		return nil
	}
}
