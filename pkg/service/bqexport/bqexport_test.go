package bqexport_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/service/bqexport"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

func TestToBQSchema(t *testing.T) {
	schema := bqexport.ToBQSchemaForTest([]export.Column{
		{Name: "id", Type: export.TypeInt}, // REQUIRED
		{Name: "title", Type: export.TypeString, Nullable: true},
		{Name: "score", Type: export.TypeFloat, Nullable: true},
		{Name: "flag", Type: export.TypeBool, Nullable: true},
		{Name: "at", Type: export.TypeTimestamp, Nullable: true},
		{Name: "labels", Type: export.TypeString, Repeated: true}, // REPEATED, not REQUIRED
	})

	gt.Array(t, schema).Length(6)

	byName := map[string]*bq.FieldSchema{}
	for _, f := range schema {
		byName[f.Name] = f
	}
	gt.Value(t, byName["id"].Type).Equal(bq.IntegerFieldType)
	gt.Bool(t, byName["id"].Required).True()
	gt.Value(t, byName["title"].Type).Equal(bq.StringFieldType)
	gt.Bool(t, byName["title"].Required).False()
	gt.Value(t, byName["score"].Type).Equal(bq.FloatFieldType)
	gt.Value(t, byName["flag"].Type).Equal(bq.BooleanFieldType)
	gt.Value(t, byName["at"].Type).Equal(bq.TimestampFieldType)
	gt.Bool(t, byName["labels"].Repeated).True()
	gt.Bool(t, byName["labels"].Required).False()
}

func TestStagingTableName(t *testing.T) {
	// The generated name is interpolated into a SQL statement, so it must stay
	// inside the identifier charset the sink validates against.
	safeIdent := func(s string) bool {
		for _, r := range s {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			default:
				return false
			}
		}
		return s != ""
	}

	first := bqexport.StagingTableNameForTest("job_run_events")
	second := bqexport.StagingTableNameForTest("job_run_events")

	gt.Bool(t, strings.HasPrefix(first, "job_run_events_stg_")).True()
	gt.Bool(t, safeIdent(first)).True()
	gt.Bool(t, safeIdent(second)).True()
	// A per-call random suffix is what keeps concurrent runs from colliding.
	gt.Value(t, first).NotEqual(second)
	// uuid without hyphens is 32 hex characters.
	gt.Number(t, len(strings.TrimPrefix(first, "job_run_events_stg_"))).Equal(32)
}

func TestDDLColumnList(t *testing.T) {
	cols := []export.Column{
		{Name: "id", Type: export.TypeInt},                        // REQUIRED
		{Name: "title", Type: export.TypeString, Nullable: true},  // NULLABLE
		{Name: "score", Type: export.TypeFloat, Nullable: true},   // NULLABLE
		{Name: "flag", Type: export.TypeBool, Nullable: true},     // NULLABLE
		{Name: "at", Type: export.TypeTimestamp, Nullable: true},  // NULLABLE
		{Name: "labels", Type: export.TypeString, Repeated: true}, // REPEATED
	}

	// GoogleSQL DDL type names, not the legacy bq.FieldType spellings: a
	// column rendered as INTEGER/FLOAT/BOOLEAN would fail the swap statement.
	gt.String(t, bqexport.DDLColumnListForTest(cols)).Equal(
		"`id` INT64 NOT NULL, `title` STRING, `score` FLOAT64, `flag` BOOL, " +
			"`at` TIMESTAMP, `labels` ARRAY<STRING>")
	gt.String(t, bqexport.DDLSelectListForTest(cols)).Equal(
		"`id`, `title`, `score`, `flag`, `at`, `labels`")
}

