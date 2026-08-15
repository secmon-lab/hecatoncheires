package job_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

const (
	testSlotTTL     = 30 * time.Second
	testSlotRenew   = 10 * time.Second
	testSlotMaxHold = 2 * time.Hour
)

// testClock is a manually advanced clock so expiry / max-hold behaviour is
// exercised without sleeping.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func slotKey(n int) model.JobRunKey {
	return model.JobRunKey{
		WorkspaceID: fmt.Sprintf("ws-%d", n),
		CaseID:      int64(n + 1),
		JobID:       "daily_sweep",
	}
}

func newLimiter(t *testing.T, repo interfaces.JobSlotRepository, limit int, clock *testClock) *job.ConcurrencyLimiter {
	t.Helper()
	var holderSeq int
	var mu sync.Mutex
	l, err := job.NewConcurrencyLimiter(job.ConcurrencyLimiterDeps{
		Repo:          repo,
		Limit:         limit,
		TTL:           testSlotTTL,
		RenewInterval: testSlotRenew,
		MaxHold:       testSlotMaxHold,
		NewHolderID: func() string {
			mu.Lock()
			defer mu.Unlock()
			holderSeq++
			return fmt.Sprintf("holder-%d", holderSeq)
		},
		Clock: clock.Now,
	})
	gt.NoError(t, err).Required()
	return l
}

func TestConcurrencyLimiterDeps_Validate(t *testing.T) {
	base := job.ConcurrencyLimiterDeps{
		Repo:          memory.New().JobSlot(),
		Limit:         2,
		TTL:           testSlotTTL,
		RenewInterval: testSlotRenew,
		MaxHold:       testSlotMaxHold,
	}
	gt.NoError(t, base.Validate())

	t.Run("nil repo", func(t *testing.T) {
		d := base
		d.Repo = nil
		gt.Error(t, d.Validate())
	})
	t.Run("non-positive limit", func(t *testing.T) {
		d := base
		d.Limit = 0
		gt.Error(t, d.Validate())
	})
	t.Run("non-positive ttl", func(t *testing.T) {
		d := base
		d.TTL = 0
		gt.Error(t, d.Validate())
	})
	t.Run("non-positive renew interval", func(t *testing.T) {
		d := base
		d.RenewInterval = 0
		gt.Error(t, d.Validate())
	})
	t.Run("renew interval not shorter than ttl", func(t *testing.T) {
		d := base
		d.RenewInterval = d.TTL
		gt.Error(t, d.Validate())
	})
	t.Run("max hold shorter than ttl", func(t *testing.T) {
		d := base
		d.MaxHold = d.TTL - time.Second
		gt.Error(t, d.Validate())
	})

	t.Run("NewConcurrencyLimiter rejects invalid deps", func(t *testing.T) {
		d := base
		d.Limit = -1
		l, err := job.NewConcurrencyLimiter(d)
		gt.Error(t, err)
		gt.Value(t, l).Nil()
	})
}

func TestConcurrencyLimiter_AdmitsUpToLimit(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()
	limiter := newLimiter(t, repo, 3, clock)

	holds := make([]*job.SlotHoldForTest, 0, 3)
	seen := make(map[int]bool)
	for i := 0; i < 3; i++ {
		hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(i))
		gt.NoError(t, err).Required()
		gt.Value(t, hold).NotNil().Required()
		gt.Bool(t, seen[job.SlotHoldIndexForTest(hold)]).False() // each run gets its own slot
		seen[job.SlotHoldIndexForTest(hold)] = true
		gt.Number(t, job.SlotHoldIndexForTest(hold)).GreaterOrEqual(0)
		gt.Number(t, job.SlotHoldIndexForTest(hold)).LessOrEqual(2)
		holds = append(holds, hold)
	}

	// Fourth run finds no free slot: (nil, nil) means "give up", not an error.
	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(3))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).Nil()

	// Releasing one makes room again, on the index that was freed.
	freed := job.SlotHoldIndexForTest(holds[1])
	job.ReleaseSlotForTest(ctx, holds[1])
	hold, err = job.AcquireSlotForTest(ctx, limiter, slotKey(4))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()
	gt.Value(t, job.SlotHoldIndexForTest(hold)).Equal(freed)

	job.ReleaseSlotForTest(ctx, hold)
	job.ReleaseSlotForTest(ctx, holds[0])
	job.ReleaseSlotForTest(ctx, holds[2])
	async.Wait()

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}

