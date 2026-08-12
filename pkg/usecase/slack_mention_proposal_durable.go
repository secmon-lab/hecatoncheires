package usecase

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

// proposalHost is the Slack side of a finished case-draft turn.
//
// It rebuilds the same slackDraftHandler the in-process path uses, from the
// Target and the stored Session rather than from a closure: the completion
// handler runs after the turn, possibly on another instance, where the values the
// spawning call held no longer exist.
type proposalHost struct {
	uc *MentionProposalUseCase
}

// Propose renders the finished draft into the preview the human reviews.
func (h proposalHost) Propose(ctx context.Context, target proposal.Target, m proposal.MaterializePayload) error {
	handler, session, err := h.handler(ctx, target)
	if err != nil {
		return err
	}
	return handler.Materialize(ctx, session, m)
}

// Ask posts the planner's question form.
func (h proposalHost) Ask(ctx context.Context, target proposal.Target, q proposal.QuestionPayload) error {
	handler, session, err := h.handler(ctx, target)
	if err != nil {
		return err
	}
	if err := handler.Question(ctx, session, q); err != nil {
		return err
	}
	// The form records itself on the Session in memory; persisting it is this
	// host's job. The in-process path got that for free — the runtime held the
	// same Session instance and wrote it when the turn ended — but here the run
	// has no instance to write, so a form left unsaved would be read back as
	// stale and the user's answer refused.
	session.UpdatedAt = time.Now().UTC()
	if err := h.uc.repo.Session().Put(ctx, session); err != nil {
		return goerr.Wrap(err, "persist the pending question",
			goerr.V("session_id", session.ID))
	}
	return nil
}

// ReportFallback removes the placeholder and tells the user the turn reached no
// conclusion, so nobody is left watching a "working on it" message forever.
func (h proposalHost) ReportFallback(ctx context.Context, target proposal.Target, reason string) error {
	if target.ProcessingTS != "" {
		h.uc.removeProcessingMessage(ctx, target.ChannelID, target.ProcessingTS)
	}
	h.uc.notifyDraftFallback(ctx, target.ChannelID, target.ThreadTS, reason)
	return nil
}

// handler rebuilds the draft handler for a finished run.
func (h proposalHost) handler(ctx context.Context, target proposal.Target) (*slackDraftHandler, *model.Session, error) {
	session, err := h.uc.repo.Session().GetByThread(ctx, target.ChannelID, target.ThreadTS)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "load the session of a finished draft turn",
			goerr.V("channel_id", target.ChannelID), goerr.V("thread_ts", target.ThreadTS))
	}
	if session == nil {
		return nil, nil, goerr.New("the session of a finished draft turn is gone",
			goerr.V("channel_id", target.ChannelID), goerr.V("thread_ts", target.ThreadTS))
	}
	if session.ProposalID == "" {
		return nil, nil, goerr.New("the session of a finished draft turn names no proposal",
			goerr.V("session_id", session.ID))
	}
	creator := session.CreatorUserID
	if creator == "" {
		creator = target.ActorUserID
	}
	// The candidate list is the whole registry: which workspaces were plausible was
	// the run's own judgement, and it has already made it.
	return newSlackDraftHandler(
		h.uc.repo, h.uc.registry, h.uc.slackService,
		target.ChannelID, target.ThreadTS, "", creator,
		h.uc.registry.List(), session.ProposalID,
		target.ProcessingTS, target.PreviewTS,
	), session, nil
}

// runDraftTurn dispatches one case-draft turn to whichever runtime is wired. The
// durable runtime is preferred once it is bound: the run survives an instance
// restart and its draft is delivered by the completion handler.
func (uc *MentionProposalUseCase) runDraftTurn(ctx context.Context, req proposal.TurnRequest) (*proposal.Result, error) {
	if uc.durableDraft != nil {
		return uc.durableDraft.StartTurn(ctx, req)
	}
	return uc.draftUC.RunTurn(ctx, req)
}

// draftReady reports whether either case-draft runtime can take a turn.
func (uc *MentionProposalUseCase) draftReady() bool {
	return uc.draftUC != nil || uc.durableDraft != nil
}

// BindDurableDraft wires the durable case-draft agent. It is a separate step from
// construction because registering the agent needs this usecase as its completion
// handler, and building the Kernel needs the filled registry.
func (uc *MentionProposalUseCase) BindDurableDraft(d *proposal.Durable) {
	if uc != nil {
		uc.durableDraft = d
	}
}

// DurableDraftHost returns this usecase as the case-draft completion handler.
func (uc *MentionProposalUseCase) DurableDraftHost() proposal.Host {
	return proposalHost{uc: uc}
}
