package export

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// customFieldPrefix namespaces per-workspace custom field columns so they never
// collide with a fixed column. field ids match ^[a-z][a-z0-9_]*$, so
// "field_<id>" is always a valid, collision-free column name.
const customFieldPrefix = "field_"

// caseColumns describes the "cases" table: fixed Case columns plus one
// "field_<id>" column per workspace field definition.
func caseColumns(schema *config.FieldSchema) []Column {
	return append(fixedCaseColumns(), customFieldColumns(schema)...)
}

// caseRows renders the "cases" table's rows.
func caseRows(ctx context.Context, schema *config.FieldSchema, cases []*model.Case) []map[string]any {
	rows := make([]map[string]any, 0, len(cases))
	for _, c := range cases {
		row := map[string]any{
			"id":               c.ID,
			"title":            c.Title,
			"description":      c.Description,
			"status":           string(c.Status),
			"board_status":     c.BoardStatus,
			"reporter_id":      c.ReporterID,
			"assignee_ids":     c.AssigneeIDs,
			"channel_user_ids": c.ChannelUserIDs,
			"slack_channel_id": c.SlackChannelID,
			"slack_thread_ts":  c.SlackThreadTS,
			"is_private":       c.IsPrivate,
			"is_test":          c.IsTest,
			"request_key":      c.RequestKey,
			"archived_at":      c.ArchivedAt,
			"created_at":       c.CreatedAt,
			"updated_at":       c.UpdatedAt,
		}
		addCustomFieldValues(ctx, row, schema, c.FieldValues)
		rows = append(rows, row)
	}
	return rows
}

// actionColumns describes the "actions" table.
func actionColumns() []Column {
	return []Column{
		{Name: "id", Type: TypeInt},
		{Name: "case_id", Type: TypeInt, Nullable: true},
		{Name: "title", Type: TypeString, Nullable: true},
		{Name: "description", Type: TypeString, Nullable: true},
		{Name: "assignee_id", Type: TypeString, Nullable: true},
		{Name: "slack_message_ts", Type: TypeString, Nullable: true},
		{Name: "status", Type: TypeString, Nullable: true},
		{Name: "due_date", Type: TypeTimestamp, Nullable: true},
		{Name: "archived_at", Type: TypeTimestamp, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
}

// actionRows renders the "actions" table's rows.
func actionRows(actions []*model.Action) []map[string]any {
	rows := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		rows = append(rows, map[string]any{
			"id":               a.ID,
			"case_id":          a.CaseID,
			"title":            a.Title,
			"description":      a.Description,
			"assignee_id":      a.AssigneeID,
			"slack_message_ts": a.SlackMessageTS,
			"status":           string(a.Status),
			"due_date":         a.DueDate,
			"archived_at":      a.ArchivedAt,
			"created_at":       a.CreatedAt,
			"updated_at":       a.UpdatedAt,
		})
	}
	return rows
}

// memoFieldSchema is the workspace's memo field schema, nil-safe when memos are
// disabled.
func memoFieldSchema(memoConfig *config.MemoConfig) *config.FieldSchema {
	if memoConfig == nil {
		return nil
	}
	return memoConfig.FieldSchema
}

// memoColumns describes the "memos" table: fixed Memo columns plus the
// workspace's memo field schema.
func memoColumns(memoConfig *config.MemoConfig) []Column {
	return append(fixedMemoColumns(), customFieldColumns(memoFieldSchema(memoConfig))...)
}

// memoRows renders the "memos" table's rows.
func memoRows(ctx context.Context, memoConfig *config.MemoConfig, memos []*model.Memo) []map[string]any {
	schema := memoFieldSchema(memoConfig)
	rows := make([]map[string]any, 0, len(memos))
	for _, m := range memos {
		row := map[string]any{
			"id":           string(m.ID),
			"workspace_id": m.WorkspaceID,
			"case_id":      m.CaseID,
			"title":        m.Title,
			"creator_id":   m.CreatorID,
			"archived_at":  m.ArchivedAt,
			"created_at":   m.CreatedAt,
			"updated_at":   m.UpdatedAt,
		}
		addCustomFieldValues(ctx, row, schema, m.FieldValues)
		rows = append(rows, row)
	}
	return rows
}

// jobRunColumns describes the "job_runs" table: the per-(case, job) summary of
// the most recent run, joinable to job_run_logs on
// (workspace_id, case_id, job_id).
func jobRunColumns() []Column {
	return []Column{
		{Name: "workspace_id", Type: TypeString},
		{Name: "case_id", Type: TypeInt},
		{Name: "job_id", Type: TypeString},
		{Name: "last_run_at", Type: TypeTimestamp, Nullable: true},
		{Name: "last_status", Type: TypeString, Nullable: true},
		{Name: "last_error", Type: TypeString, Nullable: true},
		{Name: "last_run_id", Type: TypeString, Nullable: true},
		{Name: "last_trace_id", Type: TypeString, Nullable: true},
		{Name: "lease_until", Type: TypeTimestamp, Nullable: true},
		{Name: "suspended_run_id", Type: TypeString, Nullable: true},
		{Name: "suspended_at", Type: TypeTimestamp, Nullable: true},
	}
}

// jobRunRows renders the "job_runs" table's rows.
func jobRunRows(runs []*model.JobRun) []map[string]any {
	rows := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		rows = append(rows, map[string]any{
			"workspace_id":     r.WorkspaceID,
			"case_id":          r.CaseID,
			"job_id":           r.JobID,
			"last_run_at":      r.LastRunAt,
			"last_status":      string(r.LastStatus),
			"last_error":       r.LastError,
			"last_run_id":      r.LastRunID,
			"last_trace_id":    r.LastTraceID,
			"lease_until":      r.LeaseUntil,
			"suspended_run_id": r.SuspendedRunID,
			"suspended_at":     r.SuspendedAt,
		})
	}
	return rows
}

