// Package memo exposes the Case-scoped memo gollem tools available to agents
// running in a Case context. Every tool is pinned to a single (workspace, case)
// so an agent can only read and mutate memos within the Case it is working.
// Mutations route through the MemoMutator surface (the MemoUseCase) so field
// validation and access control are identical to the WebUI path.
package memo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/gollem-dev/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
)

// MemoMutator is the narrow surface of MemoUseCase the memo tools depend on.
// Defined here so the package does not import pkg/usecase and create a cycle.
type MemoMutator interface {
	CreateMemo(ctx context.Context, workspaceID string, caseID int64, title string, fields map[string]model.FieldValue) (*model.Memo, error)
	UpdateMemo(ctx context.Context, workspaceID string, caseID int64, id model.MemoID, title *string, fields map[string]model.FieldValue) (*model.Memo, error)
	ArchiveMemo(ctx context.Context, workspaceID string, caseID int64, id model.MemoID) (*model.Memo, error)
}

// Deps groups the dependencies the memo tools need.
type Deps struct {
	Repo        interfaces.Repository
	WorkspaceID string
	CaseID      int64
	// MemoUC routes create / update / archive through the unified usecase entry
	// point. Required for the writer tools; they fail loudly when it is nil.
	MemoUC MemoMutator
	// Schema resolves field types for coercing the `fields` parameter. nil means
	// the workspace has no memo schema; the field-bearing tools then error out.
	Schema *config.FieldSchema
}

// New builds the full memo tool set (read + create/update/archive).
func New(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&listMemosTool{deps: deps},
		&getMemoTool{deps: deps},
		&createMemoTool{deps: deps},
		&updateMemoTool{deps: deps},
		&archiveMemoTool{deps: deps},
	}
}

// NewReadOnly builds the read-only subset (list / get). Used by agents that may
// inspect memos but must not mutate them.
func NewReadOnly(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&listMemosTool{deps: deps},
		&getMemoTool{deps: deps},
	}
}

// memoToMap renders a memo for a tool response.
func memoToMap(m *model.Memo) map[string]any {
	out := map[string]any{
		"id":           string(m.ID),
		"case_id":      m.CaseID,
		"title":        m.Title,
		"creator_id":   m.CreatorID,
		"archived":     m.IsArchived(),
		"field_values": renderFieldValues(m.FieldValues),
		"created_at":   m.CreatedAt.Format(time.RFC3339),
		"updated_at":   m.UpdatedAt.Format(time.RFC3339),
	}
	if m.ArchivedAt != nil {
		out["archived_at"] = m.ArchivedAt.Format(time.RFC3339)
	}
	return out
}

type listMemosTool struct {
	deps Deps
}

func (t *listMemosTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__list_memos",
		Description: "List the memos of the current case. By default archived memos are " +
			"excluded (an archived memo is a soft-deleted memory). Pass include_archived=true " +
			"to include them. Returns id, title and field values for each memo.",
		Parameters: map[string]*gollem.Parameter{
			"include_archived": {
				Type:        gollem.TypeBoolean,
				Description: "When true, include archived memos. Default false.",
			},
		},
	}
}

func (t *listMemosTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing memos...")
	scope := interfaces.MemoArchiveScopeActiveOnly
	if v, ok := args["include_archived"]; ok {
		if b, ok := v.(bool); ok && b {
			scope = interfaces.MemoArchiveScopeAll
		}
	}
	memos, err := t.deps.Repo.Memo().List(ctx, t.deps.WorkspaceID, t.deps.CaseID, interfaces.MemoListOptions{ArchiveScope: scope})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list memos",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID))
	}
	items := make([]map[string]any, len(memos))
	for i, m := range memos {
		items[i] = memoToMap(m)
	}
	return map[string]any{"memos": items}, nil
}

type getMemoTool struct {
	deps Deps
}

