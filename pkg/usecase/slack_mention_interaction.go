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
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	goslack "github.com/slack-go/slack"
)

// SlackCallbackIDDraftEdit is the modal callback_id for the Case-draft Edit
// modal opened from the preview ephemeral.
const SlackCallbackIDDraftEdit = "mention_draft_edit_modal"

// editMetadata is serialized into the modal's PrivateMetadata so we can recover
// the originating draft on view_submission.
type editMetadata struct {
	ProposalID         string `json:"proposal_id"`
	WorkspaceID        string `json:"workspace_id"`
	EphemeralChannelID string `json:"ephemeral_channel_id"`
	EphemeralMessageTS string `json:"ephemeral_message_ts"`
	ResponseURL        string `json:"response_url"`
}

// HandleSelectWorkspace runs when the user changes the workspace selector on
// the preview ephemeral. It re-routes the request through proposal.UseCase.RunTurn
// with a TriggerWSSwitch trigger; the planner re-materialises against the new
// workspace's schema using the existing conversation history. The lock-first
// ordering of F4-3 is preserved by setting InferenceInProgress before the turn
// starts so concurrent interactions (Submit/Edit/Cancel) refuse.
func (uc *MentionProposalUseCase) HandleSelectWorkspace(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
	if callback == nil || action == nil {
		return goerr.New("nil callback or action")
	}
	if uc.draftUC == nil {
		return goerr.New("draft usecase is not configured")
	}
	proposalID, ok := parseProposalIDFromSelectorBlockID(action.BlockID)
	if !ok {
		return goerr.New("workspace selector BlockID is missing draft ID",
			goerr.V("block_id", action.BlockID))
	}
	d, err := uc.repo.CaseProposal().Get(ctx, proposalID)
	if err != nil {
		return goerr.Wrap(err, "failed to load draft for workspace switch",
			goerr.V("proposal_id", proposalID))
	}
	if d == nil {
		return goerr.New("draft not found for workspace switch",
			goerr.V("proposal_id", proposalID))
	}

	if d.InferenceInProgress {
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

	// (1) Lock the preview UI immediately. The user sees a "materializing…"
	// row while the planner runs.
	lockBlocks, lockFallback := buildLockBlocks(entry.Workspace.Name)
	if err := uc.respondReplaceOriginal(ctx, callback.ResponseURL, lockBlocks, lockFallback); err != nil {
		return goerr.Wrap(err, "failed to render lock state on workspace switch")
	}

	// (2) Mark inference in progress so concurrent interactions refuse.
	if err := uc.repo.CaseProposal().SetMaterialization(ctx, d.ID, newWorkspaceID, nil, true); err != nil {
		return goerr.Wrap(err, "failed to mark inference in progress")
	}

	// (3) Look up the existing Session for this thread.
	session, err := uc.repo.Session().GetByThread(ctx, d.Source.ChannelID, d.EphemeralMessageTS)
	if err != nil {
		return goerr.Wrap(err, "failed to load session for ws-switch")
	}
	if session == nil {
		// No session yet — fall back to thread TS from the draft source.
		threadTS := d.Source.ThreadTS
		if threadTS == "" {
			threadTS = d.Source.MentionTS
		}
		session, err = uc.repo.Session().GetByThread(ctx, d.Source.ChannelID, threadTS)
		if err != nil {
			return goerr.Wrap(err, "failed to load session via thread TS")
		}
	}
	if session == nil {
		return goerr.New("no session found for ws-switch",
			goerr.V("proposal_id", string(d.ID)))
	}

	candidates := uc.accessibleWorkspaces(callback.User.ID)
	threadTS := session.ThreadTS

	// (4) Build host handler. previewTS carries the TS of the existing
	// preview the user clicked on; Materialize will UpdateMessage that
	// row in place so the workspace switch reads as a same-position
	// morph rather than a fresh post at the thread end. processingTS
	// is empty — there is no "⏳ Drafting…" placeholder on this path.
	handler := newSlackDraftHandler(
		uc.repo, uc.registry, uc.slackService,
		d.Source.ChannelID, threadTS, "", callback.User.ID,
		candidates, d.ID, "", d.EphemeralMessageTS,
	)

	// (5) Run the planner turn. TriggerTS is empty for this synthetic event
	// — there is no Slack-side TS to dedup on. The lock layer treats an
	// empty TriggerKey as "always proceed (or busy)", which is what we want
	// for explicit user clicks.
	//
	// The synthetic user input names the workspace explicitly so the planner
	// resumes against the new workspace without re-running its own selection
	// — matches the "Trigger context" branch in the planner prompt.
	userInput := "[system event] The user has switched the active workspace to " + entry.Workspace.ID + "."
	result, runErr := uc.draftUC.RunTurn(ctx, proposal.TurnRequest{
		Session:          session,
		UserInput:        userInput,
		Trigger:          proposal.TriggerWSSwitch,
		TriggerTS:        "",
		ActorUserID:      callback.User.ID,
		ExistingProposal: d,
		Handler:          handler,
	})
	if runErr != nil {
		errBlocks, errFallback := buildMaterializationErrorBlocks(entry.Workspace.Name)
		if respErr := uc.respondReplaceOriginal(ctx, callback.ResponseURL, errBlocks, errFallback); respErr != nil {
			errutil.Handle(ctx, goerr.Wrap(respErr, "failed to render materialization-failure block"),
				"could not surface materialization failure to user")
		}
		// Best effort: clear the in-progress flag so the user can retry.
		if clearErr := uc.repo.CaseProposal().SetMaterialization(ctx, d.ID, d.SelectedWorkspaceID, d.Materialization, false); clearErr != nil {
			errutil.Handle(ctx, clearErr, "failed to clear inference-in-progress flag after ws-switch failure")
		}
		return goerr.Wrap(runErr, "ws-switch turn failed")
	}
	switch result.Status {
	case proposal.StatusBusy, proposal.StatusIdempotent:
		// Locked / duplicate — the lock UI we already posted is the user
		// signal; nothing more to do.
		return nil
	case proposal.StatusFallback:
		// Planner exhausted budget — render an error block so the user
		// isn't stuck on the locked preview.
		errBlocks, errFallback := buildMaterializationErrorBlocks(entry.Workspace.Name)
		if respErr := uc.respondReplaceOriginal(ctx, callback.ResponseURL, errBlocks, errFallback); respErr != nil {
			errutil.Handle(ctx, goerr.Wrap(respErr, "render fallback block on ws-switch"),
				"could not surface ws-switch fallback to user")
		}
		if clearErr := uc.repo.CaseProposal().SetMaterialization(ctx, d.ID, d.SelectedWorkspaceID, d.Materialization, false); clearErr != nil {
			errutil.Handle(ctx, clearErr, "failed to clear inference-in-progress flag after ws-switch fallback")
		}
		return nil
	}
	return nil
}

// HandleSubmit creates the Case using the current materialization and posts
// the completion notification.
func (uc *MentionProposalUseCase) HandleSubmit(ctx context.Context, caseUC *CaseUseCase, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
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
		return goerr.New("draft has no materialization to submit", goerr.V("proposal_id", draft.ID))
	}

	// Brief lock during creation to prevent double-submit.
	lockBlocks, _ := buildSubmittingBlocks()
	if err := uc.respondReplaceOriginal(ctx, callback.ResponseURL, lockBlocks, "Creating case…"); err != nil {
		errutil.Handle(ctx, err, "failed to render submitting state")
	}

	// Slack interactivity does not pass through the Web auth middleware
	// that normally seeds the persistCase reporter fallback, so inject
	// the clicker as the auth-context Token explicitly. Without this
	// line, every Slack-originated case lands with an empty ReporterID
	// and the Drafts / Cases UI renders an empty Reporter cell.
	mat := draft.Materialization
	createCtx := auth.ContextWithToken(ctx, &auth.Token{Sub: callback.User.ID})
	created, err := caseUC.CreateCase(
		createCtx,
		draft.SelectedWorkspaceID,
		mat.Title,
		mat.Description,
		[]string{callback.User.ID},
		mat.CustomFieldValues,
		false,
		mat.IsTest,
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
			goerr.V("proposal_id", draft.ID),
			goerr.V("workspace_id", draft.SelectedWorkspaceID))
	}

	uc.updatePreviewWithCreated(ctx, draft.EphemeralChannelID, draft.EphemeralMessageTS, created)

	if err := uc.repo.CaseProposal().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft after submit")
	}
	return nil
}

