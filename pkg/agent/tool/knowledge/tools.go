// Package knowledge exposes the workspace-wide shared-knowledge gollem tools
// available to agents. Read tools (search / get / list_tags) are always offered;
// write tools (create / update) are wired only when the agent is permitted to
// mutate shared knowledge (i.e. not while processing a private case). Every
// operation routes through the KnowledgeUseCase surface (Accessor / Mutator) so
// embedding, validation, and tag normalization match the WebUI path.
package knowledge

import (
	"context"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// KnowledgeAccessor is the read surface the knowledge tools depend on. Defined
// here so the package does not import pkg/usecase (which would create a cycle).
type KnowledgeAccessor interface {
	SearchKnowledge(ctx context.Context, workspaceID, query string, tags []string, limit int) ([]*model.Knowledge, error)
	GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error)
	ListTags(ctx context.Context, workspaceID string) ([]string, error)
}

// KnowledgeMutator is the write surface the knowledge tools depend on.
type KnowledgeMutator interface {
	CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tags []string) (*model.Knowledge, error)
	UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tags *[]string) (*model.Knowledge, error)
}

// defaultSearchLimit bounds how many entries the search tool returns when the
// agent does not specify a limit. Kept here (not in the usecase) because it is a
// tool-presentation default, set at this call site per the project convention.
const defaultSearchLimit = 10

// Deps groups the dependencies the knowledge tools need. WorkspaceID is pinned
// at construction. Accessor is required (read tools). Mutator is required only
// for the write tools (New); it is nil for NewReadOnly.
type Deps struct {
	WorkspaceID string
	Accessor    KnowledgeAccessor
	Mutator     KnowledgeMutator
}

// New builds the full knowledge tool set (read + create/update). Use this when
// the agent is allowed to write shared knowledge.
func New(deps Deps) []gollem.Tool {
	tools := NewReadOnly(deps)
	return append(tools,
		&createKnowledgeTool{deps: deps},
		&updateKnowledgeTool{deps: deps},
	)
}

// NewReadOnly builds the read-only subset (search / get / list_tags). Used when
// the agent may consult shared knowledge but must not mutate it — e.g. while
// processing a private case, whose contents must not leak into the shared base.
func NewReadOnly(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&searchKnowledgeTool{deps: deps},
		&getKnowledgeTool{deps: deps},
		&listTagsTool{deps: deps},
	}
}

// knowledgeToMap renders a knowledge entry for a tool response. The embedding is
// intentionally omitted.
func knowledgeToMap(k *model.Knowledge) map[string]any {
	tags := k.Tags
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{
		"id":         string(k.ID),
		"title":      k.Title,
		"claim":      k.Claim,
		"tags":       tags,
		"created_at": k.CreatedAt.Format(time.RFC3339),
		"updated_at": k.UpdatedAt.Format(time.RFC3339),
	}
}

type searchKnowledgeTool struct{ deps Deps }

func (t *searchKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__search_knowledge",
		Description: "Search the workspace-wide shared knowledge base for entries relevant to a " +
			"query. Knowledge captures organization-specific facts / rules / decisions that are not " +
			"in your general knowledge. Results are ranked by semantic relevance (falling back to " +
			"keyword match). Optionally pre-filter by tags (AND).",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "Natural-language search query.",
				Required:    true,
			},
			"tags": {
				Type:        gollem.TypeArray,
				Description: "Optional tags to AND-filter candidates before ranking.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
			},
			"limit": {
				Type:        gollem.TypeNumber,
				Description: "Maximum number of entries to return. Defaults to 10.",
			},
		},
	}
}

func (t *searchKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Searching knowledge...")
	query, err := requireString(args, "query")
	if err != nil {
		return nil, err
	}
	tags, err := optionalStringSlice(args, "tags")
	if err != nil {
		return nil, err
	}
	limit := defaultSearchLimit
	if v, ok := args["limit"]; ok && v != nil {
		if f, ok := v.(float64); ok && int(f) > 0 {
			limit = int(f)
		}
	}
	items, err := t.deps.Accessor.SearchKnowledge(ctx, t.deps.WorkspaceID, query, tags, limit)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to search knowledge", goerr.V("workspace_id", t.deps.WorkspaceID))
	}
	out := make([]map[string]any, len(items))
	for i, k := range items {
		out[i] = knowledgeToMap(k)
	}
	return map[string]any{"knowledge": out}, nil
}

type getKnowledgeTool struct{ deps Deps }

func (t *getKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "knowledge__get_knowledge",
		Description: "Get the full details (title, Markdown claim, tags) of one shared knowledge entry by id.",
		Parameters: map[string]*gollem.Parameter{
			"knowledge_id": {
				Type:        gollem.TypeString,
				Description: "The id (UUID) of the knowledge entry.",
				Required:    true,
			},
		},
	}
}

func (t *getKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Getting knowledge...")
	id, err := extractKnowledgeID(args)
	if err != nil {
		return nil, err
	}
	k, err := t.deps.Accessor.GetKnowledge(ctx, t.deps.WorkspaceID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get knowledge",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("knowledge_id", id))
	}
	return knowledgeToMap(k), nil
}

