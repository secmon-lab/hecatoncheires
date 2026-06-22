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
// (prompts, sink, tool resolver, schema) come in via Runner.Run's
// RunRequest. Runner is safe for concurrent use across goroutines — it
// holds no per-call state internally.
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

// Run drives a single plan-and-execute turn end-to-end. The flow is:
//
//  1. Validate the request.
//  2. Build the planner system prompt (host base + planexec loop rules).
//  3. Set up a trace.Recorder bound to req.TraceID and req.TraceMetadata.
//  4. Loop while the planner budget allows:
//     a. Execute one planner LLM round with the appropriate schema
//     (planSchema on round 1; replanSchema thereafter).
//     b. Parse + validate the JSON. On validation failure, surface the
//     error to errutil.Handle (benign tag) and retry within the same
//     logical round, charging the next planner call against the
//     budget.
//     c. If Question is present (replan rounds only): call OnQuestion;
//     Terminate=true → exit and return Completed/EndedWithQuestion.
//     d. If Tasks is empty (replan rounds only): exit the loop and run
//     the final-response phase.
//     e. Otherwise: budget-check, executePhase, fold observations into
//     the next planner-round input, continue.
//  5. After the loop exits, call generateFinalResponse to produce the
//     user-visible terminal output (plain text or structured JSON).
//
// Errors from the planner / sub-agents / final-response are wrapped
// with goerr context and surfaced; the caller (host) decides whether
// to render a fallback message of its choice.
func (r *Runner) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	if err := req.Validate(); err != nil {
		return nil, goerr.Wrap(err, "validate run request")
	}

	logger := logging.From(ctx)

	systemPrompt, err := renderPlannerSystemPrompt(plannerPromptInput{
		HostPrompt:      req.SystemPrompt,
		Language:        req.LanguageLabel,
		KnownToolIDs:    req.KnownToolIDs,
		AllowQuestion:   req.AllowQuestion,
		AllowDirect:     req.AllowDirect,
		StructuredFinal: req.FinalOutputSchema != nil,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "render planner system prompt")
	}

	recorder := trace.New(
		trace.WithRepository(r.traceRepo),
		trace.WithTraceID(req.TraceID),
		trace.WithMetadata(req.TraceMetadata),
	)
	// context.WithoutCancel detaches the cleanup from the caller's
	// cancellation tree so the final trace flush still runs (and can
	// reach Firestore) when the host already cancelled ctx — e.g. a
	// heartbeat-driven turn-lock loss or a parent request timeout.
	// Using ctx directly here means the persisted-trace I/O fails
	// silently at exactly the moment the trace is most valuable for
	// debugging.
	defer func() {
		cleanupCtx := context.WithoutCancel(ctx)
		if err := recorder.Finish(cleanupCtx); err != nil {
			errutil.Handle(cleanupCtx, err, "planexec: persist agent trace")
		}
	}()

	bg := newBudget(r.budget)
	nextInput := bg.formatPrefix() + "\n\n" + req.UserInput

	var allResults []PhaseSummary
	logicalRound := 0

	for {
		if !bg.canPlannerCall() {
			return r.fallbackBudget(allResults), nil
		}
		bg.plannerUsed++

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
			gollem.WithSystemPrompt(systemPrompt),
			gollem.WithTools(req.PlannerTools...),
			gollem.WithHistoryRepository(r.historyRepo, req.HistoryKey),
			gollem.WithTrace(recorder),
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(schema),
			gollem.WithLoopLimit(plannerPerCallLoopLimit),
		)

		resp, execErr := plannerAgent.Execute(ctx, gollem.Text(nextInput))
		if execErr != nil {
			return nil, goerr.Wrap(execErr, "planner execute",
				goerr.V("trace_id", req.TraceID),
				goerr.V("planner_used", bg.plannerUsed))
		}
		if resp == nil || resp.IsEmpty() {
			return nil, goerr.New("planner returned empty response",
				goerr.V("trace_id", req.TraceID),
				goerr.V("planner_used", bg.plannerUsed))
		}

		raw := []byte(resp.Texts[0])
		logger.Debug("planexec planner round",
			"trace_id", req.TraceID,
			"round", bg.plannerUsed,
			"raw_len", len(raw),
		)

		if isFirstRound {
			p, perr := parsePlanResult(raw, req.KnownToolIDs, req.AllowDirect)
			if perr != nil {
				// Retry within the same logical round; the cost is
				// charged via the next loop iteration's bg.plannerUsed++.
				errutil.Handle(ctx, goerr.Wrap(perr, "planner output failed validation; retrying",
					goerr.T(errutil.TagBenign),
				), "planner output failed validation; retrying")
				nextInput = formatRetryInput(bg, perr)
				continue
			}
			// First valid plan accepted — promote to logicalRound 1.
			logicalRound = 1

			// Direct fast path: the planner judged the request trivial enough
			// to answer without any investigation. Skip the plan/execute/
			// replan machinery and produce a plain-text reply in a single
			// tool-enabled ReAct loop. OnFinalize / FinalOutputSchema are not
			// consulted here — direct mode is reserved for replies that need
			// no structured terminal action.
			if p.Direct != nil {
				req.Sink.PlanProposed(ctx, PlanInfo{Round: logicalRound, Reasoning: p.Message, IsReplan: false, Direct: true})
				tools := req.ToolResolver.Resolve(p.Direct.Tools)
				text, derr := generateDirectResponse(
					ctx,
					r.llm,
					r.historyRepo,
					recorder,
					systemPrompt,
					req.HistoryKey,
					req.LanguageLabel,
					req.UserInput,
					tools,
					r.budget.SubAgentLoopMax,
				)
				if derr != nil {
					errutil.Handle(ctx, goerr.Wrap(derr, "planexec: direct response failed",
						goerr.V("trace_id", req.TraceID)), "planexec: direct response failed")
					return &RunResult{
						Status:         StatusFallbackError,
						Direct:         true,
						FallbackReason: derr.Error(),
					}, nil
				}
				return &RunResult{
					Status:    StatusCompleted,
					FinalText: text,
					Direct:    true,
				}, nil
			}

			req.Sink.PlanProposed(ctx, PlanInfo{Round: logicalRound, Reasoning: p.Message, IsReplan: false})

			tasks := p.Tasks
			results := r.runPhase(ctx, logicalRound, tasks, &req)
			allResults = append(allResults, PhaseSummary{
				Phase:   logicalRound,
				Tasks:   tasks,
				Results: results,
			})
			nextInput = bg.formatPrefix() + "\n\n" + formatObservationsAsUserTurn(tasks, results)
			continue
		}

		// Replan round.
		rr, rerr := parseReplanResult(raw, req.KnownToolIDs, req.AllowQuestion)
		if rerr != nil {
			errutil.Handle(ctx, goerr.Wrap(rerr, "replan output failed validation; retrying",
				goerr.T(errutil.TagBenign),
			), "replan output failed validation; retrying")
			nextInput = formatRetryInput(bg, rerr)
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
				return &RunResult{
					Status:            StatusCompleted,
					AllResults:        allResults,
					EndedWithQuestion: true,
				}, nil
			}
			// Continue with the user's answers folded into the next
			// planner-round input.
			nextInput = formatQuestionAnswers(bg, *rr.Question, qr.Items)
			continue
		}

		if len(rr.Tasks) == 0 {
			// The planner wants to terminate. Produce the final response and,
			// when an OnFinalize hook is wired, let the host validate AND
			// commit it. A non-nil error from OnFinalize (validation failed OR
			// the terminal side effect failed) folds back as another round so
			// the planner can investigate / ask / re-emit until it succeeds or
			// the round budget is exhausted.
			text, rawJSON, finalErr := generateFinalResponse(
				ctx,
				r.llm,
				r.historyRepo,
				recorder,
				systemPrompt,
				req.HistoryKey,
				req.LanguageLabel,
				allResults,
				req.FinalOutputSchema,
			)
			if finalErr != nil {
				// Surface to errutil so the operator sees the reason even
				// though the host gets a graceful RunResult back.
				errutil.Handle(ctx, finalErr, "planexec: final response failed")
				return &RunResult{
					Status:         StatusFallbackError,
					AllResults:     allResults,
					FallbackReason: finalErr.Error(),
				}, nil
			}
			if req.OnFinalize != nil {
				if commitErr := req.OnFinalize(ctx, rawJSON); commitErr != nil {
					// Validation / commit rejection is expected with LLM
					// output and we retry inline; tag benign so the operator
					// still sees the line in logs but Sentry does not page on
					// every LLM hiccup.
					errutil.Handle(ctx, goerr.Wrap(commitErr, "final output rejected; retrying",
						goerr.T(errutil.TagBenign),
					), "final output rejected; retrying")
					nextInput = formatRetryInput(bg, commitErr)
					continue
				}
			}
			return &RunResult{
				Status:     StatusCompleted,
				FinalText:  text,
				FinalRaw:   rawJSON,
				AllResults: allResults,
			}, nil
		}

		tasks := rr.Tasks
		results := r.runPhase(ctx, logicalRound, tasks, &req)
		allResults = append(allResults, PhaseSummary{
			Phase:   logicalRound,
			Tasks:   tasks,
			Results: results,
		})
		nextInput = bg.formatPrefix() + "\n\n" + formatObservationsAsUserTurn(tasks, results)
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
	return executePhase(ctx, tasks, req.Sink, req.ToolResolver, r.llm, r.budget.SubAgentLoopMax)
}

// fallbackBudget assembles the StatusFallbackBudget RunResult and emits
// a Sink.Notify so the host gets a chance to surface the cause without
// having to inspect the status itself.
func (r *Runner) fallbackBudget(allResults []PhaseSummary) *RunResult {
	reason := "planner budget exhausted"
	return &RunResult{
		Status:         StatusFallbackBudget,
		AllResults:     allResults,
		FallbackReason: reason,
	}
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