// jobRunLogColumns describes the "job_run_logs" table: one row per agent run
// against a Case, TOML-configured Jobs and mention-triggered runs alike (the
// event_type column discriminates them — see model.EventTypeMention).
//
// PendingInteraction is deliberately not exported: it is transient state that
// exists only while a run sits at AWAITING_INPUT, and a Table column carries a
// scalar, so the nested question form has no faithful representation here.
func jobRunLogColumns() []Column {
	return []Column{
		{Name: "workspace_id", Type: TypeString},
		{Name: "case_id", Type: TypeInt},
		{Name: "job_id", Type: TypeString},
		{Name: "run_id", Type: TypeString},
		{Name: "trace_id", Type: TypeString, Nullable: true},
		{Name: "stage", Type: TypeString, Nullable: true},
		{Name: "started_at", Type: TypeTimestamp, Nullable: true},
		{Name: "ended_at", Type: TypeTimestamp, Nullable: true},
		{Name: "error", Type: TypeString, Nullable: true},
		{Name: "executor_kind", Type: TypeString, Nullable: true},
		{Name: "executor_version", Type: TypeString, Nullable: true},
		{Name: "event_type", Type: TypeString, Nullable: true},
		{Name: "event_trigger_at", Type: TypeTimestamp, Nullable: true},
		{Name: "system_prompt", Type: TypeString, Nullable: true},
		{Name: "input_tokens", Type: TypeInt, Nullable: true},
		{Name: "output_tokens", Type: TypeInt, Nullable: true},
		{Name: "cache_creation_input_tokens", Type: TypeInt, Nullable: true},
		{Name: "cache_read_input_tokens", Type: TypeInt, Nullable: true},
		{Name: "llm_call_count", Type: TypeInt, Nullable: true},
		{Name: "tool_call_count", Type: TypeInt, Nullable: true},
		// The cost is exported in nano-USD, the unit it is stored in: an integer
		// sums exactly across runs, which is what a spend query does with it.
		{Name: "cost_nano_usd", Type: TypeInt, Nullable: true},
		{Name: "model", Type: TypeString, Nullable: true},
	}
}