// HandleEdit opens the dynamic Edit modal for the currently-selected workspace.
func (uc *MentionProposalUseCase) HandleEdit(ctx context.Context, callback *goslack.InteractionCallback, action *goslack.BlockAction) error {
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
		ProposalID:         string(draft.ID),
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
// every ActionBlock is stripped (workspace selector + buttons live in the
// same ActionBlock now, both go), and a "Canceled" tail is appended. The
// underlying draft is deleted from the repository.
func (uc *MentionProposalUseCase) HandleCancel(ctx context.Context, callback *goslack.InteractionCallback, _ *goslack.BlockAction) error {
	if callback == nil {
		return goerr.New("nil callback")
	}
	draft, err := uc.locateDraftFromCallback(ctx, callback)
	if err != nil {
		_ = uc.appendCanceledTail(ctx, callback, "", "")
		return nil
	}

	if err := uc.appendCanceledTail(ctx, callback, draft.EphemeralChannelID, draft.EphemeralMessageTS); err != nil {
		errutil.Handle(ctx, err, "failed to render canceled tail")
	}
	if err := uc.repo.CaseProposal().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft on cancel")
	}
	return nil
}

// appendCanceledTail rebuilds the original message minus all ActionBlocks
// (which carry both the workspace selector and the action buttons) and
// appends a "Canceled" context block at the end. Updates the original
// thread message via chat.update; response_url's replace_original is
// unreliable for thread replies (returns 500 in some flows).
func (uc *MentionProposalUseCase) appendCanceledTail(ctx context.Context, callback *goslack.InteractionCallback, channelID, messageTS string) error {
	original := callback.Message.Blocks.BlockSet
	kept := make([]goslack.Block, 0, len(original))
	for _, b := range original {
		if _, isAction := b.(*goslack.ActionBlock); isAction {
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
	const fallback = "Case draft canceled"
	if channelID != "" && messageTS != "" && uc.slackService != nil {
		return uc.slackService.UpdateMessage(ctx, channelID, messageTS, kept, fallback)
	}
	return uc.respondReplaceOriginal(ctx, callback.ResponseURL, kept, fallback)
}

// HandleEditSubmit processes the view_submission for the Edit modal.
func (uc *MentionProposalUseCase) HandleEditSubmit(ctx context.Context, caseUC *CaseUseCase, callback *goslack.InteractionCallback) error {
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

	draft, err := uc.repo.CaseProposal().Get(ctx, model.CaseProposalID(meta.ProposalID))
	if err != nil {
		return goerr.Wrap(err, "draft not found for edit submit", goerr.V("proposal_id", meta.ProposalID))
	}

	blockValues := callback.View.State.Values

	title := readPlainInput(blockValues, blockIDDraftEditTitle, actionIDDraftEditTitle)
	description := readPlainInput(blockValues, blockIDDraftEditDescription, actionIDDraftEditDescription)
	fieldValues := extractFieldValues(blockValues)
	isTest := readDraftEditTestFlag(blockValues)

	// Same auth-context injection as the preview-submit path above —
	// the modal submission is also a Slack interactivity callback with
	// no Web auth Token in context, so persistCase has nowhere to read
	// the reporter from unless we set it here.
	createCtx := auth.ContextWithToken(ctx, &auth.Token{Sub: callback.User.ID})
	created, err := caseUC.CreateCase(
		createCtx,
		meta.WorkspaceID,
		title,
		description,
		[]string{callback.User.ID},
		fieldValues,
		false,
		isTest,
		callback.Team.ID,
		uuid.New().String(),
	)
	if err != nil {
		return goerr.Wrap(err, "failed to create case from edit modal",
			goerr.V("proposal_id", meta.ProposalID))
	}

	uc.updatePreviewWithCreated(ctx, meta.EphemeralChannelID, meta.EphemeralMessageTS, created)

	if err := uc.repo.CaseProposal().Delete(ctx, draft.ID); err != nil {
		errutil.Handle(ctx, err, "failed to delete draft after edit submit")
	}
	return nil
}

// --- helpers ---

// locateDraftFromCallback finds the draft associated with a button-based
// interaction (Submit/Edit/Cancel). Each button carries draft.ID in
// action.Value. For the workspace static_select, use parseProposalIDFromSelectorBlockID
// against the selector's BlockID instead.
func (uc *MentionProposalUseCase) locateDraftFromCallback(ctx context.Context, callback *goslack.InteractionCallback) (*model.CaseProposal, error) {
	for _, a := range callback.ActionCallback.BlockActions {
		if a.Value != "" {
			if d, err := uc.repo.CaseProposal().Get(ctx, model.CaseProposalID(a.Value)); err == nil {
				return d, nil
			}
		}
	}
	return nil, goerr.New("could not resolve draft from interaction callback")
}

// parseProposalIDFromSelectorBlockID parses the draft ID encoded into the
// workspace selector's BlockID by buildPreviewBlocks (format:
// "<BlockIDDraftWSSelect>:<proposalID>").
func parseProposalIDFromSelectorBlockID(blockID string) (model.CaseProposalID, bool) {
	prefix := BlockIDDraftWSSelect + ":"
	if !strings.HasPrefix(blockID, prefix) {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(blockID, prefix))
	if id == "" {
		return "", false
	}
	return model.CaseProposalID(id), true
}

// respondReplaceOriginal POSTs to the interaction's response_url to replace
// the original message with new blocks. Works for both regular thread
// messages and ephemerals; we no longer post ephemerals, but the API path
// is the same.
func (uc *MentionProposalUseCase) respondReplaceOriginal(ctx context.Context, responseURL string, blocks []goslack.Block, fallback string) error {
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

func (uc *MentionProposalUseCase) respondLocked(ctx context.Context, callback *goslack.InteractionCallback) {
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
func (uc *MentionProposalUseCase) updatePreviewWithCreated(ctx context.Context, channelID, messageTS string, created *model.Case) {
	if channelID == "" || messageTS == "" || uc.slackService == nil {
		return
	}
	blocks, fallback := buildCaseCreatedTailBlocks(ctx, created)
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

// readDraftEditTestFlag reads the Test-case checkbox from the proposal Edit
// modal. The modal always renders the checkbox, so its submitted state is
// authoritative; absent / unticked means false.
func readDraftEditTestFlag(blockValues map[string]map[string]goslack.BlockAction) bool {
	block, ok := blockValues[blockIDDraftEditTest]
	if !ok {
		return false
	}
	action, ok := block[actionIDDraftEditTest]
	if !ok {
		return false
	}
	for _, opt := range action.SelectedOptions {
		if opt.Value == caseOptionValueTest {
			return true
		}
	}
	return false
}
