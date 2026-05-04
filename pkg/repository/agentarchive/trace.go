package agentarchive

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/storage"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem/trace"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// CloudStorageTraceRepository persists gollem trace.Trace blobs as JSON
// objects in a Cloud Storage bucket. It satisfies trace.Repository.
//
// Trace metadata.Labels MUST contain a "session_id" key — that ID anchors the
// trace to its parent AgentSession and determines the object path.
type CloudStorageTraceRepository struct {
	client *storage.Client
	bucket string
	prefix string
}

var _ trace.Repository = (*CloudStorageTraceRepository)(nil)

// NewCloudStorageTraceRepository builds a trace.Repository backed by the
// given client/bucket.
func NewCloudStorageTraceRepository(client *storage.Client, bucket, prefix string) *CloudStorageTraceRepository {
	return &CloudStorageTraceRepository{client: client, bucket: bucket, prefix: prefix}
}

// SessionIDLabel is the Trace metadata label key that carries the AgentSession
// ID. Callers MUST populate it when constructing the recorder.
const SessionIDLabel = "session_id"

// Save writes the trace as JSON under the bucket. The session ID is read from
// trace.Metadata.Labels["session_id"]; an error is returned if missing.
func (r *CloudStorageTraceRepository) Save(ctx context.Context, t *trace.Trace) error {
	if t == nil {
		return goerr.New("trace is nil")
	}
	sessionID := t.Metadata.Labels[SessionIDLabel]
	if sessionID == "" {
		return goerr.New("trace metadata.Labels[session_id] is required",
			goerr.V("trace_id", t.TraceID),
		)
	}
	if t.TraceID == "" {
		return goerr.New("trace ID is required")
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return goerr.Wrap(err, "failed to marshal trace",
			goerr.V("trace_id", t.TraceID),
		)
	}

	objName := traceObjectPath(r.prefix, sessionID, t.TraceID)
	w := r.client.Bucket(r.bucket).Object(objName).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := w.Write(data); err != nil {
		safe.Close(ctx, w)
		return goerr.Wrap(err, "failed to write trace object",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}
	if err := w.Close(); err != nil {
		return goerr.Wrap(err, "failed to close trace writer",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}
	return nil
}
