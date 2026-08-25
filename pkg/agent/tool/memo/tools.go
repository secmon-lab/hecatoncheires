// Package memo exposes the Case-scoped memo gollem tools available to agents
// running in a Case context. Every tool is pinned to a single (workspace, case)
// so an agent can only read and mutate memos within the Case it is working.
// Mutations route through the MemoMutator surface (the MemoUseCase) so field
// validation and access control are identical to the WebUI path.
package memo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
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

// New builds the full memo tool set: the two read tools plus the single batch
// write tool. There is deliberately no per-memo create / update / archive tool
// — writing one memo per call cost one LLM round trip each, and each round trip
// re-sends the whole message history, so bookkeeping cost scaled with
// (number of mutations x context size). Offering both shapes would leave that
// path open, so the batch tool is the only way to write.
func New(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&listMemosTool{deps: deps},
		&getMemoTool{deps: deps},
		&applyMemoChangesTool{deps: deps},
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

// listMemosDefaultLimit is how many memos memo__list_memos returns when the
// caller does not ask for a specific limit. A tool response stays in the message
// history for the rest of the run and is re-sent on every later model call, so
// the default is deliberately small; an agent that needs more asks for the next
// page.
const listMemosDefaultLimit = 10

// listMemosMaxLimit is the largest page a caller may ask for. A larger request
// is capped rather than rejected so an over-eager model still gets a result.
const listMemosMaxLimit = 50

type listMemosTool struct {
	deps Deps
}

func (t *listMemosTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__list_memos",
		Description: "List the memos of the current case, newest first. Archived memos are " +
			"never returned: archiving is how a memory is deliberately dropped, so an archived " +
			"memo stays out of scope even when you are looking for earlier context. Narrow the " +
			"result to a creation-time window with created_after / created_before. One page of " +
			"at most `limit` memos is returned (default 10); total_count reports how many " +
			"matched and has_more says whether older ones were left out, which you can fetch by " +
			"advancing offset. Returns id, title and field values for each memo. This response " +
			"stays in the conversation and is re-sent on every later model call, so ask for the " +
			"narrowest window and limit that answers your question instead of paging through " +
			"everything.",
		Parameters: map[string]*gollem.Parameter{
			"created_after": {
				Type: gollem.TypeString,
				Description: "RFC3339 lower bound (inclusive) on the memo creation time. Subtract " +
					"from the current time given in the system prompt to ask for a recent window, " +
					"e.g. the last 7 days. Omit for no lower bound.",
			},
			"created_before": {
				Type: gollem.TypeString,
				Description: "RFC3339 upper bound (exclusive) on the memo creation time. Omit for " +
					"no upper bound.",
			},
			"limit": {
				Type: gollem.TypeInteger,
				Description: "Maximum number of memos to return in one page, newest first. " +
					"Defaults to 10; a value above 50 is capped at 50 and a value below 1 " +
					"falls back to the default.",
			},
			"offset": {
				Type: gollem.TypeInteger,
				Description: "How many of the newest matching memos to skip before the page " +
					"starts. Defaults to 0. Pass offset + returned_count of the previous " +
					"response to read the next page. Memos created between two calls shift the " +
					"list, so a memo can repeat across pages.",
			},
		},
	}
}