// jobRunLogRows renders the "job_run_logs" table's rows.
func jobRunLogRows(logs []*model.JobRunLog) []map[string]any {
	rows := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		rows = append(rows, map[string]any{
			"workspace_id":                l.WorkspaceID,
			"case_id":                     l.CaseID,
			"job_id":                      l.JobID,
			"run_id":                      l.RunID,
			"trace_id":                    l.TraceID,
			"stage":                       string(l.Stage),
			"started_at":                  l.StartedAt,
			"ended_at":                    l.EndedAt,
			"error":                       l.Error,
			"executor_kind":               l.ExecutorKind,
			"executor_version":            l.ExecutorVersion,
			"event_type":                  l.EventType,
			"event_trigger_at":            l.EventTriggerAt,
			"system_prompt":               l.SystemPrompt,
			"input_tokens":                l.InputTokens,
			"output_tokens":               l.OutputTokens,
			"cache_creation_input_tokens": l.CacheCreationInputTokens,
			"cache_read_input_tokens":     l.CacheReadInputTokens,
			"llm_call_count":              l.LLMCallCount,
			"tool_call_count":             l.ToolCallCount,
			"cost_nano_usd":               l.CostNanoUSD,
			"model":                       l.Model,
		})
	}
	return rows
}

// jobRunEventColumns describes the "job_run_events" table: the per-call timeline
// of every exported run, one row per LLM call, tool execution or run error.
// Joins to job_run_logs on (workspace_id, case_id, job_id, run_id); `sequence`
// is the authoritative order within a run (doc ids may diverge under clock
// skew), and `parent_sequence` links a TOOL_CALL back to the LLM_RESPONSE that
// requested it.
//
// The four event kinds share one flat table rather than one table each, so a run
// reads back as a single ordered scan. Only the columns belonging to a row's
// `kind` are populated; the rest are NULL.
func jobRunEventColumns() []Column {
	return []Column{
		{Name: "workspace_id", Type: TypeString},
		{Name: "case_id", Type: TypeInt},
		{Name: "job_id", Type: TypeString},
		{Name: "run_id", Type: TypeString},
		{Name: "trace_id", Type: TypeString, Nullable: true},
		{Name: "event_id", Type: TypeString},
		{Name: "sequence", Type: TypeInt},
		{Name: "occurred_at", Type: TypeTimestamp, Nullable: true},
		{Name: "kind", Type: TypeString, Nullable: true},
		{Name: "parent_sequence", Type: TypeInt, Nullable: true},
		{Name: "phase", Type: TypeString, Nullable: true},
		{Name: "agent_label", Type: TypeString, Nullable: true},

		// LLM_REQUEST / LLM_RESPONSE.
		{Name: "model", Type: TypeString, Nullable: true},
		// conversation_id groups the LLM_REQUEST rows of one conversation, and
		// messages_prefix_len says how many of that conversation's leading
		// messages the earlier rows already carried. messages_json holds only
		// what follows; see model.LLMRequestPayload and docs/export.md.
		{Name: "conversation_id", Type: TypeString, Nullable: true},
		{Name: "messages_prefix_len", Type: TypeInt, Nullable: true},
		{Name: "messages_json", Type: TypeString, Nullable: true},
		{Name: "tools_json", Type: TypeString, Nullable: true},
		{Name: "texts_json", Type: TypeString, Nullable: true},
		{Name: "function_calls_json", Type: TypeString, Nullable: true},
		{Name: "input_tokens", Type: TypeInt, Nullable: true},
		{Name: "output_tokens", Type: TypeInt, Nullable: true},
		{Name: "cache_creation_input_tokens", Type: TypeInt, Nullable: true},
		{Name: "cache_read_input_tokens", Type: TypeInt, Nullable: true},
		{Name: "duration_ms", Type: TypeInt, Nullable: true},

		// TOOL_CALL.
		{Name: "tool_name", Type: TypeString, Nullable: true},
		{Name: "tool_arguments_json", Type: TypeString, Nullable: true},
		{Name: "tool_result_json", Type: TypeString, Nullable: true},
		{Name: "tool_is_error", Type: TypeBool, Nullable: true},
		{Name: "tool_error_message", Type: TypeString, Nullable: true},
		{Name: "tool_started_at", Type: TypeTimestamp, Nullable: true},
		{Name: "tool_ended_at", Type: TypeTimestamp, Nullable: true},

		// RUN_ERROR.
		{Name: "error_stage", Type: TypeString, Nullable: true},
		{Name: "error_message", Type: TypeString, Nullable: true},
	}
}

