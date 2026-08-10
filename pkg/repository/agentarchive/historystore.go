package agentarchive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/safe"
)

const (
	processesDir = "processes"
	historyDir   = "history"
)

// processHistoryObjectPath returns the object name for one committed version of
// a Process's conversation.
//
//	{prefix}/v1/processes/{processID}/history/{ref}.json
//
// It is deliberately a different layout from historyObjectPath: that one stores
// a single mutable blob per session, which is the opposite of what agentkit
// needs. Versions here are immutable and addressed by ref.
func processHistoryObjectPath(prefix string, pid agentkit.ProcessID, ref agentkit.HistoryRef) string {
	return joinObjectPath(prefix, versionDir, processesDir, string(pid), historyDir, string(ref)+".json")
}

// errBlobNotFound is what a blobStore reports for an absent object. It is
// internal: HistoryStore translates it into agentkit.ErrHistoryVersionMissing,
// which is the error the kernel discriminates on.
var errBlobNotFound = goerr.New("blob not found")

// blobStore is the narrow slice of object storage HistoryStore needs. It exists
// so the store's own logic — object naming, ref minting, the immutability of a
// saved version, and the missing-version error — is exercised by the agentkit
// contract suite without a Cloud Storage endpoint. The Cloud Storage adapter
// below holds nothing but the SDK calls.
type blobStore interface {
	Put(ctx context.Context, name string, data []byte) error
	Get(ctx context.Context, name string) ([]byte, error)
	Delete(ctx context.Context, name string) error
}

// HistoryStore persists a Process's conversation History as immutable versions.
// It satisfies agentkit.HistoryStore.
//
// The immutability is load-bearing: Process.HistoryRef commits in the same
// atomic write as Process.State, so a version that is never referenced is simply
// never read, and History rolls back with State. Overwriting a version would
// break that — a rolled-back State would come back paired with the newer
// conversation.
type HistoryStore struct {
	blobs  blobStore
	prefix string
}

var _ agentkit.HistoryStore = (*HistoryStore)(nil)

// NewCloudStorageHistoryStore builds a HistoryStore backed by a Cloud Storage
// bucket.
func NewCloudStorageHistoryStore(client *storage.Client, bucket, prefix string) *HistoryStore {
	return &HistoryStore{blobs: &gcsBlobStore{client: client, bucket: bucket}, prefix: prefix}
}

// NewMemoryHistoryStore builds an in-process HistoryStore for tests and for the
// memory repository backend, where no bucket is configured.
func NewMemoryHistoryStore() *HistoryStore {
	return &HistoryStore{blobs: newMemoryBlobStore()}
}

// Save stores h as a new version and returns the ref naming it. The ref is a
// uuid v7 so an operator reading an object listing sees the versions in the
// order they were written.
//
// A nil history is rejected rather than stored as an empty one. gollem gates
// deserialization on History.Version, and a zero-valued History carries version
// 0, so writing one would produce a version that can never be loaded again —
// the kernel would record its ref and then fail every transition that reads it.
// Failing here instead surfaces the caller's bug at the point it happens.
func (s *HistoryStore) Save(ctx context.Context, pid agentkit.ProcessID, h *gollem.History) (agentkit.HistoryRef, error) {
	if pid == "" {
		return "", goerr.New("process id is required")
	}
	if h == nil {
		return "", goerr.New("history is required", goerr.V("process", pid))
	}

	data, err := json.Marshal(h)
	if err != nil {
		return "", goerr.Wrap(err, "marshal process history", goerr.V("process", pid))
	}

	ref := agentkit.HistoryRef(uuid.Must(uuid.NewV7()).String())
	name := processHistoryObjectPath(s.prefix, pid, ref)
	if err := s.blobs.Put(ctx, name, data); err != nil {
		return "", goerr.Wrap(err, "write process history version",
			goerr.V("process", pid), goerr.V("ref", ref))
	}
	return ref, nil
}

