// Package bqexport is the BigQuery implementation of export.Sink (an
// external-system adapter, hence pkg/service). Each WriteTable is a full refresh
// (洗替) of one table: it evolves the table schema in place (additive, via
// bqs.Merge + Table.Update — never a drop/recreate), TRUNCATEs the existing
// rows, then appends the fresh rows through the Storage Write API (managedwriter,
// PendingStream). The schema-diff and Storage-Write mechanics follow
// secmon-lab/swarm's pkg/infra/bq; the TRUNCATE-based full refresh is added here
// because the Storage Write API is append-only and has no truncate mode of its
// own.
package bqexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	mw "cloud.google.com/go/bigquery/storage/managedwriter"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/m-mizutani/bqs"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	// appendBatchRows bounds how many rows go in one AppendRows call (matches
	// swarm's batch size).
	appendBatchRows = 256
	// descriptorScope is the arbitrary root message name for the generated proto
	// descriptor.
	descriptorScope = "root"

	// Schema-propagation retry bounds. After a Table.Update, the Storage Write
	// backend can lag several minutes before it accepts the new columns; until
	// then an append returns SCHEMA_MISMATCH_EXTRA_FIELDS. These bound the
	// exponential backoff that absorbs that lag. They are properties of the
	// BigQuery service, not deployment-tunable configuration, so they live here.
	schemaPropagationMaxElapsed = 15 * time.Minute
	backoffInitialInterval      = 10 * time.Second
	backoffMultiplier           = 2.0
	backoffMaxInterval          = 2 * time.Minute
)

// Identifier allow-lists validated at the adapter boundary before an identifier
// is interpolated into a SQL statement. The sink does not trust its caller (the
// config layer validates too, but a public Sink method must guard itself).
var (
	// safeIdentPattern bounds dataset and table names (BigQuery's own charset).
	safeIdentPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	// safeProjectPattern bounds a GCP project id, allowing the legacy
	// "domain.com:project" form. It excludes backticks and other SQL-breaking
	// characters.
	safeProjectPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

// Sink writes export tables to BigQuery. It is safe to reuse across WriteTable
// calls; it holds no per-table state.
type Sink struct {
	bq       *bq.Client
	mw       *mw.Client
	project  string
	location string
}

var _ export.Sink = (*Sink)(nil)

// New creates a Sink with both a BigQuery client (metadata / TRUNCATE) and a
// managedwriter client (Storage Write appends). project is required; location is
// used only when a dataset must be created.
func New(ctx context.Context, project, location string) (*Sink, error) {
	if project == "" {
		return nil, goerr.New("bigquery project is required")
	}
	if !safeProjectPattern.MatchString(project) {
		return nil, goerr.New("invalid bigquery project id", goerr.V("project", project))
	}
	bqClient, err := bq.NewClient(ctx, project)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to create bigquery client", goerr.V("project", project))
	}
	mwClient, err := mw.NewClient(ctx, project)
	if err != nil {
		safe.Close(ctx, bqClient)
		return nil, goerr.Wrap(err, "failed to create managedwriter client", goerr.V("project", project))
	}
	return &Sink{bq: bqClient, mw: mwClient, project: project, location: location}, nil
}