func TestEncodeRow(t *testing.T) {
	schema := bqexport.ToBQSchemaForTest([]export.Column{
		{Name: "id", Type: export.TypeInt},
		{Name: "title", Type: export.TypeString, Nullable: true},
		{Name: "at", Type: export.TypeTimestamp, Nullable: true},
	})
	msgDesc, _, err := bqexport.RowDescriptorForTest(schema)
	gt.NoError(t, err).Required()
	colTypes := map[string]export.ColumnType{
		"id": export.TypeInt, "title": export.TypeString, "at": export.TypeTimestamp,
	}

	t.Run("a populated row encodes to non-empty proto bytes", func(t *testing.T) {
		b, err := bqexport.EncodeRowForTest(msgDesc, colTypes, map[string]any{
			"id": int64(7), "title": "hello", "at": time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
		})
		gt.NoError(t, err).Required()
		gt.Number(t, len(b)).GreaterOrEqual(1)
	})

	t.Run("row size grows with the payload it carries", func(t *testing.T) {
		// The byte-bounded batching relies on the encoded length tracking the
		// actual payload, so a bigger cell must produce a bigger row.
		small, err := bqexport.EncodeRowForTest(msgDesc, colTypes, map[string]any{
			"id": int64(1), "title": "x",
		})
		gt.NoError(t, err).Required()
		large, err := bqexport.EncodeRowForTest(msgDesc, colTypes, map[string]any{
			"id": int64(1), "title": strings.Repeat("x", 4096),
		})
		gt.NoError(t, err).Required()
		gt.Number(t, len(large)-len(small)).GreaterOrEqual(4000)
	})

	t.Run("an unencodable value is an error", func(t *testing.T) {
		_, err := bqexport.EncodeRowForTest(msgDesc, colTypes, map[string]any{
			"id": int64(1), "at": "not-a-time",
		})
		gt.Error(t, err)
	})
}

func TestEncodeValue(t *testing.T) {
	t.Run("timestamp time.Time to micros", func(t *testing.T) {
		ts := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
		v, include, err := bqexport.EncodeValueForTest(export.TypeTimestamp, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, include).True()
		gt.Value(t, v).Equal(ts.UnixMicro())
	})

	t.Run("timestamp non-nil pointer", func(t *testing.T) {
		ts := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
		v, include, err := bqexport.EncodeValueForTest(export.TypeTimestamp, &ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, include).True()
		gt.Value(t, v).Equal(ts.UnixMicro())
	})

	t.Run("nil pointer timestamp omitted", func(t *testing.T) {
		var ts *time.Time
		_, include, err := bqexport.EncodeValueForTest(export.TypeTimestamp, ts)
		gt.NoError(t, err).Required()
		gt.Bool(t, include).False()
	})

	t.Run("zero time omitted", func(t *testing.T) {
		_, include, err := bqexport.EncodeValueForTest(export.TypeTimestamp, time.Time{})
		gt.NoError(t, err).Required()
		gt.Bool(t, include).False()
	})

	t.Run("nil value omitted", func(t *testing.T) {
		_, include, err := bqexport.EncodeValueForTest(export.TypeString, nil)
		gt.NoError(t, err).Required()
		gt.Bool(t, include).False()
	})

	t.Run("string passthrough", func(t *testing.T) {
		v, include, err := bqexport.EncodeValueForTest(export.TypeString, "hello")
		gt.NoError(t, err).Required()
		gt.Bool(t, include).True()
		gt.Value(t, v).Equal("hello")
	})

	t.Run("unexpected timestamp type errors", func(t *testing.T) {
		_, _, err := bqexport.EncodeValueForTest(export.TypeTimestamp, "not-a-time")
		gt.Error(t, err)
	})
}

// readRows returns every row of dataset.table keyed by column name.
func readRows(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) []map[string]bq.Value {
	t.Helper()
	it := client.Dataset(dataset).Table(table).Read(ctx)
	var out []map[string]bq.Value
	for {
		var row map[string]bq.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		gt.NoError(t, err).Required()
		out = append(out, row)
	}
	return out
}

// tableSchema returns the live schema of dataset.table indexed by column name.
func tableSchema(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) map[string]*bq.FieldSchema {
	t.Helper()
	md, err := client.Dataset(dataset).Table(table).Metadata(ctx)
	gt.NoError(t, err).Required()
	byName := map[string]*bq.FieldSchema{}
	for _, f := range md.Schema {
		byName[f.Name] = f
	}
	return byName
}