func (t *listMemosTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	tool.Update(ctx, "Listing memos...")
	createdAfter, err := optionalTime(args, "created_after")
	if err != nil {
		return nil, err
	}
	createdBefore, err := optionalTime(args, "created_before")
	if err != nil {
		return nil, err
	}
	// The page-size policy lives here at the caller rather than inside the
	// argument helper: a value below one means "unset", and an over-large one is
	// capped instead of rejected so an over-eager model still gets a page.
	limitArg, limitSet, err := optionalCount(args, "limit")
	if err != nil {
		return nil, err
	}
	limit := listMemosDefaultLimit
	if limitSet && limitArg >= 1 {
		limit = int(min(limitArg, int64(listMemosMaxLimit)))
	}
	// offset stays an int64 here and is narrowed below, only once it is known to
	// be under the match count, so a value past the end cannot overflow int.
	offsetArg, offsetSet, err := optionalCount(args, "offset")
	if err != nil {
		return nil, err
	}
	var offset int64
	if offsetSet && offsetArg > 0 {
		offset = offsetArg
	}
	// Archived memos are never listed. Archiving is how an agent drops a memory,
	// so reading it back through the same tool would leave archiving without any
	// effect on what the next run sees. memo__get_memo still resolves an archived
	// memo for a caller that already holds its id.
	//
	// The window's own consistency (after < before) is checked by the
	// repository's List, so a contradictory window surfaces as
	// interfaces.ErrMemoListOptions through the wrap below.
	opts := interfaces.MemoListOptions{
		ArchiveScope:  interfaces.MemoArchiveScopeActiveOnly,
		CreatedAfter:  createdAfter,
		CreatedBefore: createdBefore,
	}
	memos, err := t.deps.Repo.Memo().List(ctx, t.deps.WorkspaceID, t.deps.CaseID, opts)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to list memos",
			goerr.V("workspace_id", t.deps.WorkspaceID), goerr.V("case_id", t.deps.CaseID))
	}

	total := len(memos)
	// Narrow the offset only once it is known to be below the match count, so a
	// caller-supplied value near math.MaxInt64 cannot overflow int and produce a
	// negative start index.
	start := total
	if offset < int64(total) {
		start = int(offset)
	}
	end := min(start+limit, total)
	// List returns CreatedAt ascending; walk it backwards so index 0 of the page
	// is the newest memo and a truncated page drops the oldest.
	items := make([]map[string]any, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, memoToMap(memos[total-1-i]))
	}
	return map[string]any{
		"memos":          items,
		"offset":         start,
		"total_count":    total,
		"returned_count": len(items),
		"has_more":       start+len(items) < total,
	}, nil
}

