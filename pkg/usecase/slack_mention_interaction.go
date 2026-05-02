package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	goslack "github.com/slack-go/slack"
)

// SlackCallbackIDDraftEdit is the modal callback_id for the Case-draft Edit
// modal opened from the preview ephemeral.
const SlackCallbackIDDraftEdit = "mention_draft_edit_modal"

// editMetadata is serialized into the modal's PrivateMetadata so we can recover
// the originating draft on view_submission.
type editMetadata struct {
	DraftID            string `json:"draft_id"`
	WorkspaceID        string `json:"workspace_id"`
	EphemeralChannelID string `json:"ephemeral_channel_id"`
	EphemeralMessageTS string `json:"ephemeral_message_ts"`
	ResponseURL        string `json:"response_url"`
}

// HandleSelectWorkspace runs when the user changes the workspace selector on
// the preview ephemeral. It enforces the lock-first ordering described in
// F4-3 to prevent double-submission.
func (uc *MentionDraftUseCase) HandleSelectWorkspace(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	// The static_select carries the workspace ID in SelectedOption.Value.
	// The draft ID is encoded into the selector block's BlockID as
	// "<BlockIDDraftWSSelect>:<draftID>" by buildPreviewBlocks.
	draftID, ok := parseDraftIDFromSelectorBlockID(action.BlockID)
	if !ok {
		return goerr.New("workspace selector BlockID is missing draft ID",
			goerr.V("block_id", action.BlockID))
	}
	draft, err := uc.repo.CaseDraft().Get(ctx, draftID)
	if err != nil {
		return goerr.Wrap(err, "failed to load draft for workspace switch",
			goerr.V("draft_id", draftID))
	}
	if draft == nil {
		return goerr.New("draft not found for workspace switch",
			goerr.V("draft_id", draftID))
	}

	if draft.InferenceInProgress {
		uc.respondLocked(ctx, callback)
		return nil
	}

	newWorkspaceID := action.SelectedOption.Value
	if newWorkspaceID == "" {
		return goerr.New("workspace selector returned empty value")
	}
	entry, err := uc.registry.Get(newWorkspaceID)
	if err != nil {
		return goerr.Wrap(err, "selected workspace not found")
	}

	// (1) Lock state first — must complete before AI inference begins.
	lockBlocks, lockFallback := buildLockBlocks(entry.Workspace.Name)
	if err := uc.respondReplaceOriginal(ctx, callback.ResponseURL, lockBlocks, lockFallback); err != nil {
		return goerr.Wrap(err, "failed to render lock state on workspace switch")
	}

	// (2) Mark inference in progress so concurrent interactions can refuse.
	if err := uc.repo.CaseDraft().SetMaterialization(ctx, draft.ID, newWorkspaceID, nil, true); err != nil {
		return goerr.Wrap(err, "failed to mark inference in progress")
	}

	// (3) Run materialization for the new workspace's schema (retries internally).
	candidates := uc.accessibleWorkspaces(callback.User.ID)
	mat, err := uc.materializer.Materialize(ctx, draft, MaterializeContext{
		Workspace:        entry,
		EstimationReason: "user explicitly switched to this workspace via the preview selector",
		OtherCandidates:  candidates,
	})
	if err != nil {
		// Surface the failure to the user by replacing the locked ephemeral
		// with an error message instead of leaving them stuck in the
		// "processing…" state.
		errBlocks, errFallback := buildMaterializationErrorBlocks(entry.Workspace.Name)
		if respErr := uc.respondReplaceOriginal(ctx, callback.ResponseURL, errBlocks, errFallback); respErr != nil {
			errutil.Handle(ctx, goerr.Wrap(respErr, "failed to render materialization-failure block"),
				"could not surface materialization failure to user")
		}
		// Clear the in-progress flag so the user can try switching again.
		if clearErr := uc.repo.CaseDraft().SetMaterialization(ctx, draft.ID, draft.SelectedWorkspaceID, draft.Materialization, false); clearErr != nil {
			errutil.Handle(ctx, clearErr, "failed to clear inference-in-progress flag after materialization failure")
		}
		return goerr.Wrap(err, "failed to materialize for new workspace")
	}

	// (4) Persist the new materialization and clear the lock.
	if err := uc.repo.CaseDraft().SetMaterialization(ctx, draft.ID, newWorkspaceID, mat, false); err != nil {
		return goerr.Wrap(err, "failed to save new materialization")
	}

	// (5) Re-render the preview body.
	draft.SelectedWorkspaceID = newWorkspaceID
	draft.Materialization = mat
	draft.InferenceInProgress = false
	blocks, fallback := buildPreviewBlocks(draft, entry, candidates)
	if err := uc.respondReplaceOriginal(ctx, callback.ResponseURL, blocks, fallback); err != nil {
		return goerr.Wrap(err, "failed to render new preview after workspace switch")
	}
	return nil
}