func (t *getMemoTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "memo__get_memo",
		Description: "Get the full details of a memo in the current case by its id.",
		Parameters: map[string]*gollem.Parameter{
			"memo_id": {
				Type:        gollem.TypeString,
				Description: "The id (UUID) of the memo to retrieve.",
				Required:    true,
			},
		},
	}
}

func (t *getMemoTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	id, err := extractMemoID(args)
	if err != nil {
		return nil, err
	}
	tool.Update(ctx, "Getting memo...")
	m, err := t.deps.Repo.Memo().Get(ctx, t.deps.WorkspaceID, t.deps.CaseID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get memo",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID), goerr.V("memo_id", id))
	}
	return memoToMap(m), nil
}

type createMemoTool struct {
	deps Deps
}

func (t *createMemoTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__create_memo",
		Description: "Create a new memo in the current case. A memo records a unit of memory " +
			"(fact / observation / hypothesis / decision) about the case. Provide a concise title " +
			"and the custom field values defined by the workspace memo schema (see the system prompt " +
			"for the memo definition, field ids, types, and option ids). All required fields must be set.",
		Parameters: map[string]*gollem.Parameter{
			"title": {
				Type:        gollem.TypeString,
				Description: "A concise one-line title for the memo.",
				Required:    true,
			},
			"fields": fieldsParameter(),
		},
	}
}

func (t *createMemoTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Creating memo...")
	if t.deps.MemoUC == nil {
		return nil, goerr.New("memo: MemoUC is not configured")
	}
	title, err := requireString(args, "title")
	if err != nil {
		return nil, err
	}
	fields, err := t.coerceFields(args)
	if err != nil {
		return nil, err
	}
	created, err := t.deps.MemoUC.CreateMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, title, fields)
	if err != nil {
		return nil, goerr.Wrap(err, "create memo",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID))
	}
	return memoToMap(created), nil
}

func (t *createMemoTool) coerceFields(args map[string]any) (map[string]model.FieldValue, error) {
	return coerceFieldsArg(args, t.deps.Schema)
}

type updateMemoTool struct {
	deps Deps
}

func (t *updateMemoTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__update_memo",
		Description: "Update a memo in the current case. Submit only the fields you intend to " +
			"change; omitted custom fields are preserved. Title is a full replacement when provided. " +
			"The merged result is fully validated, so required fields must remain satisfied.",
		Parameters: map[string]*gollem.Parameter{
			"memo_id": {
				Type:        gollem.TypeString,
				Description: "The id (UUID) of the memo to update.",
				Required:    true,
			},
			"title": {
				Type:        gollem.TypeString,
				Description: "New title (full replacement). Omit to preserve the existing title.",
			},
			"fields": fieldsParameter(),
		},
	}
}

func (t *updateMemoTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Updating memo...")
	if t.deps.MemoUC == nil {
		return nil, goerr.New("memo: MemoUC is not configured")
	}
	id, err := extractMemoID(args)
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

	fields, err := coerceFieldsArg(args, t.deps.Schema)
	if err != nil {
		return nil, err
	}

	updated, err := t.deps.MemoUC.UpdateMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, id, titlePtr, fields)
	if err != nil {
		return nil, goerr.Wrap(err, "update memo",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID), goerr.V("memo_id", id))
	}
	return memoToMap(updated), nil
}

type archiveMemoTool struct {
	deps Deps
}

func (t *archiveMemoTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__archive_memo",
		Description: "Archive (soft-delete) a memo in the current case. The memo is not destroyed " +
			"and can be restored from the WebUI; archived memos no longer appear in the default memo list.",
		Parameters: map[string]*gollem.Parameter{
			"memo_id": {
				Type:        gollem.TypeString,
				Description: "The id (UUID) of the memo to archive.",
				Required:    true,
			},
		},
	}
}

