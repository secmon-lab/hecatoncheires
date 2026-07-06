package planexec

import (
	"context"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// Validatable is the constraint on Run's structured final-output type. The
// generic layer decodes the planner's terminal JSON into T, then calls
// Validate() and regenerates on failure: gollem's response-schema check
// verifies the JSON shape only, so Validate() is where a host enforces its
// domain invariants (required fields, allowed values). RunText / ResumeText
// produce plain text and do not use this.
type Validatable interface {
	Validate() error
}

// RunResult is the outcome of a run. T is the structured final-output type for
// Run[T]; RunText / ResumeText instantiate it as string (the reply travels in
// Text and Data is unused). AllResults carries the per-phase observation trail
// regardless of status so a fallback can still report what was learnt.
type RunResult[T any] struct {
	Status RunStatus
	// Data is the validated structured final output. Non-nil ONLY for a Run[T]
	// turn that completed via an explicit finalize (not direct, not question,
	// not fallback). Always nil for RunText / ResumeText.
	Data *T
	// Text is the plain-text terminal reply: the whole output of RunText /
	// ResumeText, OR the round-1 direct fast-path reply on any run
	// (Direct == true). Empty for a structured Run[T] that finalized normally
	// (the payload is in Data instead).
	Text string
	// Direct is true when the turn terminated via the round-1 direct fast path
	// (no investigation phase ran); Text holds the reply and Data is nil.
	Direct bool
	// EndedWithQuestion is true when the loop exited because OnQuestion returned
	// QuestionResult{Terminate: true}. Status is still StatusCompleted; the host
	// uses this to distinguish "we answered" from "we asked".
	EndedWithQuestion bool
	// AllResults is the per-phase observation trail.
	AllResults []PhaseSummary
	// FallbackReason is the human-readable cause for a StatusFallback* result.
	FallbackReason string
}

// Run drives a single structured plan-and-execute turn end-to-end and returns a
// validated *T as the terminal output. The flow is: build the planner prompt →
// loop (plan → sub-agents → replan) until the planner emits an explicit
// finalize → generate the final JSON, decode into T, run T.Validate(), and
// regenerate on failure (bounded). Side effects (case status changes etc.) are
// performed by the sub-agents' tools inside the loop, never by planexec; the
// host applies whatever the returned *T describes.
//
// Run is a package function, not a Runner method, because Go methods cannot be
// generic. The Runner carries the shared backend deps; T is the caller's
// terminal-output type.
func Run[T Validatable](ctx context.Context, r *Runner, req RunRequest) (*RunResult[T], error) {
	if err := req.Validate(); err != nil {
		return nil, goerr.Wrap(err, "validate run request")
	}
	rc, finish, err := r.setup(ctx, req, true)
	if err != nil {
		return nil, err
	}
	defer finish()

	lr, err := r.runLoop(ctx, req, rc, nil, 0, initialPlannerInput(rc.bg, req.UserInput))
	if err != nil {
		return nil, err
	}
	return finalizeStructured[T](ctx, r, req, rc, lr)
}

// RunText drives a plain-text turn: same loop as Run, but the terminal output
// is free-form text (RunResult.Text). Used by hosts whose deliverable is a
// message or a side effect performed by a sub-agent tool (e.g. the Job host),
// not a structured object the host must apply.
func RunText(ctx context.Context, r *Runner, req RunRequest) (*RunResult[string], error) {
	if err := req.Validate(); err != nil {
		return nil, goerr.Wrap(err, "validate run request")
	}
	rc, finish, err := r.setup(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer finish()

	lr, err := r.runLoop(ctx, req, rc, nil, 0, initialPlannerInput(rc.bg, req.UserInput))
	if err != nil {
		return nil, err
	}
	return finalizeText(ctx, r, req, rc, lr)
}

// ResumeText re-enters a suspended plain-text turn after the user answered the
// planner's Question. It mirrors RunText's setup (same system prompt, a fresh
// trace recorder bound to the same TraceID, a fresh budget) but enters the loop
// at a replan round (logicalRound 1) with the answers folded in as the first
// input. The conversation history keyed by HistoryKey already carries the prior
// observations, so the planner re-plans with full context without re-executing
// the completed phases.
func ResumeText(ctx context.Context, r *Runner, req ResumeRequest) (*RunResult[string], error) {
	if err := req.Validate(); err != nil {
		return nil, goerr.Wrap(err, "validate resume request")
	}
	rc, finish, err := r.setup(ctx, req.RunRequest, false)
	if err != nil {
		return nil, err
	}
	defer finish()

	nextInput := formatQuestionAnswers(rc.bg, req.Question, req.Answers)
	lr, err := r.runLoop(ctx, req.RunRequest, rc, nil, 1, nextInput)
	if err != nil {
		return nil, err
	}
	return finalizeText(ctx, r, req.RunRequest, rc, lr)
}

// initialPlannerInput is the first user message fed to the planner: the budget
// prefix line followed by the host's user input.
func initialPlannerInput(bg *budget, userInput string) string {
	return bg.formatPrefix() + "\n\n" + userInput
}

// finalizeStructured maps a loop outcome onto a typed RunResult[T], generating
// (and validating) the structured final output only for the finalize outcome.
func finalizeStructured[T Validatable](ctx context.Context, r *Runner, req RunRequest, rc *runContext, lr *loopResult) (*RunResult[T], error) {
	res := &RunResult[T]{AllResults: lr.allResults}
	switch lr.outcome {
	case loopQuestion:
		res.Status = StatusCompleted
		res.EndedWithQuestion = true
	case loopDirect:
		res.Status = StatusCompleted
		res.Direct = true
		res.Text = lr.directText
	case loopFallbackBudget:
		res.Status = StatusFallbackBudget
		res.FallbackReason = lr.fallbackReason
	case loopFallbackError:
		res.Status = StatusFallbackError
		res.FallbackReason = lr.fallbackReason
	case loopFinalize:
		data, ferr := generateValidatedFinal[T](ctx, r, rc, req.LanguageLabel, req.HistoryKey, lr.allResults)
		if ferr != nil {
			// Surface to errutil so the operator sees the reason even though the
			// host gets a graceful fallback RunResult back.
			errutil.Handle(ctx, ferr, "planexec: final output failed")
			res.Status = StatusFallbackError
			res.FallbackReason = ferr.Error()
			return res, nil
		}
		res.Status = StatusCompleted
		res.Data = data
	default:
		return nil, goerr.New("planexec: unknown loop outcome", goerr.V("outcome", int(lr.outcome)))
	}
	return res, nil
}

// finalizeText maps a loop outcome onto a plain-text RunResult, generating the
// free-form final response only for the finalize outcome.
func finalizeText(ctx context.Context, r *Runner, req RunRequest, rc *runContext, lr *loopResult) (*RunResult[string], error) {
	res := &RunResult[string]{AllResults: lr.allResults}
	switch lr.outcome {
	case loopQuestion:
		res.Status = StatusCompleted
		res.EndedWithQuestion = true
	case loopDirect:
		res.Status = StatusCompleted
		res.Direct = true
		res.Text = lr.directText
	case loopFallbackBudget:
		res.Status = StatusFallbackBudget
		res.FallbackReason = lr.fallbackReason
	case loopFallbackError:
		res.Status = StatusFallbackError
		res.FallbackReason = lr.fallbackReason
	case loopFinalize:
		text, _, ferr := generateFinalResponse(ctx, r.llm, r.historyRepo, rc.traced, rc.systemPrompt, req.HistoryKey, req.LanguageLabel, lr.allResults, nil)
		if ferr != nil {
			errutil.Handle(ctx, ferr, "planexec: final response failed")
			res.Status = StatusFallbackError
			res.FallbackReason = ferr.Error()
			return res, nil
		}
		res.Status = StatusCompleted
		res.Text = text
	default:
		return nil, goerr.New("planexec: unknown loop outcome", goerr.V("outcome", int(lr.outcome)))
	}
	return res, nil
}