type listTagsTool struct{ deps Deps }

func (t *listTagsTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "knowledge__list_tags",
		Description: "List the distinct tags used across the workspace knowledge base. Useful before creating or searching to reuse existing tags.",
		Parameters:  map[string]*gollem.Parameter{},
	}
}

func (t *listTagsTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing knowledge tags...")
	tags, err := t.deps.Accessor.ListTags(ctx, t.deps.WorkspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list knowledge tags", goerr.V("workspace_id", t.deps.WorkspaceID))
	}
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{"tags": tags}, nil
}

type createKnowledgeTool struct{ deps Deps }

func (t *createKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__create_knowledge",
		Description: "Create a new shared knowledge entry for the workspace. Use this to record " +
			"organization-specific knowledge worth reusing on future cases (operating rules, proper " +
			"nouns, past decisions, threat intel). The claim is a Markdown body. At least one tag is required.",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "A concise one-line title.",
				Required:    true,
			},
			"claim": {
				Type:        gollem.TypeString,
				Description: "The knowledge body as Markdown (headings, lists, code, links allowed).",
			},
			"tags": {
				Type:        gollem.TypeArray,
				Description: "One or more tags for classification (at least one required).",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
				Required:    true,
			},
		},
	}
}

func (t *createKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Creating knowledge...")
	if t.deps.Mutator == nil {
		return nil, goerr.New("knowledge: write is not permitted in this context")
	}
	title, err := requireString(args, "title")
	if err != nil {
		return nil, err
	}
	claim := optionalString(args, "claim")
	tags, err := optionalStringSlice(args, "tags")
	if err != nil {
		return nil, err
	}
	created, err := t.deps.Mutator.CreateKnowledge(ctx, t.deps.WorkspaceID, title, claim, tags)
	if err != nil {
		return nil, goerr.Wrap(err, "create knowledge", goerr.V("workspace_id", t.deps.WorkspaceID))
	}
	return knowledgeToMap(created), nil
}

type updateKnowledgeTool struct{ deps Deps }

func (t *updateKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__update_knowledge",
		Description: "Update an existing shared knowledge entry. Submit only the fields you intend to " +
			"change; omitted fields are preserved. Title and claim are full replacements when provided; " +
			"tags, when provided, replace the whole tag set (and must be non-empty).",
		Parameters: map[string]*gollem.Parameter{
			"knowledge_id": {
				Type:        gollem.TypeString,
				Description: "The id (UUID) of the knowledge entry to update.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "New title (full replacement). Omit to preserve.",
			},
			"claim": {
				Type:        gollem.TypeString,
				Description: "New Markdown claim body (full replacement). Omit to preserve.",
			},
			"tags": {
				Type:        gollem.TypeArray,
				Description: "New tag set (full replacement, must be non-empty). Omit to preserve.",
				Items:       &gollem.Parameter{Type: gollem.TypeString},
			},
		},
	}
}

func (t *updateKnowledgeTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating knowledge...")
	if t.deps.Mutator == nil {
		return nil, goerr.New("knowledge: write is not permitted in this context")
	}
	id, err := extractKnowledgeID(args)
	if err != nil {
		return nil, err
	}

	var titlePtr *string
	if v, ok := args["title"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("title must be a string", goerr.V("type", typeOf(v)))
		}
		titlePtr = &s
	}
	var claimPtr *string
	if v, ok := args["claim"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("claim must be a string", goerr.V("type", typeOf(v)))
		}
		claimPtr = &s
	}
	var tagsPtr *[]string
	if v, ok := args["tags"]; ok && v != nil {
		ss, err := toStringSlice(v)
		if err != nil {
			return nil, goerr.Wrap(err, "tags invalid")
		}
		tagsPtr = &ss
	}

	updated, err := t.deps.Mutator.UpdateKnowledge(ctx, t.deps.WorkspaceID, id, titlePtr, claimPtr, tagsPtr)
	if err != nil {
		return nil, goerr.Wrap(err, "update knowledge",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("knowledge_id", id))
	}
	return knowledgeToMap(updated), nil
}

func extractKnowledgeID(args map[string]any) (model.KnowledgeID, error) {
	v, ok := args["knowledge_id"]
	if !ok || v == nil {
		return "", goerr.New("knowledge_id is required")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", goerr.New("knowledge_id must be a non-empty string", goerr.V("type", typeOf(v)))
	}
	return model.KnowledgeID(s), nil
}

func requireString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", goerr.New(key + " is required")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", goerr.New(key+" must be a non-empty string", goerr.V("type", typeOf(v)))
	}
	return s, nil
}

func optionalString(args map[string]any, key string) string {
	if v, ok := args[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func optionalStringSlice(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	return toStringSlice(v)
}

func toStringSlice(v any) ([]string, error) {
	switch a := v.(type) {
	case []string:
		return a, nil
	case []any:
		out := make([]string, 0, len(a))
		for _, item := range a {
			s, ok := item.(string)
			if !ok {
				return nil, goerr.New("array item must be string", goerr.V("type", typeOf(item)))
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, goerr.New("value must be an array of strings", goerr.V("type", typeOf(v)))
	}
}

func typeOf(v any) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", v)
}
