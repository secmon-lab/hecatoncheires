package job_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

// attrValues flattens the summary attributes into a lookup so a test names the
// field it asserts on rather than an index.
func attrValues(attrs []slog.Attr) map[string]slog.Value {
	out := make(map[string]slog.Value, len(attrs))
	for _, a := range attrs {
		out[a.Key] = a.Value
	}
	return out
}

func TestTickStats_CountsEveryOutcome(t *testing.T) {
	start := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	stats := job.NewTickStatsForTest(start)

	for range 4 {
		job.TickStatsAddDueForTest(stats)
	}
	scheduled := string(model.JobEventDomainScheduled)
	job.TickStatsRecordOutcomeForTest(stats, scheduled, job.OutcomeCompletedForTest, "run-1", 2, 5_000)
	job.TickStatsRecordOutcomeForTest(stats, scheduled, job.OutcomeFailedForTest, "run-2", 2, 1_000)
	job.TickStatsRecordOutcomeForTest(stats, scheduled, job.OutcomeSkippedSlotsFullForTest, "", 2, 0)
	job.TickStatsRecordOutcomeForTest(stats, scheduled, job.OutcomeSkippedLeaseForTest, "", 2, 0)
	job.TickStatsRecordOutcomeForTest(stats, scheduled, job.OutcomeSkippedRunningForTest, "", 2, 0)

	// 10s of wall clock against 2 slots = 20s of capacity; 6s of it was held.
	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, start.Add(10*time.Second), true))

	gt.Value(t, attrs["due_total"].Int64()).Equal(int64(4))
	// started counts the attempts that reached the executor, i.e. those with a
	// run id — the two skips must not inflate it.
	gt.Value(t, attrs["started"].Int64()).Equal(int64(2))
	gt.Value(t, attrs["completed"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["failed"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["suspended"].Int64()).Equal(int64(0))
	gt.Value(t, attrs["skipped_slots_full"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["skipped_lease"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["skipped_suspended"].Int64()).Equal(int64(0))
	gt.Value(t, attrs["elapsed_ms"].Int64()).Equal(int64(10_000))
	gt.Value(t, attrs["settled"].Bool()).Equal(true)

	gt.Value(t, attrs["slot_limit"].Int64()).Equal(int64(2))
	gt.Value(t, attrs["slot_busy_ms"].Int64()).Equal(int64(6_000))
	gt.Value(t, attrs["slot_idle_ms"].Int64()).Equal(int64(14_000))
}

// A lifecycle event published from inside a Job run inherits the sweep's
// context. Counting it would make the outcomes exceed due_total, which only
// counts scheduled dispatches.
func TestTickStats_IgnoresNonScheduledRuns(t *testing.T) {
	start := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	stats := job.NewTickStatsForTest(start)

	job.TickStatsAddDueForTest(stats)
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainScheduled), job.OutcomeCompletedForTest, "run-1", 0, 0)
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainCase), job.OutcomeCompletedForTest, "run-2", 0, 0)
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainManual), job.OutcomeFailedForTest, "run-3", 0, 0)

	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, start.Add(time.Second), true))
	gt.Value(t, attrs["due_total"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["started"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["completed"].Int64()).Equal(int64(1))
	gt.Value(t, attrs["failed"].Int64()).Equal(int64(0))
}

// With the gate disabled there is no capacity to measure, so reporting zeros
// would read as "no slot was busy" rather than "no slot exists".
func TestTickStats_OmitsSlotAttrsWhenUngated(t *testing.T) {
	start := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	stats := job.NewTickStatsForTest(start)
	job.TickStatsAddDueForTest(stats)
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainScheduled), job.OutcomeCompletedForTest, "run-1", 0, 0)

	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, start.Add(time.Second), true))
	gt.Map(t, attrs).HasKey("due_total")
	for _, key := range []string{"slot_limit", "slot_busy_ms", "slot_idle_ms"} {
		if _, ok := attrs[key]; ok {
			gt.Bool(t, ok).False() // slot attributes must be absent, not zero
		}
	}
}

// A clock that goes backwards must not produce a negative elapsed time, and the
// idle figure must not go below zero when the recorded holds exceed capacity.
func TestTickStats_ClampsNegativeDurations(t *testing.T) {
	start := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	stats := job.NewTickStatsForTest(start)
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainScheduled), job.OutcomeCompletedForTest, "run-1", 1, 90_000)

	attrs := attrValues(job.TickStatsLogAttrsForTest(stats, start.Add(-time.Minute), true))
	gt.Value(t, attrs["elapsed_ms"].Int64()).Equal(int64(0))
	gt.Value(t, attrs["slot_busy_ms"].Int64()).Equal(int64(90_000))
	gt.Value(t, attrs["slot_idle_ms"].Int64()).Equal(int64(0))
}

func TestTickStats_WaitRunsReportsTimeout(t *testing.T) {
	stats := job.NewTickStatsForTest(time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC))

	// Nothing dispatched: settled immediately.
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, time.Second)).True()

	// One run still in flight: the wait gives up and says so.
	release := make(chan struct{})
	done := make(chan struct{})
	stats.BeginRunForTest()
	go func() {
		<-release
		stats.EndRunForTest()
		close(done)
	}()
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, 20*time.Millisecond)).False()

	close(release)
	<-done
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, time.Second)).True()
}

// The same code paths run outside a sweep — lifecycle events, manual runs,
// resumes — where the collector is absent and every call must be inert.
func TestTickStats_NilReceiverIsInert(t *testing.T) {
	gt.Value(t, job.TickStatsFromForTest(context.Background())).Nil()

	var stats *job.TickStatsForTest
	job.TickStatsAddDueForTest(stats)
	stats.BeginRunForTest()
	stats.EndRunForTest()
	job.TickStatsRecordOutcomeForTest(stats, string(model.JobEventDomainScheduled), job.OutcomeCompletedForTest, "run-1", 1, 1)
	gt.Bool(t, job.TickStatsWaitRunsForTest(stats, time.Second)).True()
	gt.Array(t, job.TickStatsLogAttrsForTest(stats, time.Now(), true)).Length(0)
}

func TestTickStats_ContextRoundTrip(t *testing.T) {
	stats := job.NewTickStatsForTest(time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC))
	ctx := job.WithTickStatsForTest(context.Background(), stats)
	gt.Value(t, job.TickStatsFromForTest(ctx)).Equal(stats)
}
