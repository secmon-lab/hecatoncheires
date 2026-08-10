package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
)

// threadcaseHost is the Slack side of a finished thread-mode turn. Every method
// runs after the turn — possibly on another instance — so each rebuilds what it
// needs from the Target and the stored Session rather than from values the
// spawning call held.
type threadcaseHost struct {
	uc *AgentUseCase
}

// ApplyMention posts a mention turn's reply, or writes its proposed content onto
// the case and confirms it.
func (h threadcaseHost) ApplyMention(ctx context.Context, target threadcase.Target, d *threadcase.Decision) error {
	if d == nil {
		return nil
	}
	entry, err := h.uc.deps.Registry.Get(target.WorkspaceID)
	if err != nil {
		return goerr.Wrap(err, "resolve the workspace of a finished mention turn",
			goerr.V("workspace_id", target.WorkspaceID))
	}
	// traceMsg is nil: the progress message was drawn by the run itself, from
	// whichever instance claimed each transition, so there is no in-process handle
	// to finalize. finalizeTrace posts the reply as its own thread message.
	h.uc.applyMentionDecision(ctx, target.WorkspaceID, entry, target.CaseID,
		target.ChannelID, target.ThreadTS, nil, d)
	return nil
}

// CreateCase commits a create turn's proposal and posts its outcome.
func (h threadcaseHost) CreateCase(ctx context.Context, target threadcase.Target, p threadcase.CreatePayload) error {
	req, session, err := h.createContext(ctx, target)
	if err != nil {
		return err
	}
	// requestKey is empty: the thread creation paths dedup by the existing message
	// ts (ReactionClaim + GetBySlackThread), not by a request key.
	c, err := h.uc.deps.CaseUC.createThreadBoundCase(ctx, target.WorkspaceID,
		target.ChannelID, target.ThreadTS, req.reporter, p.Title, p.Description, p.Fields, "")
	if err != nil {
		return goerr.Wrap(err, "create the thread-bound case",
			goerr.V("workspace_id", target.WorkspaceID))
	}
	h.uc.bindSessionToCase(ctx, target.ChannelID, target.ThreadTS, c.ID)
	h.uc.postCreatedCaseOutcome(ctx, req, session, c)
	return nil
}

// AskQuestion posts the planner's question. A create turn gets the interactive
// form (its answer resumes the creation); a mention turn gets a plain reply.
func (h threadcaseHost) AskQuestion(ctx context.Context, target threadcase.Target, q threadcase.QuestionPayload) error {
	if target.CaseID != 0 {
		return h.uc.postThreadcaseQuestion(ctx, target.ChannelID, target.ThreadTS, q)
	}
	session, err := h.session(ctx, target)
	if err != nil {
		return err
	}
	// The Submit button carries the case thread so the resume finds the session
	// regardless of which thread the form is displayed in.
	return h.uc.postThreadCreateQuestionForm(ctx, session,
		target.UIChannelID, target.UIThreadTS, target.ChannelID, target.ThreadTS,
		session.CreatorUserID, q)
}

// ReportFallback tells the user the turn reached no conclusion. For a create turn
// it also undoes what the start of the flow put in place: the reaction claim is
// released so a future reaction can retry, and the "Creating a case…" placeholder
// is marked failed so the monitored channel does not imply work is ongoing.
func (h threadcaseHost) ReportFallback(ctx context.Context, target threadcase.Target, reason string) error {
	h.uc.replyUserError(ctx, fallbackReasonError(reason), "thread case turn fallback",
		target.UIChannelID, target.UIThreadTS)
	if target.CaseID != 0 {
		return nil
	}
	session, err := h.session(ctx, target)
	if err != nil {
		return err
	}
	if session.PendingQuestion != nil {
		// The turn ended on a fallback, not on a question; there is nothing to undo
		// for a flow still waiting on the user.
		return nil
	}
	if session.ReactionSourceMessageTS != "" {
		h.uc.releaseReactionClaim(ctx, target.WorkspaceID,
			session.ReactionSourceChannelID, session.ReactionSourceMessageTS)
		entry, gerr := h.uc.deps.Registry.Get(target.WorkspaceID)
		if gerr != nil {
			return goerr.Wrap(gerr, "resolve the workspace of a failed create turn",
				goerr.V("workspace_id", target.WorkspaceID))
		}
		failMonitoredThreadAnchor(ctx, h.uc.deps.SlackService, entry, target.ThreadTS)
	}
	return nil
}

// createContext rebuilds the creation request a finished create turn belongs to.
// Everything in it is derived from the run's scope and its stored Session, which
// is what lets the outcome be posted from an instance that never saw the trigger.
func (h threadcaseHost) createContext(ctx context.Context, target threadcase.Target) (caseCreateReq, *model.Session, error) {
	session, err := h.session(ctx, target)
	if err != nil {
		return caseCreateReq{}, nil, err
	}
	entry, err := h.uc.deps.Registry.Get(target.WorkspaceID)
	if err != nil {
		return caseCreateReq{}, nil, goerr.Wrap(err,
			"resolve the workspace of a finished create turn",
			goerr.V("workspace_id", target.WorkspaceID))
	}
	return caseCreateReq{
		entry:         entry,
		caseChannel:   target.ChannelID,
		caseTS:        target.ThreadTS,
		uiChannel:     target.UIChannelID,
		uiTS:          target.UIThreadTS,
		reporter:      session.CreatorUserID,
		sourceChannel: session.ReactionSourceChannelID,
		sourceTS:      session.ReactionSourceMessageTS,
	}, session, nil
}

// session loads the Session a finished run belongs to.
func (h threadcaseHost) session(ctx context.Context, target threadcase.Target) (*model.Session, error) {
	ssn, err := h.uc.deps.Repo.Session().GetByThread(ctx, target.ChannelID, target.ThreadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "load the session of a finished turn",
			goerr.V("channel_id", target.ChannelID), goerr.V("thread_ts", target.ThreadTS))
	}
	if ssn == nil {
		return nil, goerr.New("the session of a finished turn is gone",
			goerr.V("channel_id", target.ChannelID), goerr.V("thread_ts", target.ThreadTS))
	}
	return ssn, nil
}

// runThreadCaseTurn dispatches one thread-mode turn to whichever runtime is
// wired. The durable runtime is preferred once it is bound: the run survives an
// instance restart and its decision is applied by the completion handler.
func (uc *AgentUseCase) runThreadCaseTurn(ctx context.Context, req threadcase.TurnRequest) (*threadcase.Result, error) {
	if uc.durableThreadcase != nil {
		return uc.durableThreadcase.StartTurn(ctx, req)
	}
	return uc.threadcase.RunTurn(ctx, req)
}

// threadCaseReady reports whether either thread-mode runtime can take a turn.
func (uc *AgentUseCase) threadCaseReady() bool {
	return uc.threadcase != nil || uc.durableThreadcase != nil
}