// The exported Acquire is what the agent runtime calls. It must report a full
// gate as (nil, nil) — an untyped nil, not a typed nil in a non-nil interface,
// which the runtime would read as "admitted" and let the run through.
func TestConcurrencyLimiter_AcquireReportsAFullGateAsNoHold(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	limiter := newLimiter(t, repo, 1, newTestClock())

	ref := agentkernel.SlotRef{WorkspaceID: "ws-1", CaseID: 9, JobID: "job-nightly"}
	first, err := limiter.Acquire(ctx, ref)
	gt.NoError(t, err).Required()
	gt.Value(t, first).NotNil().Required()

	// The slot records which run holds it, so an operator can see what occupies
	// capacity.
	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(1).Required()
	gt.String(t, stored[0].WorkspaceID).Equal("ws-1")
	gt.Number(t, stored[0].CaseID).Equal(9)
	gt.String(t, stored[0].JobID).Equal("job-nightly")

	second, err := limiter.Acquire(ctx, agentkernel.SlotRef{WorkspaceID: "ws-1", CaseID: 10, JobID: "job-other"})
	gt.NoError(t, err).Required()
	gt.Value(t, second).Nil()
	gt.Bool(t, second == nil).True() // untyped nil, so the runtime refuses the claim

	first.Release(ctx)
	async.Wait()

	// Capacity is back.
	third, err := limiter.Acquire(ctx, agentkernel.SlotRef{WorkspaceID: "ws-1", CaseID: 11, JobID: "job-third"})
	gt.NoError(t, err).Required()
	gt.Value(t, third).NotNil().Required()
	third.Release(ctx)
	async.Wait()
}

// Release through the runtime's interface must be idempotent: the claim bracket
// releases on its own path out, and a caller may release again.
func TestConcurrencyLimiter_AcquireReleaseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	limiter := newLimiter(t, repo, 1, newTestClock())

	hold, err := limiter.Acquire(ctx, agentkernel.SlotRef{WorkspaceID: "ws-1", CaseID: 1, JobID: "job-a"})
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	hold.Release(ctx)
	hold.Release(ctx)
	async.Wait()

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}

func TestConcurrencyLimiter_RecordsRunIdentity(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()
	limiter := newLimiter(t, repo, 1, clock)

	key := slotKey(7)
	hold, err := job.AcquireSlotForTest(ctx, limiter, key)
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(1).Required()
	gt.Value(t, stored[0].Index).Equal(0)
	gt.Value(t, stored[0].WorkspaceID).Equal(key.WorkspaceID)
	gt.Value(t, stored[0].CaseID).Equal(key.CaseID)
	gt.Value(t, stored[0].JobID).Equal(key.JobID)
	gt.Bool(t, stored[0].AcquiredAt.Equal(clock.Now())).True()
	gt.Bool(t, stored[0].ExpiresAt.Equal(clock.Now().Add(testSlotTTL))).True()

	job.ReleaseSlotForTest(ctx, hold)
	async.Wait()
}

func TestConcurrencyLimiter_TakesOverExpiredSlot(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()
	limiter := newLimiter(t, repo, 1, clock)

	first, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, first).NotNil().Required()

	// Still occupied inside the TTL.
	blocked, err := job.AcquireSlotForTest(ctx, limiter, slotKey(1))
	gt.NoError(t, err).Required()
	gt.Value(t, blocked).Nil()

	// Past the TTL (as if the holder's instance died without releasing) the
	// slot is reusable.
	clock.advance(testSlotTTL + time.Second)
	taken, err := job.AcquireSlotForTest(ctx, limiter, slotKey(2))
	gt.NoError(t, err).Required()
	gt.Value(t, taken).NotNil().Required()
	gt.Value(t, job.SlotHoldIndexForTest(taken)).Equal(job.SlotHoldIndexForTest(first))

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(1).Required()
	gt.Value(t, stored[0].WorkspaceID).Equal(slotKey(2).WorkspaceID)

	// The superseded holder's Release must not evict the new holder.
	job.ReleaseSlotForTest(ctx, first)
	stored, err = repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(1).Required()
	gt.Value(t, stored[0].WorkspaceID).Equal(slotKey(2).WorkspaceID)

	job.ReleaseSlotForTest(ctx, taken)
	async.Wait()
}