// optionalCount parses an optional integer argument. The second result reports
// whether the caller supplied a usable value; an absent key, a nil value, and an
// empty string all mean "unset" — models routinely pass "" instead of omitting a
// parameter, the same tolerance optionalTime already applies. Deciding what an
// unset or out-of-range count means is the caller's job, not this helper's.
//
// An out-of-range number is saturated here instead of being handed to
// tool.ExtractInt64. Converting an out-of-range float64 to int64 is
// implementation-defined: on amd64 a huge positive value lands on a negative
// int64, which would turn a far-past-the-end offset back into the first page
// and an enormous limit back into the default. Saturating preserves the sign,
// so an out-of-range request keeps meaning "past the end" / "more than the
// maximum" on every target.
//
// Both decoded number types are handled, because gollem keeps a literal that a
// float64 cannot hold exactly as a json.Number (internal/jsonutil) — which is
// precisely the out-of-range case this saturation exists for. Reading only
// float64 would let such a value fall through to tool.ExtractInt64 and fail.
func optionalCount(args map[string]any, key string) (int64, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	if s, isString := v.(string); isString && s == "" {
		return 0, false, nil
	}
	if num, isNumber := v.(json.Number); isNumber {
		if i, err := num.Int64(); err == nil {
			return i, true, nil
		}
		// JSON guarantees the literal is a syntactically valid number, so
		// Float64 fails only on overflow — and returns ±Inf with the error,
		// which the saturation below turns back into "past the end" /
		// "more than the maximum".
		f, err := num.Float64()
		if err != nil && !math.IsInf(f, 0) {
			return 0, false, goerr.Wrap(err, key+" must be an integer", goerr.V(key, num.String()))
		}
		v = f
	}
	if f, isFloat := v.(float64); isFloat {
		switch {
		case math.IsNaN(f):
			return 0, false, goerr.New(key+" must be an integer", goerr.V(key, f))
		case f >= math.MaxInt64:
			return math.MaxInt64, true, nil
		case f < math.MinInt64:
			return math.MinInt64, true, nil
		}
		return int64(f), true, nil
	}
	n, err := tool.ExtractInt64(args, key)
	if err != nil {
		return 0, false, goerr.Wrap(err, "invalid "+key)
	}
	return n, true, nil
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

// memoOpKind discriminates the three operations memo__apply_memo_changes can
// carry in one call.
type memoOpKind string

const (
	memoOpCreate  memoOpKind = "create"
	memoOpUpdate  memoOpKind = "update"
	memoOpArchive memoOpKind = "archive"
)

// applyMemoChangesMaxOps bounds the total number of operations one
// memo__apply_memo_changes call may carry. It is not advertised through
// gollem.Parameter.MaxItems on purpose: gollem rejects the whole call on a
// MaxItems violation, which would make an over-long batch behave differently
// from every other input problem this tool reports.
const applyMemoChangesMaxOps = 50

// memoOp is one parsed entry of a memo__apply_memo_changes call. A non-nil
// ParseErr means the entry never reaches MemoUC and is reported as a failed
// item instead of aborting the whole batch.
type memoOp struct {
	// Kind selects which MemoMutator method applies this entry.
	Kind memoOpKind
	// Index is the 0-based position within its own input array, so the model
	// can map a failure back onto the arguments it sent.
	Index int
	// MemoID is set for update / archive; empty for create.
	MemoID model.MemoID
	// Title is always non-nil for create. For update, nil means "preserve".
	Title *string
	// Fields nil means "no field change".
	Fields   map[string]model.FieldValue
	ParseErr error
}

type applyMemoChangesTool struct {
	deps Deps
}

func (t *applyMemoChangesTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "memo__apply_memo_changes",
		Description: "Apply every memo change in ONE call: create new memos, update existing ones, " +
			"and archive superseded ones together. This is the only way to write memos — settle on the " +
			"full set of changes first, then submit them here in a single call instead of one call per " +
			"memo. A memo records a unit of memory (fact / observation / hypothesis / decision) about " +
			"the case; see the system prompt for the memo definition, field ids, types, and option ids. " +
			"Entries are applied in order (every create, then every update, then every archive) and " +
			"independently: the response reports each entry's outcome, so one bad entry does not " +
			"discard the rest. Give every argument the type declared below, though — a mistyped " +
			"value is rejected before any entry is applied. At most 50 entries per call.",
		Parameters: map[string]*gollem.Parameter{
			"creates": {
				Type: gollem.TypeArray,
				Description: "Memos to create. Each entry needs a title plus the custom field values " +
					"defined by the workspace memo schema. All required fields must be set.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"title": {
							Type:        gollem.TypeString,
							Description: "Required. A concise one-line title for the new memo.",
						},
						"fields": fieldsParameter(),
					},
				},
			},
			"updates": {
				Type: gollem.TypeArray,
				Description: "Memos to update. Submit only what you intend to change; omitted custom " +
					"fields are preserved and the merged result is fully validated, so required fields " +
					"must remain satisfied. Each entry needs at least one of title / fields.",
				Items: &gollem.Parameter{
					Type: gollem.TypeObject,
					Properties: map[string]*gollem.Parameter{
						"memo_id": {
							Type:        gollem.TypeString,
							Description: "Required. The id (UUID) of the memo to update.",
						},
						"title": {
							Type:        gollem.TypeString,
							Description: "New title (full replacement). Omit to preserve the existing title.",
						},
						"fields": fieldsParameter(),
					},
				},
			},
			"archives": {
				Type: gollem.TypeArray,
				Description: "Ids (UUIDs) of memos to archive (soft-delete). An archived memo is not " +
					"destroyed and can be restored from the WebUI; it no longer appears in the default " +
					"memo list.",
				Items: &gollem.Parameter{Type: gollem.TypeString},
			},
		},
	}
}