// jobRunEventRows renders one repository read's worth of "job_run_events" rows.
// It is called once per run rather than once per workspace: the caller streams
// each run's timeline into the sink and releases it before reading the next.
func jobRunEventRows(ctx context.Context, events []*model.JobRunEvent) []map[string]any {
	rows := make([]map[string]any, 0, len(events))
	for _, e := range events {
		row := map[string]any{
			"workspace_id":    e.WorkspaceID,
			"case_id":         e.CaseID,
			"job_id":          e.JobID,
			"run_id":          e.RunID,
			"trace_id":        e.TraceID,
			"event_id":        e.EventID,
			"sequence":        e.Sequence,
			"occurred_at":     e.OccurredAt,
			"kind":            string(e.Kind),
			"parent_sequence": e.ParentSequence,
			"phase":           e.Phase,
			"agent_label":     e.AgentLabel,
		}
		addEventPayload(ctx, row, e)
		rows = append(rows, row)
	}
	return rows
}

// addEventPayload fills the kind-specific cells of an event row. Exactly one
// payload pointer is set on a valid event (model.JobRunEvent.Validate enforces
// it), so at most one branch contributes.
func addEventPayload(ctx context.Context, row map[string]any, e *model.JobRunEvent) {
	if p := e.LLMRequest; p != nil {
		row["model"] = p.Model
		row["conversation_id"] = p.ConversationID
		row["messages_prefix_len"] = int64(p.MessagesPrefixLen)
		row["messages_json"] = encodeEventJSON(ctx, e, "messages", p.Messages)
		row["tools_json"] = encodeEventJSON(ctx, e, "tools", p.Tools)
	}
	if p := e.LLMResponse; p != nil {
		row["model"] = p.Model
		row["texts_json"] = encodeEventJSON(ctx, e, "texts", p.Texts)
		row["function_calls_json"] = encodeEventJSON(ctx, e, "function_calls", p.FunctionCalls)
		row["input_tokens"] = p.InputTokens
		row["output_tokens"] = p.OutputTokens
		row["cache_creation_input_tokens"] = p.CacheCreationInputTokens
		row["cache_read_input_tokens"] = p.CacheReadInputTokens
		row["duration_ms"] = p.DurationMs
	}
	if p := e.ToolCall; p != nil {
		row["tool_name"] = p.ToolName
		// Already JSON as stored by the trace handler; kept verbatim.
		row["tool_arguments_json"] = p.ArgumentsJSON
		row["tool_result_json"] = p.ResultJSON
		row["tool_is_error"] = p.IsError
		row["tool_error_message"] = p.ErrorMessage
		row["tool_started_at"] = p.StartedAt
		row["tool_ended_at"] = p.EndedAt
	}
	if p := e.RunError; p != nil {
		row["error_stage"] = p.Stage
		row["error_message"] = p.Message
	}
}

// encodeEventJSON marshals a structured payload field to its JSON column value.
// A marshal failure is reported (non-fatal) and the cell left NULL — one
// unencodable payload must not abort the export of the rest of the timeline.
//
// An absent field marshals to "null" and is written as NULL; an empty slice is
// preserved as "[]". That distinction does NOT reach BigQuery today, because both
// repository backends decode a stored empty array back into a nil slice (pinned
// by "Append + List returns an empty payload slice as nil" in
// pkg/repository/job_run_test.go), so an empty payload arrives here as nil. The
// encoding is kept faithful anyway so this layer is not the one losing the
// information; see docs/export.md for what a consumer can actually rely on.
func encodeEventJSON(ctx context.Context, e *model.JobRunEvent, field string, v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "marshal job run event payload field",
			goerr.V("field", field),
			goerr.V("run_id", e.RunID),
			goerr.V("sequence", e.Sequence)),
			"export job run event payload")
		return nil
	}
	if string(b) == "null" {
		return nil
	}
	if len(b) > maxEventJSONBytes {
		errutil.Handle(ctx, goerr.New("job run event payload field exceeds the export cell limit",
			goerr.V("field", field),
			goerr.V("run_id", e.RunID),
			goerr.V("sequence", e.Sequence),
			goerr.V("bytes", len(b)),
			goerr.V("limit_bytes", maxEventJSONBytes)),
			"export job run event payload")
		return oversizedJSONPlaceholder(len(b))
	}
	return string(b)
}

