// Package bqexport is the BigQuery implementation of export.Sink (an
// external-system adapter, hence pkg/service). Each WriteTable is a full
// refresh (洗替) of one table, and it never reads or evolves the destination's
// existing schema: it writes the rows into a throwaway, uniquely-named staging
// table created from the desired schema (Storage Write API, PendingStream),
// then replaces the destination with that staging table in a single
// CREATE OR REPLACE TABLE ... AS SELECT statement.
//
// Two constraints shape that design. The Storage Write API is append-only with
// no truncate mode, so a full refresh needs a separate replace step. And
// deleting a table then recreating it under the same name leaves the write
// backend's metadata stale for a while, so appends can be routed to the
// deleted table and the rows silently lost — hence the Storage Write
// destination is always a brand-new name, never the destination table.
//
// The consequence for consumers is that the destination's schema is exactly
// the desired schema after every run: a column no longer produced disappears,
// and a column whose type or mode changed is replaced rather than rejected.
// The Storage-Write mechanics follow secmon-lab/swarm's pkg/infra/bq.
package bqexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	bq "cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	mw "cloud.google.com/go/bigquery/storage/managedwriter"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	// appendBatchRows bounds how many rows go in one AppendRows call (matches
	// swarm's batch size). appendMaxRequestBytes bounds the same call by size;
	// whichever limit is reached first ends the batch.
	appendBatchRows = 256

	// appendMaxRequestBytes bounds the total encoded size of one AppendRows
	// call. The API rejects a request over 10 MB and the limit cannot be
	// raised, so this leaves headroom for the stream name, the schema
	// descriptor and proto framing. It is a property of the BigQuery service,
	// not deployment-tunable configuration, so it lives here.
	appendMaxRequestBytes = 9 * 1024 * 1024

	// descriptorScope is the arbitrary root message name for the generated proto
	// descriptor.
	descriptorScope = "root"

	// jobCleanupTimeout bounds the calls made while unwinding a swap this
	// process can no longer follow — cancelling the query job and dropping the
	// staging table. They run on a context detached from the caller's, so they
	// need a bound of their own.
	jobCleanupTimeout = 30 * time.Second

	// stagingTableTTL is the lifetime stamped on a staging table. It is far
	// longer than one export run and far shorter than the daily schedule, so a
	// staging table orphaned by a crashed job is reclaimed by BigQuery itself
	// instead of waiting for an operator.
	stagingTableTTL = 6 * time.Hour
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

// New creates a Sink with both a BigQuery client (metadata / swap) and a
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

// WriteTable fully refreshes dataset.table: write every row into a fresh
// staging table carrying the desired schema, then replace the destination with
// it. The destination is only touched by the final swap, so a failure anywhere
// before it leaves the previous snapshot intact.
func (s *Sink) WriteTable(ctx context.Context, dataset string, t *export.Table) error {
	if !safeIdentPattern.MatchString(dataset) {
		return goerr.New("invalid dataset name for export", goerr.V("dataset", dataset))
	}
	if !safeIdentPattern.MatchString(t.Name) {
		return goerr.New("invalid table name for export", goerr.V("table", t.Name))
	}
	// Column names reach the swap statement, so they are validated here too.
	// A table with no columns is rejected rather than sent as invalid DDL.
	if len(t.Columns) == 0 {
		return goerr.New("export table has no columns",
			goerr.V("dataset", dataset), goerr.V("table", t.Name))
	}
	for _, c := range t.Columns {
		if !safeIdentPattern.MatchString(c.Name) {
			return goerr.New("invalid column name for export",
				goerr.V("dataset", dataset), goerr.V("table", t.Name), goerr.V("column", c.Name))
		}
	}
	location, err := s.ensureDataset(ctx, dataset)
	if err != nil {
		return err
	}
	desired := toBQSchema(t.Columns)

	staging, err := s.createStagingTable(ctx, dataset, t.Name, desired)
	if err != nil {
		return err
	}
	defer s.dropStagingTable(ctx, dataset, staging)

	if len(t.Rows) > 0 {
		if err := s.appendRows(ctx, dataset, staging, desired, t.Columns, t.Rows); err != nil {
			return err
		}
	}
	// An empty staging table still goes through the swap: a refresh that
	// produced no rows must leave the destination empty, not stale.
	return s.swapTable(ctx, dataset, t.Name, staging, location, t.Columns)
}

// ddlColumnList renders the destination's column declarations for the swap
// statement. It is built from the export columns rather than from bq.Schema
// because bq.FieldType carries the legacy type names (INTEGER, FLOAT, BOOLEAN)
// that GoogleSQL DDL does not accept.
func ddlColumnList(cols []export.Column) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		switch {
		case c.Repeated:
			parts = append(parts, fmt.Sprintf("`%s` ARRAY<%s>", c.Name, ddlTypeName(c.Type)))
		case c.Nullable:
			parts = append(parts, fmt.Sprintf("`%s` %s", c.Name, ddlTypeName(c.Type)))
		default:
			parts = append(parts, fmt.Sprintf("`%s` %s NOT NULL", c.Name, ddlTypeName(c.Type)))
		}
	}
	return strings.Join(parts, ", ")
}

