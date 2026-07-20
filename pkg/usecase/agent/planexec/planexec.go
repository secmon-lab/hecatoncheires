package planexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// plannerPerCallLoopLimit is the gollem-side loop bound per planner
// Execute. The planner is allowed PlannerTools (host-supplied), so a
// single round legitimately consumes a few iterations (tool call →
// tool response → final JSON). Set generously so a planner that wants
// to call several tools before emitting the JSON still has room.
const plannerPerCallLoopLimit = 8

// Runner is the reusable plan-and-execute engine. One Runner is wired up
// at process startup with the shared backend deps (LLM client, history
// repo, trace repo) and the global budget defaults; per-call inputs
// (prompts, sink, tool resolver, schema) come in via the RunRequest handed
// to the Run / RunText / ResumeText package functions. Runner is safe for
// concurrent use across goroutines — it holds no per-call state internally.
type Runner struct {
	llm         gollem.LLMClient
	historyRepo gollem.HistoryRepository
	traceRepo   trace.Repository
	budget      BudgetConfig
}

// RunnerDeps is the constructor-time bundle. Every field is required.
type RunnerDeps struct {
	LLMClient   gollem.LLMClient
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository
	Budget      BudgetConfig
}

// Validate enforces RunnerDeps' required-field contract. Surfaced as a
// method (not just baked into NewRunner) so the CLI wiring can sanity-
// check its config independent of constructor invocation.
func (d *RunnerDeps) Validate() error {
	if d == nil {
		return goerr.New("runner deps is nil")
	}
	if d.LLMClient == nil {
		return goerr.New("llm client is required")
	}
	if d.HistoryRepo == nil {
		return goerr.New("history repo is required")
	}
	if d.TraceRepo == nil {
		return goerr.New("trace repo is required")
	}
	if err := d.Budget.Validate(); err != nil {
		return goerr.Wrap(err, "budget config invalid")
	}
	return nil
}

// NewRunner constructs a Runner. Returns a goerr if RunnerDeps is
// incomplete so wiring failures surface at startup rather than at the
// first Slack mention.
func NewRunner(deps RunnerDeps) (*Runner, error) {
	if err := deps.Validate(); err != nil {
		return nil, goerr.Wrap(err, "validate runner deps")
	}
	return &Runner{
		llm:         deps.LLMClient,
		historyRepo: deps.HistoryRepo,
		traceRepo:   deps.TraceRepo,
		budget:      deps.Budget,
	}, nil
}

// runContext bundles the per-turn setup shared by every entry point: the
// rendered planner prompt, the trace recorder plus the combined trace handler
// (archive recorder + host handler), and the fresh budget. The returned finish
// func flushes the recorder and MUST be deferred by the entry point AFTER final
// output generation, so a structured final LLM call is still captured in the
// trace (context.WithoutCancel keeps the flush alive when the caller cancelled).
type runContext struct {
	systemPrompt string
	recorder     *trace.Recorder
	traced       trace.Handler
	bg           *budget
}

func (r *Runner) setup(ctx context.Context, req RunRequest, structuredFinal bool) (*runContext, func(), error) {
	systemPrompt, err := buildPlannerSystemPrompt(req, structuredFinal)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "render planner system prompt")
	}

	recorder := trace.New(
		trace.WithRepository(r.traceRepo),
		trace.WithTraceID(req.TraceID),
		trace.WithMetadata(req.TraceMetadata),
	)
	// context.WithoutCancel detaches the cleanup from the caller's
	// cancellation tree so the final trace flush still runs (and can reach
	// Firestore) when the host already cancelled ctx — e.g. a heartbeat-driven
	// turn-lock loss or a parent request timeout. Using ctx directly here means
	// the persisted-trace I/O fails silently at exactly the moment the trace is
	// most valuable for debugging.
	finish := func() {
		cleanupCtx := context.WithoutCancel(ctx)
		if err := recorder.Finish(cleanupCtx); err != nil {
			errutil.Handle(cleanupCtx, err, "planexec: persist agent trace")
		}
	}

	// traced combines the run's internal archive recorder with the host's
	// optional per-event handler (req.TraceHandler). Sub-agents combine
	// req.TraceHandler with their own per-task LLM-call counter instead (see
	// runPhase), so the archive recorder stays scoped to the planner / direct /
	// final agents exactly as before. When req.TraceHandler is nil (the proposal
	// host), combineTrace returns recorder unchanged.
	return &runContext{
		systemPrompt: systemPrompt,
		recorder:     recorder,
		traced:       combineTrace(recorder, req.TraceHandler),
		bg:           newBudget(r.budget),
	}, finish, nil
}