// HandleSubmit creates the Case using the current materialization and posts
// the completion notification.
func (uc *MentionDraftUseCase) HandleSubmit(ctx context.Context, caseUC *CaseUseCase, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	if caseUC == nil {
		return goerr.New("CaseUseCase is required for Submit")
	}

	draft, err := uc.locateDraftFromCallback(ctx, callback)
	if err != nil {
		return err
	}
	if draft.InferenceInProgress {
		uc.respondLocked(ctx, callback)
		return nil
	}
	if draft.Materialization == nil {
		return goerr.New("draft has no materialization to submit", goerr.V("draft_id", draft.ID))
	}

	// Brief lock during creation to prevent double-submit.
	lockBlocks, _ := buildSubmittingBlocks()
	if err := uc.respondReplaceOriginal(ctx, callback.ResponseURL, lockBlocks, "Creating case…"); err != nil {
		errutil.Handle(ctx, err, "failed to render submitting state")
	}

	mat := draft.Materialization
	created, err := caseUC.CreateCase(
		ctx,
		draft.SelectedWorkspaceID,
		mat.Title,
		mat.Description,
		[]string{callback.User.ID},
		mat.CustomFieldValues,
		false,
		callback.Team.ID,
		uuid.New().String(),
	)
	if err != nil {
		// Re-render preview so user can retry / Edit.
		entry, getErr := uc.registry.Get(draft.SelectedWorkspaceID)
		if getErr == nil {
			candidates := uc.accessibleWorkspaces(callback.User.ID)
			blocks, fallback := buildPreviewBlocks(draft, entry, candidates)
			_ = uc.respondReplaceOriginal(ctx, callback.ResponseURL, blocks, fallback+" (creation failed; please use Edit to fill required fields)")
		}
		return goerr.Wrap(err, "failed to create case from draft",
			goerr.V("draft_id", draft.ID),
			goerr.V("workspace_id", draft.SelectedWorkspaceID))
	}

	entry, _ := uc.registry.Get(draft.SelectedWorkspaceID)
	uc.updatePreviewWithCreated(ctx, draft.EphemeralChannelID, draft.EphemeralMessageTS, entry, created)

	if err := uc.repo.CaseDraft().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft after submit")
	}
	return nil
}

// HandleEdit opens the dynamic Edit modal for the currently-selected workspace.
func (uc *MentionDraftUseCase) HandleEdit(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	draft, err := uc.locateDraftFromCallback(ctx, callback)
	if err != nil {
		return err
	}
	if draft.InferenceInProgress {
		uc.respondLocked(ctx, callback)
		return nil
	}

	entry, err := uc.registry.Get(draft.SelectedWorkspaceID)
	if err != nil {
		return goerr.Wrap(err, "selected workspace not found for Edit")
	}

	meta := editMetadata{
		DraftID:            string(draft.ID),
		WorkspaceID:        draft.SelectedWorkspaceID,
		EphemeralChannelID: draft.EphemeralChannelID,
		EphemeralMessageTS: draft.EphemeralMessageTS,
		ResponseURL:        callback.ResponseURL,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return goerr.Wrap(err, "failed to marshal edit metadata")
	}

	view := buildDraftEditModal(entry, draft.Materialization, string(metaJSON))
	if err := uc.slackService.OpenView(ctx, callback.TriggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open draft edit modal")
	}
	return nil
}

