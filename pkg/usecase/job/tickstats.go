package job

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// runOutcome labels how one JobRunner.Run / Resume attempt ended. Every attempt
// reports exactly one of these on its summary log line, so a tick's dispatched
// events and its executed runs can be reconciled from the outcome field alone
// instead of by differencing two different log messages.
type runOutcome string

const (
	// outcomeCompleted is a run that reached the executor and returned without
	// error.
	outcomeCompleted runOutcome = "completed"
	// outcomeFailed covers every failure, including the prepare-stage ones
	// that never reached the executor and the fail-closed slot read error.
	outcomeFailed runOutcome = "failed"
	// outcomeSuspended is an interactive run that asked the user and paused.
	outcomeSuspended runOutcome = "suspended"
	// outcomeSpawned is a run handed to the durable agent runtime. It is
	// distinct from completed on purpose: the attempt recorded the run and
	// returned, and whether the run itself succeeds is reported later by its own
	// completion handler, on whichever instance drove it. Counting it as
	// completed here would claim an outcome this attempt never observed.
	outcomeSpawned runOutcome = "spawned"
	// outcomeSkippedLease is a trigger that found another runner holding the
	// (workspace, case, job) lease.
	outcomeSkippedLease runOutcome = "skipped_lease"
	// outcomeSkippedSuspended is a trigger that stepped aside for a genuinely
	// open question on the same tuple.
	outcomeSkippedSuspended runOutcome = "skipped_suspended"
	// outcomeSkippedRunning is a trigger that found a durable run of the same
	// tuple still live. It is separate from skipped_lease because the lease no
	// longer spans the run: a durable run is recorded and the lease released
	// immediately, so this is the outcome that keeps a long run from collecting one
	// spurious failed run per tick.
	outcomeSkippedRunning runOutcome = "skipped_running"
	// outcomeSkippedSlotsFull is a scheduled trigger refused by the
	// deployment-wide concurrency gate.
	outcomeSkippedSlotsFull runOutcome = "skipped_slots_full"
	// outcomeSkippedStale is a resume whose run is no longer awaiting input
	// (already answered, completed, or expired). It cannot appear in a sweep
	// summary: a resume is only ever triggered by a question submit, never by
	// a sweep, so logAttrs does not carry a counter for it.
	outcomeSkippedStale runOutcome = "skipped_stale"
)

// tickStatsKey is the private context key under which a sweep stashes its
// tickStats. A private struct type keeps it from colliding with any other
// package's context values.
type tickStatsKey struct{}

// tickStats accumulates one sweep's outcome counts so the sweep can close with
// a single reconcilable summary line.
//
// Multi-instance safety: this is in-memory state confined to ONE continuous
// processing flow — the Scan call plus the run goroutines it dispatched, all on
// this instance — and it is dropped when that flow ends. It is not a
// cross-request registry: nothing looks it up by id, and nothing outside the
// sweep can reach it. Persisting it would mean writing counters on the very
// path whose cost we are trying to measure.
//
// Every method is safe on a nil receiver, because the same code paths run
// outside a sweep (lifecycle events, manual runs, resumes), where
// tickStatsFrom returns nil and the calls must be no-ops.
type tickStats struct {
	// startedAt is the sweep's t0. It anchors both the reported elapsed time
	// and the slot capacity the busy time is measured against.
	startedAt time.Time

	// wg tracks the runs this sweep dispatched but has not seen finish.
	wg sync.WaitGroup

	// mu guards everything below: the dispatched runs report concurrently.
	mu         sync.Mutex
	due        int
	started    int
	outcomes   map[runOutcome]int
	slotBusyMs int64
	slotLimit  int
}

// newTickStats returns a stats collector anchored at the sweep's start time.
func newTickStats(startedAt time.Time) *tickStats {
	return &tickStats{
		startedAt: startedAt,
		outcomes:  make(map[runOutcome]int, 6),
	}
}

// withTickStats attaches the collector to ctx so the dispatched runs can report
// into it. async.Dispatch propagates context values, so the reporting works
// from the run goroutines.
func withTickStats(ctx context.Context, s *tickStats) context.Context {
	return context.WithValue(ctx, tickStatsKey{}, s)
}