// ddlSelectList renders the projection the swap reads out of the staging table.
// The columns are named explicitly so the destination's column order is the one
// this export defines, independent of how the staging table was built.
func ddlSelectList(cols []export.Column) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		parts = append(parts, "`"+c.Name+"`")
	}
	return strings.Join(parts, ", ")
}

// ddlTypeName maps a logical column type to its GoogleSQL type name.
func ddlTypeName(ct export.ColumnType) string {
	switch ct {
	case export.TypeInt:
		return "INT64"
	case export.TypeFloat:
		return "FLOAT64"
	case export.TypeBool:
		return "BOOL"
	case export.TypeTimestamp:
		return "TIMESTAMP"
	default:
		return "STRING"
	}
}

// ensureDataset creates the dataset (with the configured location) when absent
// and returns the dataset's effective location. The location is used to run the
// swap query job in the dataset's own region: the configured s.location is
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

// stagingTableName derives the throwaway table name for one WriteTable call.
// The random suffix keeps concurrent or retried runs from colliding, and
// dropping the uuid's hyphens keeps the result inside safeIdentPattern.
func stagingTableName(table string) string {
	return table + "_stg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// createStagingTable creates the throwaway table that the rows are written to
// and returns its name. The expiration is set at creation so the table is
// reclaimed even if this process dies before the deferred drop runs.
func (s *Sink) createStagingTable(ctx context.Context, dataset, table string, desired bq.Schema) (string, error) {
	staging := stagingTableName(table)
	if !safeIdentPattern.MatchString(staging) {
		return "", goerr.New("generated an invalid staging table name",
			goerr.V("dataset", dataset), goerr.V("table", table), goerr.V("staging", staging))
	}
	tbl := s.bq.DatasetInProject(s.project, dataset).Table(staging)
	md := &bq.TableMetadata{
		Name:           staging,
		Schema:         desired,
		ExpirationTime: time.Now().Add(stagingTableTTL),
	}
	if err := tbl.Create(ctx, md); err != nil {
		return "", goerr.Wrap(err, "failed to create staging table",
			goerr.V("dataset", dataset), goerr.V("table", table), goerr.V("staging", staging))
	}
	return staging, nil
}

// swapTable replaces dest with staging — schema, rows and all — in one
// statement.
//
// The rows are read with a SELECT rather than copied with
// `CREATE OR REPLACE TABLE dest COPY staging`: a table copy operates on
// committed storage and does not see rows still in the write-optimized storage
// the Storage Write API lands them in, so a copy right after the append
// produces an empty table without reporting anything wrong. The query engine
// does see them.
//
// The column list is spelled out rather than left to `AS SELECT *` because a
// bare CTAS makes every output column NULLABLE, which would drop the REQUIRED
// mode from the schema this export defines.
//
// location is the dataset's real region, so the query job runs where the
// tables live.
func (s *Sink) swapTable(ctx context.Context, dataset, dest, staging, location string, columns []export.Column) error {
	// Every identifier was validated before we got here: dataset, dest and the
	// column names at WriteTable's boundary, staging in createStagingTable.
	q := s.bq.Query(fmt.Sprintf(
		"CREATE OR REPLACE TABLE `%s.%s.%s` (%s) AS SELECT %s FROM `%s.%s.%s`",
		s.project, dataset, dest, ddlColumnList(columns), ddlSelectList(columns),
		s.project, dataset, staging))
	if location != "" {
		q.Location = location
	}
	job, err := q.Run(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to start table swap",
			goerr.V("dataset", dataset), goerr.V("table", dest), goerr.V("staging", staging))
	}
	status, err := job.Wait(ctx)
	if err != nil {
		// Wait gives up when the caller's context ends but never cancels the
		// job, so an unattended swap can still land minutes later — after this
		// run reported failure, and possibly on top of a newer snapshot written
		// by a re-run. Ask BigQuery to stop it.
		s.cancelSwapJob(ctx, job, dataset, dest, staging)
		return goerr.Wrap(err, "failed to wait for table swap; the swap may or may not have applied",
			goerr.V("dataset", dataset), goerr.V("table", dest), goerr.V("staging", staging))
	}
	if err := status.Err(); err != nil {
		return goerr.Wrap(err, "table swap job failed",
			goerr.V("dataset", dataset), goerr.V("table", dest), goerr.V("staging", staging))
	}
	return nil
}

// cancelSwapJob asks BigQuery to stop a swap this process can no longer
// observe. It is best-effort by nature: Jobs.cancel is asynchronous and a swap
// that already committed cannot be undone, so the caller still has to report
// the outcome as unknown rather than as "the destination was left alone".
func (s *Sink) cancelSwapJob(ctx context.Context, job *bq.Job, dataset, dest, staging string) {
	cctx, cancel := detachedContext(ctx)
	defer cancel()
	if err := job.Cancel(cctx); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to cancel table swap job",
			goerr.V("dataset", dataset), goerr.V("table", dest), goerr.V("staging", staging)),
			"cancel export table swap")
	}
}