func TestConcurrencyLimiter_IgnoresSlotsBeyondLimit(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()

	// A hold left over from a larger previous limit.
	leftover := &model.JobSlot{
		Index:       5,
		HolderID:    "old-holder",
		WorkspaceID: "ws-old",
		CaseID:      99,
		JobID:       "daily_sweep",
		AcquiredAt:  clock.Now(),
		ExpiresAt:   clock.Now().Add(testSlotTTL),
	}
	acquired, err := repo.TryAcquire(ctx, leftover, clock.Now())
	gt.NoError(t, err).Required()
	gt.Bool(t, acquired).True()

	limiter := newLimiter(t, repo, 2, clock)
	for i := 0; i < 2; i++ {
		hold, acqErr := job.AcquireSlotForTest(ctx, limiter, slotKey(i))
		gt.NoError(t, acqErr).Required()
		gt.Value(t, hold).NotNil().Required()
		gt.Number(t, job.SlotHoldIndexForTest(hold)).LessOrEqual(1)
		defer job.ReleaseSlotForTest(ctx, hold)
	}
	// The leftover record does not consume one of the two new slots, and the
	// new limit is still enforced.
	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(9))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).Nil()
}

// A refusal is only actionable with the occupancy behind it: "the limit is 2"
// alone cannot distinguish a genuinely saturated gate from a misread one.
func TestConcurrencyLimiter_ReportsObservedOccupancy(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()
	limiter := newLimiter(t, repo, 2, clock)

	// Nothing held yet.
	first, occupied, limit, err := job.AcquireSlotObservedForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, first).NotNil().Required()
	gt.Number(t, occupied).Equal(0)
	gt.Number(t, limit).Equal(2)

	// One held.
	second, occupied, limit, err := job.AcquireSlotObservedForTest(ctx, limiter, slotKey(1))
	gt.NoError(t, err).Required()
	gt.Value(t, second).NotNil().Required()
	gt.Number(t, occupied).Equal(1)
	gt.Number(t, limit).Equal(2)

	// Saturated: the refusal reports both slots as occupied.
	blocked, occupied, limit, err := job.AcquireSlotObservedForTest(ctx, limiter, slotKey(2))
	gt.NoError(t, err).Required()
	gt.Value(t, blocked).Nil()
	gt.Number(t, occupied).Equal(2)
	gt.Number(t, limit).Equal(2)

	job.ReleaseSlotForTest(ctx, first)
	job.ReleaseSlotForTest(ctx, second)
	async.Wait()
}

// An invalid key never reaches the slot listing, so there is no occupancy to
// report — but the limit is still known and must be reported rather than zero.
func TestConcurrencyLimiter_ObservationOnInvalidKey(t *testing.T) {
	ctx := context.Background()
	limiter := newLimiter(t, memory.New().JobSlot(), 3, newTestClock())

	hold, occupied, limit, err := job.AcquireSlotObservedForTest(ctx, limiter, model.JobRunKey{WorkspaceID: "ws"})
	gt.Error(t, err)
	gt.Value(t, hold).Nil()
	gt.Number(t, occupied).Equal(0)
	gt.Number(t, limit).Equal(3)
}

// The hold time is what says whether the capacity was actually working; it is
// logged at release so a run that dies before its own summary still records it.
func TestConcurrencyLimiter_ReleaseReportsHoldDuration(t *testing.T) {
	repo := memory.New().JobSlot()
	clock := newTestClock()
	limiter := newLimiter(t, repo, 2, clock)

	ctx, out := jsonLogContext(context.Background())
	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	clock.advance(7 * time.Second)
	held := job.ReleaseSlotForTest(ctx, hold)
	gt.Value(t, held).Equal(7 * time.Second)
	async.Wait()

	rec, ok := findLogRecord(out.lines(), "job concurrency slot released")
	gt.Bool(t, ok).True().Required()
	gt.Value(t, rec["slot_hold_ms"]).Equal(float64(7_000))
	gt.Value(t, rec["slot_index"]).Equal(float64(job.SlotHoldIndexForTest(hold)))
	gt.Value(t, rec["slot_limit"]).Equal(float64(2))
}

