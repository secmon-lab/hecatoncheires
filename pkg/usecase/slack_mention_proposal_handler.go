package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// slackDraftHandler renders one finished case-draft turn into Slack. It is
// per-turn: the run's completion handler rebuilds a fresh slackDraftHandler from
// the Process metadata and the stored Session, calls Question or Materialize on
// it, and discards it. State that needs to outlast the turn (CaseProposal,
// Session) is persisted via repo.
type slackDraftHandler struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackService slacksvc.Service

	// Per-turn context.
	channelID   string
	threadTS    string
	candidates  []*model.WorkspaceEntry
	creatorUser string
	proposalID  model.CaseProposalID
	mentionTS   string

	// processingTS is the TS of the initial "⏳ Drafting…" placeholder
	// that HandleAppMention posts at mention time. On the mention
	// path, Materialize posts the preview as a fresh thread reply at
	// the bottom and collapses this placeholder into a short breadcrumb
	// pointing readers to the new preview.
	processingTS string
	// previewTS is set only on the workspace-switch path
	// (HandleSelectWorkspace) and carries the TS of the EXISTING
	// preview message the user clicked on. Materialize updates that
	// message in place so the switch reads as a same-position rewrite,
	// preserving the original UX where the preview "morphs" into the
	// new workspace's content. previewTS and processingTS are mutually
	// exclusive — at most one is set per turn.
	previewTS string
}

// newSlackDraftHandler builds a per-turn handler with the host context the
// finished turn needs. processingTS and previewTS are mutually exclusive:
//
//   - processingTS: mention path. The "⏳ Drafting…" placeholder TS;
//     Materialize collapses it into a breadcrumb after posting the
//     preview as a fresh thread reply at the bottom.
//   - previewTS: workspace-switch path. The TS of the existing preview
//     message the user clicked on; Materialize updates that message in
//     place so the switch reads as a same-position rewrite.
//
// Pass "" for both on paths that have no anchor (e.g. thread-reply
// resume, question-answer resume) — Materialize then just posts a
// fresh preview at the thread end.
func newSlackDraftHandler(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slacksvc.Service,
	channelID, threadTS, mentionTS, creatorUser string,
	candidates []*model.WorkspaceEntry,
	proposalID model.CaseProposalID,
	processingTS, previewTS string,
) *slackDraftHandler {
	return &slackDraftHandler{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		channelID:    channelID,
		threadTS:     threadTS,
		candidates:   candidates,
		creatorUser:  creatorUser,
		proposalID:   proposalID,
		mentionTS:    mentionTS,
		processingTS: processingTS,
		previewTS:    previewTS,
	}
}

// Question renders the planner's terminal question payload as a Block Kit
// form posted to the thread. Each item becomes an InputBlock with either
// radio_buttons (select) or checkboxes (multi_select), capped by a Submit
// button at the bottom. The question payload is mirrored onto the Session
// so the submit handler can label answers back against the original text
// even after the planner advances and rebuilds the surrounding messages.
func (h *slackDraftHandler) Question(ctx context.Context, ssn *model.Session, q proposal.QuestionPayload) error {
	// Mention the original requester in the form header so they get paged
	// the moment we ask. We pull the user from the Session (not h.creatorUser
	// alone) so resume-via-thread-reply paths surface the right person too.
	requester := h.creatorUser
	if ssn != nil && ssn.CreatorUserID != "" {
		requester = ssn.CreatorUserID
	}
	blocks, fallback := buildProposalQuestionBlocks(ctx, q, h.proposalID, requester)
	ts, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, fallback)
	if err != nil {
		return goerr.Wrap(err, "post draft question form",
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.threadTS),
		)
	}

	pq := &model.PendingQuestion{
		PostedChannelID: h.channelID,
		PostedMessageTS: ts,
		Reason:          q.Reason,
		Items:           make([]model.PendingQuestionItem, len(q.Items)),
	}
	for i, it := range q.Items {
		pq.Items[i] = model.PendingQuestionItem{
			ID: it.ID, Text: it.Text,
			Type:    string(it.Type),
			Options: append([]string(nil), it.Options...),
		}
	}
	ssn.PendingQuestion = pq
	return nil
}

