package agentarchive

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"cloud.google.com/go/storage"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

// CloudStorageHistoryRepository persists gollem.History as JSON objects in a
// Cloud Storage bucket. It satisfies gollem.HistoryRepository.
type CloudStorageHistoryRepository struct {
	client *storage.Client
	bucket string
	prefix string
}

var _ gollem.HistoryRepository = (*CloudStorageHistoryRepository)(nil)

// NewCloudStorageHistoryRepository builds a HistoryRepository backed by the
// given client/bucket.
func NewCloudStorageHistoryRepository(client *storage.Client, bucket, prefix string) *CloudStorageHistoryRepository {
	return &CloudStorageHistoryRepository{client: client, bucket: bucket, prefix: prefix}
}

// Load returns the persisted history for the given sessionID. It returns
// (nil, nil) when no history is stored yet, or when the persisted blob is
// unreadable (corrupt JSON, version mismatch); in the latter cases the error
// is logged via errutil.Handle so the conversation can restart cleanly.
func (r *CloudStorageHistoryRepository) Load(ctx context.Context, sessionID string) (*gollem.History, error) {
	if sessionID == "" {
		return nil, goerr.New("sessionID is required")
	}

	objName := historyObjectPath(r.prefix, sessionID)
	obj := r.client.Bucket(r.bucket).Object(objName)
	rc, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, nil
		}
		return nil, goerr.Wrap(err, "failed to open history object",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}
	defer safe.Close(ctx, rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to read history object",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}

	var h gollem.History
	if err := json.Unmarshal(data, &h); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "history blob is unreadable; restarting session",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		), "agent history load fallback")
		return nil, nil
	}
	return &h, nil
}

// Save writes the history JSON to Cloud Storage, overwriting any existing
// blob. A nil history is treated as a no-op: gollem hands us whatever the
// session reports, and a session with no turns yet legitimately has no
// history to persist.
func (r *CloudStorageHistoryRepository) Save(ctx context.Context, sessionID string, history *gollem.History) error {
	if sessionID == "" {
		return goerr.New("sessionID is required")
	}
	if history == nil {
		return nil
	}

	data, err := json.Marshal(history)
	if err != nil {
		return goerr.Wrap(err, "failed to marshal history")
	}

	objName := historyObjectPath(r.prefix, sessionID)
	w := r.client.Bucket(r.bucket).Object(objName).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := w.Write(data); err != nil {
		safe.Close(ctx, w)
		return goerr.Wrap(err, "failed to write history object",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}
	if err := w.Close(); err != nil {
		return goerr.Wrap(err, "failed to close history writer",
			goerr.V("bucket", r.bucket),
			goerr.V("object", objName),
		)
	}
	return nil
}
