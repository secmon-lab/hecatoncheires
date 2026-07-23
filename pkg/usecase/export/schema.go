package export

import (
	"context"
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

// buildCaseTable builds the "cases" table: fixed Case columns plus one
// "field_<id>" column per workspace field definition.
func buildCaseTable(ctx context.Context, schema *config.FieldSchema, cases []*model.Case) *Table {
	cols := append(fixedCaseColumns(), customFieldColumns(schema)...)
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
			"created_at":       c.CreatedAt,
			"updated_at":       c.UpdatedAt,
		}
		addCustomFieldValues(ctx, row, schema, c.FieldValues)
		rows = append(rows, row)
	}
	return &Table{Name: "cases", Columns: cols, Rows: rows}
}

// buildActionTable builds the "actions" table.
func buildActionTable(actions []*model.Action) *Table {
	cols := []Column{
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
	return &Table{Name: "actions", Columns: cols, Rows: rows}
}

// buildMemoTable builds the "memos" table: fixed Memo columns plus the
// workspace's memo field schema (nil-safe when memos are disabled).
func buildMemoTable(ctx context.Context, memoConfig *config.MemoConfig, memos []*model.Memo) *Table {
	var schema *config.FieldSchema
	if memoConfig != nil {
		schema = memoConfig.FieldSchema
	}
	cols := append(fixedMemoColumns(), customFieldColumns(schema)...)
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
	return &Table{Name: "memos", Columns: cols, Rows: rows}
}

// buildKnowledgeTable builds the "knowledge" table (Embedding is intentionally
// excluded — it is never exposed).
func buildKnowledgeTable(entries []*model.Knowledge) *Table {
	cols := []Column{
		{Name: "id", Type: TypeString},
		{Name: "workspace_id", Type: TypeString, Nullable: true},
		{Name: "title", Type: TypeString, Nullable: true},
		{Name: "claim", Type: TypeString, Nullable: true},
		{Name: "tag_ids", Type: TypeString, Repeated: true},
		{Name: "creator_id", Type: TypeString, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
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
	return &Table{Name: "knowledge", Columns: cols, Rows: rows}
}

// buildTagTable builds the "tags" table.
func buildTagTable(tags []*model.Tag) *Table {
	cols := []Column{
		{Name: "id", Type: TypeString},
		{Name: "workspace_id", Type: TypeString, Nullable: true},
		{Name: "name", Type: TypeString, Nullable: true},
		{Name: "created_at", Type: TypeTimestamp, Nullable: true},
		{Name: "updated_at", Type: TypeTimestamp, Nullable: true},
	}
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
	return &Table{Name: "tags", Columns: cols, Rows: rows}
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