// HandleCancel marks the draft preview as canceled in place: the existing
// blocks are kept (so the conversation has a record of what was drafted),
// the action buttons are stripped, and a "Canceled" tail is appended. The
// underlying draft is deleted from the repository.
func (uc *MentionDraftUseCase) HandleCancel(ctx context.Context, callback *goslack.InteractionCallback, _ *goslack.BlockAction) error {
	if callback == nil {
		return goerr.New("nil callback")
	}
	draft, err := uc.locateDraftFromCallback(ctx, callback)
	if err != nil {
		// Draft already gone — best-effort: just append a canceled tail.
		_ = uc.respondAppendCanceledTail(ctx, callback)
		return nil
	}

	if err := uc.respondAppendCanceledTail(ctx, callback); err != nil {
		errutil.Handle(ctx, err, "failed to render canceled tail")
	}
	if err := uc.repo.CaseDraft().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft on cancel")
	}
	return nil
}

// respondAppendCanceledTail rebuilds the original message minus the action
// buttons and appends a "Canceled" context block at the end, then sends the
// result via response_url replace_original.
func (uc *MentionDraftUseCase) respondAppendCanceledTail(ctx context.Context, callback *goslack.InteractionCallback) error {
	original := callback.Message.Blocks.BlockSet
	kept := make([]goslack.Block, 0, len(original))
	for _, b := range original {
		if ab, ok := b.(*goslack.ActionBlock); ok && ab.BlockID == BlockIDDraftActions {
			continue
		}
		kept = append(kept, b)
	}
	kept = append(kept,
		goslack.NewDividerBlock(),
		goslack.NewContextBlock(
			"mention_draft_canceled",
			goslack.NewTextBlockObject(goslack.MarkdownType, "❌ *Canceled*", false, false),
		),
	)
	return uc.respondReplaceOriginal(ctx, callback.ResponseURL, kept, "Case draft canceled")
}

// HandleEditSubmit processes the view_submission for the Edit modal.
func (uc *MentionDraftUseCase) HandleEditSubmit(ctx context.Context, caseUC *CaseUseCase, callback *goslack.InteractionCallback) error {
	if callback == nil {
		return goerr.New("nil callback")
	}
	if caseUC == nil {
		return goerr.New("CaseUseCase is required for Edit submit")
	}

	var meta editMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return goerr.Wrap(err, "failed to parse edit metadata")
	}

	draft, err := uc.repo.CaseDraft().Get(ctx, model.CaseDraftID(meta.DraftID))
	if err != nil {
		return goerr.Wrap(err, "draft not found for edit submit", goerr.V("draft_id", meta.DraftID))
	}

	blockValues := callback.View.State.Values

	title := readPlainInput(blockValues, blockIDDraftEditTitle, actionIDDraftEditTitle)
	description := readPlainInput(blockValues, blockIDDraftEditDescription, actionIDDraftEditDescription)
	fieldValues := extractFieldValues(blockValues)

	created, err := caseUC.CreateCase(
		ctx,
		meta.WorkspaceID,
		title,
		description,
		[]string{callback.User.ID},
		fieldValues,
		false,
		callback.Team.ID,
		uuid.New().String(),
	)
	if err != nil {
		return goerr.Wrap(err, "failed to create case from edit modal",
			goerr.V("draft_id", meta.DraftID))
	}

	entry, _ := uc.registry.Get(meta.WorkspaceID)
	uc.updatePreviewWithCreated(ctx, meta.EphemeralChannelID, meta.EphemeralMessageTS, entry, created)

	if err := uc.repo.CaseDraft().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft after edit submit")
	}
	return nil
}

// --- helpers ---