// loopOutcome classifies how the plan/replan loop terminated. Final output
// generation (structured / text) happens in the generic entry point AFTER the
// loop, keyed on this outcome, so planexec itself performs no side effects.
type loopOutcome int

const (
	// loopFinalize: the planner explicitly declared completion (Finalize). The
	// entry point generates the user-visible terminal output.
	loopFinalize loopOutcome = iota
	// loopQuestion: OnQuestion returned QuestionResult{Terminate:true}; the turn
	// ended waiting for the user. No final output is generated.
	loopQuestion
	// loopDirect: the round-1 direct fast path produced a plain-text reply
	// (directText). No investigation phase ran and no final output is generated.
	loopDirect
	// loopFallbackBudget: the planner budget was exhausted before the loop could
	// terminate. fallbackReason carries the cause.
	loopFallbackBudget
	// loopFallbackError: an internal path gave up before terminating (e.g. the
	// direct reply failed). fallbackReason carries the cause.
	loopFallbackError
)

// loopResult is the loop's terminal disposition. The entry point turns it into
// the typed RunResult, generating the final output only for loopFinalize.
type loopResult struct {
	outcome        loopOutcome
	allResults     []PhaseSummary
	directText     string
	fallbackReason string
}

// runLoop drives the plan/replan loop and returns the terminal disposition
// WITHOUT generating the user-visible final output — that is the entry point's
// job (Run[T] / RunText). Run enters it at logicalRound 0 (fresh plan with the
// job prompt as input); ResumeText enters at logicalRound 1 (a replan round)
// with the user's answers as the first input and a fresh budget. No plan
// snapshot is threaded in on resume — the conversation history (shared
// HistoryKey) already carries the prior rounds' observations, so the planner
// sees them on the next Load.
//
// A non-nil error is a hard failure (planner execute error, empty response,
// OnQuestion callback error) that the caller propagates; the graceful fallback
// dispositions travel in loopResult instead.
func (r *Runner) runLoop(
	ctx context.Context,
	req RunRequest,
	rc *runContext,
	allResults []PhaseSummary,
	logicalRound int,
	nextInput string,
) (*loopResult, error) {
	logger := logging.From(ctx)

	// lastCause holds the most recent validation / generation failure so a
	// budget-exhausted fallback can name the real reason instead of a generic
	// "budget exhausted" (see budgetFallbackReason).
	var lastCause error

	for {
		if !rc.bg.canPlannerCall() {
			return &loopResult{
				outcome:        loopFallbackBudget,
				allResults:     allResults,
				fallbackReason: budgetFallbackReason(lastCause),
			}, nil
		}
		rc.bg.plannerUsed++

		isFirstRound := logicalRound == 0
		var schema *gollem.Parameter
		if isFirstRound {
			schema = planSchema(schemaOptions{
				knownToolIDs:  req.KnownToolIDs,
				allowQuestion: req.AllowQuestion,
				allowDirect:   req.AllowDirect,
			})
		} else {
			schema = replanSchema(schemaOptions{
				knownToolIDs:  req.KnownToolIDs,
				allowQuestion: req.AllowQuestion,
			})
		}

		plannerAgent := gollem.New(r.llm,
			gollem.WithSystemPrompt(rc.systemPrompt),
			gollem.WithTools(req.PlannerTools...),
			gollem.WithHistoryRepository(r.historyRepo, req.HistoryKey),
			gollem.WithTrace(rc.traced),
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(schema),
			gollem.WithLoopLimit(plannerPerCallLoopLimit),
			gollem.WithPromptCache(true),
		)

		resp, execErr := plannerAgent.Execute(ctx, gollem.Text(nextInput))
		if execErr != nil {
			return nil, goerr.Wrap(execErr, "planner execute",
				goerr.V("trace_id", req.TraceID),
				goerr.V("planner_used", rc.bg.plannerUsed))
		}
		if resp == nil || resp.IsEmpty() {
			return nil, goerr.New("planner returned empty response",
				goerr.V("trace_id", req.TraceID),
				goerr.V("planner_used", rc.bg.plannerUsed))
		}

		raw := []byte(resp.Texts[0])
		logger.Debug("planexec planner round",
			"trace_id", req.TraceID,
			"round", rc.bg.plannerUsed,
			"raw_len", len(raw),
		)

		if isFirstRound {
			p, perr := parsePlanResult(raw, req.KnownToolIDs, req.AllowDirect)
			if perr != nil {
				// Retry within the same logical round; the cost is charged via
				// the next loop iteration's bg.plannerUsed++.
				lastCause = perr
				errutil.Handle(ctx, goerr.Wrap(perr, "planner output failed validation; retrying",
					goerr.T(errutil.TagBenign),
				), "planner output failed validation; retrying")
				nextInput = formatRetryInput(rc.bg, perr)
				continue
			}
			// First valid plan accepted — promote to logicalRound 1.
			logicalRound = 1

			// Direct fast path: the planner judged the request trivial enough to
			// answer without any investigation. Skip the plan/execute/replan
			// machinery and produce a plain-text reply in a single tool-enabled
			// ReAct loop. Structured-final generation is not consulted here —
			// direct mode is reserved for replies that need no terminal action.
			//
			// The direct agent gets req.SystemPrompt (the host's base persona),
			// NOT the rendered planner prompt: the latter carries the planner
			// protocol's "respond with a single JSON object, no prose" output
			// rules, which directly contradict the plain-text reply the direct
			// user prompt asks for and would push the model toward malformed JSON.
			if p.Direct != nil {
				req.Sink.PlanProposed(ctx, PlanInfo{Round: logicalRound, Reasoning: p.Message, IsReplan: false, Direct: true})
				tools := req.ToolResolver.Resolve(p.Direct.Tools)
				text, derr := generateDirectResponse(
					ctx,
					r.llm,
					r.historyRepo,
					rc.traced,
					req.SystemPrompt,
					req.HistoryKey,
					req.LanguageLabel,
					req.UserInput,
					tools,
					r.budget.SubAgentLoopMax,
				)
				if derr != nil {
					errutil.Handle(ctx, goerr.Wrap(derr, "planexec: direct response failed",
						goerr.V("trace_id", req.TraceID)), "planexec: direct response failed")
					return &loopResult{
						outcome:        loopFallbackError,
						allResults:     allResults,
						fallbackReason: derr.Error(),
					}, nil
				}
				return &loopResult{outcome: loopDirect, allResults: allResults, directText: text}, nil
			}

			req.Sink.PlanProposed(ctx, PlanInfo{Round: logicalRound, Reasoning: p.Message, IsReplan: false})

			tasks := p.Tasks
			results := r.runPhase(ctx, logicalRound, tasks, &req)
			allResults = append(allResults, PhaseSummary{
				Phase:   logicalRound,
				Tasks:   tasks,
				Results: results,
			})
			nextInput = rc.bg.formatPrefix() + "\n\n" + formatObservationsAsUserTurn(tasks, results)
			continue
		}

		// Replan round.
		rr, rerr := parseReplanResult(raw, req.KnownToolIDs, req.AllowQuestion)
		if rerr != nil {
			lastCause = rerr
			errutil.Handle(ctx, goerr.Wrap(rerr, "replan output failed validation; retrying",
				goerr.T(errutil.TagBenign),
			), "replan output failed validation; retrying")
			nextInput = formatRetryInput(rc.bg, rerr)
			continue
		}

		logicalRound++
		req.Sink.PlanProposed(ctx, PlanInfo{Round: logicalRound, Reasoning: rr.Message, IsReplan: true})

		if rr.Question != nil {
			qr, qerr := req.OnQuestion(ctx, *rr.Question)
			if qerr != nil {
				return nil, goerr.Wrap(qerr, "on question callback")
			}
			if qr.Terminate {
				return &loopResult{outcome: loopQuestion, allResults: allResults}, nil
			}
			// Continue with the user's answers folded into the next planner-round
			// input.
			nextInput = formatQuestionAnswers(rc.bg, *rr.Question, qr.Items)
			continue
		}

		if rr.Finalize != nil {
			// The planner explicitly declared completion. The loop ends; the
			// entry point generates the user-visible final output. Empty tasks
			// with no explicit finalize never reach here — parseReplanResult
			// rejects that shape and folds it back into another replan round.
			return &loopResult{outcome: loopFinalize, allResults: allResults}, nil
		}

		tasks := rr.Tasks
		results := r.runPhase(ctx, logicalRound, tasks, &req)
		allResults = append(allResults, PhaseSummary{
			Phase:   logicalRound,
			Tasks:   tasks,
			Results: results,
		})
		nextInput = rc.bg.formatPrefix() + "\n\n" + formatObservationsAsUserTurn(tasks, results)
	}
}

