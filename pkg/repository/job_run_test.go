package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runJobRunRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Get returns ErrJobRunNotFound for missing record", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "missing",
		}
		_, err := repo.JobRun().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrJobRunNotFound)
	})

	t.Run("RecordRun round-trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "job-1",
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		err := repo.JobRun().RecordRun(ctx, key, model.JobRunStatusSuccess, now, "trace-abc", "")
		gt.NoError(t, err).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.Key).Equal(key)
		gt.Bool(t, got.LastRunAt.Equal(now)).True()
		gt.Value(t, got.LastStatus).Equal(model.JobRunStatusSuccess)
		gt.String(t, got.LastError).Equal("")
		gt.String(t, got.LastTraceID).Equal("trace-abc")
		gt.Bool(t, got.LeaseUntil.IsZero()).True()
	})

	t.Run("RecordRun FAILED stores error message", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "job-fail",
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		err := repo.JobRun().RecordRun(ctx, key, model.JobRunStatusFailed, now, "trace-x", "llm timeout")
		gt.NoError(t, err).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.LastStatus).Equal(model.JobRunStatusFailed)
		gt.String(t, got.LastError).Equal("llm timeout")
	})

	t.Run("TryAcquireLease succeeds when idle", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-1",
		}
		now := time.Now().UTC()
		acquired, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Bool(t, got.LeaseUntil.After(now)).True()
	})

	t.Run("TryAcquireLease blocks while lease is active", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-block",
		}
		now := time.Now().UTC()
		first, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, first).True()

		second, err := repo.JobRun().TryAcquireLease(ctx, key, now.Add(time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, second).False()
	})

	t.Run("TryAcquireLease reclaims after lease expiry", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-reclaim",
		}
		t0 := time.Now().UTC()
		first, err := repo.JobRun().TryAcquireLease(ctx, key, t0, time.Second)
		gt.NoError(t, err).Required()
		gt.Bool(t, first).True()

		// Lease elapsed.
		second, err := repo.JobRun().TryAcquireLease(ctx, key, t0.Add(2*time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, second).True()
	})

	t.Run("ReleaseLease lets the next acquirer in", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-release",
		}
		now := time.Now().UTC()
		_, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.NoError(t, repo.JobRun().ReleaseLease(ctx, key)).Required()

		again, err := repo.JobRun().TryAcquireLease(ctx, key, now.Add(time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, again).True()
	})

	t.Run("ReleaseLease is idempotent without prior acquire", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "no-prior",
		}
		gt.NoError(t, repo.JobRun().ReleaseLease(ctx, key))
	})

	t.Run("RecordRun also clears lease", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "rec-clear",
		}
		now := time.Now().UTC()
		_, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx, key, model.JobRunStatusSuccess, now.Add(time.Second), "tr", "")).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Bool(t, got.LeaseUntil.IsZero()).True()
	})

	t.Run("ListByCase returns runs scoped to the (workspace, case) pair", func(t *testing.T) {
		repo := newRepo(t)
		ws := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseA := time.Now().UnixNano()
		caseB := time.Now().UnixNano() + 1
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseA, JobID: "a1"},
			model.JobRunStatusSuccess, now, "t1", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseA, JobID: "a2"},
			model.JobRunStatusSuccess, now, "t2", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseB, JobID: "b1"},
			model.JobRunStatusSuccess, now, "t3", "")).Required()

		runsA, err := repo.JobRun().ListByCase(ctx, ws, caseA)
		gt.NoError(t, err).Required()
		gt.Array(t, runsA).Length(2).Required()
		for _, r := range runsA {
			gt.Value(t, r.Key.CaseID).Equal(caseA)
			gt.Value(t, r.Key.WorkspaceID).Equal(ws)
		}

		runsB, err := repo.JobRun().ListByCase(ctx, ws, caseB)
		gt.NoError(t, err).Required()
		gt.Array(t, runsB).Length(1).Required()
		gt.Value(t, runsB[0].Key.JobID).Equal("b1")
	})

	t.Run("ListByCase returns empty for a case with no runs", func(t *testing.T) {
		repo := newRepo(t)
		ws := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		runs, err := repo.JobRun().ListByCase(ctx, ws, time.Now().UnixNano())
		gt.NoError(t, err).Required()
		gt.Array(t, runs).Length(0)
	})

	t.Run("ListByCase scopes by workspace (same case id in different workspaces)", func(t *testing.T) {
		repo := newRepo(t)
		ws1 := fmt.Sprintf("ws1-%d", time.Now().UnixNano())
		ws2 := fmt.Sprintf("ws2-%d", time.Now().UnixNano())
		caseShared := time.Now().UnixNano()
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws1, CaseID: caseShared, JobID: "j"},
			model.JobRunStatusSuccess, now, "t1", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws2, CaseID: caseShared, JobID: "j"},
			model.JobRunStatusSuccess, now, "t2", "")).Required()

		runs1, err := repo.JobRun().ListByCase(ctx, ws1, caseShared)
		gt.NoError(t, err).Required()
		gt.Array(t, runs1).Length(1).Required()
		gt.Value(t, runs1[0].LastTraceID).Equal("t1")
	})

	t.Run("invalid key surfaces error", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.JobRun().Get(ctx, model.JobRunKey{})
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, interfaces.ErrJobRunNotFound)).False()
	})
}

func TestJobRunRepository_Memory(t *testing.T) {
	runJobRunRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestJobRunRepository_Firestore(t *testing.T) {
	runJobRunRepositoryTest(t, newFirestoreRepository)
}
