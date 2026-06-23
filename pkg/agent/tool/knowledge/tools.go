// Package knowledge exposes the workspace-wide shared-knowledge gollem tools
// available to agents. Read tools (search / get / list_tags) are always offered;
// write tools (create_tag / update_tag / delete_tag / create_knowledge /
// update_knowledge) are wired only when the agent is permitted to mutate shared
// knowledge (i.e. not while processing a private case). Every operation routes
// through the use-case surface (Accessor / Mutator) so embedding, validation,
// and tag-existence checks match the WebUI path.
//
// Tags are first-class entities identified by an immutable id. A knowledge entry
// references tags ONLY by id and must carry at least one. A tag cannot be
// created inline while writing knowledge — it must be created first via
// create_tag (which returns its id).
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
	SearchKnowledge(ctx context.Context, workspaceID, query string, tagIDs []model.TagID, limit int) ([]*model.Knowledge, error)
	GetKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID) (*model.Knowledge, error)
	ListTags(ctx context.Context, workspaceID string) ([]*model.Tag, error)
}

// KnowledgeMutator is the write surface the knowledge tools depend on.
type KnowledgeMutator interface {
	CreateTag(ctx context.Context, workspaceID, name string) (*model.Tag, error)
	UpdateTag(ctx context.Context, workspaceID string, id model.TagID, name string) (*model.Tag, error)
	DeleteTag(ctx context.Context, workspaceID string, id model.TagID) error
	CreateKnowledge(ctx context.Context, workspaceID, title, claim string, tagIDs []model.TagID) (*model.Knowledge, error)
	UpdateKnowledge(ctx context.Context, workspaceID string, id model.KnowledgeID, title, claim *string, tagIDs *[]model.TagID) (*model.Knowledge, error)
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

// New builds the full knowledge tool set (read + tag/knowledge writes). Use this
// when the agent is allowed to write shared knowledge.
func New(deps Deps) []gollem.Tool {
	tools := NewReadOnly(deps)
	return append(tools,
		&createTagTool{deps: deps},
		&updateTagTool{deps: deps},
		&deleteTagTool{deps: deps},
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
// intentionally omitted. Tags are surfaced as their ids (tag_ids); call
// list_tags to resolve ids to names.
func knowledgeToMap(k *model.Knowledge) map[string]any {
	ids := make([]string, 0, len(k.TagIDs))
	for _, id := range k.TagIDs {
		ids = append(ids, string(id))
	}
	return map[string]any{
		"id":         string(k.ID),
		"title":      k.Title,
		"claim":      k.Claim,
		"tag_ids":    ids,
		"created_at": k.CreatedAt.Format(time.RFC3339),
		"updated_at": k.UpdatedAt.Format(time.RFC3339),
	}
}

// tagToMap renders a tag for a tool response.
func tagToMap(t *model.Tag) map[string]any {
	return map[string]any{
		"id":         string(t.ID),
		"name":       t.Name,
		"created_at": t.CreatedAt.Format(time.RFC3339),
		"updated_at": t.UpdatedAt.Format(time.RFC3339),
	}
}

type searchKnowledgeTool struct{ deps Deps }

func (t *searchKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__search_knowledge",
		Description: "Search the workspace-wide shared knowledge base for entries relevant to a " +
			"query. Knowledge captures organization-specific facts / rules / decisions that are not " +
			"in your general knowledge. Results are ranked by semantic relevance (falling back to " +
			"keyword match). Each result lists its tag_ids; use list_tags to resolve ids to names. " +
			"Optionally pre-filter by tag_ids (AND).",
		Parameters: map[string]*gollem.Parameter{
			"query": {
				Type:        gollem.TypeString,
				Description: "Natural-language search query.",
				Required:    true,
			},
			"tag_ids": {
				Type:        gollem.TypeArray,
				Description: "Optional tag ids to AND-filter candidates before ranking.",
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
	tagIDs, err := optionalTagIDSlice(args, "tag_ids")
	if err != nil {
		return nil, err
	}
	limit := defaultSearchLimit
	if v, ok := args["limit"]; ok && v != nil {
		if f, ok := v.(float64); ok && int(f) > 0 {
			limit = int(f)
		}
	}
	items, err := t.deps.Accessor.SearchKnowledge(ctx, t.deps.WorkspaceID, query, tagIDs, limit)
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
		Description: "Get the full details (title, Markdown claim, tag_ids) of one shared knowledge entry by id.",
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
		Name: "knowledge__list_tags",
		Description: "List the tags defined in the workspace, each with its id and (optional) name. " +
			"Always call this before creating a tag or tagging knowledge so you can reuse an existing " +
			"tag id instead of creating a duplicate.",
		Parameters: map[string]*gollem.Parameter{},
	}
}

func (t *listTagsTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing tags...")
	tags, err := t.deps.Accessor.ListTags(ctx, t.deps.WorkspaceID)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list tags", goerr.V("workspace_id", t.deps.WorkspaceID))
	}
	out := make([]map[string]any, 0, len(tags))
	for _, tg := range tags {
		out = append(out, tagToMap(tg))
	}
	return map[string]any{"tags": out}, nil
}

type createTagTool struct{ deps Deps }

func (t *createTagTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__create_tag",
		Description: "Create a new tag and return its immutable id. Tags are first-class entities; " +
			"knowledge entries reference them by id. You MUST call list_tags first and only create a tag " +
			"once you have confirmed that no existing tag is suitable — never create a near-duplicate.",
		Parameters: map[string]*gollem.Parameter{
			"name": {
				Type:        gollem.TypeString,
				Description: "Optional human-facing name for the tag.",
			},
		},
	}
}

func (t *createTagTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Creating tag...")
	if t.deps.Mutator == nil {
		return nil, goerr.New("knowledge: write is not permitted in this context")
	}
	name := optionalString(args, "name")
	created, err := t.deps.Mutator.CreateTag(ctx, t.deps.WorkspaceID, name)
	if err != nil {
		return nil, goerr.Wrap(err, "create tag", goerr.V("workspace_id", t.deps.WorkspaceID))
	}
	return tagToMap(created), nil
}

type updateTagTool struct{ deps Deps }

func (t *updateTagTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "knowledge__update_tag",
		Description: "Rename an existing tag. The tag id never changes; only its name is updated.",
		Parameters: map[string]*gollem.Parameter{
			"tag_id": {
				Type:        gollem.TypeString,
				Description: "The id of the tag to rename.",
				Required:    true,
			},
			"name": {
				Type:        gollem.TypeString,
				Description: "The new name for the tag.",
				Required:    true,
			},
		},
	}
}

func (t *updateTagTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating tag...")
	if t.deps.Mutator == nil {
		return nil, goerr.New("knowledge: write is not permitted in this context")
	}
	id, err := extractTagID(args)
	if err != nil {
		return nil, err
	}
	name, err := requireString(args, "name")
	if err != nil {
		return nil, err
	}
	updated, err := t.deps.Mutator.UpdateTag(ctx, t.deps.WorkspaceID, id, name)
	if err != nil {
		return nil, goerr.Wrap(err, "update tag",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("tag_id", id))
	}
	return tagToMap(updated), nil
}

type deleteTagTool struct{ deps Deps }

func (t *deleteTagTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__delete_tag",
		Description: "Delete a tag. This is only allowed when NO knowledge entry references the tag " +
			"(reference count zero); if even one knowledge entry still uses it the call fails. Re-tag " +
			"those entries onto another tag first, then delete.",
		Parameters: map[string]*gollem.Parameter{
			"tag_id": {
				Type:        gollem.TypeString,
				Description: "The id of the tag to delete.",
				Required:    true,
			},
		},
	}
}

