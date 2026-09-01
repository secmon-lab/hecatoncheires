package job

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// ConcurrencyLimiterDeps groups what the limiter needs. Every duration is
// supplied by the caller (the CLI wiring layer owns the values) so no default
// is buried in this package.
type ConcurrencyLimiterDeps struct {
	Repo interfaces.JobSlotRepository

	// Limit is the number of slots, i.e. the maximum number of concurrent
	// runs across the whole deployment. Must be positive — a caller that
	// wants no limit builds no limiter at all.
	Limit int

	// TTL is how long an acquired slot stays occupied without a renewal. It
	// is the delay between a holder's last heartbeat and the slot becoming
	// reusable, so it doubles as the recovery time after an instance dies.
	TTL time.Duration
	// RenewInterval is the heartbeat period. Must be shorter than TTL.
	RenewInterval time.Duration
	// MaxHold bounds how long one hold may keep renewing itself. It is a
	// backstop against a leaked hold pinning a slot forever, not a run
	// timeout: the run continues, only the renewal stops.
	MaxHold time.Duration

	// NewHolderID generates the per-acquisition holder token. nil → UUIDv7.
	NewHolderID func() string
	// Clock returns the current wall-clock time. nil → time.Now().UTC().
	Clock func() time.Time
}

// Validate enforces the relationships the limiter relies on.
func (d ConcurrencyLimiterDeps) Validate() error {
	if d.Repo == nil {
		return goerr.New("job slot repository is nil")
	}
	if d.Limit <= 0 {
		return goerr.New("job concurrency limit must be positive",
			goerr.V("limit", d.Limit))
	}
	if d.TTL <= 0 {
		return goerr.New("job slot ttl must be positive", goerr.V("ttl", d.TTL))
	}
	if d.RenewInterval <= 0 {
		return goerr.New("job slot renew interval must be positive",
			goerr.V("renew_interval", d.RenewInterval))
	}
	if d.RenewInterval >= d.TTL {
		// A heartbeat that fires no more often than the TTL cannot keep a
		// live run's slot alive.
		return goerr.New("job slot renew interval must be shorter than ttl",
			goerr.V("renew_interval", d.RenewInterval), goerr.V("ttl", d.TTL))
	}
	if d.MaxHold < d.TTL {
		return goerr.New("job slot max hold must be at least the ttl",
			goerr.V("max_hold", d.MaxHold), goerr.V("ttl", d.TTL))
	}
	return nil
}

// ConcurrencyLimiter admits at most Limit concurrent runs across every
// instance of the deployment by handing out one of Limit execution slots
// (model.JobSlot). It is stateless beyond its configuration — the slots live
// in the shared repository — so every instance enforces the same limit.
//
// It satisfies agentkernel.SlotGate, which is how the agent runtime consults it:
// once per claim, holding the slot for exactly as long as a worker is driving the
// run. That is a change in what the number bounds — a run suspended waiting for
// its children gives its slot back, having no work in flight of its own — and it
// is the accurate reading: the limit exists to bound concurrent LLM traffic.
// Asserted here so a change to either side is a compile error rather than a
// deployment that silently runs ungated.
var (
	_ agentkernel.SlotGate = (*ConcurrencyLimiter)(nil)
	_ agentkernel.SlotHold = (*slotHold)(nil)
)

type ConcurrencyLimiter struct {
	repo          interfaces.JobSlotRepository
	limit         int
	ttl           time.Duration
	renewInterval time.Duration
	maxHold       time.Duration
	newHolderID   func() string
	now           func() time.Time
}

// NewConcurrencyLimiter builds the limiter. It fails rather than silently
// degrading to "no limit": a deployment that asked for a limit must not run
// unbounded because of a wiring mistake.
func NewConcurrencyLimiter(deps ConcurrencyLimiterDeps) (*ConcurrencyLimiter, error) {
	if err := deps.Validate(); err != nil {
		return nil, goerr.Wrap(err, "invalid job concurrency limiter deps")
	}
	newHolderID := deps.NewHolderID
	if newHolderID == nil {
		newHolderID = func() string { return uuid.Must(uuid.NewV7()).String() }
	}
	now := deps.Clock
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &ConcurrencyLimiter{
		repo:          deps.Repo,
		limit:         deps.Limit,
		ttl:           deps.TTL,
		renewInterval: deps.RenewInterval,
		maxHold:       deps.MaxHold,
		newHolderID:   newHolderID,
		now:           now,
	}, nil
}

// slotHold is an acquired slot. It owns the heartbeat goroutine that keeps the
// slot alive, so the holder MUST call release when the work finishes.
type slotHold struct {
	index    int
	holderID string
	limiter  *ConcurrencyLimiter
	stop     chan struct{}
	once     sync.Once
	// acquiredAt is when this hold claimed the slot, so release can report how
	// long the slot was occupied.
	acquiredAt time.Time
}

