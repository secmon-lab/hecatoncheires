package draft

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

//go:embed prompts/planner.md
var plannerSystemPrompt string

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

	// EstimatedWS / Candidates / EstimationReason / ExistingDraft are
	// pure-domain facts the host has already resolved; the planner uses
	// them to make a sensible workspace choice without re-running the
	// estimation logic.
	EstimatedWS      *model.WorkspaceEntry
	Candidates       []*model.WorkspaceEntry
	EstimationReason string
	ExistingDraft    *model.CaseDraft

	// Handler implements the host-side terminal action dispatchers and
	// trace updates.
	Handler Handler
}

// Status discriminates the four terminal shapes RunTurn can return to the
// host.
type Status int

const (
	// StatusCompleted means the turn ran end-to-end and (if applicable)
	// the host has already been called with the terminal action.
	StatusCompleted Status = iota
	// StatusBusy means another turn was running on this Session;
	// Handler.PostBusy was invoked. The host should not re-post on the
	// same trigger.
	StatusBusy
	// StatusIdempotent means the trigger duplicates a turn already in
	// flight (Slack event re-delivery). Drop silently.
	StatusIdempotent
)

// Result is the outcome of RunTurn.
type Result struct {
	Status Status
	// EndedWith is the SessionEndReason recorded on Session when the turn
	// hit a terminal action. Zero-valued for StatusBusy / StatusIdempotent.
	EndedWith model.SessionEndReason
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
	// Build a fresh planner agent per round. We deliberately do not reuse a
	// single agent across multiple Execute calls because gollem's loop
	// budget is shared across calls within an agent, and we want each
	// planner round to be a single, isolated LLM call. A new agent reloads
	// history from the repository each time, which is the correct semantic.
	newPlannerAgent := func() *gollem.Agent {
		return gollem.New(uc.deps.LLMClient,
			gollem.WithSystemPrompt(plannerSystemPrompt),
			gollem.WithHistoryRepository(uc.deps.HistoryRepo, req.Session.ID),
			gollem.WithTrace(recorder),
			gollem.WithContentType(gollem.ContentTypeJSON),
			gollem.WithResponseSchema(planSchema()),
			gollem.WithLoopLimit(plannerPerCallLoopLimit),
		)
	}

	budget := agent.NewBudget(uc.plannerLoopMax, uc.subAgentMaxPerTurn, uc.subAgentLoopMax)

	// First-round user input: budget prefix + caller-supplied text.
	nextInput := budget.FormatPrefix() + "\n\n" + req.UserInput

	for {
		if !budget.CanPlannerCall() {
			return uc.fallback(turnCtx, req, "planner budget exhausted")
		}
		budget.PlannerUsed++
		handler.Trace(turnCtx, i18n.T(turnCtx, i18n.MsgDraftTracePlanning))

		resp, execErr := newPlannerAgent().Execute(turnCtx, gollem.Text(nextInput))
		if execErr != nil {
			return nil, goerr.Wrap(execErr, "planner execute",
				goerr.V("planner_used", budget.PlannerUsed),
				goerr.V("trigger_ts", req.TriggerTS),
			)
		}
		if len(resp.Texts) == 0 {
			return nil, goerr.New("planner returned empty response")
		}
		p, parseErr := parseAndValidate([]byte(resp.Texts[0]))
		if parseErr != nil {
			// Retry with the validation error fed back as user input
			// (so the LLM has a concrete instruction). Each retry
			// consumes one planner slot. Surface the failure in the
			// Slack trace so the user can see *why* successive Planning
			// rounds are firing without progress.
			logging.From(turnCtx).Warn("planner output failed validation; retrying",
				"error", parseErr.Error(),
			)
			handler.Trace(turnCtx, i18n.T(turnCtx, i18n.MsgDraftTracePlannerRetry))
			nextInput = budget.FormatPrefix() + "\n\nYour previous output failed validation: " + parseErr.Error() + ". Please re-emit a JSON object that matches the response schema."
			continue
		}

		if p.Action == actionInvestigate {
			if got := len(p.Investigate.Tasks); got > budget.SubAgentRemaining() {
				nextInput = budget.FormatPrefix() + "\n\n" +
					fmt.Sprintf("Your last plan requested %d investigation tasks, but only %d sub-agent slots remain. Re-plan with fewer tasks, or pick a terminal action.", got, budget.SubAgentRemaining())
				continue
			}
		}

		handler.Trace(turnCtx, i18n.T(turnCtx, i18n.MsgDraftTracePlannerAction, string(p.Action), p.Reasoning))

		switch p.Action {
		case actionInvestigate:
			budget.SubAgentUsed += len(p.Investigate.Tasks)
			results := uc.runInvestigationsParallel(turnCtx, p.Investigate, handler, resolver)
			nextInput = budget.FormatPrefix() + "\n\n" + formatObservationsAsUserTurn(p.Investigate, results)
			continue
		case actionPostMessage:
			if err := handler.PostMessage(turnCtx, req.Session, p.PostMessage.Text); err != nil {
				return nil, goerr.Wrap(err, "handler PostMessage")
			}
			return uc.finalize(turnCtx, req.Session, model.SessionEndedWithMessage)
		case actionPostQuestion:
			if err := handler.PostQuestion(turnCtx, req.Session, QuestionPayload{
				Text: p.PostQuestion.Text, Options: p.PostQuestion.Options, Reason: p.PostQuestion.Reason,
			}); err != nil {
				return nil, goerr.Wrap(err, "handler PostQuestion")
			}
			return uc.finalize(turnCtx, req.Session, model.SessionEndedWithQuestion)
		case actionMaterialize:
			if err := handler.Materialize(turnCtx, req.Session, MaterializePayload{
				WorkspaceID:       p.Materialize.WorkspaceID,
				Title:             p.Materialize.Title,
				Description:       p.Materialize.Description,
				CustomFieldValues: p.Materialize.CustomFieldValues,
			}); err != nil {
				return nil, goerr.Wrap(err, "handler Materialize")
			}
			return uc.finalize(turnCtx, req.Session, model.SessionEndedWithMaterialize)
		default:
			return nil, goerr.New("planner returned unknown action", goerr.V("action", string(p.Action)))
		}
	}
}

