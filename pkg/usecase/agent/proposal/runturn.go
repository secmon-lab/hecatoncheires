package proposal

import (
	"context"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/wsmeta"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// TurnRequest is the input for one open-mode turn.
type TurnRequest struct {
	// Session is the per-thread Session row. The runtime reads / writes
	// the turn-lock fields via deps.StartTurn.
	Session *model.Session

	// UserInput is the planner's first user message. For app_mention this
	// is the mention text; for thread_reply it is the reply text; for
	// ws_switch it is a synthetic system event sentence (see spec §2.11).
	UserInput string

	// Trigger discriminates the entry point — drives prompt hints.
	Trigger Trigger

	// TriggerTS is the Slack TS used as both the trace ID and the lock
	// trigger key. For ws_switch, the host generates a UUID and uses it
	// here.
	TriggerTS string

	// ActorUserID is the Slack user who initiated the turn. Pass to
	// sub-agent tools that need actor authorisation.
	ActorUserID string

	// ExistingProposal is the prior draft persisted by the host (when this
	// turn is resuming an existing draft, e.g. ws-switch or thread reply).
	// The planner does not consume it directly — the host uses it to keep
	// preview state coherent across turns.
	ExistingProposal *model.CaseProposal

	// Handler implements the host-side terminal action dispatchers and
	// trace updates.
	Handler Handler
}

// Status discriminates the terminal shapes RunTurn can return to the host.
type Status int

const (
	// StatusCompleted means the turn ran end-to-end and the host has been
	// called with a terminal action (Question or Materialize).
	StatusCompleted Status = iota
	// StatusBusy means another turn was running on this Session;
	// Handler.PostBusy was invoked. The host should not re-post on the
	// same trigger.
	StatusBusy
	// StatusIdempotent means the trigger duplicates a turn already in
	// flight (Slack event re-delivery). Drop silently.
	StatusIdempotent
	// StatusFallback means the planner exhausted its budget or hit an
	// internal error before reaching a terminal action. The host should
	// post a system fallback message (e.g. "I couldn't reach a conclusion;
	// please mention me again with more context"). The runtime does NOT
	// post anything itself in this case.
	StatusFallback
)

// Result is the outcome of RunTurn.
type Result struct {
	Status Status
	// EndedWith is the SessionEndReason recorded on Session when the turn
	// hit a terminal action. Zero-valued for StatusBusy / StatusIdempotent.
	EndedWith model.SessionEndReason
	// FallbackReason describes why StatusFallback was returned (e.g.
	// "planner budget exhausted"). Non-empty only when Status==Fallback.
	FallbackReason string
}

// RunTurn drives one open-mode planner round-trip on the supplied Session.
// The flow is:
//
//  1. Acquire the per-thread turn lock (Phase A) and start the heartbeat
//     goroutine. Return StatusBusy / StatusIdempotent if appropriate.
//  2. Build a planner gollem agent (no tools, JSON response schema,
//     WithLoopLimit(1)) bound to Session.ID via WithHistoryRepository.
//  3. Loop while the planner budget allows: call agent.Execute with the
//     next user input (initial UserInput on round 1, formatted observations
//     after each investigate phase), parse + validate the JSON plan, and
//     either continue (investigate) or invoke a terminal Handler method.
//  4. When the planner budget is exhausted without a terminal action,
//     fall back to a PostMessage with the "couldn't reach a conclusion"
//     copy.
//
// Errors from validation / planner / sub-agent failures are surfaced; the
// caller decides whether to post a Slack-side error message.
func (uc *UseCase) RunTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if err := validateTurnRequest(&req); err != nil {
		return nil, err
	}
	handler := req.Handler

	handle, err := uc.deps.StartTurn(ctx, req.Session, req.TriggerTS)
	if err != nil {
		return nil, goerr.Wrap(err, "start draft turn")
	}
	if handle.Idempotent {
		return &Result{Status: StatusIdempotent}, nil
	}
	if !handle.Acquired {
		if err := handler.PostBusy(ctx, handle.Session, agent.BusyInfo{
			StartedAt: handle.Session.TurnStartedAt,
			OwnerID:   handle.Session.TurnOwnerID,
		}); err != nil {
			errutil.Handle(ctx, err, "post draft busy notice")
		}
		return &Result{Status: StatusBusy}, nil
	}
	defer handle.Release(ctx)

	turnCtx := handle.Ctx
	req.Session = handle.Session

	// Trace recorder for the durable trace artifact. Trace ID = TurnID
	// (the UUID v7 minted by StartTurn) so traces are uniformly UUID-shaped
	// regardless of trigger kind. The originating Slack TS is preserved as
	// a metadata label.
	recorder := trace.New(
		trace.WithRepository(uc.deps.TraceRepo),
		trace.WithTraceID(handle.OwnerID),
		trace.WithMetadata(trace.TraceMetadata{
			Labels: map[string]string{
				labelSessionID:        req.Session.ID,
				labelChannelID:        req.Session.ChannelID,
				labelThreadTS:         req.Session.ThreadTS,
				labelTriggerTS:        req.TriggerTS,
				labelTriggerKind:      triggerString(req.Trigger),
				labelCreatorUserID:    req.Session.CreatorUserID,
				labelTriggerActorUser: req.ActorUserID,
			},
		}),
	)
	defer func() {
		// Use the parent ctx (not turnCtx) so the final trace flush
		// still runs even when the turn was cancelled by lock loss /
		// heartbeat staleness — that's exactly when the trace is most
		// valuable for debugging.
		if err := recorder.Finish(ctx); err != nil {
			errutil.Handle(ctx, err, "draft: persist agent trace")
		}
	}()

	resolver := uc.buildToolSetResolver(req)
	// Render the system prompt with the registered workspace list +
	// per-workspace custom field schemas so the planner has the exact
	// vocabulary to choose a workspace and fill its required fields.
	systemPrompt, err := renderPlannerPrompt(uc.deps.Registry, plannerLanguageLabel(turnCtx))
	if err != nil {
		return nil, goerr.Wrap(err, "render planner prompt")
	}
	logging.From(turnCtx).Debug("draft turn started",
		"session_id", req.Session.ID,
		"turn_id", handle.OwnerID,
		"trigger", triggerString(req.Trigger),
		"user_input_len", len(req.UserInput),
		"system_prompt_len", len(systemPrompt),
		"workspace_count", workspaceRegistryCount(uc.deps.Registry),
	)
	// Build a fresh planner agent per round. We deliberately do not reuse a
	// single agent across multiple Execute calls because gollem's loop
	// budget is shared across calls within an agent, and we want each
	// planner round to be a single, isolated LLM call. A new agent reloads
	// history from the repository each time, which is the correct semantic.
	//
	// The planner is tool-enabled: wsmeta exposes `list_workspaces` and
	// `get_workspace` so the planner can pull a workspace's field schema
	// and source list on demand instead of having the entire registry
	// inlined into the system prompt. The response schema is still wired
	// up — gollem applies it to the final assistant text, after the
	// tool-call rounds settle.
	plannerTools := wsmeta.New(wsmeta.Deps{
		Registry:   uc.deps.Registry,
		SourceRepo: uc.deps.Repo.Source(),
	})
	newPlannerAgent := func(roundKey string) *gollem.Agent {
		return gollem.New(uc.deps.LLMClient,
			gollem.WithSystemPrompt(systemPrompt),
			gollem.WithTools(plannerTools...),
			gollem.WithHistoryRepository(uc.deps.HistoryRepo, req.Session.ID),
			gollem.WithTrace(recorder),
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(planSchema()),
			gollem.WithLoopLimit(plannerPerCallLoopLimit),
			gollem.WithContentBlockMiddleware(newPlannerProgressMiddleware(handler, roundKey)),
		)
	}

	budget := agent.NewBudget(uc.plannerLoopMax, uc.subAgentLoopMax)

	// First-round user input: budget prefix + caller-supplied text.
	nextInput := budget.FormatPrefix() + "\n\n" + req.UserInput

	// logicalRound counts the planner's *logical* rounds: a single planner
	// attempt that may span multiple Execute calls if validation fails. All
	// transient lines for one logical round (Planning…, retry, the
	// re-Planning… that follows, the final action selection, plus
	// per-iteration tool-call / thought lines from the middleware) share
	// one Slack message and replace each other in place. A fresh logical
	// round opens a new Slack message — that is the boundary the user
	// sees as "this round is over, the next one started".
	logicalRound := 0

	for {
		if !budget.CanPlannerCall() {
			return uc.fallback(turnCtx, req, "planner budget exhausted")
		}
		if logicalRound == 0 {
			logicalRound = 1
		}
		budget.PlannerUsed++
		roundKey := fmt.Sprintf("plan-%d", logicalRound)
		handler.TraceRound(turnCtx, roundKey, i18n.T(turnCtx, i18n.MsgProposalTracePlanning))

		roundStarted := time.Now()
		resp, execErr := newPlannerAgent(roundKey).Execute(turnCtx, gollem.Text(nextInput))
		roundElapsed := time.Since(roundStarted).Round(time.Millisecond)
		if execErr != nil {
			return nil, goerr.Wrap(execErr, "planner execute",
				goerr.V("planner_used", budget.PlannerUsed),
				goerr.V("trigger_ts", req.TriggerTS),
				goerr.V("elapsed", roundElapsed),
			)
		}
		var firstTextLen int
		if len(resp.Texts) > 0 {
			firstTextLen = len(resp.Texts[0])
		}
		logging.From(turnCtx).Debug("planner round completed",
			"round", budget.PlannerUsed,
			"elapsed", roundElapsed,
			"texts_count", len(resp.Texts),
			"first_text_len", firstTextLen,
			"is_empty", resp.IsEmpty(),
		)
		if resp.IsEmpty() {
			return nil, goerr.New("planner returned empty response",
				goerr.V("round", budget.PlannerUsed),
				goerr.V("elapsed", roundElapsed),
				goerr.V("texts_count", len(resp.Texts)),
				goerr.V("first_text_len", firstTextLen),
			)
		}
		p, parseErr := parseAndValidate([]byte(resp.Texts[0]))
		if parseErr != nil {
			// Retry with the validation error fed back as user input
			// (so the LLM has a concrete instruction). Each retry
			// consumes one planner slot. Surface the failure in the
			// Slack trace so the user can see *why* successive Planning
			// rounds are firing without progress. We stay on the same
			// roundKey so the retry line replaces the prior planning
			// content in place.
			// Validation failures are expected with LLM outputs and we
			// retry inline; tag benign so the operator still sees the line
			// in the logs but Sentry does not page on every LLM hiccup.
			errutil.Handle(turnCtx, goerr.Wrap(parseErr, "planner output failed validation; retrying",
				goerr.T(errutil.TagBenign),
			), "planner output failed validation; retrying")
			handler.TraceRound(turnCtx, roundKey, i18n.T(turnCtx, i18n.MsgProposalTracePlannerRetry))
			nextInput = budget.FormatPrefix() + "\n\nYour previous output failed validation: " + parseErr.Error() + ". Please re-emit a JSON object that matches the response schema."
			continue
		}

		handler.TraceRound(turnCtx, roundKey, i18n.T(turnCtx, i18n.MsgProposalTracePlannerAction, string(p.Action), p.Reasoning))

		switch p.Action {
		case actionInvestigate:
			results := uc.runInvestigationsParallel(turnCtx, p.Investigate, handler, resolver)
			nextInput = budget.FormatPrefix() + "\n\n" + formatObservationsAsUserTurn(p.Investigate, results)
			// A fresh logical round begins after investigation
			// observations come back — the next "🤔 Planning…"
			// must open a new Slack message, not replace this
			// round's "→ investigate — …" line.
			logicalRound++
			continue
		case actionQuestion:
			payload := QuestionPayload{Reason: p.Question.Reason}
			payload.Items = make([]QuestionItem, len(p.Question.Items))
			for i, it := range p.Question.Items {
				payload.Items[i] = QuestionItem{
					ID: it.ID, Text: it.Text,
					Type:    QuestionItemType(it.Type),
					Options: it.Options,
				}
			}
			if err := handler.Question(turnCtx, req.Session, payload); err != nil {
				return nil, goerr.Wrap(err, "handler Question")
			}
			return uc.finalize(turnCtx, req.Session, model.SessionEndedWithQuestion)
		case actionMaterialize:
			if err := handler.Materialize(turnCtx, req.Session, MaterializePayload{
				WorkspaceID:       p.Materialize.WorkspaceID,
				Title:             p.Materialize.Title,
				Description:       p.Materialize.Description,
				CustomFieldValues: p.Materialize.CustomFieldValues,
				IsTest:            p.Materialize.IsTest,
			}); err != nil {
				return nil, goerr.Wrap(err, "handler Materialize")
			}
			return uc.finalize(turnCtx, req.Session, model.SessionEndedWithMaterialize)
		default:
			return nil, goerr.New("planner returned unknown action", goerr.V("action", string(p.Action)))
		}
	}
}