func (t *deleteTagTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Deleting tag...")
	if t.deps.Mutator == nil {
		return nil, goerr.New("knowledge: write is not permitted in this context")
	}
	id, err := extractTagID(args)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Mutator.DeleteTag(ctx, t.deps.WorkspaceID, id); err != nil {
		return nil, goerr.Wrap(err, "delete tag",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("tag_id", id))
	}
	return map[string]any{"deleted": true, "tag_id": string(id)}, nil
}

type createKnowledgeTool struct{ deps Deps }

func (t *createKnowledgeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "knowledge__create_knowledge",
		Description: "Create a new shared knowledge entry for the workspace. Use this to record " +
			"organization-specific knowledge worth reusing on future cases (operating rules, proper " +
			"nouns, past decisions, threat intel). The claim is a Markdown body. At least one tag_id is " +
			"required, and every tag_id must already exist — create tags with create_tag first; you " +
			"cannot introduce a new tag here.",
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
			"tag_ids": {
				Type:        gollem.TypeArray,
				Description: "One or more existing tag ids (at least one required). Use list_tags / create_tag to obtain ids.",
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
	tagIDs, err := optionalTagIDSlice(args, "tag_ids")
	if err != nil {
		return nil, err
	}
	created, err := t.deps.Mutator.CreateKnowledge(ctx, t.deps.WorkspaceID, title, claim, tagIDs)
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
			"tag_ids, when provided, replace the whole tag set (must be non-empty and every id must " +
			"already exist).",
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
			"tag_ids": {
				Type:        gollem.TypeArray,
				Description: "New tag id set (full replacement, must be non-empty, ids must exist). Omit to preserve.",
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
	var tagIDsPtr *[]model.TagID
	if v, ok := args["tag_ids"]; ok && v != nil {
		ids, err := toTagIDSlice(v)
		if err != nil {
			return nil, goerr.Wrap(err, "tag_ids invalid")
		}
		tagIDsPtr = &ids
	}

	updated, err := t.deps.Mutator.UpdateKnowledge(ctx, t.deps.WorkspaceID, id, titlePtr, claimPtr, tagIDsPtr)
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

func extractTagID(args map[string]any) (model.TagID, error) {
	v, ok := args["tag_id"]
	if !ok || v == nil {
		return "", goerr.New("tag_id is required")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", goerr.New("tag_id must be a non-empty string", goerr.V("type", typeOf(v)))
	}
	return model.TagID(s), nil
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

func optionalTagIDSlice(args map[string]any, key string) ([]model.TagID, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	return toTagIDSlice(v)
}

func toTagIDSlice(v any) ([]model.TagID, error) {
	ss, err := toStringSlice(v)
	if err != nil {
		return nil, err
	}
	out := make([]model.TagID, 0, len(ss))
	for _, s := range ss {
		out = append(out, model.TagID(s))
	}
	return out, nil
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
