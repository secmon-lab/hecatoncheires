package agentproc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/agentkit/repository/repotest"
	"github.com/google/uuid"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentproc"
)

// newRepository builds a fresh, empty Repository for one repotest factory call.
//
// Isolation comes from a random project id: the Firestore emulator partitions
// data by project, so every call gets an empty store even though the collection
// names are fixed. That is why this suite always targets the emulator and does
// not honour TEST_FIRESTORE_PROJECT_ID the way the interfaces.Repository tests
// do — repotest requires an empty store per factory call, and a shared real
// project cannot provide one.
//
// It never skips: a missing emulator fails loudly with a connection error
// instead of passing as a no-op.
func newRepository(t *testing.T) *agentproc.Repository {
	t.Helper()

	if _, ok := os.LookupEnv("FIRESTORE_EMULATOR_HOST"); !ok {
		// Same port as the interfaces.Repository suite (see
		// pkg/repository/case_test.go) so one emulator serves both.
		gt.NoError(t, os.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:28615")).Required()
	}

	projectID := "agentproc-" + uuid.Must(uuid.NewV7()).String()
	client, err := firestore.NewClient(context.Background(), projectID)
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, client.Close())
	})

	repo, err := agentproc.New(client)
	gt.NoError(t, err).Required()
	return repo
}

// TestRepository_Firestore runs agentkit's Repository contract suite. It covers
// Rev CAS, guards, the three uniqueness constraints, every claim condition, the
// fresh lease token per claim, unclean-reclaim counting, HistoryRef /
// InheritedHistory round-tripping, event ordering and cursors, no double-claim
// under 100 concurrent claimers, and deep-copy-on-read.
func TestRepository_Firestore(t *testing.T) {
	repotest.Run(t, func(t *testing.T) agentkit.Repository {
		return newRepository(t)
	})
}

func TestNew(t *testing.T) {
	t.Run("nil client is rejected", func(t *testing.T) {
		repo, err := agentproc.New(nil)
		gt.Value(t, err).NotNil()
		gt.Value(t, repo).Nil()
	})
}

func TestNewWithProject(t *testing.T) {
	t.Run("empty project id is rejected", func(t *testing.T) {
		repo, err := agentproc.NewWithProject(context.Background(), "", "")
		gt.Value(t, err).NotNil()
		gt.Value(t, repo).Nil()
	})
}

// TestClaimAtFor pins the derivation the claim query depends on. Each branch
// decides whether a row is reachable by "ClaimAt <= now", so a wrong value here
// either starves a runnable Process or hands the worker one it must not touch.
func TestClaimAtFor(t *testing.T) {
	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	lease := base.Add(2 * time.Minute)
	wake := base.Add(5 * time.Minute)

	testCases := map[string]struct {
		proc *agentkit.Process
		want time.Time
	}{
		// The caller supplies its own `now`, which may precede a row written
		// moments ago, so a pending row with no wake time must be claimable at
		// any instant rather than from its own CreatedAt.
		"pending without wake time is claimable at any time": {
			proc: &agentkit.Process{Status: agentkit.ProcessPending, CreatedAt: base.Add(time.Hour)},
			want: agentproc.ClaimImmediatelyForTest,
		},
		"pending with wake time serves the retry backoff": {
			proc: &agentkit.Process{Status: agentkit.ProcessPending, CreatedAt: base, WakeAt: &wake},
			want: wake,
		},
		"waiting without wake time never wakes by itself": {
			proc: &agentkit.Process{Status: agentkit.ProcessWaiting, CreatedAt: base},
			want: agentproc.ClaimNeverForTest,
		},
		"waiting with wake time wakes at the deadline": {
			proc: &agentkit.Process{Status: agentkit.ProcessWaiting, CreatedAt: base, WakeAt: &wake},
			want: wake,
		},
		"running is reclaimable when the lease expires": {
			proc: &agentkit.Process{Status: agentkit.ProcessRunning, CreatedAt: base, UpdatedAt: base, LeaseUntil: &lease},
			want: lease,
		},
		"running without a lease is reclaimable at any time": {
			proc: &agentkit.Process{Status: agentkit.ProcessRunning, CreatedAt: base, UpdatedAt: base.Add(time.Hour)},
			want: agentproc.ClaimImmediatelyForTest,
		},
		"succeeded is never claimable": {
			proc: &agentkit.Process{Status: agentkit.ProcessSucceeded, CreatedAt: base},
			want: agentproc.ClaimNeverForTest,
		},
		"failed is never claimable": {
			proc: &agentkit.Process{Status: agentkit.ProcessFailed, CreatedAt: base},
			want: agentproc.ClaimNeverForTest,
		},
		"cancelled is never claimable": {
			proc: &agentkit.Process{Status: agentkit.ProcessCancelled, CreatedAt: base},
			want: agentproc.ClaimNeverForTest,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := agentproc.ClaimAtForTest(tc.proc)
			gt.Bool(t, got.Equal(tc.want)).True()
		})
	}
}