func (t *applyMemoChangesTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.deps.MemoUC == nil {
		return nil, goerr.New("memo: MemoUC is not configured")
	}

	ops, err := parseApplyMemoChanges(args, t.deps.Schema)
	if err != nil {
		return nil, err
	}

	tool.Update(ctx, fmt.Sprintf("Applying %d memo change(s)...", len(ops)))

	results := make([]map[string]any, 0, len(ops))
	applied, failed := 0, 0
	for _, op := range ops {
		item := map[string]any{"op": string(op.Kind), "index": op.Index}

		if op.ParseErr != nil {
			// A malformed entry is the model's own mistake and it can repair it
			// by re-emitting the call, so it is reported back to the model but
			// deliberately NOT escalated through errutil — that path reaches the
			// operator's Sentry, where an argument typo is pure noise.
			item["ok"] = false
			item["error"] = op.ParseErr.Error()
			results = append(results, item)
			failed++
			continue
		}

		m, err := t.applyOne(ctx, op)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "apply memo change",
				goerr.V("workspace_id", t.deps.WorkspaceID),
				goerr.V("case_id", t.deps.CaseID),
				goerr.V("op", string(op.Kind)),
				goerr.V("index", op.Index),
				goerr.V("memo_id", op.MemoID)), "memo apply item failed")
			item["ok"] = false
			item["error"] = err.Error()
			results = append(results, item)
			failed++
			continue
		}

		item["ok"] = true
		item["memo"] = memoToMap(m)
		results = append(results, item)
		applied++
	}

	// Returned with a nil error even when every entry failed. The per-entry
	// reasons are what let the model repair its input, and collapsing them into
	// a single tool error would also throw away the successes of a partial
	// batch. No error is swallowed: each one is reported here and, when it came
	// from a write, also sent through errutil above.
	return map[string]any{
		"results": results,
		"applied": applied,
		"failed":  failed,
	}, nil
}

func (t *applyMemoChangesTool) applyOne(ctx context.Context, op memoOp) (*model.Memo, error) {
	switch op.Kind {
	case memoOpCreate:
		return t.deps.MemoUC.CreateMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, *op.Title, op.Fields)
	case memoOpUpdate:
		return t.deps.MemoUC.UpdateMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, op.MemoID, op.Title, op.Fields)
	case memoOpArchive:
		return t.deps.MemoUC.ArchiveMemo(ctx, t.deps.WorkspaceID, t.deps.CaseID, op.MemoID)
	default:
		return nil, goerr.New("unknown memo operation", goerr.V("op", string(op.Kind)))
	}
}

// parseApplyMemoChanges turns the tool arguments into the ordered operation
// list. Problems with an individual entry are recorded on that entry's
// ParseErr rather than returned, so one bad entry cannot discard the rest;
// only problems that make the batch as a whole unreadable are returned.
//
// The applied order is every create, then every update, then every archive.
// A memo created in this call cannot be referenced by the same call (its id is
// only known from the response), so grouping never breaks a dependency, and
// putting archives last makes "revise, then retire the same memo" work.
func parseApplyMemoChanges(args map[string]any, schema *config.FieldSchema) ([]memoOp, error) {
	creates, err := optionalArray(args, "creates")
	if err != nil {
		return nil, err
	}
	updates, err := optionalArray(args, "updates")
	if err != nil {
		return nil, err
	}
	archives, err := optionalArray(args, "archives")
	if err != nil {
		return nil, err
	}

	total := len(creates) + len(updates) + len(archives)
	if total == 0 {
		return nil, goerr.New("at least one of creates / updates / archives must be non-empty")
	}
	if total > applyMemoChangesMaxOps {
		return nil, goerr.New("too many memo changes in one call",
			goerr.V("count", total), goerr.V("max", applyMemoChangesMaxOps))
	}

	ops := make([]memoOp, 0, total)
	for i, entry := range creates {
		ops = append(ops, parseCreateOp(i, entry, schema))
	}
	for i, entry := range updates {
		ops = append(ops, parseUpdateOp(i, entry, schema))
	}
	for i, entry := range archives {
		ops = append(ops, parseArchiveOp(i, entry))
	}
	return ops, nil
}

