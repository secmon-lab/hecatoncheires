package usecase

import (
	"context"
	"fmt"
	"strings"
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

	// processingMu guards the ts of the running progress message we
	// update on each Trace tick. Concurrent sub-agent traces all funnel
	// through Trace and we update the same Slack message in place.
	processingMu sync.Mutex
	processingTS string
	traceLines   []string
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

// PostMessage replies to the thread with a plain text message.
func (h *slackDraftHandler) PostMessage(ctx context.Context, _ *model.Session, text string) error {
	if _, err := h.slackService.PostThreadReply(ctx, h.channelID, h.threadTS, text); err != nil {
		return goerr.Wrap(err, "post thread reply",
			goerr.V("channel_id", h.channelID),
			goerr.V("thread_ts", h.threadTS),
		)
	}
	return nil
}

// PostQuestion replies with a question. Options, when present, are listed
// inline as a bullet list — Slack interactive components for these are out
// of scope for the first cut.
func (h *slackDraftHandler) PostQuestion(ctx context.Context, ssn *model.Session, q draft.QuestionPayload) error {
	body := q.Text
	if len(q.Options) > 0 {
		var b strings.Builder
		b.WriteString(q.Text)
		b.WriteString("\n")
		for _, opt := range q.Options {
			fmt.Fprintf(&b, "• %s\n", opt)
		}
		body = b.String()
	}
	return h.PostMessage(ctx, ssn, body)
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

// Trace appends one progress line to a per-turn Slack context block message
// and updates it in place. The first call posts a new message; subsequent
// calls update.
func (h *slackDraftHandler) Trace(ctx context.Context, line string) {
	h.processingMu.Lock()
	defer h.processingMu.Unlock()

	h.traceLines = append(h.traceLines, line)
	blocks := buildTraceContextBlocks(h.traceLines)
	fallback := strings.Join(h.traceLines, "\n")
	logger := logging.From(ctx)

	if h.processingTS == "" {
		ts, err := h.slackService.PostThreadMessage(ctx, h.channelID, h.threadTS, blocks, fallback)
		if err != nil {
			logger.Error("post draft trace message", "error", err.Error())
			return
		}
		h.processingTS = ts
	} else {
		if err := h.slackService.UpdateMessage(ctx, h.channelID, h.processingTS, blocks, fallback); err != nil {
			logger.Error("update draft trace message", "error", err.Error())
		}
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
