package job

import (
	"context"
	"strconv"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem/trace"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// PlanexecJobExecutor drives a Job through the shared planexec
// (plan-and-execute) runtime. Selected when model.Job.Strategy ==
// JobStrategyPlanexec. Jobs run unattended, so the executor:
//   - disables the Question section in the planner prompt
//     (RunRequest.AllowQuestion = false),
//   - asks for plain-text final output (RunRequest.FinalOutputSchema =
//     nil) and surfaces it as ExecuteResult.Summary, and
//   - exposes every ToolBuilder-produced tool to every sub-agent (one
//     bucket named "default" — TaskPlan-level tool sub-selection is a
//     proposal concern, not a Job concern).
type PlanexecJobExecutor struct {
	runner *planexec.Runner
}

// NewPlanexecJobExecutor wraps a constructed planexec.Runner. Returns
// an error if the runner is nil so wiring failures surface at startup.
func NewPlanexecJobExecutor(runner *planexec.Runner) (*PlanexecJobExecutor, error) {
	if runner == nil {
		return nil, goerr.New("planexec runner is required")
	}
	return &PlanexecJobExecutor{runner: runner}, nil
}

// Execute satisfies the JobExecutor interface. It translates
// ExecuteRequest into planexec.RunRequest, runs the loop, and maps the
// planexec.RunResult back into the Job-level ExecuteResult shape.
func (e *PlanexecJobExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error) {
	if e == nil || e.runner == nil {
		return nil, goerr.New("planexec job executor not initialized")
	}
	if req.SystemPrompt == "" {
		return nil, goerr.New("system prompt is required",
			goerr.V("job_id", req.JobID))
	}
	if req.Prompt == "" {
		return nil, goerr.New("user prompt is required",
			goerr.V("job_id", req.JobID))
	}
	if req.TraceID == "" {
		return nil, goerr.New("trace id is required for planexec executor",
			goerr.V("job_id", req.JobID))
	}
	if req.HistoryKey == "" {
		return nil, goerr.New("history key is required for planexec executor",
			goerr.V("job_id", req.JobID))
	}

	// Wire progress callback through the tool context so individual
	// tool calls (executed by sub-agents) can surface back to the
	// caller via tool.Update — matches the single-loop executor's
	// behaviour for parity.
	if req.ProgressFunc != nil {
		ctx = tool.WithUpdate(ctx, req.ProgressFunc)
	}

	resolver := newJobToolResolver(req.Tools)
	sink := newJobSink(req.ProgressFunc)

	startedAt := time.Now().UTC()
	result, err := e.runner.Run(ctx, planexec.RunRequest{
		HistoryKey:    req.HistoryKey,
		TraceID:       req.TraceID,
		TraceMetadata: planexecTraceMetadata(req),
		LanguageLabel: req.Language,
		UserInput:     req.Prompt,
		SystemPrompt:  req.SystemPrompt,
		ToolResolver:  resolver,
		KnownToolIDs:  resolver.KnownIDs(),
		AllowQuestion: false, // Jobs run unattended.
		// FinalOutputSchema is nil → plain-text summary surfaces as
		// RunResult.FinalText, which we copy into ExecuteResult.Summary.
		Sink: sink,
	})
	endedAt := time.Now().UTC()
	if err != nil {
		return nil, goerr.Wrap(err, "planexec run",
			goerr.V("job_id", req.JobID),
			goerr.V("trace_id", req.TraceID))
	}

	phases := phaseTracesFromPlanexec(result, startedAt, endedAt)

	switch result.Status {
	case planexec.StatusCompleted:
		return &ExecuteResult{
			Status:    ExecuteStatusSuccess,
			Summary:   result.FinalText,
			LoopCount: len(result.AllResults),
			Phases:    phases,
		}, nil
	case planexec.StatusFallbackBudget, planexec.StatusFallbackError:
		// A fallback path is reported as a failed run with the cause
		// inlined into the summary so the operator sees it in the
		// JobRunLog row without having to drill into events.
		return nil, goerr.New("planexec job did not complete",
			goerr.V("job_id", req.JobID),
			goerr.V("status", result.Status),
			goerr.V("reason", result.FallbackReason))
	default:
		return nil, goerr.New("planexec returned unknown status",
			goerr.V("job_id", req.JobID),
			goerr.V("status", result.Status))
	}
}

// phaseTracesFromPlanexec maps planexec.PhaseSummary entries into
// job.PhaseTrace entries for the JobRunLog. The wall-clock window for
// each sub-phase is not tracked at the planexec layer (it spans a fan-
// out + replan cycle), so we collapse all phases to the executor's
// overall startedAt / endedAt for now. ToolCalls is filled in from the
// number of tasks that actually ran — a coarse proxy.
func phaseTracesFromPlanexec(result *planexec.RunResult, startedAt, endedAt time.Time) []PhaseTrace {
	if result == nil || len(result.AllResults) == 0 {
		return []PhaseTrace{{
			Name:      "planexec",
			StartedAt: startedAt,
			EndedAt:   endedAt,
		}}
	}
	traces := make([]PhaseTrace, 0, len(result.AllResults))
	for _, ps := range result.AllResults {
		traces = append(traces, PhaseTrace{
			Name:      "phase-" + strconv.Itoa(ps.Phase),
			StartedAt: startedAt,
			EndedAt:   endedAt,
			ToolCalls: len(ps.Results),
		})
	}
	return traces
}

// planexecTraceMetadata builds the trace.Recorder labels. JobRunner has
// already created its own trace handler (req.TraceHandler) to capture
// fine-grained gollem events; planexec's recorder runs in parallel
// against the trace.Repository so callers can replay the planner
// reasoning later. The session_id label is required by
// agentarchive.MemoryTraceRepository — we use the RunID-shaped TraceID
// so memory and Firestore behave identically.
func planexecTraceMetadata(req ExecuteRequest) trace.TraceMetadata {
	labels := map[string]string{
		"session_id": req.TraceID,
		"job_id":     req.JobID,
	}
	if req.HistoryKey != "" {
		labels["history_key"] = req.HistoryKey
	}
	return trace.TraceMetadata{Labels: labels}
}