// A clock that goes backwards must not report a negative hold.
func TestConcurrencyLimiter_ReleaseClampsNegativeHold(t *testing.T) {
	ctx := context.Background()
	clock := newTestClock()
	limiter := newLimiter(t, memory.New().JobSlot(), 1, clock)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	clock.advance(-time.Minute)
	gt.Value(t, job.ReleaseSlotForTest(ctx, hold)).Equal(time.Duration(0))
	async.Wait()
}

func TestConcurrencyLimiter_KeyIsValidated(t *testing.T) {
	ctx := context.Background()
	limiter := newLimiter(t, memory.New().JobSlot(), 1, newTestClock())
	hold, err := job.AcquireSlotForTest(ctx, limiter, model.JobRunKey{WorkspaceID: "ws"})
	gt.Error(t, err)
	gt.Value(t, hold).Nil()
}

// --- failure injection ------------------------------------------------

// fakeSlotRepo lets tests fail individual operations and observe the
// heartbeat. Every method is safe for concurrent use because the renew loop
// runs in its own goroutine.
type fakeSlotRepo struct {
	mu         sync.Mutex
	slots      map[int]model.JobSlot
	listErr    error
	acquireErr error
	renewErr   error
	renewCalls []renewCall
	renewed    chan struct{}
}

type renewCall struct {
	index     int
	holderID  string
	expiresAt time.Time
}

func newFakeSlotRepo() *fakeSlotRepo {
	return &fakeSlotRepo{
		slots:   make(map[int]model.JobSlot),
		renewed: make(chan struct{}, 64),
	}
}

func (f *fakeSlotRepo) List(_ context.Context) ([]*model.JobSlot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*model.JobSlot, 0, len(f.slots))
	for i := range f.slots {
		s := f.slots[i]
		out = append(out, &s)
	}
	return out, nil
}

func (f *fakeSlotRepo) TryAcquire(_ context.Context, slot *model.JobSlot, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	if existing, ok := f.slots[slot.Index]; ok && existing.IsHeld(now) {
		return false, nil
	}
	f.slots[slot.Index] = *slot
	return true, nil
}

func (f *fakeSlotRepo) Renew(_ context.Context, index int, holderID string, expiresAt time.Time) error {
	f.mu.Lock()
	f.renewCalls = append(f.renewCalls, renewCall{index: index, holderID: holderID, expiresAt: expiresAt})
	err := f.renewErr
	if err == nil {
		if existing, ok := f.slots[index]; ok && existing.HolderID == holderID {
			existing.ExpiresAt = expiresAt
			f.slots[index] = existing
		} else {
			err = goerr.Wrap(interfaces.ErrJobSlotNotHeld, "fake: not held")
		}
	}
	f.mu.Unlock()
	select {
	case f.renewed <- struct{}{}:
	default:
	}
	return err
}

func (f *fakeSlotRepo) Release(_ context.Context, index int, holderID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.slots[index]; ok && existing.HolderID == holderID {
		delete(f.slots, index)
	}
	return nil
}

// setRenewErr swaps the injected Renew failure while the heartbeat is running.
func (f *fakeSlotRepo) setRenewErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewErr = err
}

// waitForRenewedExpiry blocks until the stored expiry for index differs from
// before, and returns the new value. Waiting on the *effect* (not merely on a
// Renew attempt) is what proves a renewal actually landed.
func (f *fakeSlotRepo) waitForRenewedExpiry(index int, before time.Time) time.Time {
	for {
		<-f.renewed
		f.mu.Lock()
		s, ok := f.slots[index]
		f.mu.Unlock()
		if ok && !s.ExpiresAt.Equal(before) {
			return s.ExpiresAt
		}
	}
}

func (f *fakeSlotRepo) renewSnapshot() []renewCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]renewCall, len(f.renewCalls))
	copy(out, f.renewCalls)
	return out
}

func (f *fakeSlotRepo) storedExpiry(index int) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.slots[index]
	return s.ExpiresAt, ok
}

