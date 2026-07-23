package bqexport_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/m-mizutani/bqs"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/service/bqexport"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestDiffSchema(t *testing.T) {
	base := bqexport.ToBQSchemaForTest([]export.Column{
		{Name: "id", Type: export.TypeInt},
		{Name: "title", Type: export.TypeString, Nullable: true},
	})

	t.Run("no change", func(t *testing.T) {
		merged, changed, err := bqexport.DiffSchemaForTest(base, base)
		gt.NoError(t, err).Required()
		gt.Bool(t, changed).False()
		gt.Array(t, merged).Length(2)
	})

	t.Run("additive change detected", func(t *testing.T) {
		desired := bqexport.ToBQSchemaForTest([]export.Column{
			{Name: "id", Type: export.TypeInt},
			{Name: "title", Type: export.TypeString, Nullable: true},
			{Name: "field_new", Type: export.TypeString, Nullable: true},
		})
		merged, changed, err := bqexport.DiffSchemaForTest(base, desired)
		gt.NoError(t, err).Required()
		gt.Bool(t, changed).True()
		gt.Array(t, merged).Length(3)
	})

	t.Run("type conflict is an error", func(t *testing.T) {
		conflicting := bqexport.ToBQSchemaForTest([]export.Column{
			{Name: "id", Type: export.TypeInt},
			{Name: "title", Type: export.TypeInt, Nullable: true}, // was STRING
		})
		_, _, err := bqexport.DiffSchemaForTest(base, conflicting)
		gt.Error(t, err).Is(bqs.ErrConflictField)
	})
}

func TestIsSchemaMismatch(t *testing.T) {
	// A gRPC status carrying a StorageError with SCHEMA_MISMATCH_EXTRA_FIELDS is
	// the retryable schema-propagation error.
	st, err := status.New(codes.InvalidArgument, "schema mismatch").WithDetails(
		&storagepb.StorageError{Code: storagepb.StorageError_SCHEMA_MISMATCH_EXTRA_FIELDS},
	)
	gt.NoError(t, err).Required()
	gt.Bool(t, bqexport.IsSchemaMismatchForTest(st.Err())).True()

	// A different StorageError code is not retryable.
	other, err := status.New(codes.InvalidArgument, "offset").WithDetails(
		&storagepb.StorageError{Code: storagepb.StorageError_OFFSET_ALREADY_EXISTS},
	)
	gt.NoError(t, err).Required()
	gt.Bool(t, bqexport.IsSchemaMismatchForTest(other.Err())).False()

	// A plain error is not a schema mismatch.
	gt.Bool(t, bqexport.IsSchemaMismatchForTest(errors.New("boom"))).False()
	gt.Bool(t, bqexport.IsSchemaMismatchForTest(nil)).False()
}

func TestPreserveExistingSchema(t *testing.T) {
	// An existing column carrying policy tags / description must survive an
	// additive schema update; a genuinely new column is appended at the end.
	existing := bq.Schema{
		{Name: "id", Type: bq.IntegerFieldType, Required: true},
		{Name: "title", Type: bq.StringFieldType, Description: "the title",
			PolicyTags: &bq.PolicyTagList{Names: []string{"projects/p/locations/l/taxonomies/t/policyTags/1"}}},
	}
	desired := bqexport.ToBQSchemaForTest([]export.Column{
		{Name: "id", Type: export.TypeInt},
		{Name: "title", Type: export.TypeString, Nullable: true}, // no PolicyTags/Description
		{Name: "field_new", Type: export.TypeString, Nullable: true},
	})

	result := bqexport.PreserveExistingSchemaForTest(existing, desired)

	gt.Array(t, result).Length(3).Required()
	gt.Value(t, result[0].Name).Equal("id")
	gt.Value(t, result[1].Name).Equal("title")
	// existing column metadata preserved (not overwritten by desired's bare field)
	gt.Value(t, result[1].Description).Equal("the title")
	gt.Value(t, result[1].PolicyTags).NotNil().Required()
	gt.Array(t, result[1].PolicyTags.Names).Equal([]string{"projects/p/locations/l/taxonomies/t/policyTags/1"})
	// new column appended at the end
	gt.Value(t, result[2].Name).Equal("field_new")
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

// countRows returns the number of rows in dataset.table.
func countRows(t *testing.T, ctx context.Context, client *bq.Client, dataset, table string) int {
	t.Helper()
	it := client.Dataset(dataset).Table(table).Read(ctx)
	n := 0
	for {
		var row map[string]bq.Value
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		gt.NoError(t, err).Required()
		n++
	}
	return n
}

// TestSink_LiveBigQuery exercises the sink-level edges against a real dataset:
// create+append, full-refresh to zero rows, and a type conflict surfacing as an
// error. Gated on TEST_BIGQUERY_PROJECT_ID / TEST_BIGQUERY_DATASET_ID.
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

	tableName := fmt.Sprintf("bqexport_live_%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = client.Dataset(dataset).Table(tableName).Delete(ctx) })

	cols := []export.Column{
		{Name: "id", Type: export.TypeInt},
		{Name: "name", Type: export.TypeString, Nullable: true},
		{Name: "at", Type: export.TypeTimestamp, Nullable: true},
	}
	now := time.Now()

	// Create + append two rows.
	gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{
		Name:    tableName,
		Columns: cols,
		Rows: []map[string]any{
			{"id": int64(1), "name": "a", "at": now},
			{"id": int64(2), "name": "b", "at": now},
		},
	})).Required()
	gt.Number(t, countRows(t, ctx, client, dataset, tableName)).Equal(2)

	// Full refresh to zero rows: the table is truncated, schema preserved.
	gt.NoError(t, sink.WriteTable(ctx, dataset, &export.Table{Name: tableName, Columns: cols})).Required()
	gt.Number(t, countRows(t, ctx, client, dataset, tableName)).Equal(0)

	// Type conflict (id STRING vs INT64) surfaces as an error; the table stands.
	err = sink.WriteTable(ctx, dataset, &export.Table{
		Name:    tableName,
		Columns: []export.Column{{Name: "id", Type: export.TypeString}},
	})
	gt.Error(t, err)
}
