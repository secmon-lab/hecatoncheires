package job

import (
	"context"
	"log/slog"
	"time"

	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

// JobInteractorForTest exposes the JobInteractor type for tests.
type JobInteractorForTest = JobInteractor

// NewJobInteractorForTest constructs a JobInteractor for tests.
func NewJobInteractorForTest(
	repo interfaces.Repository,
	poster jobQuestionPoster,
	key model.JobRunKey,
	runID, channelID, threadTS, requesterUserID string,
	runningLog *model.JobRunLog,
	handler *runtrace.Handler,
	now func() time.Time,
) *JobInteractor {
	return newJobInteractor(repo, poster, key, runID, channelID, threadTS, requesterUserID, runningLog, handler, now)
}

// ParseJobQuestionAnswersForTest exposes parseJobQuestionAnswers.
func ParseJobQuestionAnswersForTest(pending *model.PendingInteraction, state *goslack.BlockActionStates) []interaction.Answer {
	return parseJobQuestionAnswers(pending, state)
}

// DecodeJobQuestionRefForTest decodes a Submit-button value and returns the
// resume context fields.
func DecodeJobQuestionRefForTest(value string) (workspaceID string, caseID int64, jobID, runID string, err error) {
	ref, err := decodeJobQuestionRef(value)
	if err != nil {
		return "", 0, "", "", err
	}
	return ref.WorkspaceID, ref.CaseID, ref.JobID, ref.RunID, nil
}

// JobQuestionPosterForTest exposes the narrow poster interface so tests can
// supply a fake.
type JobQuestionPosterForTest = jobQuestionPoster

// JobRunTraceHandlerForTest exposes the shared runtrace.Handler under the
// name the runner tests use to assert on the executor's TraceHandler.
type JobRunTraceHandlerForTest = runtrace.Handler

// TruncateRunesForTest exposes truncateRunes for tests in other packages.
var TruncateRunesForTest = truncateRunes

// SlotHoldForTest exposes the acquired-slot handle type. The concurrency gate
// lives entirely inside this package (JobRunner.Run is its only caller), so
// the type and its methods stay unexported in production code.
type SlotHoldForTest = slotHold

// AcquireSlotForTest exposes ConcurrencyLimiter.acquire.
func AcquireSlotForTest(ctx context.Context, l *ConcurrencyLimiter, key model.JobRunKey) (*SlotHoldForTest, error) {
	h, _, err := l.acquire(ctx, key)
	return h, err
}

// AcquireSlotObservedForTest is AcquireSlotForTest plus the admission
// observation the runner reports on its summary line.
func AcquireSlotObservedForTest(ctx context.Context, l *ConcurrencyLimiter, key model.JobRunKey) (hold *SlotHoldForTest, occupied, limit int, err error) {
	h, obs, err := l.acquire(ctx, key)
	return h, obs.Occupied, obs.Limit, err
}

// ReleaseSlotForTest exposes slotHold.release and its hold duration.
func ReleaseSlotForTest(ctx context.Context, h *SlotHoldForTest) time.Duration {
	return h.release(ctx)
}

// SlotHoldIndexForTest reports which execution slot a hold occupies. Tests
// assert on it to prove distinct runs land on distinct slots; production code
// has no reason to know the index.
func SlotHoldIndexForTest(h *SlotHoldForTest) int {
	if h == nil {
		return -1
	}
	return h.index
}

// WithQuietForTest exposes withQuiet for tests in other packages.
var WithQuietForTest = withQuiet

// IsQuietForTest exposes isQuiet for tests in other packages.
var IsQuietForTest = isQuiet

// ValidateEventForTest exposes Event.validate so the per-domain invariants can
// be table-tested directly instead of through eight Publish scenarios.
var ValidateEventForTest = Event.validate

// TickStatsForTest exposes the per-sweep outcome collector. It is unexported in
// production because only the scanner may create one.
type TickStatsForTest = tickStats

// NewTickStatsForTest constructs a collector anchored at startedAt.
func NewTickStatsForTest(startedAt time.Time) *TickStatsForTest {
	return newTickStats(startedAt)
}

// WithTickStatsForTest attaches a collector to ctx, as the scanner does.
func WithTickStatsForTest(ctx context.Context, s *TickStatsForTest) context.Context {
	return withTickStats(ctx, s)
}

// TickStatsFromForTest reads the collector back out of a context.
func TickStatsFromForTest(ctx context.Context) *TickStatsForTest {
	return tickStatsFrom(ctx)
}

// TickStatsAddDueForTest counts one published (job, case) pair.
func TickStatsAddDueForTest(s *TickStatsForTest) { s.addDue() }

// BeginRunForTest / EndRunForTest bracket a dispatched run, as Publish does.
func (s *tickStats) BeginRunForTest() { s.beginRun() }
func (s *tickStats) EndRunForTest()   { s.endRun() }

// TickStatsWaitRunsForTest waits for the dispatched runs, reporting whether all
// of them were accounted for.
func TickStatsWaitRunsForTest(s *TickStatsForTest, timeout time.Duration) bool {
	return s.waitRuns(timeout)
}

// TickStatsRecordOutcomeForTest reports one finished attempt, as JobRunner does
// via its run summary. runID is what marks the attempt as having started.
func TickStatsRecordOutcomeForTest(s *TickStatsForTest, domain, outcome, runID string, slotLimit int, slotHoldMs int64) {
	s.recordRun(&runSummary{
		domain:     domain,
		outcome:    runOutcome(outcome),
		runID:      runID,
		slotGated:  slotLimit > 0,
		slotLimit:  slotLimit,
		slotHoldMs: slotHoldMs,
	})
}

// TickStatsLogAttrsForTest renders the summary attributes.
func TickStatsLogAttrsForTest(s *TickStatsForTest, now time.Time, settled bool) []slog.Attr {
	return s.logAttrs(now, settled)
}

// Run outcome labels, exposed so tests assert against the same constants the
// production code emits instead of re-typing the strings.
const (
	OutcomeCompletedForTest        = string(outcomeCompleted)
	OutcomeFailedForTest           = string(outcomeFailed)
	OutcomeSuspendedForTest        = string(outcomeSuspended)
	OutcomeSkippedLeaseForTest     = string(outcomeSkippedLease)
	OutcomeSkippedSuspendedForTest = string(outcomeSkippedSuspended)
	OutcomeSkippedSlotsFullForTest = string(outcomeSkippedSlotsFull)
)