// Close releases both underlying clients.
func (s *Sink) Close() error {
	var errs []error
	if err := s.mw.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := s.bq.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// WriteTable fully refreshes dataset.table: ensure dataset/table exist and the
// schema is evolved, TRUNCATE the old rows, then append the new rows.
func (s *Sink) WriteTable(ctx context.Context, dataset string, t *export.Table) error {
	if !safeIdentPattern.MatchString(dataset) {
		return goerr.New("invalid dataset name for export", goerr.V("dataset", dataset))
	}
	if !safeIdentPattern.MatchString(t.Name) {
		return goerr.New("invalid table name for export", goerr.V("table", t.Name))
	}
	location, err := s.ensureDataset(ctx, dataset)
	if err != nil {
		return err
	}
	desired := toBQSchema(t.Columns)
	schema, created, err := s.ensureTableSchema(ctx, dataset, t.Name, desired)
	if err != nil {
		return err
	}
	// A freshly created table is already empty — skip the TRUNCATE.
	if !created {
		if err := s.truncate(ctx, dataset, t.Name, location); err != nil {
			return err
		}
	}
	if len(t.Rows) == 0 {
		return nil
	}
	return s.appendRows(ctx, dataset, t.Name, schema, t.Columns, t.Rows)
}

// ensureDataset creates the dataset (with the configured location) when absent
// and returns the dataset's effective location. The location is used to run the
// TRUNCATE query job in the dataset's own region: the configured s.location is
// only a hint for creation and may differ from an existing dataset's real
// region, which would otherwise send the job to the wrong location.
func (s *Sink) ensureDataset(ctx context.Context, dataset string) (string, error) {
	ds := s.bq.DatasetInProject(s.project, dataset)
	if md, err := ds.Metadata(ctx); err == nil {
		return md.Location, nil
	} else if !isHTTPStatus(err, 404) {
		return "", goerr.Wrap(err, "failed to get dataset metadata", goerr.V("dataset", dataset))
	}
	md := &bq.DatasetMetadata{Name: dataset}
	if s.location != "" {
		md.Location = s.location
	}
	if err := ds.Create(ctx, md); err != nil {
		if isHTTPStatus(err, 409) { // lost a concurrent create; re-read for its location
			if m2, e2 := ds.Metadata(ctx); e2 == nil {
				return m2.Location, nil
			}
			return s.location, nil
		}
		return "", goerr.Wrap(err, "failed to create dataset", goerr.V("dataset", dataset))
	}
	return s.location, nil
}

// ensureTableSchema creates the table when absent, else evolves its schema
// additively (bqs.Merge) and applies the change with Table.Update. It returns
// the schema to write against and whether the table was just created.
func (s *Sink) ensureTableSchema(ctx context.Context, dataset, table string, desired bq.Schema) (bq.Schema, bool, error) {
	tbl := s.bq.DatasetInProject(s.project, dataset).Table(table)
	md, err := tbl.Metadata(ctx)
	if err != nil {
		if !isHTTPStatus(err, 404) {
			return nil, false, goerr.Wrap(err, "failed to get table metadata",
				goerr.V("dataset", dataset), goerr.V("table", table))
		}
		if err := tbl.Create(ctx, &bq.TableMetadata{Name: table, Schema: desired}); err != nil {
			return nil, false, goerr.Wrap(err, "failed to create table",
				goerr.V("dataset", dataset), goerr.V("table", table))
		}
		return desired, true, nil
	}

	// bqs.Merge is used ONLY to validate compatibility (type/mode conflicts) and
	// to detect whether anything changed — NOT as the schema to push. bqs.Merge
	// replaces a same-named field with the desired FieldSchema, which carries no
	// PolicyTags / Description, so pushing it would strip existing columns'
	// policy tags (column ACLs) and descriptions on Update. Instead push a schema
	// that keeps every existing column verbatim and only appends genuinely-new
	// columns at the end, so metadata and column ACLs survive schema evolution.
	_, changed, err := diffSchema(md.Schema, desired)
	if err != nil {
		// A type/mode conflict is surfaced, never auto-resolved by dropping the
		// column: manual migration is an operator decision.
		return nil, false, goerr.Wrap(err, "schema conflict; manual migration required",
			goerr.V("dataset", dataset), goerr.V("table", table))
	}
	if !changed {
		return md.Schema, false, nil
	}
	updateSchema := preserveExistingSchema(md.Schema, desired)
	if _, err := tbl.Update(ctx, bq.TableMetadataToUpdate{Schema: updateSchema}, md.ETag); err != nil {
		return nil, false, goerr.Wrap(err, "failed to update table schema",
			goerr.V("dataset", dataset), goerr.V("table", table))
	}
	return updateSchema, false, nil
}

// preserveExistingSchema builds the schema to push on an additive update: every
// existing column verbatim (retaining PolicyTags, Description, order), followed
// by the desired columns that do not yet exist. Existing columns are never
// rebuilt from desired, so their metadata survives; new columns are appended at
// the end as BigQuery requires. Columns present only in existing (e.g. a custom
// field removed from config) are retained — the export never drops columns.
func preserveExistingSchema(existing, desired bq.Schema) bq.Schema {
	have := make(map[string]bool, len(existing))
	result := make(bq.Schema, 0, len(existing)+len(desired))
	for _, f := range existing {
		result = append(result, f)
		have[f.Name] = true
	}
	for _, f := range desired {
		if !have[f.Name] {
			result = append(result, f)
		}
	}
	return result
}

// diffSchema merges desired into existing (additive; conflicts error) and
// reports whether the merge changed anything. It is a pure function so the
// schema-evolution policy is unit-testable without a BigQuery client.
func diffSchema(existing, desired bq.Schema) (bq.Schema, bool, error) {
	merged, err := bqs.Merge(existing, desired)
	if err != nil {
		return nil, false, err
	}
	if bqs.Equal(existing, merged) {
		return merged, false, nil
	}
	return merged, true, nil
}

// truncate empties the table, preserving its schema/labels (metadata operation).
// location is the dataset's real region, so the query job runs where the table
// lives.
func (s *Sink) truncate(ctx context.Context, dataset, table, location string) error {
	// Identifiers are validated at WriteTable's boundary before we get here.
	q := s.bq.Query(fmt.Sprintf("TRUNCATE TABLE `%s.%s.%s`", s.project, dataset, table))
	if location != "" {
		q.Location = location
	}
	job, err := q.Run(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to start truncate",
			goerr.V("dataset", dataset), goerr.V("table", table))
	}
	status, err := job.Wait(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to wait for truncate",
			goerr.V("dataset", dataset), goerr.V("table", table))
	}
	if err := status.Err(); err != nil {
		return goerr.Wrap(err, "truncate job failed",
			goerr.V("dataset", dataset), goerr.V("table", table))
	}
	return nil
}

