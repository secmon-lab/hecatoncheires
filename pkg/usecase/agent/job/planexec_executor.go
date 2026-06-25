package job

import (
	"context"
	"strconv"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/interaction"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
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

// PlanexecJobExecutor is the only executor that supports resuming a
// suspended interactive run.
var _ ResumableJobExecutor = (*PlanexecJobExecutor)(nil)

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
	if req.Interactive && req.Interactor == nil {
		return nil, goerr.New("interactor is required when interactive",
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
		// Forward the JobRunner's per-event trace handler so the planexec
		// runtime records LLM / tool events into the JobRunEvent timeline.
		// Without this, planexec Jobs show an empty event timeline even on
		// success (the run's own archive recorder writes to a different
		// repository than the UI reads).
		TraceHandler:  req.TraceHandler,
		LanguageLabel: req.Language,
		UserInput:     req.Prompt,
		SystemPrompt:  req.SystemPrompt,
		ToolResolver:  resolver,
		KnownToolIDs:  resolver.KnownIDs(),
		// Interactive jobs may ask the user mid-run; non-interactive jobs
		// run fully unattended (the historical default).
		AllowQuestion: req.Interactive,
		OnQuestion:    buildOnQuestion(req),
		// AllowDirect lets a trivially-answerable job skip the investigation
		// machinery and reply in a single tool-enabled pass. Jobs surface a
		// plain-text summary (FinalOutputSchema nil), which is exactly what
		// the direct path produces, so no terminal-shape change is needed.
		AllowDirect: true,
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

	return e.mapResult(result, startedAt, endedAt, req)
}

// Resume re-enters a suspended interactive run after the user answered. It
// reconstructs the planexec.Question from the persisted pending interaction,
// converts the host-neutral answers, and drives planexec.Resume (which
// re-enters at a replan round with the same HistoryKey). The OnQuestion
// callback is wired again so the resumed turn can itself ask a follow-up.
func (e *PlanexecJobExecutor) Resume(ctx context.Context, req ExecuteRequest, pending model.PendingInteraction, answers []interaction.Answer) (*ExecuteResult, error) {
	if e == nil || e.runner == nil {
		return nil, goerr.New("planexec job executor not initialized")
	}
	if req.HistoryKey == "" {
		return nil, goerr.New("history key is required for planexec resume",
			goerr.V("job_id", req.JobID))
	}
	if req.Interactor == nil {
		return nil, goerr.New("interactor is required for resume",
			goerr.V("job_id", req.JobID))
	}
	if len(answers) == 0 {
		return nil, goerr.New("resume requires at least one answer",
			goerr.V("job_id", req.JobID))
	}

	if req.ProgressFunc != nil {
		ctx = tool.WithUpdate(ctx, req.ProgressFunc)
	}

	resolver := newJobToolResolver(req.Tools)
	sink := newJobSink(req.ProgressFunc)
	// Resume always re-enables questions so a follow-up question can be
	// asked; mark the request interactive so buildOnQuestion wires the
	// callback regardless of the original flag.
	req.Interactive = true

	startedAt := time.Now().UTC()
	result, err := e.runner.Resume(ctx, planexec.ResumeRequest{
		RunRequest: planexec.RunRequest{
			HistoryKey:    req.HistoryKey,
			TraceID:       req.TraceID,
			TraceMetadata: planexecTraceMetadata(req),
			LanguageLabel: req.Language,
			UserInput:     req.Prompt,
			SystemPrompt:  req.SystemPrompt,
			ToolResolver:  resolver,
			KnownToolIDs:  resolver.KnownIDs(),
			AllowQuestion: true,
			OnQuestion:    buildOnQuestion(req),
			AllowDirect:   true,
			Sink:          sink,
		},
		Question: pendingToQuestion(pending),
		Answers:  answersToQuestionAnswers(answers),
	})
	endedAt := time.Now().UTC()
	if err != nil {
		return nil, goerr.Wrap(err, "planexec resume",
			goerr.V("job_id", req.JobID),
			goerr.V("trace_id", req.TraceID))
	}

	return e.mapResult(result, startedAt, endedAt, req)
}

// buildOnQuestion returns the planexec OnQuestion callback for this run, or
// nil when the run is not interactive. The callback adapts the planner's
// planexec.Question into a host-neutral interaction.Request, hands it to the
// Interactor (which suspends the run and posts the Slack form), and maps the
// Paused outcome back to a turn-terminating QuestionResult so planexec exits
// with EndedWithQuestion. The Job host only supports the pause/resume model,
// never a synchronous in-loop answer, so the callback always terminates.
func buildOnQuestion(req ExecuteRequest) func(context.Context, planexec.Question) (planexec.QuestionResult, error) {
	if !req.Interactive || req.Interactor == nil {
		return nil
	}
	return func(ctx context.Context, q planexec.Question) (planexec.QuestionResult, error) {
		ir := questionToInteractionRequest(q)
		if _, err := req.Interactor.Solicit(ctx, ir); err != nil {
			return planexec.QuestionResult{}, goerr.Wrap(err, "solicit user interaction",
				goerr.V("job_id", req.JobID))
		}
		// Solicit suspended the run (persisted the pending interaction,
		// posted the form). Terminate the planexec turn; resume happens
		// out-of-band when the user answers.
		return planexec.QuestionResult{Terminate: true}, nil
	}
}

// mapResult translates a planexec.RunResult into the Job-level
// ExecuteResult shape, distinguishing the suspended-for-input outcome from a
// normal completion.
func (e *PlanexecJobExecutor) mapResult(result *planexec.RunResult, startedAt, endedAt time.Time, req ExecuteRequest) (*ExecuteResult, error) {
	phases := phaseTracesFromPlanexec(result, startedAt, endedAt)

	switch result.Status {
	case planexec.StatusCompleted:
		if result.EndedWithQuestion {
			// The run suspended to ask the user; the Interactor already
			// persisted state and posted the form. Tell the runner to leave
			// the run at AWAITING_INPUT rather than finishing it.
			return &ExecuteResult{
				Status:    ExecuteStatusAwaitingInput,
				LoopCount: len(result.AllResults),
				Phases:    phases,
			}, nil
		}
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

// questionToInteractionRequest converts a planexec.Question (runtime) into a
// host-neutral interaction.Request (port).
func questionToInteractionRequest(q planexec.Question) interaction.Request {
	items := make([]interaction.Item, len(q.Items))
	for i, it := range q.Items {
		items[i] = interaction.Item{
			ID:      it.ID,
			Text:    it.Text,
			Type:    interaction.ItemType(it.Type),
			Options: it.Options,
		}
	}
	return interaction.Request{Reason: q.Reason, Items: items}
}

// pendingToQuestion reconstructs a planexec.Question from the persisted
// model.PendingInteraction so the resumed planner input can label the
// answers against the original prompts.
func pendingToQuestion(p model.PendingInteraction) planexec.Question {
	items := make([]planexec.QuestionItem, len(p.Items))
	for i, it := range p.Items {
		items[i] = planexec.QuestionItem{
			ID:      it.ID,
			Text:    it.Text,
			Type:    planexec.QuestionItemType(it.Type),
			Options: it.Options,
		}
	}
	return planexec.Question{Reason: p.Reason, Items: items}
}

// answersToQuestionAnswers converts host-neutral interaction.Answer values
// into planexec.QuestionAnswer values for the resume input.
func answersToQuestionAnswers(answers []interaction.Answer) []planexec.QuestionAnswer {
	out := make([]planexec.QuestionAnswer, len(answers))
	for i, a := range answers {
		out[i] = planexec.QuestionAnswer{
			ID:       a.ID,
			Choice:   a.Choice,
			Choices:  a.Choices,
			FreeText: a.FreeText,
		}
	}
	return out
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

// planexecTraceMetadata builds the trace.Recorder labels. planexec's
// recorder runs against the trace.Repository so callers can replay the
// planner reasoning later; it is wired alongside JobRunner's per-event
// handler (forwarded via RunRequest.TraceHandler), which is what
// populates the JobRunEvent timeline. The session_id label is required by
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