// tickStatsFrom returns the sweep's collector, or nil outside a sweep.
func tickStatsFrom(ctx context.Context) *tickStats {
	s, _ := ctx.Value(tickStatsKey{}).(*tickStats)
	return s
}

// addDue counts one (job, case) pair the sweep raised an event for.
func (s *tickStats) addDue() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.due++
	s.mu.Unlock()
}

// beginRun registers a dispatched run the sweep will wait for.
func (s *tickStats) beginRun() {
	if s == nil {
		return
	}
	s.wg.Add(1)
}

// endRun marks a dispatched run as finished.
func (s *tickStats) endRun() {
	if s == nil {
		return
	}
	s.wg.Done()
}

// recordRun folds one finished attempt into the tally.
//
// Only scheduled-domain runs count. A Job agent that mutates its case publishes
// a lifecycle event from inside the run, and that event's run inherits this
// context; counting it would make the outcome totals exceed due_total, which
// only ever counts scheduled dispatches.
func (s *tickStats) recordRun(sum *runSummary) {
	if s == nil || sum == nil {
		return
	}
	if sum.domain != string(model.JobEventDomainScheduled) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes[sum.outcome]++
	// A non-empty RunID means the run log was created, i.e. the attempt got
	// past admission and prepare into the executor.
	if sum.runID != "" {
		s.started++
	}
	s.slotBusyMs += sum.slotHoldMs
	if sum.slotGated {
		s.slotLimit = sum.slotLimit
	}
}

// waitRuns blocks until every dispatched run has finished, or timeout elapses.
// It reports whether every run was accounted for; a false result means the
// summary is a partial count and must say so.
//
// The bare goroutine is deliberate: async.Dispatch is for a request's async
// tail (it adds panic recovery and error reporting, and enrols in the global
// in-flight WaitGroup that async.Wait blocks on). This one only waits on
// another WaitGroup and closes a channel, and enrolling it globally would make
// a hung run block async.Wait as well. It ends when the runs end, timeout or
// not, so it cannot outlive them.
func (s *tickStats) waitRuns(timeout time.Duration) bool {
	if s == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// logAttrs renders the sweep summary. All six outcome counters are emitted even
// when zero so a query never has to distinguish "none" from "field absent".
//
// The slot attributes are emitted only when a run actually observed the gate:
// with the limit disabled there is no capacity to measure, and reporting zeros
// would read as "no slot was ever busy" rather than "no slot exists".
func (s *tickStats) logAttrs(now time.Time, settled bool) []slog.Attr {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsedMs := max(now.Sub(s.startedAt).Milliseconds(), 0)
	attrs := []slog.Attr{
		slog.Int("due_total", s.due),
		slog.Int("started", s.started),
		slog.Int("completed", s.outcomes[outcomeCompleted]),
		slog.Int("failed", s.outcomes[outcomeFailed]),
		slog.Int("suspended", s.outcomes[outcomeSuspended]),
		slog.Int("skipped_slots_full", s.outcomes[outcomeSkippedSlotsFull]),
		slog.Int("skipped_lease", s.outcomes[outcomeSkippedLease]),
		slog.Int("skipped_suspended", s.outcomes[outcomeSkippedSuspended]),
		slog.Int("skipped_running", s.outcomes[outcomeSkippedRunning]),
		slog.Int64("elapsed_ms", elapsedMs),
		slog.Bool("settled", settled),
	}
	if s.slotLimit > 0 {
		capacityMs := int64(s.slotLimit) * elapsedMs
		attrs = append(attrs,
			slog.Int("slot_limit", s.slotLimit),
			slog.Int64("slot_busy_ms", s.slotBusyMs),
			// Idle is an UPPER BOUND on the real figure: slotBusyMs sums only
			// the holds this instance took during this sweep, so a hold left
			// over from an earlier sweep, or one taken by another instance,
			// is not subtracted. Read it together with skipped_slots_full:
			// a large idle next to many refusals is the case where capacity
			// went unused.
			slog.Int64("slot_idle_ms", max(capacityMs-s.slotBusyMs, 0)),
		)
	}
	return attrs
}
