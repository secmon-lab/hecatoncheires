package job

import (
	"context"
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
	return l.acquire(ctx, key)
}

// ReleaseSlotForTest exposes slotHold.release.
func ReleaseSlotForTest(ctx context.Context, h *SlotHoldForTest) {
	h.release(ctx)
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