func newFastLimiter(t *testing.T, repo interfaces.JobSlotRepository, limit int, clock *testClock, maxHold time.Duration) *job.ConcurrencyLimiter {
	t.Helper()
	l, err := job.NewConcurrencyLimiter(job.ConcurrencyLimiterDeps{
		Repo:  repo,
		Limit: limit,
		// Short real-time durations so the heartbeat fires promptly; the
		// clock injected below still drives expiry / max-hold decisions.
		TTL:           20 * time.Millisecond,
		RenewInterval: 2 * time.Millisecond,
		MaxHold:       maxHold,
		NewHolderID:   func() string { return "holder-fast" },
		Clock:         clock.Now,
	})
	gt.NoError(t, err).Required()
	return l
}

func TestConcurrencyLimiter_AcquireFailsWhenSlotStateUnreadable(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	repo.listErr = goerr.New("firestore unavailable")
	limiter := newLimiter(t, repo, 2, newTestClock())

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.Error(t, err)
	gt.Value(t, hold).Nil()
}

func TestConcurrencyLimiter_AcquireFailsWhenClaimErrors(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	repo.acquireErr = goerr.New("transaction aborted")
	limiter := newLimiter(t, repo, 2, newTestClock())

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.Error(t, err)
	gt.Value(t, hold).Nil()
}

func TestConcurrencyLimiter_HeartbeatExtendsThenStopsOnRelease(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	clock := newTestClock()
	limiter := newFastLimiter(t, repo, 1, clock, testSlotMaxHold)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	// Wait for a real renewal rather than sleeping a fixed span.
	<-repo.renewed
	clock.advance(5 * time.Millisecond)
	<-repo.renewed

	job.ReleaseSlotForTest(ctx, hold)
	async.Wait() // returns only once the heartbeat goroutine exited

	calls := repo.renewSnapshot()
	gt.Number(t, len(calls)).GreaterOrEqual(2).Required()
	for _, c := range calls {
		gt.Value(t, c.index).Equal(job.SlotHoldIndexForTest(hold))
		gt.Value(t, c.holderID).Equal("holder-fast")
		// Every renewal pushes the expiry one TTL past the current clock.
		gt.Bool(t, c.expiresAt.After(clock.Now().Add(-time.Second))).True()
	}
	// The last renewal used the advanced clock, proving the loop kept running.
	last := calls[len(calls)-1]
	gt.Bool(t, last.expiresAt.Equal(clock.Now().Add(20*time.Millisecond))).True()

	// Release removed the record, so no expiry remains to extend.
	_, ok := repo.storedExpiry(job.SlotHoldIndexForTest(hold))
	gt.Bool(t, ok).False()

	// No renewal happens after Release.
	before := len(repo.renewSnapshot())
	async.Wait()
	gt.Value(t, len(repo.renewSnapshot())).Equal(before)
}

func TestConcurrencyLimiter_HeartbeatStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := newFakeSlotRepo()
	clock := newTestClock()
	limiter := newFastLimiter(t, repo, 1, clock, testSlotMaxHold)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()
	<-repo.renewed

	cancel()
	async.Wait() // the heartbeat observed the cancellation and returned

	before := len(repo.renewSnapshot())
	gt.Number(t, before).GreaterOrEqual(1)

	// The slot record survives cancellation: freeing it is Release's job.
	_, ok := repo.storedExpiry(job.SlotHoldIndexForTest(hold))
	gt.Bool(t, ok).True()

	job.ReleaseSlotForTest(context.Background(), hold)
	_, ok = repo.storedExpiry(job.SlotHoldIndexForTest(hold))
	gt.Bool(t, ok).False()
}

func TestConcurrencyLimiter_HeartbeatStopsAfterMaxHold(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	clock := newTestClock()
	limiter := newFastLimiter(t, repo, 1, clock, time.Minute)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()
	<-repo.renewed

	// Past MaxHold the heartbeat gives up on its own — no Release needed.
	clock.advance(2 * time.Minute)
	async.Wait()

	before := len(repo.renewSnapshot())
	async.Wait()
	gt.Value(t, len(repo.renewSnapshot())).Equal(before)

	// The slot is left to expire by TTL rather than being renewed forever.
	expiry, ok := repo.storedExpiry(job.SlotHoldIndexForTest(hold))
	gt.Bool(t, ok).True()
	gt.Bool(t, expiry.Before(clock.Now())).True()

	job.ReleaseSlotForTest(ctx, hold)
}