// Materialize is the meat of the draft handler. The planner has decided to
// produce a CaseProposal preview; we validate the workspace, build the
// WorkspaceMaterialization (coercing planner JSON to typed FieldValues),
// persist via SetMaterialization, and post (or update in place) the preview
// Block Kit.
func (h *slackDraftHandler) Materialize(ctx context.Context, ssn *model.Session, m proposal.MaterializePayload) error {
	logger := logging.From(ctx)

	if h.registry == nil {
		return goerr.New("workspace registry is nil")
	}
	entry, err := h.registry.Get(m.WorkspaceID)
	if err != nil {
		return goerr.Wrap(err, "resolve materialize workspace", goerr.V("workspace_id", m.WorkspaceID))
	}

	mat := &model.WorkspaceMaterialization{
		Title:             m.Title,
		Description:       m.Description,
		IsTest:            m.IsTest,
		CustomFieldValues: map[string]model.FieldValue{},
	}
	if entry.FieldSchema != nil {
		defByID := make(map[string]config.FieldDefinition, len(entry.FieldSchema.Fields))
		for _, fd := range entry.FieldSchema.Fields {
			defByID[fd.ID] = fd
		}
		for fieldID, raw := range m.CustomFieldValues {
			fd, ok := defByID[fieldID]
			if !ok {
				// Field hallucinated outside schema — drop silently.
				continue
			}
			coerced, ok := coerceFieldValue(raw, fd.Type)
			if !ok {
				errutil.Handle(ctx, goerr.New("planner returned a value of unexpected type for field",
					goerr.V("field_id", fieldID),
					goerr.V("expected_type", fd.Type),
					goerr.V("raw_value", raw),
				), "draft handler: field coercion failed; skipping field")
				continue
			}
			mat.CustomFieldValues[fieldID] = model.FieldValue{
				FieldID: types.FieldID(fieldID),
				Type:    fd.Type,
				Value:   coerced,
			}
		}
	}

	// Persist with InferenceInProgress=false — the inference (planner +
	// sub-agents) has just completed for this materialize call.
	if err := h.repo.CaseProposal().SetMaterialization(ctx, h.proposalID, m.WorkspaceID, mat, false); err != nil {
		return goerr.Wrap(err, "persist materialization",
			goerr.V("proposal_id", string(h.proposalID)),
			goerr.V("workspace_id", m.WorkspaceID),
		)
	}

	// Reload draft so the preview reflects the just-persisted state and
	// holds any prior fields that should survive (RawMessages, etc.).
	d, err := h.repo.CaseProposal().Get(ctx, h.proposalID)
	if err != nil {
		return goerr.Wrap(err, "reload draft after materialize")
	}
	if d == nil {
		return goerr.New("draft missing after materialize", goerr.V("proposal_id", string(h.proposalID)))
	}

	blocks, fallback := buildPreviewBlocks(ctx, d, entry, h.candidates)

	processingTS := h.processingTS
	previewTS := h.previewTS

	var ts string
	switch {
	case previewTS != "":
		// Workspace-switch path: rewrite the preview the user clicked
		// on in place so the switch reads as a same-position morph.
		if err := h.slackService.UpdateMessage(ctx, h.channelID, previewTS, blocks, fallback); err != nil {
			return goerr.Wrap(err, "update preview in place on workspace switch",
				goerr.V("channel_id", h.channelID),
				goerr.V("preview_ts", previewTS),
			)
		}
		ts = previewTS
	default:
		// Mention / thread-reply path: post the preview as a fresh
		// thread reply so it sits chronologically AFTER the planner
		// trace messages (RegisterTasks / Trace / TraceRound) that
		// have already been posted during this turn. Slack orders
		// thread replies by their original `ts`; updating the
		// processing-placeholder TS in place (as the prior
		// implementation did) kept the preview pinned at the
		// mention-time position above every trace line.
		newTS, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, fallback)
		if err != nil {
			return goerr.Wrap(err, "post preview message",
				goerr.V("channel_id", h.channelID),
				goerr.V("thread_ts", h.threadTS),
			)
		}
		ts = newTS

		// Collapse the now-stale processing placeholder into a short
		// breadcrumb pointing readers to the freshly-posted preview
		// further down. The preview is already live, so a failed
		// update is non-fatal — the user just sees a "⏳ Drafting…"
		// stub that never advanced.
		if processingTS != "" {
			completedBlocks, completedFallback := buildProcessingCompletedBlocks(ctx)
			if err := h.slackService.UpdateMessage(ctx, h.channelID, processingTS, completedBlocks, completedFallback); err != nil {
				errutil.Handle(ctx, goerr.Wrap(err, "collapse processing placeholder after preview post",
					goerr.V("channel_id", h.channelID),
					goerr.V("processing_ts", processingTS),
				), "non-fatal: preview already posted")
			}
		}
	}

	// Persist ephemeral ref so interaction handlers (Submit/Edit/Cancel)
	// can locate the message for in-place updates.
	d.EphemeralChannelID = h.channelID
	d.EphemeralMessageTS = ts
	if err := h.repo.CaseProposal().Save(ctx, d); err != nil {
		return goerr.Wrap(err, "save draft with ephemeral ref")
	}

	// Persist the ProposalID on the Session so future thread replies / WS
	// switches can look up the draft from the session row.
	ssn.ProposalID = h.proposalID
	if err := h.repo.Session().Put(ctx, ssn); err != nil {
		errutil.Handle(ctx, err, "save session with draft id")
	}

	logger.Info("draft materialized via planner",
		"proposal_id", string(h.proposalID),
		"workspace_id", m.WorkspaceID,
		"channel_id", h.channelID,
	)
	return nil
}