// Load returns the version named by ref.
//
// A ref that cannot be resolved is an error, never an empty conversation: the
// Process record names this version, so its absence is data loss. Reporting it
// as "nothing saved yet" would silently restart the conversation from scratch
// and the operator would never learn why the agent forgot everything.
func (s *HistoryStore) Load(ctx context.Context, pid agentkit.ProcessID, ref agentkit.HistoryRef) (*gollem.History, error) {
	if pid == "" || ref == "" {
		return nil, goerr.New("process id and history ref are required",
			goerr.V("process", pid), goerr.V("ref", ref))
	}

	name := processHistoryObjectPath(s.prefix, pid, ref)
	data, err := s.blobs.Get(ctx, name)
	if err != nil {
		if errors.Is(err, errBlobNotFound) {
			return nil, goerr.Wrap(agentkit.ErrHistoryVersionMissing, "process history version not found",
				goerr.V("process", pid), goerr.V("ref", ref))
		}
		return nil, goerr.Wrap(err, "read process history version",
			goerr.V("process", pid), goerr.V("ref", ref))
	}

	var h gollem.History
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, goerr.Wrap(agentkit.ErrHistoryVersionMissing, "process history version is unreadable",
			goerr.V("process", pid), goerr.V("ref", ref), goerr.V("cause", err.Error()))
	}
	return &h, nil
}

// Discard reports that a version is no longer referenced and deletes it.
//
// It returns nothing because the kernel would only log a failure and carry on.
// A version that outlives its reference costs storage, not correctness, so a
// failed delete is reported to the operator and otherwise ignored.
func (s *HistoryStore) Discard(ctx context.Context, pid agentkit.ProcessID, ref agentkit.HistoryRef) {
	if pid == "" || ref == "" {
		return
	}
	name := processHistoryObjectPath(s.prefix, pid, ref)
	if err := s.blobs.Delete(ctx, name); err != nil {
		if errors.Is(err, errBlobNotFound) {
			return
		}
		errutil.Handle(ctx, goerr.Wrap(err, "discard process history version",
			goerr.V("process", pid), goerr.V("ref", ref)), "discard process history version")
	}
}

// gcsBlobStore is the Cloud Storage adapter. It holds no logic beyond the SDK
// calls and the absent-object translation.
type gcsBlobStore struct {
	client *storage.Client
	bucket string
}

func (g *gcsBlobStore) Put(ctx context.Context, name string, data []byte) error {
	w := g.client.Bucket(g.bucket).Object(name).NewWriter(ctx)
	w.ContentType = "application/json"
	if _, err := w.Write(data); err != nil {
		safe.Close(ctx, w)
		return goerr.Wrap(err, "write object", goerr.V("bucket", g.bucket), goerr.V("object", name))
	}
	if err := w.Close(); err != nil {
		return goerr.Wrap(err, "close object writer", goerr.V("bucket", g.bucket), goerr.V("object", name))
	}
	return nil
}

func (g *gcsBlobStore) Get(ctx context.Context, name string) ([]byte, error) {
	rc, err := g.client.Bucket(g.bucket).Object(name).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, errBlobNotFound
		}
		return nil, goerr.Wrap(err, "open object", goerr.V("bucket", g.bucket), goerr.V("object", name))
	}
	defer safe.Close(ctx, rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, goerr.Wrap(err, "read object", goerr.V("bucket", g.bucket), goerr.V("object", name))
	}
	return data, nil
}

func (g *gcsBlobStore) Delete(ctx context.Context, name string) error {
	if err := g.client.Bucket(g.bucket).Object(name).Delete(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return errBlobNotFound
		}
		return goerr.Wrap(err, "delete object", goerr.V("bucket", g.bucket), goerr.V("object", name))
	}
	return nil
}

// memoryBlobStore is the in-process adapter.
type memoryBlobStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func newMemoryBlobStore() *memoryBlobStore {
	return &memoryBlobStore{objects: make(map[string][]byte)}
}

func (m *memoryBlobStore) Put(_ context.Context, name string, data []byte) error {
	stored := make([]byte, len(data))
	copy(stored, data)
	m.mu.Lock()
	m.objects[name] = stored
	m.mu.Unlock()
	return nil
}

func (m *memoryBlobStore) Get(_ context.Context, name string) ([]byte, error) {
	m.mu.RLock()
	data, ok := m.objects[name]
	m.mu.RUnlock()
	if !ok {
		return nil, errBlobNotFound
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (m *memoryBlobStore) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	_, ok := m.objects[name]
	delete(m.objects, name)
	m.mu.Unlock()
	if !ok {
		return errBlobNotFound
	}
	return nil
}