// fallback returns a StatusFallback result so the host can render whatever
// system message it likes (the runtime no longer has a PostMessage channel).
// The session is NOT finalised with a SessionEndReason here: post_message
// was retired, so the planner's only terminal actions are question /
// materialize. Fallback is a runtime-internal failure mode.
func (uc *UseCase) fallback(ctx context.Context, req TurnRequest, reason string) (*Result, error) {
	// Fallback is a runtime-internal failure mode — the planner reached a
	// state we did not design for. Surface to Sentry so we can investigate.
	errutil.Handle(ctx, goerr.New("draft turn fallback",
		goerr.V("reason", reason),
		goerr.V("trigger_ts", req.TriggerTS),
	), "draft turn fallback")
	return &Result{Status: StatusFallback, FallbackReason: reason}, nil
}

// finalize stamps the session end reason and persists. LastMentionTS is
// expected to have been set by the host before calling RunTurn (when the
// trigger has a Slack mention TS), so we don't touch it here.
func (uc *UseCase) finalize(ctx context.Context, ssn *model.Session, ended model.SessionEndReason) (*Result, error) {
	ssn.LastAction = ended
	ssn.UpdatedAt = time.Now().UTC()
	if err := uc.deps.Repo.Session().Put(ctx, ssn); err != nil {
		errutil.Handle(ctx, err, "draft: persist session lastAction")
	}
	return &Result{Status: StatusCompleted, EndedWith: ended}, nil
}

