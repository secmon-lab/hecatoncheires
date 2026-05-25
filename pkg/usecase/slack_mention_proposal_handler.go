package usecase

import (
	"context"
	"sync"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	agentcommon "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// slackDraftHandler implements proposal.Handler for the host (Slack)-side of
// the open-mode mention flow. It is per-mention: HandleAppMention builds a
// fresh slackDraftHandler, hands it to proposal.UseCase.RunTurn, and discards
// it on return. State that needs to outlast the turn (CaseProposal, Session)
// is persisted via repo.
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

	// processingMu serialises trace writes so concurrent goroutines
	// (sub-agent fan-out, planner main thread) cannot interleave Slack
	// posts mid-event.
	processingMu sync.Mutex
	// processingTS is the TS of the initial "⏳ Drafting…" placeholder
	// that HandleAppMention posts at mention time. On the mention
	// path, Materialize posts the preview as a fresh thread reply at
	// the bottom (so it sits AFTER the planner trace messages) and
	// collapses this placeholder into a short breadcrumb pointing
	// readers to the new preview. Phase-trace lines are posted as
	// fresh thread replies (see Trace) and never touch this TS.
	processingTS string
	// previewTS is set only on the workspace-switch path
	// (HandleSelectWorkspace) and carries the TS of the EXISTING
	// preview message the user clicked on. Materialize updates that
	// message in place so the switch reads as a same-position rewrite,
	// preserving the original UX where the preview "morphs" into the
	// new workspace's content. previewTS and processingTS are mutually
	// exclusive — at most one is set per turn.
	previewTS string
	// taskTS holds the Slack message TS for each registered task. Each
	// task is posted as its own thread reply so that subsequent
	// per-task updates land in place — the message stays anchored at
	// its registration position even as later phase-trace lines or
	// other task messages are appended below.
	taskTS map[string]string
	// roundTS holds the Slack message TS reserved for each planner
	// round (keyed by the runtime's roundKey, e.g. "plan-1"). The
	// first TraceRound call for a key posts a fresh thread reply and
	// stores the TS here; subsequent calls with the same key REPLACE
	// that message in place via UpdateMessage, so the
	// "Planning… → retry → Planning… → action" sequence collapses to
	// one self-updating context block.
	roundTS map[string]string
}

// newSlackDraftHandler builds a per-turn handler with the host context the
// draft runtime needs. processingTS and previewTS are mutually exclusive:
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
	blocks, fallback := buildProposalQuestionBlocks(q, h.proposalID, requester)
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

	blocks, fallback := buildPreviewBlocks(d, entry, h.candidates)

	h.processingMu.Lock()
	processingTS := h.processingTS
	previewTS := h.previewTS
	h.processingMu.Unlock()

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

// Trace posts one phase-level progress line as a fresh thread reply.
// Each call creates a new Slack message rather than appending to a
// shared progress buffer, so the trace renders as a vertical timeline
// in the thread — no single context block grows over time and shoves
// later content (task blocks, the question form, the preview) around.
// Per-task transitions live in their own Slack messages (see
// RegisterTasks / TraceTask).
//
// Trace does not touch any shared state on h, so it deliberately runs
// outside processingMu — a slow Slack post must not block parallel
// sub-agent TraceTask calls.
func (h *slackDraftHandler) Trace(ctx context.Context, line string) {
	if line == "" {
		return
	}
	blocks := buildTraceContextBlocks([]string{line})
	if _, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post draft trace message",
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.threadTS),
		), "post draft trace message failed")
	}
}

// TraceRound posts a fresh thread reply on the first call for a given
// roundKey, and replaces the prior content of that message in place on
// every subsequent call with the same key. This collapses transient
// state inside one planner round
// ("Planning… → retry → Planning… → action", plus per-tool-call
// transitions surfaced by the planner middleware) into a single
// self-updating context block, instead of stacking them as separate
// thread replies.
//
// processingMu protects h.roundTS only; the Slack API call runs
// outside the lock so a slow post / update never blocks parallel
// sub-agent TraceTask updates. TraceRound is called from the planner
// goroutine alone (planner main + middleware), so no two TraceRound
// calls for the same roundKey can race against the post-then-store
// gap below.
func (h *slackDraftHandler) TraceRound(ctx context.Context, roundKey, line string) {
	if roundKey == "" || line == "" {
		return
	}
	h.processingMu.Lock()
	if h.roundTS == nil {
		h.roundTS = make(map[string]string)
	}
	ts, exists := h.roundTS[roundKey]
	h.processingMu.Unlock()

	blocks := buildTraceContextBlocks([]string{line})
	if exists {
		if err := h.slackService.UpdateMessage(ctx, h.channelID, ts, blocks, line); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "update draft round trace",
				goerr.V("round_key", roundKey),
				goerr.V("ts", ts),
			), "draft handler: round trace update failed")
		}
		return
	}
	newTS, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post draft round trace",
			goerr.V("round_key", roundKey),
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.threadTS),
		), "draft handler: round trace post failed")
		return
	}
	h.processingMu.Lock()
	h.roundTS[roundKey] = newTS
	h.processingMu.Unlock()
}