// Release frees the slot. It satisfies agentkernel.SlotHold, which is how the
// agent runtime's claim bracket hands the slot back when a claim ends.
//
// The hold time it computes is logged rather than returned here, because the
// runtime has no summary line to fold it into — unlike the pre-agentkit
// JobRunner, which reported it alongside the run's other stage timings.
func (h *slotHold) Release(ctx context.Context) { _ = h.release(ctx) }

// Acquire takes a slot for one agent run, satisfying agentkernel.SlotGate.
//
// It returns (nil, nil) when every slot is occupied. The agent runtime turns
// that into a refused claim: the run goes back to pending and is tried again
// after a backoff, so a full gate delays work instead of failing it.
func (l *ConcurrencyLimiter) Acquire(ctx context.Context, ref agentkernel.SlotRef) (agentkernel.SlotHold, error) {
	hold, obs, err := l.acquire(ctx, model.JobRunKey{
		WorkspaceID: ref.WorkspaceID, CaseID: ref.CaseID, JobID: ref.JobID,
	})
	if err != nil {
		return nil, err
	}
	if hold == nil {
		// Logged here rather than by the caller: the occupancy that caused the
		// refusal is only visible at this layer, and without it an operator sees a
		// delayed run with no reason for the delay.
		logging.From(ctx).Debug("no job execution slot free",
			slog.Int("slot_occupied", obs.Occupied),
			slog.Int("slot_limit", obs.Limit),
			slog.String("job_id", ref.JobID))
		// A typed nil in a non-nil interface would read as "acquired" to the
		// caller, so the untyped nil is returned explicitly.
		return nil, nil
	}
	return hold, nil
}

// slotObservation is what one admission attempt saw of the gate. The caller
// reports it so a refusal shows the occupancy behind it, not just the limit.
type slotObservation struct {
	// Occupied is how many slots inside [0, limit) were held at the moment the
	// attempt listed them. It is a snapshot: another instance may claim or free
	// a slot while the attempt probes.
	Occupied int
	// Limit is the configured number of slots.
	Limit int
}

// acquire claims one free slot for the given run and starts its heartbeat.
// It returns (nil, obs, nil) when every slot is occupied — the caller's
// contract is to give up, not to wait. A repository failure returns an error:
// the caller cannot tell how many runs are in flight, so it must not proceed.
// The observation is filled in on every path that got far enough to make one.
func (l *ConcurrencyLimiter) acquire(ctx context.Context, key model.JobRunKey) (*slotHold, slotObservation, error) {
	if l == nil {
		return nil, slotObservation{}, goerr.New("concurrency limiter is nil")
	}
	obs := slotObservation{Limit: l.limit}
	if err := key.Validate(); err != nil {
		return nil, obs, goerr.Wrap(err, "invalid job run key for slot acquire")
	}

	// listedAt pairs with the List snapshot below: it decides which indices
	// *looked* free. The authoritative check happens inside TryAcquire, with a
	// fresher clock (see claimAt).
	listedAt := l.now()
	stored, err := l.repo.List(ctx)
	if err != nil {
		return nil, obs, goerr.Wrap(err, "list job slots")
	}

	// Records at or beyond the limit belong to a previously configured
	// (larger) limit; their runs finish on their own, and new acquisitions
	// only ever consider [0, limit).
	held := make(map[int]bool, len(stored))
	for _, s := range stored {
		if s == nil || s.Index >= l.limit {
			continue
		}
		if s.IsHeld(listedAt) {
			held[s.Index] = true
		}
	}
	obs.Occupied = len(held)

	holderID := l.newHolderID()
	// Start probing at a holder-derived offset so simultaneous acquirers do
	// not all pile onto index 0. A hash rather than math/rand: deterministic,
	// dependency-free, and not a weak-RNG finding for gosec.
	// #nosec G115 -- Validate guarantees limit > 0, and the modulo result is
	// strictly less than limit, so both conversions stay in range.
	start := int(hashOffset(holderID) % uint64(l.limit))
	for i := 0; i < l.limit; i++ {
		index := (start + i) % l.limit
		if held[index] {
			continue
		}
		// Read the clock per attempt rather than reusing listedAt: the List
		// above and a retried transaction can take long enough that a TTL
		// measured from listedAt would leave the slot nearly expired the
		// moment it is written.
		claimAt := l.now()
		slot := &model.JobSlot{
			Index:       index,
			HolderID:    holderID,
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			AcquiredAt:  claimAt,
			ExpiresAt:   claimAt.Add(l.ttl),
		}
		acquired, acqErr := l.repo.TryAcquire(ctx, slot, claimAt)
		if acqErr != nil {
			return nil, obs, goerr.Wrap(acqErr, "try acquire job slot",
				goerr.V("index", index))
		}
		if !acquired {
			// Lost the race to another instance; try the next free index.
			continue
		}
		hold := &slotHold{
			index:      index,
			holderID:   holderID,
			limiter:    l,
			stop:       make(chan struct{}),
			acquiredAt: claimAt,
		}
		l.startRenew(ctx, hold, claimAt)
		return hold, obs, nil
	}
	// Every slot in range is occupied, or was taken while we probed.
	return nil, obs, nil
}