func TestConcurrencyLimiter_HeartbeatStopsWhenSlotTakenOver(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	repo.renewErr = goerr.Wrap(interfaces.ErrJobSlotNotHeld, "taken over")
	clock := newTestClock()
	limiter := newFastLimiter(t, repo, 1, clock, testSlotMaxHold)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	// Losing the slot is terminal: the first ErrJobSlotNotHeld ends the loop.
	async.Wait()
	calls := repo.renewSnapshot()
	gt.Array(t, calls).Length(1)

	job.ReleaseSlotForTest(ctx, hold)
}

func TestConcurrencyLimiter_HeartbeatSurvivesTransientRenewFailure(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSlotRepo()
	// A backend hiccup, NOT a lost slot: giving up here would let the slot
	// expire under a running job and admit a run over the limit.
	repo.renewErr = goerr.New("firestore unavailable")
	clock := newTestClock()
	limiter := newFastLimiter(t, repo, 1, clock, testSlotMaxHold)

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()
	index := job.SlotHoldIndexForTest(hold)
	initialExpiry, ok := repo.storedExpiry(index)
	gt.Bool(t, ok).True().Required()

	// Three failures in a row and the heartbeat is still trying.
	for range 3 {
		<-repo.renewed
	}

	// Recovery is picked up without restarting the run. Advance the clock first
	// so the recovered renewal writes a distinguishable expiry.
	clock.advance(5 * time.Millisecond)
	repo.setRenewErr(nil)
	extended := repo.waitForRenewedExpiry(index, initialExpiry)

	job.ReleaseSlotForTest(ctx, hold)
	async.Wait()

	calls := repo.renewSnapshot()
	gt.Number(t, len(calls)).GreaterOrEqual(4)
	for _, c := range calls {
		gt.Value(t, c.index).Equal(index)
		gt.Value(t, c.holderID).Equal("holder-fast")
	}
	gt.Bool(t, extended.Equal(clock.Now().Add(20*time.Millisecond))).True()
}

func TestConcurrencyLimiter_ConcurrentAcquireNeverExceedsLimit(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	clock := newTestClock()
	const limit = 3
	const contenders = 8
	limiter := newLimiter(t, repo, limit, clock)

	// Every goroutine is released at once so the acquisitions genuinely race:
	// a non-atomic claim would let two of them win the same index.
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	holds := make([]*job.SlotHoldForTest, contenders)
	errs := make([]error, contenders)
	for i := range contenders {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			holds[i], errs[i] = job.AcquireSlotForTest(ctx, limiter, slotKey(i))
		}(i)
	}
	start.Done()
	done.Wait()

	seen := make(map[int]bool, limit)
	granted := 0
	for i := range contenders {
		gt.NoError(t, errs[i])
		if holds[i] == nil {
			continue
		}
		granted++
		index := job.SlotHoldIndexForTest(holds[i])
		gt.Bool(t, seen[index]).False() // no index handed out twice
		seen[index] = true
		gt.Number(t, index).GreaterOrEqual(0)
		gt.Number(t, index).LessOrEqual(limit - 1)
	}
	gt.Value(t, granted).Equal(limit)

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(limit)

	for i := range contenders {
		if holds[i] != nil {
			job.ReleaseSlotForTest(ctx, holds[i])
		}
	}
	async.Wait()
	stored, err = repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}

func TestConcurrencyLimiter_ReleaseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := memory.New().JobSlot()
	limiter := newLimiter(t, repo, 1, newTestClock())

	hold, err := job.AcquireSlotForTest(ctx, limiter, slotKey(0))
	gt.NoError(t, err).Required()
	gt.Value(t, hold).NotNil().Required()

	job.ReleaseSlotForTest(ctx, hold)
	job.ReleaseSlotForTest(ctx, hold) // must not panic on the already-closed stop channel
	async.Wait()

	stored, err := repo.List(ctx)
	gt.NoError(t, err).Required()
	gt.Array(t, stored).Length(0)
}