// runPhase wraps Sink.PhaseStarted + executePhase so the boilerplate
// (TaskInfo conversion, budget bookkeeping) does not clutter the
// main loop.
func (r *Runner) runPhase(
	ctx context.Context,
	logicalRound int,
	tasks []TaskPlan,
	req *RunRequest,
) []TaskResult {
	infos := make([]TaskInfo, len(tasks))
	for i, t := range tasks {
		infos[i] = TaskInfo{ID: t.ID, Title: t.Title}
	}
	req.Sink.PhaseStarted(ctx, logicalRound, infos)
	// req.TraceHandler (the host's per-event handler, e.g. the Job timeline)
	// is combined with each sub-agent's own LLM-call counter inside
	// executePhase so sub-agent LLM / tool events reach the host timeline.
	// The archive recorder stays planner/direct/final-scoped (not threaded
	// here) to preserve the existing archive shape.
	return executePhase(ctx, tasks, req.Sink, req.ToolResolver, r.llm, r.budget.SubAgentLoopMax, req.TraceHandler, req.AllowSubAgentWrites)
}

// budgetFallbackReason names the cause of a budget-exhausted fallback. When the
// loop was burning rounds retrying a failing planner / replan output, the last
// such failure is the real story; a bare "budget exhausted" hides it.
func budgetFallbackReason(lastCause error) string {
	if lastCause != nil {
		return "planner budget exhausted; last failure: " + lastCause.Error()
	}
	return "planner budget exhausted"
}