// appendRows appends every row via a pending stream, retrying under a bounded
// exponential backoff when the append hits SCHEMA_MISMATCH_EXTRA_FIELDS (the
// multi-minute schema-propagation delay after a Table.Update).
func (s *Sink) appendRows(ctx context.Context, dataset, table string, schema bq.Schema, columns []export.Column, rows []map[string]any) error {
	rctx, cancel := context.WithTimeout(ctx, schemaPropagationMaxElapsed)
	defer cancel()

	interval := backoffInitialInterval
	for attempt := 1; ; attempt++ {
		err := s.appendOnce(rctx, dataset, table, schema, columns, rows)
		if err == nil {
			return nil
		}
		if !isSchemaMismatch(err) {
			return err
		}
		logging.From(ctx).Warn("append hit schema mismatch (propagation delay); retrying",
			"dataset", dataset, "table", table, "attempt", attempt)
		select {
		case <-rctx.Done():
			return goerr.Wrap(err, "append failed after schema-propagation retries",
				goerr.V("dataset", dataset), goerr.V("table", table))
		case <-time.After(interval):
		}
		interval = min(time.Duration(float64(interval)*backoffMultiplier), backoffMaxInterval)
	}
}

// appendOnce performs one full pending-stream write: build the descriptor from
// the current schema, append the rows in batches, finalize, and batch-commit.
func (s *Sink) appendOnce(ctx context.Context, dataset, table string, schema bq.Schema, columns []export.Column, rows []map[string]any) error {
	storageSchema, err := adapt.BQSchemaToStorageTableSchema(schema)
	if err != nil {
		return goerr.Wrap(err, "failed to convert schema to storage schema")
	}
	descriptor, err := adapt.StorageSchemaToProto2Descriptor(storageSchema, descriptorScope)
	if err != nil {
		return goerr.Wrap(err, "failed to build proto descriptor")
	}
	msgDesc, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		return goerr.New("storage schema did not produce a message descriptor")
	}
	dp, err := adapt.NormalizeDescriptor(msgDesc)
	if err != nil {
		return goerr.Wrap(err, "failed to normalize descriptor")
	}

	parent := mw.TableParentFromParts(s.project, dataset, table)
	stream, err := s.mw.NewManagedStream(ctx,
		mw.WithDestinationTable(parent),
		mw.WithType(mw.PendingStream),
		mw.WithSchemaDescriptor(dp))
	if err != nil {
		return goerr.Wrap(err, "failed to create managed stream")
	}
	defer safe.Close(ctx, stream)

	colTypes := columnTypeIndex(columns)
	var results []*mw.AppendResult
	for start := 0; start < len(rows); start += appendBatchRows {
		end := min(start+appendBatchRows, len(rows))
		encoded, err := encodeRows(msgDesc, colTypes, rows[start:end])
		if err != nil {
			return err
		}
		res, err := stream.AppendRows(ctx, encoded)
		if err != nil {
			return goerr.Wrap(err, "failed to append rows")
		}
		results = append(results, res)
	}
	for _, res := range results {
		if _, err := res.GetResult(ctx); err != nil {
			return goerr.Wrap(err, "append result reported an error")
		}
	}
	if _, err := stream.Finalize(ctx); err != nil {
		return goerr.Wrap(err, "failed to finalize stream")
	}
	resp, err := s.mw.BatchCommitWriteStreams(ctx, &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       mw.TableParentFromStreamName(stream.StreamName()),
		WriteStreams: []string{stream.StreamName()},
	})
	if err != nil {
		return goerr.Wrap(err, "failed to batch-commit write streams")
	}
	if streamErrs := resp.GetStreamErrors(); len(streamErrs) > 0 {
		return goerr.New("batch commit reported stream errors",
			goerr.V("stream_error_count", len(streamErrs)))
	}
	return nil
}

