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
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// slackDraftHandler implements draft.Handler for the host (Slack)-side of
// the open-mode mention flow. It is per-mention: HandleAppMention builds a
// fresh slackDraftHandler, hands it to draft.UseCase.RunTurn, and discards
// it on return. State that needs to outlast the turn (CaseDraft, Session)
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
	draftID     model.CaseDraftID
	mentionTS   string

	// processingMu serialises trace writes so concurrent goroutines
	// (sub-agent fan-out, planner main thread) cannot interleave Slack
	// posts mid-event.
	processingMu sync.Mutex
	// processingTS is the TS of the initial "processing…" placeholder
	// that HandleAppMention posts at mention time. It is NOT used as a
	// rolling progress buffer; it is updated exactly once at
	// Materialize, when the placeholder is replaced with the preview
	// blocks. Phase-trace lines are posted as fresh thread replies
	// (see Trace) and never touch this TS.
	processingTS string
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
// draft runtime needs.
func newSlackDraftHandler(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slacksvc.Service,
	channelID, threadTS, mentionTS, creatorUser string,
	candidates []*model.WorkspaceEntry,
	draftID model.CaseDraftID,
	processingTS string,
) *slackDraftHandler {
	return &slackDraftHandler{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		channelID:    channelID,
		threadTS:     threadTS,
		candidates:   candidates,
		creatorUser:  creatorUser,
		draftID:      draftID,
		mentionTS:    mentionTS,
		processingTS: processingTS,
	}
}

// Question renders the planner's terminal question payload as a Block Kit
// form posted to the thread. Each item becomes an InputBlock with either
// radio_buttons (select) or checkboxes (multi_select), capped by a Submit
// button at the bottom. The question payload is mirrored onto the Session
// so the submit handler can label answers back against the original text
// even after the planner advances and rebuilds the surrounding messages.
func (h *slackDraftHandler) Question(ctx context.Context, ssn *model.Session, q draft.QuestionPayload) error {
	// Mention the original requester in the form header so they get paged
	// the moment we ask. We pull the user from the Session (not h.creatorUser
	// alone) so resume-via-thread-reply paths surface the right person too.
	requester := h.creatorUser
	if ssn != nil && ssn.CreatorUserID != "" {
		requester = ssn.CreatorUserID
	}
	blocks, fallback := buildDraftQuestionBlocks(q, h.draftID, requester)
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
// produce a CaseDraft preview; we validate the workspace, build the
// WorkspaceMaterialization (coercing planner JSON to typed FieldValues),
// persist via SetMaterialization, and post (or update in place) the preview
// Block Kit.
func (h *slackDraftHandler) Materialize(ctx context.Context, ssn *model.Session, m draft.MaterializePayload) error {
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
	if err := h.repo.CaseDraft().SetMaterialization(ctx, h.draftID, m.WorkspaceID, mat, false); err != nil {
		return goerr.Wrap(err, "persist materialization",
			goerr.V("draft_id", string(h.draftID)),
			goerr.V("workspace_id", m.WorkspaceID),
		)
	}

	// Reload draft so the preview reflects the just-persisted state and
	// holds any prior fields that should survive (RawMessages, etc.).
	d, err := h.repo.CaseDraft().Get(ctx, h.draftID)
	if err != nil {
		return goerr.Wrap(err, "reload draft after materialize")
	}
	if d == nil {
		return goerr.New("draft missing after materialize", goerr.V("draft_id", string(h.draftID)))
	}

	blocks, fallback := buildPreviewBlocks(d, entry, h.candidates)

	// Update the in-place processing message into the preview if we have
	// one; otherwise post a fresh message.
	h.processingMu.Lock()
	processingTS := h.processingTS
	h.processingMu.Unlock()

	var ts string
	if processingTS != "" {
		if err := h.slackService.UpdateMessage(ctx, h.channelID, processingTS, blocks, fallback); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "update processing message into preview"),
				"falling back to fresh thread post")
		} else {
			ts = processingTS
		}
	}
	if ts == "" {
		newTS, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, fallback)
		if err != nil {
			return goerr.Wrap(err, "post preview message",
				goerr.V("channel_id", h.channelID),
				goerr.V("thread_ts", h.threadTS),
			)
		}
		ts = newTS
	}

	// Persist ephemeral ref so interaction handlers (Submit/Edit/Cancel)
	// can locate the message for in-place updates.
	d.EphemeralChannelID = h.channelID
	d.EphemeralMessageTS = ts
	if err := h.repo.CaseDraft().Save(ctx, d); err != nil {
		return goerr.Wrap(err, "save draft with ephemeral ref")
	}

	// Persist the DraftID on the Session so future thread replies / WS
	// switches can look up the draft from the session row.
	ssn.DraftID = h.draftID
	if err := h.repo.Session().Put(ctx, ssn); err != nil {
		errutil.Handle(ctx, err, "save session with draft id")
	}

	logger.Info("draft materialized via planner",
		"draft_id", string(h.draftID),
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
func (h *slackDraftHandler) Trace(ctx context.Context, line string) {
	if line == "" {
		return
	}
	h.processingMu.Lock()
	defer h.processingMu.Unlock()
	blocks := buildTraceContextBlocks([]string{line})
	if _, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line); err != nil {
		logging.From(ctx).Error("post draft trace message",
			"error", err.Error(),
			"channel_id", h.channelID,
			"thread_ts", h.threadTS,
		)
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
func (h *slackDraftHandler) TraceRound(ctx context.Context, roundKey, line string) {
	if roundKey == "" || line == "" {
		return
	}
	h.processingMu.Lock()
	defer h.processingMu.Unlock()
	if h.roundTS == nil {
		h.roundTS = make(map[string]string)
	}
	blocks := buildTraceContextBlocks([]string{line})
	if ts, ok := h.roundTS[roundKey]; ok {
		if err := h.slackService.UpdateMessage(ctx, h.channelID, ts, blocks, line); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "update draft round trace",
				goerr.V("round_key", roundKey),
				goerr.V("ts", ts),
			), "draft handler: round trace update failed")
		}
		return
	}
	ts, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line)
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post draft round trace",
			goerr.V("round_key", roundKey),
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.threadTS),
		), "draft handler: round trace post failed")
		return
	}
	h.roundTS[roundKey] = ts
}