func (t *archiveMemoTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Archiving memo...")
	if t.deps.MemoUC == nil {
		return nil, goerr.New("memo: MemoUC is not configured")
	}
	id, err := extractMemoID(args)
	if err != nil {
		return nil, err
	}
	archived, err := t.deps.MemoUC.ArchiveMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, id)
	if err != nil {
		return nil, goerr.Wrap(err, "archive memo",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID), goerr.V("memo_id", id))
	}
	return memoToMap(archived), nil
}

// fieldsParameter returns the shared `fields` parameter spec used by the
// create / update tools.
func fieldsParameter() *gollem.Parameter {
	return &gollem.Parameter{
		Type: gollem.TypeArray,
		Description: "Custom field assignments. Each entry sets one field defined in the workspace " +
			"memo schema (see the system prompt for ids, types, and option ids). On update, submitted " +
			"entries are merged onto existing values; omitted fields are preserved.",
		Items: &gollem.Parameter{
			Type: gollem.TypeObject,
			Properties: map[string]*gollem.Parameter{
				"field_id": {Type: gollem.TypeString, Description: "The field id from the workspace memo schema.", Required: true},
				"value":    {Type: gollem.TypeString, Description: "Scalar value (text / number / url / date / single select option id / single user id)."},
				"values":   {Type: gollem.TypeArray, Description: "Multi value (multi-select option ids / multi-user ids).", Items: &gollem.Parameter{Type: gollem.TypeString}},
			},
		},
	}
}

// coerceFieldsArg parses + coerces the optional `fields` argument against the
// memo schema. Returns nil (no error) when no fields were supplied.
func coerceFieldsArg(args map[string]any, schema *config.FieldSchema) (map[string]model.FieldValue, error) {
	v, ok := args["fields"]
	if !ok || v == nil {
		return nil, nil
	}
	if schema == nil {
		return nil, goerr.New("this workspace has no memo fields; the fields parameter is not supported")
	}
	inputs, err := parseFieldInputs(v)
	if err != nil {
		return nil, goerr.Wrap(err, "fields invalid")
	}
	coerced, violations := model.CoerceFieldInputs(schema, inputs)
	if len(violations) > 0 {
		return nil, goerr.New("invalid field value(s):\n- " + strings.Join(violations, "\n- "))
	}
	return coerced, nil
}

func extractMemoID(args map[string]any) (model.MemoID, error) {
	v, ok := args["memo_id"]
	if !ok || v == nil {
		return "", goerr.New("memo_id is required")
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", goerr.New("memo_id must be a non-empty string", goerr.V("type", typeOf(v)))
	}
	return model.MemoID(s), nil
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

// parseFieldInputs converts the gollem-decoded `fields` argument (a []any of
// per-entry maps) into model.FieldInput.
func parseFieldInputs(v any) ([]model.FieldInput, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, goerr.New("fields must be an array", goerr.V("type", typeOf(v)))
	}
	out := make([]model.FieldInput, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, goerr.New("each field entry must be an object", goerr.V("type", typeOf(item)))
		}
		fieldID, ok := m["field_id"].(string)
		if !ok || fieldID == "" {
			return nil, goerr.New("each field entry requires a non-empty field_id")
		}
		fi := model.FieldInput{FieldID: fieldID}
		if val, ok := m["value"]; ok && val != nil {
			s, ok := val.(string)
			if !ok {
				return nil, goerr.New("field value must be a string", goerr.V("field_id", fieldID), goerr.V("type", typeOf(val)))
			}
			fi.Value = s
		}
		if vals, ok := m["values"]; ok && vals != nil {
			ss, err := toStringSlice(vals)
			if err != nil {
				return nil, goerr.Wrap(err, "field values invalid", goerr.V("field_id", fieldID))
			}
			fi.Values = ss
		}
		out = append(out, fi)
	}
	return out, nil
}

func renderFieldValues(fields map[string]model.FieldValue) map[string]any {
	out := make(map[string]any, len(fields))
	for id, fv := range fields {
		out[id] = fv.Value
	}
	return out
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