// toBQSchema maps export columns to a BigQuery schema. A non-nullable,
// non-repeated column is REQUIRED.
func toBQSchema(cols []export.Column) bq.Schema {
	schema := make(bq.Schema, 0, len(cols))
	for _, c := range cols {
		schema = append(schema, &bq.FieldSchema{
			Name:     c.Name,
			Type:     toBQType(c.Type),
			Repeated: c.Repeated,
			Required: !c.Nullable && !c.Repeated,
		})
	}
	return schema
}

func toBQType(ct export.ColumnType) bq.FieldType {
	switch ct {
	case export.TypeInt:
		return bq.IntegerFieldType
	case export.TypeFloat:
		return bq.FloatFieldType
	case export.TypeBool:
		return bq.BooleanFieldType
	case export.TypeTimestamp:
		return bq.TimestampFieldType
	default:
		return bq.StringFieldType
	}
}

func columnTypeIndex(cols []export.Column) map[string]export.ColumnType {
	m := make(map[string]export.ColumnType, len(cols))
	for _, c := range cols {
		m[c.Name] = c.Type
	}
	return m
}

// encodeRows serializes rows to Storage Write proto bytes: each row is encoded
// to storage-proto-compatible JSON (TIMESTAMP -> int64 microseconds, arrays as
// JSON arrays, NULLs omitted), unmarshalled into a dynamic proto message, and
// marshalled to bytes.
func encodeRows(msgDesc protoreflect.MessageDescriptor, colTypes map[string]export.ColumnType, rows []map[string]any) ([][]byte, error) {
	out := make([][]byte, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]any, len(row))
		for name, v := range row {
			ev, include, err := encodeValue(colTypes[name], v)
			if err != nil {
				return nil, goerr.Wrap(err, "failed to encode value", goerr.V("column", name))
			}
			if include {
				obj[name] = ev
			}
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to marshal row to JSON")
		}
		msg := dynamicpb.NewMessage(msgDesc)
		if err := protojson.Unmarshal(raw, msg); err != nil {
			return nil, goerr.Wrap(err, "failed to unmarshal row into proto message")
		}
		b, err := proto.Marshal(msg)
		if err != nil {
			return nil, goerr.Wrap(err, "failed to marshal proto message")
		}
		out = append(out, b)
	}
	return out, nil
}

// encodeValue converts a natural Go value to a JSON-ready value for the given
// column type. include is false when the value should be omitted (NULL).
func encodeValue(ct export.ColumnType, v any) (value any, include bool, err error) {
	if v == nil {
		return nil, false, nil
	}
	if ct == export.TypeTimestamp {
		switch t := v.(type) {
		case time.Time:
			if t.IsZero() {
				return nil, false, nil
			}
			return t.UnixMicro(), true, nil
		case *time.Time:
			if t == nil {
				return nil, false, nil
			}
			return t.UnixMicro(), true, nil
		default:
			return nil, false, goerr.New("unexpected timestamp value type",
				goerr.V("go_type", fmt.Sprintf("%T", v)))
		}
	}
	return v, true, nil
}

// isSchemaMismatch reports whether err is the Storage Write
// SCHEMA_MISMATCH_EXTRA_FIELDS error (raised while a schema update propagates).
// It classifies by typed detail, never by message text. Follows swarm's
// isSchemaMismatchError.
func isSchemaMismatch(err error) bool {
	apiErr, ok := apierror.FromError(err)
	if !ok {
		return false
	}
	storageErr := &storagepb.StorageError{}
	if e := apiErr.Details().ExtractProtoMessage(storageErr); e == nil {
		return storageErr.GetCode() == storagepb.StorageError_SCHEMA_MISMATCH_EXTRA_FIELDS
	}
	return false
}

// isHTTPStatus reports whether err is a googleapi.Error with the given HTTP code.
func isHTTPStatus(err error, code int) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == code
	}
	return false
}
