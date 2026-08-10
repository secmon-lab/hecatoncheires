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

// newStoredProcess inserts a Process so events have something to attach to.
func newStoredProcess(t *testing.T, repo *agentproc.Repository) agentkit.ProcessID {
	t.Helper()
	ctx := context.Background()
	pid := agentkit.ProcessID(uuid.Must(uuid.NewV7()).String())
	now := time.Now().UTC()
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Processes: []*agentkit.Process{{
		ID:        pid,
		Agent:     "test",
		Status:    agentkit.ProcessPending,
		RootID:    pid,
		CreatedAt: now,
		UpdatedAt: now,
	}}})).Required()
	return pid
}

func newEvent(pid agentkit.ProcessID, id string, payload string) *agentkit.Event {
	return &agentkit.Event{
		ID:        agentkit.EventID(id),
		ProcessID: pid,
		Type:      agentkit.EventProcessCreated,
		Payload:   []byte(payload),
		At:        time.Now().UTC(),
	}
}

// TestListEventsOrdersByAppendNotByID is the reason events carry a stored
// sequence. Event ids are uuid v7, and this codebase already records that uuid
// v7 ids "may diverge under clock skew" across instances
// (interfaces.JobRunEventRepository). A durable Process moves between instances
// between claims, so an id that sorts BEFORE its predecessor is a real outcome —
// and the append order must survive it.
func TestListEventsOrdersByAppendNotByID(t *testing.T) {
	ctx := context.Background()
	repo := newRepository(t)
	pid := newStoredProcess(t, repo)

	// The second append deliberately carries the lexicographically SMALLER id,
	// as a machine with a slow clock would produce.
	first := newEvent(pid, "zz-appended-first", "1")
	second := newEvent(pid, "aa-appended-second", "2")

	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{first}})).Required()
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{second}})).Required()

	events, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{})
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()
	gt.Value(t, events[0].ID).Equal(first.ID)
	gt.Value(t, events[1].ID).Equal(second.ID)

	t.Run("the cursor follows the same order", func(t *testing.T) {
		after, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{After: first.ID})
		gt.NoError(t, err).Required()
		gt.Array(t, after).Length(1).Required()
		gt.Value(t, after[0].ID).Equal(second.ID)
	})
}

// TestApplyRejectsDuplicateEventID pins that an event is an append. Replacing an
// event would change what a caller holding its id as a cursor resumes from.
func TestApplyRejectsDuplicateEventID(t *testing.T) {
	ctx := context.Background()
	repo := newRepository(t)
	pid := newStoredProcess(t, repo)

	ev := newEvent(pid, "e-1", "first")
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{ev}})).Required()

	again := newEvent(pid, "e-1", "second")
	gt.Error(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{again}})).Is(agentkit.ErrConflict)

	events, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{})
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, string(events[0].Payload)).Equal("first")
}

// TestApplyRejectsEventForUnknownProcess pins that an event cannot be numbered
// against a Process that does not exist: there is nothing to append it to, and a
// silent write would land in an order nobody can reproduce.
func TestApplyRejectsEventForUnknownProcess(t *testing.T) {
	ctx := context.Background()
	repo := newRepository(t)

	ev := newEvent(agentkit.ProcessID("never-created"), "e-1", "x")
	gt.Error(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{ev}})).Is(agentkit.ErrConflict)
}

// TestAppendingEventsDoesNotBumpRev pins that the append counter rides on the
// Process row without changing its revision. A revision the kernel did not
// expect would fence out the very worker that asked for the append.
func TestAppendingEventsDoesNotBumpRev(t *testing.T) {
	ctx := context.Background()
	repo := newRepository(t)
	pid := newStoredProcess(t, repo)

	before, err := repo.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()

	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{
		newEvent(pid, "e-1", "1"),
		newEvent(pid, "e-2", "2"),
	}})).Required()

	after, err := repo.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	gt.Value(t, after.Rev).Equal(before.Rev)

	// The next append continues the numbering rather than restarting it.
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{
		newEvent(pid, "e-3", "3"),
	}})).Required()

	events, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{})
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()
	gt.Value(t, string(events[0].Payload)).Equal("1")
	gt.Value(t, string(events[1].Payload)).Equal("2")
	gt.Value(t, string(events[2].Payload)).Equal("3")
}

// TestEventSequenceSurvivesTheProcessLifecycle is the regression this numbering
// has to withstand. agentkit rewrites a Process row on every ordinary
// transition and on every claim, so a row written from a base of zero would
// restart the numbering: the next event would reuse a number an earlier one
// already holds, the append order would stop being a total order, and a cursor
// would skip every event sharing that number.
func TestEventSequenceSurvivesTheProcessLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newRepository(t)
	pid := newStoredProcess(t, repo)

	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{
		newEvent(pid, "e-1", "1"),
	}})).Required()

	// An ordinary Process-only transition, exactly as a strategy step commits.
	stored, err := repo.GetProcess(ctx, pid)
	gt.NoError(t, err).Required()
	updated := *stored
	updated.StateSeq = stored.StateSeq + 1
	updated.State = []byte(`{"phase":"next"}`)
	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Processes: []*agentkit.Process{&updated}})).Required()

	// A claim, which rewrites the row again.
	now := time.Now().UTC()
	claimed, err := repo.ClaimNextProcess(ctx, "worker-1", now.Add(time.Hour), now.Add(time.Hour))
	gt.NoError(t, err).Required()
	gt.Value(t, claimed).NotNil().Required()
	gt.Value(t, claimed.ID).Equal(pid)

	gt.NoError(t, repo.Apply(ctx, agentkit.ChangeSet{Events: []*agentkit.Event{
		newEvent(pid, "e-2", "2"),
	}})).Required()

	events, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{})
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()
	gt.Value(t, string(events[0].Payload)).Equal("1")
	gt.Value(t, string(events[1].Payload)).Equal("2")

	// The cursor must reach the second event. A restarted counter gives both the
	// same number, and "after the first" then excludes the second as well.
	after, err := repo.ListEvents(ctx, pid, agentkit.EventQuery{After: "e-1"})
	gt.NoError(t, err).Required()
	gt.Array(t, after).Length(1).Required()
	gt.Value(t, string(after[0].Payload)).Equal("2")
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