// buildToolSetResolver composes a per-turn tool resolver. Note that core_ro
// is the read-only subset (no mutation) — investigation sub-agents must
// not change Case state while a turn is forming.
//
// WorkspaceID is left empty for the draft mode: the host no longer pre-resolves
// a workspace, so the resolver hands every sub-agent a workspace-agnostic
// core_ro deps. Sub-agents that genuinely need a workspace context get it
// via the planner's investigate task description (the planner has just
// learnt the workspace identity from `get_workspace`).
func (uc *UseCase) buildToolSetResolver(_ TurnRequest) *agent.ToolSetResolver {
	d := uc.deps
	return agent.NewToolSetResolver(agent.ToolSetDeps{
		Core: core.Deps{
			Repo:         d.Repo,
			WorkspaceID:  "",
			ActionUC:     d.ActionUC,
			ActionStepUC: d.ActionStepUC,
			// CaseRefUC intentionally left nil: draft mode is workspace-agnostic
			// (WorkspaceID == ""), so case_ref tools that need a concrete
			// workspace+field schema to resolve reference_workspace cannot function
			// here.
		},
		Slack: slacktool.Deps{
			Bot:       d.SlackBot,
			Search:    d.SlackSearch,
			Retriever: d.SlackRetriever,
		},
		Notion:   notiontool.Deps{Client: d.NotionClient},
		GitHub:   d.GitHubClient,
		WebFetch: d.WebFetchClient,
		Jira:     d.JiraTools,
	})
}

func validateTurnRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	// TriggerTS may be empty for synthetic triggers (ws-switch). The
	// turn-lock layer treats empty TriggerKey as "no Slack-side dedup".
	if req.Handler == nil {
		return goerr.New("Handler is required")
	}
	return nil
}

// plannerPerCallLoopLimit is the gollem-side loop bound per Execute. The
// planner is tool-enabled (wsmeta), so a single round legitimately consumes
// a few iterations (e.g. tool call → tool response → final JSON). Set
// generously so a planner that wants to call list_workspaces + get_workspace
// before deciding still has room to emit the terminal JSON.
const plannerPerCallLoopLimit = 8

// plannerProgressMaxRunes bounds the LLM-message excerpt rendered into the
// planner round message during a tool-use cycle. A single Slack context
// line, so long thoughts collapse to this character budget.
const plannerProgressMaxRunes = 80

// newPlannerProgressMiddleware returns a gollem ContentBlockMiddleware that
// surfaces per-iteration planner activity onto the planner round's Slack
// message. Every LLM round, the middleware observes the response and
// pushes either "🛠 Planning — calling <tool>" (when the response carries
// a tool call) or "🤔 Planning — <excerpt>" (when only a thought is
// available) to Handler.TraceRound under the supplied roundKey. Because
// TraceRound replaces the round message in place, the user watches the
// planner's tool-use cycle play out inside one updating context block
// instead of accumulating a stack of separate trace posts.
func newPlannerProgressMiddleware(h Handler, roundKey string) gollem.ContentBlockMiddleware {
	return func(next gollem.ContentBlockHandler) gollem.ContentBlockHandler {
		return func(ctx context.Context, req *gollem.ContentRequest) (*gollem.ContentResponse, error) {
			resp, err := next(ctx, req)
			if err != nil || resp == nil {
				return resp, err
			}
			// Show the LLM's accompanying thought first so the round
			// message keeps a useful line even when the response is
			// text-only.
			for _, txt := range resp.Texts {
				excerpt := oneLineExcerpt(txt, plannerProgressMaxRunes)
				if excerpt != "" {
					h.TraceRound(ctx, roundKey, i18n.T(ctx, i18n.MsgProposalTracePlannerMessage, excerpt))
					break
				}
			}
			// A tool call is the more concrete thing to display —
			// overwrite any thought line we just set.
			if len(resp.FunctionCalls) > 0 && resp.FunctionCalls[0] != nil {
				h.TraceRound(ctx, roundKey, i18n.T(ctx, i18n.MsgProposalTracePlannerTool, resp.FunctionCalls[0].Name))
			}
			return resp, nil
		}
	}
}

// triggerString stringifies a Trigger for trace metadata labels.
func triggerString(t Trigger) string {
	switch t {
	case TriggerAppMention:
		return "app_mention"
	case TriggerThreadReply:
		return "thread_reply"
	case TriggerWSSwitch:
		return "ws_switch"
	default:
		return "unknown"
	}
}

// Trace metadata labels.
const (
	labelSessionID        = "session_id"
	labelChannelID        = "channel_id"
	labelThreadTS         = "thread_ts"
	labelTriggerTS        = "trigger_ts"
	labelTriggerKind      = "trigger_kind"
	labelCreatorUserID    = "creator_user_id"
	labelTriggerActorUser = "actor_user_id"
)