// RegisterTasks posts one fresh thread-reply message per task at the
// moment the investigation tasks are about to start. The TS of each
// post is remembered so subsequent TraceTask calls can update the
// matching message in place. Posting per task (rather than appending to
// a shared message) keeps the task block anchored at its original
// position in the thread, even after the phase-trace message grows or
// other tasks come and go below it. Calling with an empty slice is a
// no-op.
func (h *slackDraftHandler) RegisterTasks(ctx context.Context, tasks []draft.TaskInfo) {
	if len(tasks) == 0 {
		return
	}
	h.processingMu.Lock()
	defer h.processingMu.Unlock()
	if h.taskTS == nil {
		h.taskTS = make(map[string]string, len(tasks))
	}
	logger := logging.From(ctx)
	for _, ti := range tasks {
		if ti.ID == "" {
			continue
		}
		if _, exists := h.taskTS[ti.ID]; exists {
			// Re-registration is treated as a no-op; the existing
			// task message keeps whatever state TraceTask last set.
			continue
		}
		line := i18n.T(ctx, i18n.MsgDraftTraceTaskPending, ti.Title)
		blocks := buildTraceContextBlocks([]string{line})
		ts, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, line)
		if err != nil {
			logger.Error("post draft task block",
				"error", err.Error(),
				"task_id", ti.ID,
				"channel_id", h.channelID,
				"thread_ts", h.threadTS,
			)
			continue
		}
		h.taskTS[ti.ID] = ts
	}
}

// TraceTask updates the dedicated Slack message a previously-registered
// task occupies. An unknown taskID is dropped silently — the sub-agent
// has no business posting fresh blocks (block creation is the parent's
// contract via RegisterTasks), and a wrong ID typically means a bug we
// want to fail quietly rather than spam Slack with orphan rows.
func (h *slackDraftHandler) TraceTask(ctx context.Context, taskID, line string) {
	if taskID == "" || line == "" {
		return
	}
	h.processingMu.Lock()
	defer h.processingMu.Unlock()
	ts, ok := h.taskTS[taskID]
	if !ok {
		return
	}
	blocks := buildTraceContextBlocks([]string{line})
	if err := h.slackService.UpdateMessage(ctx, h.channelID, ts, blocks, line); err != nil {
		logging.From(ctx).Error("update draft task block",
			"error", err.Error(),
			"task_id", taskID,
			"ts", ts,
		)
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

// Compile-time assertion: slackDraftHandler satisfies draft.Handler.
var _ draft.Handler = (*slackDraftHandler)(nil)