// optionalArray reads an optional array argument. An absent key and a nil value
// both mean "no entries"; any other non-array value is a whole-call error,
// because entries that cannot be enumerated cannot be reported per entry.
func optionalArray(args map[string]any, key string) ([]any, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, goerr.New(key+" must be an array", goerr.V("type", typeOf(v)))
	}
	return arr, nil
}

func parseCreateOp(index int, entry any, schema *config.FieldSchema) memoOp {
	op := memoOp{Kind: memoOpCreate, Index: index}
	m, ok := entry.(map[string]any)
	if !ok {
		op.ParseErr = goerr.New("each create entry must be an object", goerr.V("type", typeOf(entry)))
		return op
	}
	title, err := requireString(m, "title")
	if err != nil {
		op.ParseErr = err
		return op
	}
	fields, err := coerceFieldsArg(m, schema)
	if err != nil {
		op.ParseErr = err
		return op
	}
	op.Title = &title
	op.Fields = fields
	return op
}

func parseUpdateOp(index int, entry any, schema *config.FieldSchema) memoOp {
	op := memoOp{Kind: memoOpUpdate, Index: index}
	m, ok := entry.(map[string]any)
	if !ok {
		op.ParseErr = goerr.New("each update entry must be an object", goerr.V("type", typeOf(entry)))
		return op
	}
	id, err := extractMemoID(m)
	if err != nil {
		op.ParseErr = err
		return op
	}
	op.MemoID = id

	if v, ok := m["title"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			op.ParseErr = goerr.New("title must be a string",
				goerr.V("memo_id", id), goerr.V("type", typeOf(v)))
			return op
		}
		op.Title = &s
	}

	fields, err := coerceFieldsArg(m, schema)
	if err != nil {
		op.ParseErr = err
		return op
	}
	op.Fields = fields

	// An update carrying neither is a write that only moves UpdatedAt, so it is
	// rejected rather than spent.
	if op.Title == nil && len(op.Fields) == 0 {
		op.ParseErr = goerr.New("update requires at least one of title, fields", goerr.V("memo_id", id))
	}
	return op
}

func parseArchiveOp(index int, entry any) memoOp {
	op := memoOp{Kind: memoOpArchive, Index: index}
	s, ok := entry.(string)
	if !ok || s == "" {
		op.ParseErr = goerr.New("each archive entry must be a non-empty memo id",
			goerr.V("type", typeOf(entry)))
		return op
	}
	op.MemoID = model.MemoID(s)
	return op
}

// fieldsParameter returns the shared `fields` parameter spec used by the
// create / update entries of memo__apply_memo_changes.
//
// field_id deliberately does NOT set Required. gollem runs ToolSpec.ValidateArgs
// before Run and rejects the WHOLE call on the first nested Required violation,
// which would discard every other entry in the batch. Requiredness is enforced
// per entry in parseFieldInputs instead, so a single malformed field entry costs
// only its own entry.
func fieldsParameter() *gollem.Parameter {
	return &gollem.Parameter{
		Type: gollem.TypeArray,
		Description: "Custom field assignments. Each entry sets one field defined in the workspace " +
			"memo schema (see the system prompt for ids, types, and option ids). On update, submitted " +
			"entries are merged onto existing values; omitted fields are preserved.",
		Items: &gollem.Parameter{
			Type: gollem.TypeObject,
			Properties: map[string]*gollem.Parameter{
				"field_id": {Type: gollem.TypeString, Description: "Required. The field id from the workspace memo schema."},
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

// optionalTime parses an optional RFC3339 timestamp argument. An absent key, a
// nil value, and an empty string all mean "unset" — models routinely pass ""
// instead of omitting a parameter, and the same tolerance is applied to the
// github__list_commits `since` argument.
func optionalTime(args map[string]any, key string) (*time.Time, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, goerr.New(key+" must be an RFC3339 string", goerr.V("type", typeOf(v)))
	}
	if s == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, goerr.Wrap(err, "invalid "+key+" (must be RFC3339)", goerr.V(key, s))
	}
	return &parsed, nil
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
