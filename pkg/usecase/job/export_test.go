package job

import (
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
)

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
