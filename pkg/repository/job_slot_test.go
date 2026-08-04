package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

// jobSlotIndexSeq keeps slot indices unique across every test in this package.
// jobSlots is a deployment-global collection (no workspace / case scoping), so
// two parallel tests reusing index 0 against the same emulator database would
// fight over the same document.
var jobSlotIndexSeq atomic.Int64

func uniqueSlotIndex() int {
	return int(time.Now().UnixNano()%1_000_000)*100 + int(jobSlotIndexSeq.Add(1))
}

// findSlot returns the record for index, or nil. Tests filter by their own
// index instead of asserting on the whole list: the Firestore collection is
// shared with every other test in this package.
func findSlot(slots []*model.JobSlot, index int) *model.JobSlot {
	for _, s := range slots {
		if s != nil && s.Index == index {
			return s
		}
	}
	return nil
}

func newSlot(index int, holderID string, now time.Time, ttl time.Duration) *model.JobSlot {
	return &model.JobSlot{
		Index:       index,
		HolderID:    holderID,
		WorkspaceID: fmt.Sprintf("ws-%d", index),
		CaseID:      int64(index),
		JobID:       "daily_sweep",
		AcquiredAt:  now,
		ExpiresAt:   now.Add(ttl),
	}
}

func runJobSlotRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()
	const ttl = 30 * time.Second

	t.Run("List omits an index that was never acquired", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		gt.Value(t, findSlot(slots, index)).Nil()
	})

	t.Run("TryAcquire round-trips every field", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC().Truncate(time.Millisecond)
		want := newSlot(index, fmt.Sprintf("holder-%d", index), now, ttl)

		acquired, err := repo.JobSlot().TryAcquire(ctx, want, now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.Index).Equal(want.Index)
		gt.Value(t, got.HolderID).Equal(want.HolderID)
		gt.Value(t, got.WorkspaceID).Equal(want.WorkspaceID)
		gt.Value(t, got.CaseID).Equal(want.CaseID)
		gt.Value(t, got.JobID).Equal(want.JobID)
		gt.Bool(t, got.AcquiredAt.Sub(want.AcquiredAt).Abs() < time.Second).True()
		gt.Bool(t, got.ExpiresAt.Sub(want.ExpiresAt).Abs() < time.Second).True()
		gt.Bool(t, got.IsHeld(now)).True()
	})

	t.Run("TryAcquire rejects an invalid record", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		bad := newSlot(index, "", now, ttl) // empty holder id
		acquired, err := repo.JobSlot().TryAcquire(ctx, bad, now)
		gt.Error(t, err)
		gt.Bool(t, acquired).False()
	})

	t.Run("TryAcquire fails while a live holder is present", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		first := newSlot(index, fmt.Sprintf("holder-a-%d", index), now, ttl)
		acquired, err := repo.JobSlot().TryAcquire(ctx, first, now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		second := newSlot(index, fmt.Sprintf("holder-b-%d", index), now, ttl)
		acquired, err = repo.JobSlot().TryAcquire(ctx, second, now.Add(time.Second))
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).False()

		// The stored record still belongs to the first holder.
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.HolderID).Equal(first.HolderID)
	})

	t.Run("TryAcquire takes over an expired holder", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		first := newSlot(index, fmt.Sprintf("holder-a-%d", index), now, ttl)
		acquired, err := repo.JobSlot().TryAcquire(ctx, first, now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		later := now.Add(ttl + time.Second)
		second := newSlot(index, fmt.Sprintf("holder-b-%d", index), later, ttl)
		acquired, err = repo.JobSlot().TryAcquire(ctx, second, later)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.HolderID).Equal(second.HolderID)
	})

	t.Run("Renew extends the expiry of the owning holder", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		holder := fmt.Sprintf("holder-%d", index)
		acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(index, holder, now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		extended := now.Add(5 * time.Minute).Truncate(time.Millisecond)
		gt.NoError(t, repo.JobSlot().Renew(ctx, index, holder, extended)).Required()

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Bool(t, got.ExpiresAt.Sub(extended).Abs() < time.Second).True()
		// Renew touches only the expiry.
		gt.Value(t, got.HolderID).Equal(holder)
		gt.Value(t, got.JobID).Equal("daily_sweep")
		gt.Bool(t, got.IsHeld(now.Add(ttl+time.Second))).True()
	})

	t.Run("Renew by another holder returns ErrJobSlotNotHeld", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		owner := fmt.Sprintf("holder-a-%d", index)
		acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(index, owner, now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		err = repo.JobSlot().Renew(ctx, index, fmt.Sprintf("holder-b-%d", index), now.Add(time.Minute))
		gt.Error(t, err).Is(interfaces.ErrJobSlotNotHeld)

		// The owner's expiry is untouched.
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Bool(t, got.ExpiresAt.Sub(now.Add(ttl)).Abs() < time.Second).True()
	})

	t.Run("Renew on a free slot returns ErrJobSlotNotHeld and creates nothing", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		err := repo.JobSlot().Renew(ctx, index, "holder-x", time.Now().UTC().Add(time.Minute))
		gt.Error(t, err).Is(interfaces.ErrJobSlotNotHeld)

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		gt.Value(t, findSlot(slots, index)).Nil()
	})

	t.Run("Release frees the slot for the owning holder", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		holder := fmt.Sprintf("holder-%d", index)
		acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(index, holder, now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		gt.NoError(t, repo.JobSlot().Release(ctx, index, holder)).Required()

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		gt.Value(t, findSlot(slots, index)).Nil()

		// Immediately re-acquirable, and Release is idempotent.
		gt.NoError(t, repo.JobSlot().Release(ctx, index, holder))
		acquired, err = repo.JobSlot().TryAcquire(ctx, newSlot(index, holder+"-2", now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()
	})

	t.Run("Release by another holder is a no-op", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		owner := fmt.Sprintf("holder-a-%d", index)
		acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(index, owner, now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		gt.NoError(t, repo.JobSlot().Release(ctx, index, fmt.Sprintf("holder-b-%d", index)))

		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.HolderID).Equal(owner)
	})

	t.Run("Renew rejects an expiry that is not after AcquiredAt", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		holder := fmt.Sprintf("holder-%d", index)
		acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(index, holder, now, ttl), now)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		// A minute in the past, not exactly AcquiredAt: Firestore stores
		// timestamps at microsecond precision, so a stored AcquiredAt is
		// slightly *below* the nanosecond-precision `now` and an expiry of
		// exactly `now` would pass the After() check on that backend only.
		// The equality boundary itself is covered by model.JobSlot's own test,
		// where no storage rounding is involved.
		// Not ErrJobSlotNotHeld: the holder owns the slot, the value is invalid.
		err = repo.JobSlot().Renew(ctx, index, holder, now.Add(-time.Minute))
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, interfaces.ErrJobSlotNotHeld)).False()

		// The stored expiry is unchanged, so the live holder keeps its slot.
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.HolderID).Equal(holder)
		gt.Bool(t, got.ExpiresAt.Sub(now.Add(ttl)).Abs() < time.Second).True()
	})

	t.Run("concurrent TryAcquire on one index admits exactly one holder", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		now := time.Now().UTC()
		const contenders = 4

		var start sync.WaitGroup
		start.Add(1)
		var done sync.WaitGroup
		results := make([]bool, contenders)
		errs := make([]error, contenders)
		holders := make([]string, contenders)
		for i := 0; i < contenders; i++ {
			holders[i] = fmt.Sprintf("holder-%d-%d", index, i)
			done.Add(1)
			go func(i int) {
				defer done.Done()
				start.Wait() // release every goroutine at once
				results[i], errs[i] = repo.JobSlot().TryAcquire(ctx, newSlot(index, holders[i], now, ttl), now)
			}(i)
		}
		start.Done()
		done.Wait()

		winners := make([]string, 0, 1)
		for i := 0; i < contenders; i++ {
			if errs[i] != nil {
				// Transaction contention may exhaust the backend's retries;
				// that is a refusal, not a second admission.
				continue
			}
			if results[i] {
				winners = append(winners, holders[i])
			}
		}
		gt.Array(t, winners).Length(1).Required()

		// The stored record belongs to that single winner.
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		got := findSlot(slots, index)
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.HolderID).Equal(winners[0])
	})

	t.Run("Renew and Release require a holder id", func(t *testing.T) {
		repo := newRepo(t)
		index := uniqueSlotIndex()
		gt.Error(t, repo.JobSlot().Renew(ctx, index, "", time.Now().UTC().Add(time.Minute)))
		gt.Error(t, repo.JobSlot().Release(ctx, index, ""))
	})

	t.Run("List returns records in ascending index order", func(t *testing.T) {
		repo := newRepo(t)
		now := time.Now().UTC()
		high := uniqueSlotIndex()
		low := high - 1
		for _, idx := range []int{high, low} {
			acquired, err := repo.JobSlot().TryAcquire(ctx, newSlot(idx, fmt.Sprintf("holder-%d", idx), now, ttl), now)
			gt.NoError(t, err).Required()
			gt.Bool(t, acquired).True()
		}
		slots, err := repo.JobSlot().List(ctx)
		gt.NoError(t, err).Required()
		for i := 1; i < len(slots); i++ {
			gt.Bool(t, slots[i-1].Index < slots[i].Index).True()
		}
	})
}

func TestJobSlotRepository_Memory(t *testing.T) {
	t.Parallel()
	runJobSlotRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestJobSlotRepository_Firestore(t *testing.T) {
	t.Parallel()
	runJobSlotRepositoryTest(t, newFirestoreRepository)
}