// fallback posts a "could not reach conclusion" message via the handler and
// finalises the turn with SessionEndedWithMessage.
func (uc *UseCase) fallback(ctx context.Context, req TurnRequest, reason string) (*Result, error) {
	logging.From(ctx).Warn("draft turn fallback", "reason", reason, "trigger_ts", req.TriggerTS)
	if err := req.Handler.PostMessage(ctx, req.Session, fallbackMessage); err != nil {
		errutil.Handle(ctx, err, "draft fallback PostMessage")
	}
	return uc.finalize(ctx, req.Session, model.SessionEndedWithMessage)
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
func (uc *UseCase) buildToolSetResolver(req TurnRequest) *agent.ToolSetResolver {
	d := uc.deps
	wsID := ""
	if req.EstimatedWS != nil {
		wsID = req.EstimatedWS.Workspace.ID
	}
	return agent.NewToolSetResolver(agent.ToolSetDeps{
		Core: core.Deps{
			Repo:         d.Repo,
			WorkspaceID:  wsID,
			ActionUC:     d.ActionUC,
			ActionStepUC: d.ActionStepUC,
		},
		Slack: slacktool.Deps{
			Bot:    d.SlackBot,
			Search: d.SlackSearch,
		},
		Notion: notiontool.Deps{Client: d.NotionClient},
		GitHub: d.GitHubClient,
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

// fallbackMessage is the English literal posted when the planner budget is
// exhausted without reaching a terminal action. Localised copy lives in
// the i18n layer (MsgKeyAgentLoopFallback) and is used by the host.
const fallbackMessage = ":warning: I couldn't reach a conclusion within the budget for this turn. Please mention me again with more context."

// plannerPerCallLoopLimit is the gollem-side loop bound per Execute. The
// planner has no tools, so the LLM should always finish in 1 iteration —
// this limit is a safety net.
const plannerPerCallLoopLimit = 4

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
