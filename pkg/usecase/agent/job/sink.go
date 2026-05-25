package job

import (
	"context"
	"fmt"
	"strings"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// jobSink adapts the planexec.Sink interface to Job's transport-side
// surface. Jobs run unattended, so we never speak to Slack here; we just
// forward every progress event to the ExecuteRequest.ProgressFunc (which
// the job runner has already piped into JobRunEvent persistence) so the
// user can watch the run from the Cases UI in real time.
//
// PlanProposed and PhaseStarted produce one Notify line each so they
// show up alongside individual TaskProgress entries in the same event
// trail — keeping the operator-facing view sequential and skim-friendly.
type jobSink struct {
	progress tool.UpdateFunc
}

// newJobSink builds a jobSink. progress may be nil; every Sink method
// becomes a no-op in that case so the caller does not need to guard.
func newJobSink(progress tool.UpdateFunc) *jobSink {
	return &jobSink{progress: progress}
}

func (s *jobSink) emit(ctx context.Context, line string) {
	if s == nil || s.progress == nil || line == "" {
		return
	}
	s.progress(ctx, line)
}

// Notify forwards free-form lines.
func (s *jobSink) Notify(ctx context.Context, line string) {
	s.emit(ctx, line)
}

// PlanProposed surfaces the planner's per-round reasoning as a single
// line so the operator can follow the agent's reasoning trail.
func (s *jobSink) PlanProposed(ctx context.Context, info planexec.PlanInfo) {
	verb := "Plan"
	if info.IsReplan {
		verb = "Replan"
	}
	if info.Reasoning == "" {
		s.emit(ctx, fmt.Sprintf("%s round %d", verb, info.Round))
		return
	}
	s.emit(ctx, fmt.Sprintf("%s round %d: %s", verb, info.Round, info.Reasoning))
}

// PhaseStarted lists the tasks about to run in parallel. The line is
// short on purpose — detailed progress comes through TaskProgress.
func (s *jobSink) PhaseStarted(ctx context.Context, phase int, tasks []planexec.TaskInfo) {
	if len(tasks) == 0 {
		return
	}
	if len(tasks) == 1 {
		s.emit(ctx, fmt.Sprintf("Phase %d: %s", phase, tasks[0].Title))
		return
	}
	titles := make([]string, 0, len(tasks))
	for _, t := range tasks {
		titles = append(titles, t.Title)
	}
	s.emit(ctx, fmt.Sprintf("Phase %d (%d tasks): %s", phase, len(tasks), joinTitles(titles)))
}

// TaskProgress forwards a per-task line.
func (s *jobSink) TaskProgress(ctx context.Context, taskID, line string) {
	if line == "" {
		return
	}
	s.emit(ctx, fmt.Sprintf("[%s] %s", taskID, line))
}

// TaskFinished marks the terminal state of one task. The summary itself
// is not inlined here (it would dominate the event trail); the operator
// can read it in the JobRun details view once the loop completes.
func (s *jobSink) TaskFinished(ctx context.Context, result planexec.TaskResult) {
	switch result.Status {
	case planexec.TaskStatusCompleted:
		s.emit(ctx, fmt.Sprintf("[%s] done", result.TaskID))
	case planexec.TaskStatusFailed:
		s.emit(ctx, fmt.Sprintf("[%s] failed: %s", result.TaskID, result.Error))
	default:
		s.emit(ctx, fmt.Sprintf("[%s] status=%s", result.TaskID, result.Status))
	}
}

// joinTitles is a small helper to format task title lists in a single
// line. Delegates to strings.Join — kept as a named helper for grep-
// friendliness and to keep the Sink calls self-explanatory.
func joinTitles(titles []string) string {
	return strings.Join(titles, ", ")
}

// Compile-time assertion: jobSink satisfies planexec.Sink.
var _ planexec.Sink = (*jobSink)(nil)