// startRenew keeps the hold's slot alive until Release, ctx cancellation, or
// MaxHold. DispatchCancelable (not Dispatch) because this goroutine must die
// with the run's context — Dispatch deliberately severs cancellation for
// request tails, which is the opposite of what a heartbeat wants.
func (l *ConcurrencyLimiter) startRenew(ctx context.Context, hold *slotHold, acquiredAt time.Time) {
	deadline := acquiredAt.Add(l.maxHold)
	async.DispatchCancelable(ctx, func(ctx context.Context) error {
		ticker := time.NewTicker(l.renewInterval)
		defer ticker.Stop()
		// consecutiveFailures counts unbroken transient renewal failures. Only
		// the first of a streak is reported: a backend outage would otherwise
		// emit one error every RenewInterval for as long as the run lasts.
		consecutiveFailures := 0
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-hold.stop:
				return nil
			case <-ticker.C:
				if l.now().After(deadline) {
					// Stop renewing so a leaked hold cannot pin the slot
					// forever. The run itself is left alone.
					return goerr.New("job slot hold exceeded max duration",
						goerr.V("index", hold.index),
						goerr.V("holder_id", hold.holderID),
						goerr.V("max_hold", l.maxHold))
				}
				err := l.repo.Renew(ctx, hold.index, hold.holderID, l.now().Add(l.ttl))
				if err == nil {
					consecutiveFailures = 0
					continue
				}
				if errors.Is(err, interfaces.ErrJobSlotNotHeld) {
					// Release and cancellation may happen while Renew is already in
					// flight. In that case the repository can report the record as
					// missing after the hold has begun its normal shutdown.
					select {
					case <-ctx.Done():
						return nil
					case <-hold.stop:
						return nil
					default:
					}
					// The slot expired and another run took it over. Killing
					// this in-flight LLM run would waste the work already
					// spent, so report and stop renewing while it continues.
					errutil.Handle(ctx, goerr.Wrap(err, "job slot was taken over",
						goerr.V("index", hold.index),
						goerr.V("holder_id", hold.holderID)), "job: renew concurrency slot")
					return nil
				}
				// A transient backend failure must NOT end the heartbeat:
				// giving up here would let the slot expire under a running
				// job and admit a run over the limit. RenewInterval is a
				// third of the TTL, so a couple of failed attempts still
				// leave time to recover before the slot expires. If it does
				// expire and is taken over, the next attempt returns
				// ErrJobSlotNotHeld above and the loop ends there.
				consecutiveFailures++
				if consecutiveFailures == 1 {
					errutil.Handle(ctx, goerr.Wrap(err, "renew job slot",
						goerr.V("index", hold.index),
						goerr.V("holder_id", hold.holderID)), "job: renew concurrency slot")
				}
			}
		}
	})
}

// release stops the heartbeat and frees the slot, and returns how long the slot
// was held. Safe to call more than once. Failures are non-fatal: the slot frees
// itself one TTL after the last renewal, so a lost release costs delay, not
// correctness.
//
// The hold time is logged here rather than only returned, because a run that
// dies between release and its own summary line would otherwise leave no record
// of how long it occupied capacity.
func (h *slotHold) release(ctx context.Context) time.Duration {
	if h == nil || h.limiter == nil {
		return 0
	}
	held := max(h.limiter.now().Sub(h.acquiredAt), 0)
	h.once.Do(func() { close(h.stop) })

	logging.From(ctx).Info("job concurrency slot released",
		slog.Int("slot_index", h.index),
		slog.Int("slot_limit", h.limiter.limit),
		slog.Int64("slot_hold_ms", held.Milliseconds()))
	// The heartbeat is not waited on: Renew never re-creates a deleted
	// record, so a renewal racing this delete just fails with
	// ErrJobSlotNotHeld and stops.
	if err := h.limiter.repo.Release(ctx, h.index, h.holderID); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "release job slot",
			goerr.V("index", h.index),
			goerr.V("holder_id", h.holderID)), "job: release concurrency slot")
	}
	return held
}

// hashOffset hashes a holder token into a stable slot offset.
func hashOffset(holderID string) uint64 {
	h := fnv.New64a()
	// Hash writes never fail (fnv's Write always returns nil error).
	_, _ = h.Write([]byte(holderID))
	return h.Sum64()
}