// formatRetryInput prepends the budget prefix and a validation-failure
// note so the LLM has a concrete instruction for its next attempt.
func formatRetryInput(bg *budget, cause error) string {
	return bg.formatPrefix() + "\n\nYour previous output failed validation: " +
		cause.Error() +
		". Please re-emit a JSON object that matches the response schema."
}

// formatQuestionAnswers folds the user's QuestionResult into the next
// planner-round input. Used only when OnQuestion returned
// QuestionResult{Terminate: false}.
func formatQuestionAnswers(bg *budget, q Question, answers []QuestionAnswer) string {
	var sb strings.Builder
	sb.WriteString(bg.formatPrefix())
	sb.WriteString("\n\n# User answers\n\n")
	byID := make(map[string]QuestionItem, len(q.Items))
	for _, it := range q.Items {
		byID[it.ID] = it
	}
	for _, ans := range answers {
		item, ok := byID[ans.ID]
		if !ok {
			fmt.Fprintf(&sb, "## %s\n(unknown question id; answer kept verbatim)\n", ans.ID)
		} else {
			fmt.Fprintf(&sb, "## %s — %s\n", ans.ID, item.Text)
		}
		switch {
		case ans.FreeText != "":
			fmt.Fprintf(&sb, "Answer (free_text): %s\n\n", ans.FreeText)
		case len(ans.Choices) > 0:
			fmt.Fprintf(&sb, "Answer (multi_select): %s\n\n", strings.Join(ans.Choices, ", "))
		case ans.Choice != "":
			fmt.Fprintf(&sb, "Answer (select): %s\n\n", ans.Choice)
		default:
			sb.WriteString("Answer: (none provided)\n\n")
		}
	}
	sb.WriteString("Use these answers to decide the next action.\n")
	return sb.String()
}