// dropStagingTable removes the throwaway table. A failure here is non-fatal:
// the refresh already succeeded or already failed on its own terms, and the
// table's expiration reclaims it regardless.
func (s *Sink) dropStagingTable(ctx context.Context, dataset, staging string) {
	dctx, cancel := detachedContext(ctx)
	defer cancel()
	if err := s.bq.DatasetInProject(s.project, dataset).Table(staging).Delete(dctx); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to drop staging table",
			goerr.V("dataset", dataset), goerr.V("staging", staging)),
			"drop export staging table")
	}
}

// detachedContext derives a short-lived context that outlives the caller's
// cancellation. Unwinding a swap has to reach BigQuery precisely when the run
// was cancelled or timed out: that is when an uncancelled job would otherwise
// keep running, and when a surviving staging table would let it finish.
func detachedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), jobCleanupTimeout)
}

// rowDescriptor derives the two proto descriptors a write needs from a
// BigQuery schema: the message descriptor each row is marshalled into, and the
// normalized descriptor the managed stream advertises to the backend.
func rowDescriptor(schema bq.Schema) (protoreflect.MessageDescriptor, *descriptorpb.DescriptorProto, error) {
	storageSchema, err := adapt.BQSchemaToStorageTableSchema(schema)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to convert schema to storage schema")
	}
	descriptor, err := adapt.StorageSchemaToProto2Descriptor(storageSchema, descriptorScope)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to build proto descriptor")
	}
	msgDesc, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, nil, goerr.New("storage schema did not produce a message descriptor")
	}
	dp, err := adapt.NormalizeDescriptor(msgDesc)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to normalize descriptor")
	}
	return msgDesc, dp, nil
}

// appendRows performs one full pending-stream write into table: build the
// descriptor from the schema, append the rows in batches bounded by both row
// count and encoded size, finalize, and batch-commit.
func (s *Sink) appendRows(ctx context.Context, dataset, table string, schema bq.Schema, columns []export.Column, rows []map[string]any) error {
	msgDesc, dp, err := rowDescriptor(schema)
	if err != nil {
		return err
	}

	parent := mw.TableParentFromParts(s.project, dataset, table)
	stream, err := s.mw.NewManagedStream(ctx,
		mw.WithDestinationTable(parent),
		mw.WithType(mw.PendingStream),
		mw.WithSchemaDescriptor(dp))
	if err != nil {
		return goerr.Wrap(err, "failed to create managed stream")
	}
	// ManagedStream.Close reports io.EOF for a stream that shut down normally,
	// so reporting it would raise one false error per table written.
	defer safe.CloseExcept(ctx, stream, io.EOF)

	colTypes := columnTypeIndex(columns)
	var results []*mw.AppendResult
	batch := make([][]byte, 0, appendBatchRows)
	batchBytes := 0

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := stream.AppendRows(ctx, batch)
		if err != nil {
			return goerr.Wrap(err, "failed to append rows",
				goerr.V("dataset", dataset), goerr.V("table", table),
				goerr.V("batch_rows", len(batch)), goerr.V("batch_bytes", batchBytes))
		}
		results = append(results, res)
		batch = make([][]byte, 0, appendBatchRows)
		batchBytes = 0
		return nil
	}

	for i, row := range rows {
		encoded, err := encodeRow(msgDesc, colTypes, row)
		if err != nil {
			return err
		}
		// A single row over the limit cannot be split. Callers keep their cells
		// bounded so this is unreachable in practice; if it fires, the caller —
		// not this batching — is what needs fixing, so surface it rather than
		// dropping the row.
		if len(encoded) > appendMaxRequestBytes {
			return goerr.New("encoded row exceeds max append request size",
				goerr.V("dataset", dataset), goerr.V("table", table),
				goerr.V("row_index", i), goerr.V("row_bytes", len(encoded)),
				goerr.V("limit_bytes", appendMaxRequestBytes))
		}
		if len(batch) > 0 &&
			(batchBytes+len(encoded) > appendMaxRequestBytes || len(batch) >= appendBatchRows) {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, encoded)
		batchBytes += len(encoded)
	}
	if err := flush(); err != nil {
		return err
	}

	for _, res := range results {
		if _, err := res.GetResult(ctx); err != nil {
			return goerr.Wrap(err, "append result reported an error",
				goerr.V("dataset", dataset), goerr.V("table", table))
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

// encodeRow serializes one row to Storage Write proto bytes: the row is encoded
// to storage-proto-compatible JSON (TIMESTAMP -> int64 microseconds, arrays as
// JSON arrays, NULLs omitted), unmarshalled into a dynamic proto message, and
// marshalled to bytes. Rows are encoded one at a time so a large table is never
// held in memory in its entirety.
func encodeRow(msgDesc protoreflect.MessageDescriptor, colTypes map[string]export.ColumnType, row map[string]any) ([]byte, error) {
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
	return b, nil
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

// isHTTPStatus reports whether err is a googleapi.Error with the given HTTP code.
func isHTTPStatus(err error, code int) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == code
	}
	return false
}
