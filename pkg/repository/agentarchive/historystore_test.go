package agentarchive_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/agentkit/historystore/historytest"
	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
)

// TestHistoryStore runs agentkit's HistoryStore contract suite. The property it
// exists for is that an older ref stays readable after newer versions are
// saved — a store that overwrites in place passes everything else and still
// breaks the rollback guarantee that pairs History with State.
//
// The memory blob backend is used because the store's own logic (object naming,
// ref minting, version immutability, the missing-version error) lives above the
// blob layer; the Cloud Storage adapter below it holds nothing but SDK calls.
func TestHistoryStore(t *testing.T) {
	historytest.Run(t, func(t *testing.T) agentkit.HistoryStore {
		return agentarchive.NewMemoryHistoryStore()
	})
}

// newHistory builds a minimal History carrying the current version, which is
// what gollem's deserialization gate requires.
func newHistory() *gollem.History {
	return &gollem.History{
		LLType:  gollem.LLMTypeOpenAI,
		Version: gollem.HistoryVersion,
	}
}

func TestHistoryStoreRejectsEmptyArguments(t *testing.T) {
	ctx := context.Background()
	store := agentarchive.NewMemoryHistoryStore()

	t.Run("save without a process id", func(t *testing.T) {
		ref, err := store.Save(ctx, "", nil)
		gt.Value(t, err).NotNil()
		gt.Value(t, ref).Equal(agentkit.HistoryRef(""))
	})

	t.Run("load without a process id", func(t *testing.T) {
		h, err := store.Load(ctx, "", "ref-1")
		gt.Value(t, err).NotNil()
		gt.Value(t, h).Nil()
	})

	t.Run("load without a ref", func(t *testing.T) {
		h, err := store.Load(ctx, "proc-1", "")
		gt.Value(t, err).NotNil()
		gt.Value(t, h).Nil()
	})
}

// TestHistoryStoreSaveNilHistory pins that a nil history is refused. A
// zero-valued gollem.History carries version 0, which gollem's deserialization
// gate rejects, so storing one would mint a ref the kernel records and can then
// never load.
func TestHistoryStoreSaveNilHistory(t *testing.T) {
	ctx := context.Background()
	store := agentarchive.NewMemoryHistoryStore()

	ref, err := store.Save(ctx, "proc-1", nil)
	gt.Value(t, err).NotNil()
	gt.Value(t, ref).Equal(agentkit.HistoryRef(""))
}

// TestHistoryStoreLoadMissingRef pins the error the kernel discriminates on.
func TestHistoryStoreLoadMissingRef(t *testing.T) {
	ctx := context.Background()
	store := agentarchive.NewMemoryHistoryStore()

	_, err := store.Load(ctx, "proc-1", "no-such-ref")
	gt.Error(t, err).Is(agentkit.ErrHistoryVersionMissing)
}

// TestHistoryStoreDiscardIsIdempotent pins that discarding twice, or discarding
// something that was never saved, is not an error path a caller has to guard.
func TestHistoryStoreDiscardIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := agentarchive.NewMemoryHistoryStore()

	ref, err := store.Save(ctx, "proc-1", newHistory())
	gt.NoError(t, err).Required()

	store.Discard(ctx, "proc-1", ref)
	store.Discard(ctx, "proc-1", ref)
	store.Discard(ctx, "proc-1", "never-saved")

	_, err = store.Load(ctx, "proc-1", ref)
	gt.Error(t, err).Is(agentkit.ErrHistoryVersionMissing)
}

func TestProcessHistoryObjectPath(t *testing.T) {
	testCases := map[string]struct {
		prefix string
		pid    agentkit.ProcessID
		ref    agentkit.HistoryRef
		want   string
	}{
		"with prefix": {
			prefix: "hct",
			pid:    "proc-1",
			ref:    "ref-1",
			want:   "hct/v1/processes/proc-1/history/ref-1.json",
		},
		"without prefix": {
			prefix: "",
			pid:    "proc-1",
			ref:    "ref-1",
			want:   "v1/processes/proc-1/history/ref-1.json",
		},
		"prefix with surrounding slashes is normalized": {
			prefix: "/hct/",
			pid:    "proc-2",
			ref:    "ref-2",
			want:   "hct/v1/processes/proc-2/history/ref-2.json",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := agentarchive.ProcessHistoryObjectPathForTest(tc.prefix, tc.pid, tc.ref)
			gt.String(t, got).Equal(tc.want)
		})
	}
}