// maxEventJSONBytes caps one job_run_events JSON cell. Each individual payload
// string is already truncated to model.MaxInlineBytes when it is recorded, but
// the arrays holding them are unbounded — an LLM_REQUEST carries the whole
// conversation as of that call — so the marshalled column needs its own cap.
// Without it a single row can outgrow the Storage Write API's per-request
// limit, which no batching can split.
const maxEventJSONBytes = model.MaxInlineBytes

// oversizedJSONPlaceholder is written in place of a JSON cell over the cap.
// Cutting the JSON at a byte offset would produce a value every consumer fails
// to parse, so the payload is replaced wholesale by a valid document that
// records how large the original was.
func oversizedJSONPlaceholder(originalBytes int) string {
	return fmt.Sprintf(`{"oversized":true,"original_bytes":%d}`, originalBytes)
}

// knowledgeColumns describes the "knowledge" table (Embedding is intentionally
// excluded — it is never exposed).
func knowledgeColumns() []Column {
	return []Column{
		{Name: "id", Type: TypeString},
		{Name: "workspace_id", Type: TypeString, Nullable: true},
		{Name: "title", Type: TypeString, Nullable: true},
		{Name: "claim", Type: TypeString, Nullable: true},
		{Name: "tag_ids", Type: TypeString, Repeated: true},
		{Name: "creator_id", Type: TypeString, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
}

// knowledgeRows renders the "knowledge" table's rows.
func knowledgeRows(entries []*model.Knowledge) []map[string]any {
	rows := make([]map[string]any, 0, len(entries))
	for _, k := range entries {
		rows = append(rows, map[string]any{
			"id":           string(k.ID),
			"workspace_id": k.WorkspaceID,
			"title":        k.Title,
			"claim":        k.Claim,
			"tag_ids":      tagIDStrings(k.TagIDs),
			"creator_id":   k.CreatorID,
			"created_at":   k.CreatedAt,
			"updated_at":   k.UpdatedAt,
		})
	}
	return rows
}

// tagColumns describes the "tags" table.
func tagColumns() []Column {
	return []Column{
		{Name: "id", Type: TypeString},
		{Name: "workspace_id", Type: TypeString, Nullable: true},
		{Name: "name", Type: TypeString, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
}

// tagRows renders the "tags" table's rows.
func tagRows(tags []*model.Tag) []map[string]any {
	rows := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		rows = append(rows, map[string]any{
			"id":           string(t.ID),
			"workspace_id": t.WorkspaceID,
			"name":         t.Name,
			"created_at":   t.CreatedAt,
			"updated_at":   t.UpdatedAt,
		})
	}
	return rows
}

// fixedCaseColumns returns the non-custom columns of the cases table. id is the
// only REQUIRED column; the rest are nullable so a partially-populated case
// never fails the write.
func fixedCaseColumns() []Column {
	return []Column{
		{Name: "id", Type: TypeInt},
		{Name: "title", Type: TypeString, Nullable: true},
		{Name: "description", Type: TypeString, Nullable: true},
		{Name: "status", Type: TypeString, Nullable: true},
		{Name: "board_status", Type: TypeString, Nullable: true},
		{Name: "reporter_id", Type: TypeString, Nullable: true},
		{Name: "assignee_ids", Type: TypeString, Repeated: true},
		{Name: "channel_user_ids", Type: TypeString, Repeated: true},
		{Name: "slack_channel_id", Type: TypeString, Nullable: true},
		{Name: "slack_thread_ts", Type: TypeString, Nullable: true},
		{Name: "is_private", Type: TypeBool, Nullable: true},
		{Name: "is_test", Type: TypeBool, Nullable: true},
		{Name: "request_key", Type: TypeString, Nullable: true},
		{Name: "archived_at", Type: TypeTimestamp, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
}

// fixedMemoColumns returns the non-custom columns of the memos table.
func fixedMemoColumns() []Column {
	return []Column{
		{Name: "id", Type: TypeString},
		{Name: "workspace_id", Type: TypeString, Nullable: true},
		{Name: "case_id", Type: TypeInt, Nullable: true},
		{Name: "title", Type: TypeString, Nullable: true},
		{Name: "creator_id", Type: TypeString, Nullable: true},
		{Name: "archived_at", Type: TypeTimestamp, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
}

// customFieldColumns maps a workspace field schema to one "field_<id>" column
// each. Returns nil when the schema is nil/empty.
func customFieldColumns(schema *config.FieldSchema) []Column {
	if schema == nil {
		return nil
	}
	cols := make([]Column, 0, len(schema.Fields))
	for _, fd := range schema.Fields {
		cols = append(cols, fieldColumn(fd))
	}
	return cols
}

// fieldColumn maps one custom field definition to its output column.
func fieldColumn(fd config.FieldDefinition) Column {
	c := Column{Name: customFieldPrefix + fd.ID, Nullable: true}
	switch fd.Type {
	case types.FieldTypeNumber:
		c.Type = TypeFloat
	case types.FieldTypeMultiSelect, types.FieldTypeMultiUser, types.FieldTypeMultiCaseRef:
		c.Type = TypeString
		c.Repeated = true
	default: // text, markdown, url, select, user, case_ref, date
		c.Type = TypeString
	}
	return c
}

// addCustomFieldValues fills the "field_<id>" cells for the fields present in
// values. A stored value whose Go type does not match its declared field type
// is reported (non-fatal) and left NULL, never crashing the export.
func addCustomFieldValues(ctx context.Context, row map[string]any, schema *config.FieldSchema, values map[string]model.FieldValue) {
	if schema == nil {
		return
	}
	for _, fd := range schema.Fields {
		fv, ok := values[fd.ID]
		if !ok {
			continue
		}
		v, ok := normalizeFieldValue(fd.Type, fv.Value)
		if !ok {
			// Report the anomaly without the raw value (may be sensitive); the
			// field id and declared type are enough to locate the bad data.
			errutil.Handle(ctx, goerr.New("unexpected custom field value type; cell exported as NULL",
				goerr.V("field_id", fd.ID), goerr.V("field_type", fd.Type)),
				"export custom field normalization")
			continue
		}
		row[customFieldPrefix+fd.ID] = v
	}
}

// normalizeFieldValue coerces a stored FieldValue.Value to the natural Go type
// expected for its column. The bool result is false when the value has an
// unexpected type (the caller then leaves the cell NULL).
func normalizeFieldValue(ft types.FieldType, v any) (any, bool) {
	switch ft {
	case types.FieldTypeNumber:
		return normalizeNumber(v)
	case types.FieldTypeMultiSelect, types.FieldTypeMultiUser, types.FieldTypeMultiCaseRef:
		return normalizeStringSlice(v)
	case types.FieldTypeDate:
		return normalizeDate(v)
	default: // text, markdown, url, select, user, case_ref -> STRING
		s, ok := v.(string)
		return s, ok
	}
}

// normalizeDate coerces a date field value to a STRING cell. A date is stored
// as either an RFC3339 / "YYYY-MM-DD" string or a time.Time (see
// model.FieldValidator.validateDate); both are kept losslessly — the string
// verbatim, the time.Time formatted as RFC3339Nano.
func normalizeDate(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case time.Time:
		return t.Format(time.RFC3339Nano), true
	default:
		return nil, false
	}
}

func normalizeNumber(v any) (any, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	default:
		return nil, false
	}
}

func normalizeStringSlice(v any) (any, bool) {
	switch t := v.(type) {
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// tagIDStrings converts a slice of TagID to plain strings.
func tagIDStrings(ids []model.TagID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}