// RegisterTasks posts one fresh thread-reply message per task at the
// moment the investigation tasks are about to start. The TS of each
// post is remembered so subsequent TraceTask calls can update the
// matching message in place. Posting per task (rather than appending to
// a shared message) keeps the task block anchored at its original
// position in the thread, even after the phase-trace message grows or
// other tasks come and go below it. Calling with an empty slice is a
// no-op.
//
// processingMu protects h.taskTS only; each Slack post runs outside
// the lock so a slow Slack does not block parallel sub-agent
// TraceTask updates. RegisterTasks is the host's contract from the
// planner goroutine and runs to completion BEFORE any sub-agent is
// spawned (see runInvestigationsParallel), so the post-then-store gap
// below cannot race against TraceTask reads of taskTS.
func (h *slackDraftHandler) RegisterTasks(ctx context.Context, tasks []proposal.TaskInfo) {
	if len(tasks) == 0 {
		return
	}
	for _, ti := range tasks {
		if ti.ID == "" {
			continue
		}
		h.processingMu.Lock()
		if h.taskTS == nil {
			h.taskTS = make(map[string]string, len(tasks))
		}
		_, exists := h.taskTS[ti.ID]
		h.processingMu.Unlock()
		if exists {
			// Re-registration is treated as a no-op; the existing
			// task message keeps whatever state TraceTask last set.
			continue
		}

		line := i18n.T(ctx, i18n.MsgProposalTraceTaskPending, ti.Title)
		blocks := buildTraceContextBlocks([]string{line})
		ts, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "post draft task block",
				goerr.V("task_id", ti.ID),
				goerr.V("channel_id", h.channelID),
				goerr.V("thread_ts", h.threadTS),
			), "post draft task block failed")
			continue
		}
		h.processingMu.Lock()
		h.taskTS[ti.ID] = ts
		h.processingMu.Unlock()
	}
}

// TraceTask updates the dedicated Slack message a previously-registered
// task occupies. An unknown taskID is dropped silently — the sub-agent
// has no business posting fresh blocks (block creation is the parent's
// contract via RegisterTasks), and a wrong ID typically means a bug we
// want to fail quietly rather than spam Slack with orphan rows.
//
// processingMu protects only the taskTS lookup; the Slack
// UpdateMessage call runs outside the lock so a slow Slack does not
// block parallel sub-agents. Each task ID is owned by one sub-agent
// goroutine, so successive TraceTask calls for the same ID are
// already serialised by Go's goroutine semantics — there is no
// in-flight reorder risk to guard against.
func (h *slackDraftHandler) TraceTask(ctx context.Context, taskID, line string) {
	if taskID == "" || line == "" {
		return
	}
	h.processingMu.Lock()
	ts, ok := h.taskTS[taskID]
	h.processingMu.Unlock()
	if !ok {
		return
	}
	blocks := buildTraceContextBlocks([]string{line})
	if err := h.slackService.UpdateMessage(ctx, h.channelID, ts, blocks, line); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "update draft task block",
			goerr.V("task_id", taskID),
			goerr.V("ts", ts),
		), "update draft task block failed")
	}
}

// PostBusy notifies the user that a previous turn is still running.
func (h *slackDraftHandler) PostBusy(ctx context.Context, _ *model.Session, _ agentcommon.BusyInfo) error {
	text := i18n.T(ctx, i18n.MsgKeyAgentBusy)
	if _, err := h.slackService.PostThreadReply(ctx, h.channelID, h.threadTS, text); err != nil {
		return goerr.Wrap(err, "post draft busy reply")
	}
	return nil
}

// Compile-time assertion: slackDraftHandler satisfies proposal.Handler.
var _ proposal.Handler = (*slackDraftHandler)(nil)