// locateDraftFromCallback finds the draft associated with a button-based
// interaction (Submit/Edit/Cancel). Each button carries draft.ID in
// action.Value. For the workspace static_select, use parseDraftIDFromSelectorBlockID
// against the selector's BlockID instead.
func (uc *MentionDraftUseCase) locateDraftFromCallback(ctx context.Context, callback *goslack.InteractionCallback) (*model.CaseDraft, error) {
	for _, a := range callback.ActionCallback.BlockActions {
		if a.Value != "" {
			if d, err := uc.repo.CaseDraft().Get(ctx, model.CaseDraftID(a.Value)); err == nil {
				return d, nil
			}
		}
	}
	return nil, goerr.New("could not resolve draft from interaction callback")
}

// parseDraftIDFromSelectorBlockID parses the draft ID encoded into the
// workspace selector's BlockID by buildPreviewBlocks (format:
// "<BlockIDDraftWSSelect>:<draftID>").
func parseDraftIDFromSelectorBlockID(blockID string) (model.CaseDraftID, bool) {
	prefix := BlockIDDraftWSSelect + ":"
	if !strings.HasPrefix(blockID, prefix) {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(blockID, prefix))
	if id == "" {
		return "", false
	}
	return model.CaseDraftID(id), true
}

// respondReplaceOriginal POSTs to the interaction's response_url to replace
// the original message with new blocks. Works for both regular thread
// messages and ephemerals; we no longer post ephemerals, but the API path
// is the same.
func (uc *MentionDraftUseCase) respondReplaceOriginal(ctx context.Context, responseURL string, blocks []goslack.Block, fallback string) error {
	if responseURL == "" {
		return goerr.New("response_url is empty")
	}
	body := map[string]any{
		"replace_original": true,
		"text":             fallback,
		"blocks":           blocks,
	}
	return postJSON(ctx, responseURL, body)
}

func (uc *MentionDraftUseCase) respondLocked(ctx context.Context, callback *goslack.InteractionCallback) {
	body := map[string]any{
		"replace_original": false,
		"response_type":    "ephemeral",
		"text":             "Inference is in progress; please wait a moment.",
	}
	if err := postJSON(ctx, callback.ResponseURL, body); err != nil {
		errutil.Handle(ctx, err, "failed to notify user of in-progress inference")
	}
}

func postJSON(ctx context.Context, url string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return goerr.Wrap(err, "failed to marshal response body")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return goerr.Wrap(err, "failed to build response_url request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return goerr.Wrap(err, "failed to POST response_url")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return goerr.New("response_url returned non-2xx",
			goerr.V("status", resp.StatusCode),
			goerr.V("body", string(respBody)),
		)
	}
	return nil
}

// updatePreviewWithCreated rewrites the original preview thread message in
// place (via chat.update) with the post-create state. response_url's
// replace_original is unreliable for thread replies, so we always use
// chat.update against the channel/ts captured when the preview was first
// posted by HandleAppMention.
func (uc *MentionDraftUseCase) updatePreviewWithCreated(ctx context.Context, channelID, messageTS string, entry *model.WorkspaceEntry, created *model.Case) {
	if channelID == "" || messageTS == "" || uc.slackService == nil {
		return
	}
	blocks, fallback := buildCreatedBlocks(entry, created)
	if err := uc.slackService.UpdateMessage(ctx, channelID, messageTS, blocks, fallback); err != nil {
		errutil.Handle(ctx, err, "failed to update preview message after case creation")
	}
}

func buildSubmittingBlocks() ([]goslack.Block, string) {
	ctxBlock := goslack.NewContextBlock(
		"mention_draft_submit_ctx",
		goslack.NewTextBlockObject(goslack.MarkdownType, "Creating case…", false, false),
	)
	return []goslack.Block{ctxBlock}, "Creating case…"
}

func readPlainInput(blockValues map[string]map[string]goslack.BlockAction, blockID, actionID string) string {
	block, ok := blockValues[blockID]
	if !ok {
		return ""
	}
	action, ok := block[actionID]
	if !ok {
		return ""
	}
	return action.Value
}