// deleteTableOnCleanup registers dataset.table for deletion and fails the test
// if the delete does not go through — a live test that quietly leaves tables
// behind in the shared test dataset would keep passing while it litters. A 404
// is the one tolerated outcome: a subtest can fail before its table is ever
// created, and turning that into a cleanup failure would bury the real one.
func deleteTableOnCleanup(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) {
	t.Helper()
	t.Cleanup(func() {
		err := client.Dataset(dataset).Table(table).Delete(ctx)
		if err == nil {
			return
		}
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == 404 {
			return
		}
		gt.NoError(t, err)
	})
}

// tableIDsWithPrefix lists the tables of a dataset whose id starts with prefix.
func tableIDsWithPrefix(t *testing.T, ctx context.Context, client *bq.Client, dataset, prefix string) []string {
	t.Helper()
	it := client.Dataset(dataset).Tables(ctx)
	var out []string
	for {
		tbl, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		gt.NoError(t, err).Required()
		if strings.HasPrefix(tbl.TableID, prefix) {
			out = append(out, tbl.TableID)
		}
	}
	return out
}

// TestSink_LiveBigQuery exercises the sink against a real dataset. It pins the
// behaviour the staging-swap design exists for: the destination ends up with
// exactly the desired schema no matter what was there before, and a write
// larger than one AppendRows request still lands in full. Gated on
// TEST_BIGQUERY_PROJECT_ID / TEST_BIGQUERY_DATASET_ID.
func TestSink_LiveBigQuery(t *testing.T) {
	project := os.Getenv("TEST_BIGQUERY_PROJECT_ID")
	dataset := os.Getenv("TEST_BIGQUERY_DATASET_ID")
	if project == "" || dataset == "" {
		t.Skip("TEST_BIGQUERY_PROJECT_ID / TEST_BIGQUERY_DATASET_ID not set; skipping live BigQuery sink test")
	}
	location := os.Getenv("TEST_BIGQUERY_LOCATION")
	ctx := context.Background()

	sink, err := bqexport.New(ctx, project, location)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { safe.Close(ctx, sink) })

	client, err := bq.NewClient(ctx, project)
	gt.NoError(t, err).Required()
	t.Cleanup(func() { safe.Close(ctx, client) })

	// newTable returns a unique table name registered for cleanup.
	newTable := func(t *testing.T, kind string) string {
		t.Helper()
		name := fmt.Sprintf("bqexport_live_%s_%d", kind, time.Now().UnixNano())
		deleteTableOnCleanup(t, ctx, client, dataset, name)
		return name
	}

	cols := []export.Column{
		{Name: "id", Type: export.TypeInt},
		{Name: "name", Type: export.TypeString, Nullable: true},
		{Name: "at", Type: export.TypeTimestamp, Nullable: true},
	}

	t.Run("create then full refresh", func(t *testing.T) {
		table := newTable(t, "refresh")
		now := time.Now()

		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: cols,
			Rows: []map[string]any{
				{"id": int64(1), "name": "a", "at": now},
				{"id": int64(2), "name": "b", "at": now},
			},
		})).Required()
		rows := readRows(t, ctx, client, dataset, table)
		gt.Array(t, rows).Length(2)

		// A refresh that produced no rows must empty the table, not leave the
		// previous snapshot behind.
		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name: table, Columns: cols,
		})).Required()
		gt.Array(t, readRows(t, ctx, client, dataset, table)).Length(0)

		schema := tableSchema(t, ctx, client, dataset, table)
		gt.Value(t, schema["id"].Type).Equal(bq.IntegerFieldType)
		gt.Value(t, schema["name"].Type).Equal(bq.StringFieldType)
		gt.Value(t, schema["at"].Type).Equal(bq.TimestampFieldType)
	})

	t.Run("a column whose type changed is replaced, not rejected", func(t *testing.T) {
		table := newTable(t, "retype")

		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: []export.Column{{Name: "id", Type: export.TypeString, Nullable: true}},
			Rows:    []map[string]any{{"id": "one"}},
		})).Required()
		gt.Value(t, tableSchema(t, ctx, client, dataset, table)["id"].Type).Equal(bq.StringFieldType)

		// Same column, incompatible type. This is the production failure:
		// previously it errored with "schema conflict; manual migration required".
		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: []export.Column{{Name: "id", Type: export.TypeInt, Nullable: true}},
			Rows:    []map[string]any{{"id": int64(1)}},
		})).Required()

		schema := tableSchema(t, ctx, client, dataset, table)
		gt.Value(t, schema["id"].Type).Equal(bq.IntegerFieldType)
		rows := readRows(t, ctx, client, dataset, table)
		gt.Array(t, rows).Length(1).Required()
		gt.Value(t, rows[0]["id"]).Equal(int64(1))
	})

	t.Run("a NULLABLE column that became REPEATED is replaced", func(t *testing.T) {
		// The exact shape of the ws_product_mgmt outage: a workspace field
		// changed from a single-value type to a multi-select one.
		table := newTable(t, "repeat")

		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: []export.Column{{Name: "field_user_type", Type: export.TypeString, Nullable: true}},
			Rows:    []map[string]any{{"field_user_type": "doctor"}},
		})).Required()
		gt.Bool(t, tableSchema(t, ctx, client, dataset, table)["field_user_type"].Repeated).False()

		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: []export.Column{{Name: "field_user_type", Type: export.TypeString, Repeated: true}},
			Rows:    []map[string]any{{"field_user_type": []string{"doctor", "nurse"}}},
		})).Required()

		gt.Bool(t, tableSchema(t, ctx, client, dataset, table)["field_user_type"].Repeated).True()
		rows := readRows(t, ctx, client, dataset, table)
		gt.Array(t, rows).Length(1).Required()
		gt.Value(t, rows[0]["field_user_type"]).Equal([]bq.Value{"doctor", "nurse"})
	})

	t.Run("a column no longer produced disappears", func(t *testing.T) {
		table := newTable(t, "drop")

		withExtra := append(append([]export.Column{}, cols...),
			export.Column{Name: "extra", Type: export.TypeString, Nullable: true})
		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: withExtra,
			Rows:    []map[string]any{{"id": int64(1), "extra": "legacy"}},
		})).Required()
		gt.Map(t, tableSchema(t, ctx, client, dataset, table)).HasKey("extra")

		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name:    table,
			Columns: cols,
			Rows:    []map[string]any{{"id": int64(1)}},
		})).Required()

		schema := tableSchema(t, ctx, client, dataset, table)
		gt.Map(t, schema).HasKey("id")
		gt.Map(t, schema).NotHasKey("extra")
	})

	t.Run("no staging table survives the write", func(t *testing.T) {
		table := newTable(t, "staging")
		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name: table, Columns: cols, Rows: []map[string]any{{"id": int64(1)}},
		})).Required()
		gt.Array(t, tableIDsWithPrefix(t, ctx, client, dataset, table+"_stg_")).Length(0)
	})

	t.Run("a payload larger than one append request lands in full", func(t *testing.T) {
		table := newTable(t, "bigrows")

		// Eight ~1.5 MB rows total ~12 MB, comfortably past the 9 MB per-request
		// budget, so the write must span more than one AppendRows call.
		const rowPayloadBytes = 1_500_000
		const rowCount = 8
		gt.Number(t, rowPayloadBytes*rowCount).Greater(bqexport.AppendMaxRequestBytesForTest)

		bigCols := []export.Column{
			{Name: "id", Type: export.TypeInt},
			{Name: "blob", Type: export.TypeString, Nullable: true},
		}
		rows := make([]map[string]any, 0, rowCount)
		for i := range rowCount {
			rows = append(rows, map[string]any{
				"id":   int64(i),
				"blob": strings.Repeat(string(rune('a'+i)), rowPayloadBytes),
			})
		}
		gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
			Name: table, Columns: bigCols, Rows: rows,
		})).Required()

		got := readRows(t, ctx, client, dataset, table)
		gt.Array(t, got).Length(rowCount).Required()
		byID := map[int64]string{}
		for _, r := range got {
			id, ok := r["id"].(int64)
			gt.Bool(t, ok).True().Required()
			blob, ok := r["blob"].(string)
			gt.Bool(t, ok).True().Required()
			byID[id] = blob
		}
		for i := range rowCount {
			blob, ok := byID[int64(i)]
			gt.Bool(t, ok).True().Required()
			gt.Number(t, len(blob)).Equal(rowPayloadBytes)
			gt.String(t, blob).Equal(strings.Repeat(string(rune('a'+i)), rowPayloadBytes))
		}
	})
}