// TestClaimNeverIsIndexable guards the reason a sentinel is used instead of a
// null: every stored ClaimAt must be a timestamp, so Firestore's cross-type
// inequality ordering cannot hand the claim query a row it must not see.
func TestClaimNeverIsIndexable(t *testing.T) {
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	gt.Bool(t, agentproc.ClaimNeverForTest.After(now)).True()
}

// TestProcessRowPreservesEveryField asserts the stored shape carries the
// Process verbatim. The repository never enumerates Process fields, so this is
// what keeps a field added to agentkit.Process from being silently dropped.
func TestProcessRowPreservesEveryField(t *testing.T) {
	lease := time.Date(2026, 3, 4, 5, 8, 7, 0, time.UTC)
	parent := agentkit.ProcessID("parent-1")
	p := &agentkit.Process{
		ID:               "proc-1",
		Agent:            "case-thread",
		Status:           agentkit.ProcessRunning,
		Metadata:         map[string]string{"workspace_id": "ws-1"},
		Output:           []byte(`{"ok":true}`),
		Failure:          &agentkit.Failure{Code: agentkit.FailureStrategyError, Message: "boom"},
		State:            []byte(`{"phase":"plan"}`),
		StateVersion:     2,
		StateSeq:         7,
		HistoryRef:       "hist-9",
		InheritedHistory: &agentkit.InheritedHistory{Process: "proc-0", Ref: "hist-0"},
		StepAttempts:     1,
		UncleanReclaims:  3,
		Metrics:          agentkit.Metrics{InputTokens: 11, OutputTokens: 22, LLMCalls: 2, ToolCalls: 5, Steps: 7, Spawns: 1},
		ParentID:         &parent,
		RootID:           "root-1",
		Subject:          &agentkit.SubjectRef{Kind: "slack-thread", ID: "ssn-1"},
		IdempotencyKey:   "trigger-1",
		CancelRequested:  true,
		CancelReason:     "operator",
		LeaseOwner:       "worker-1",
		LeaseToken:       "token-1",
		LeaseUntil:       &lease,
		Rev:              4,
		CreatedAt:        time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 3, 4, 5, 7, 7, 0, time.UTC),
	}

	stored, claimAt := agentproc.NewProcessRowForTest(p)
	gt.Value(t, stored).Equal(*p)
	gt.Bool(t, claimAt.Equal(lease)).True()
}

func TestHashID(t *testing.T) {
	t.Run("produces a firestore-safe document id", func(t *testing.T) {
		got := agentproc.HashIDForTest("workspaces/ws-1/cases/7")
		gt.Number(t, len(got)).Equal(64)
		gt.String(t, got).NotContains("/")
	})

	t.Run("is stable and collision-free for distinct inputs", func(t *testing.T) {
		a := agentproc.HashIDForTest("tasks:1")
		b := agentproc.HashIDForTest("tasks:2")
		gt.String(t, a).Equal(agentproc.HashIDForTest("tasks:1"))
		gt.String(t, a).NotEqual(b)
	})
}
