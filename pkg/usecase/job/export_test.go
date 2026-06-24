package job

import (
	"time"

	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
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
	now func() time.Time,
) *JobInteractor {
	return newJobInteractor(repo, poster, key, runID, channelID, threadTS, requesterUserID, runningLog, now)
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

// JobRunRoutingForTest exposes the internal jobRunRouting type so tests
// in other packages can construct handlers with a known routing key.
type JobRunRoutingForTest = jobRunRouting

// JobRunTraceHandlerForTest exposes jobRunTraceHandler.
type JobRunTraceHandlerForTest = jobRunTraceHandler

// RunSequencerForTest exposes runSequencer.
type RunSequencerForTest = *runSequencer

// NewRunSequencerForTest constructs a fresh runSequencer for tests.
func NewRunSequencerForTest() *runSequencer {
	return newRunSequencer()
}

// TruncateRunesForTest exposes truncateRunes for tests in other packages.
var TruncateRunesForTest = truncateRunes

// WithQuietForTest exposes withQuiet for tests in other packages.
var WithQuietForTest = withQuiet

// IsQuietForTest exposes isQuiet for tests in other packages.
var IsQuietForTest = isQuiet

// NewJobRunTraceHandlerForTest constructs a jobRunTraceHandler for tests.
// clock and truncator may be nil for defaults.
func NewJobRunTraceHandlerForTest(
	eventRepo interfaces.JobRunEventRepository,
	routing JobRunRoutingForTest,
	seq *runSequencer,
	clock func() time.Time,
	truncator payloadTruncator,
) *jobRunTraceHandler {
	return newJobRunTraceHandler(eventRepo, routing, seq, clock, truncator)
}

// EnterReflectionPhaseForTest calls enterReflectionPhase so external test
// packages can drive the reflection-phase transition without accessing
// the unexported method directly.
func (h *jobRunTraceHandler) EnterReflectionPhaseForTest() {
	h.enterReflectionPhase()
}
