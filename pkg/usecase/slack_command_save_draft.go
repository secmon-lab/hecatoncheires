package usecase

import (
	"context"
	"encoding/json"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/slack-go/slack"
)

// HandleSaveAsDraftClick handles the "Save as Draft" button click that
// originates from the Case creation modal. The block_actions interaction
// carries the in-flight view state (title, description, custom fields,
// privacy flag) plus the modal's private_metadata (workspace ID, channel,
// source team). We translate that state into a CreateDraft call so the
// half-written entry is persisted in CaseStatus.DRAFT — the user can later
// return to it from the Web Drafts page and Submit (promoting to OPEN) or
// Discard (delete).
//
// This entry point is invoked from the async tail of the block_actions
// dispatcher; the controller already returned 200 to Slack. We post an
// ephemeral receipt to the channel and replace the modal with a small
// "Saved" splash so the user has feedback without us racing the trigger_id
// TTL.
func (uc *SlackUseCases) HandleSaveAsDraftClick(ctx context.Context, caseUC *CaseUseCase, callback *slack.InteractionCallback) error {
	if caseUC == nil {
		return goerr.New("case usecase is not available")
	}
	if uc.slackService == nil {
		return goerr.New("slack service is not available")
	}

	ctx = uc.contextWithUserLang(ctx, callback.User.ID)

	// Drafts are author-scoped — they must remember which Slack user saved
	// them so ListDrafts / GetDraft / etc. can later check ownership. The
	// generic Slack interactivity path does not populate auth context (the
	// app's Slack bot token does not represent a specific Web user), so we
	// derive a minimal auth token from the callback's user ID and inject it
	// here before reaching CaseUseCase.CreateDraft.
	ctx = auth.ContextWithToken(ctx, &auth.Token{Sub: callback.User.ID})

	var meta commandMetadata
	if err := json.Unmarshal([]byte(callback.View.PrivateMetadata), &meta); err != nil {
		return goerr.Wrap(err, "failed to parse save-as-draft private_metadata")
	}

	blockValues := callback.View.State.Values

	title := ""
	if titleBlock, ok := blockValues[SlackBlockIDCaseTitle]; ok {
		if titleAction, ok := titleBlock[SlackActionIDCaseTitle]; ok {
			title = titleAction.Value
		}
	}

	description := ""
	if descBlock, ok := blockValues[SlackBlockIDCaseDescription]; ok {
		if descAction, ok := descBlock[SlackActionIDCaseDescription]; ok {
			description = descAction.Value
		}
	}

	isPrivate := false
	if privateBlock, ok := blockValues[SlackBlockIDCasePrivate]; ok {
		if privateAction, ok := privateBlock[SlackActionIDCasePrivate]; ok {
			for _, opt := range privateAction.SelectedOptions {
				if opt.Value == "private" {
					isPrivate = true
					break
				}
			}
		}
	}

	fieldValuesMap := extractFieldValues(blockValues)

	// Auto-assign the clicker, matching the regular Submit path
	// (HandleCaseCreationSubmit), so the case behaves identically once it
	// is promoted to OPEN — the clicker becomes a channel member via the
	// assignee invite during activation, even on Slack workspaces where
	// the reporter-side invite isn't enough (e.g. when ReporterID is
	// missing from a downstream caller). Keeping the two paths symmetric
	// here avoids subtly different membership outcomes between "Submit"
	// and "Save as Draft → Submit".
	assigneeIDs := []string{callback.User.ID}

	created, err := caseUC.CreateDraft(ctx, meta.WorkspaceID, title, description, assigneeIDs, fieldValuesMap, isPrivate)
	if err != nil {
		uc.notifySaveDraftFailure(ctx, meta.ChannelID, callback.User.ID)
		return goerr.Wrap(err, "failed to save draft",
			goerr.V("workspace_id", meta.WorkspaceID),
			goerr.V("user_id", callback.User.ID))
	}

	// Splash-update the modal so the user sees an explicit "Saved". This is
	// best-effort: the Save itself has already succeeded by the time we get
	// here, so an UpdateView failure must not propagate as an error to the
	// dispatcher.
	splashView := buildSaveAsDraftSplashView(ctx, created.ID)
	if updateErr := uc.slackService.UpdateView(ctx, splashView, "", "", callback.View.ID); updateErr != nil {
		errutil.Handle(ctx, goerr.Wrap(updateErr, "failed to update modal after Save as Draft",
			goerr.V("view_id", callback.View.ID),
			goerr.V(CaseIDKey, created.ID),
		), "failed to update modal after Save as Draft")
	}

	// Ephemeral receipt so the user gets feedback in their original channel
	// even if the splash modal is dismissed before they read it. When a
	// web baseURL is configured, the receipt embeds a one-click link to
	// the draft's detail page.
	if meta.ChannelID != "" {
		msg := buildDraftSavedEphemeralText(ctx, caseUC, meta.WorkspaceID, created.ID, created.Title)
		if epErr := uc.slackService.PostEphemeral(ctx, meta.ChannelID, callback.User.ID, msg); epErr != nil {
			errutil.Handle(ctx, goerr.Wrap(epErr, "failed to post Save-as-Draft ephemeral",
				goerr.V("channel_id", meta.ChannelID),
				goerr.V("user_id", callback.User.ID),
				goerr.V(CaseIDKey, created.ID),
			), "failed to post Save-as-Draft ephemeral")
		}
	}

	return nil
}

// notifySaveDraftFailure best-efforts an ephemeral apology when CreateDraft
// fails. Errors from the apology itself are downgraded to errutil.Handle so
// the original failure stays in the caller's error chain.
func (uc *SlackUseCases) notifySaveDraftFailure(ctx context.Context, channelID, userID string) {
	if channelID == "" || uc.slackService == nil {
		return
	}
	msg := i18n.T(ctx, i18n.MsgDraftSaveFailedEphemeral)
	if err := uc.slackService.PostEphemeral(ctx, channelID, userID, msg); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to post Save-as-Draft failure ephemeral",
			goerr.V("channel_id", channelID),
			goerr.V("user_id", userID),
		), "failed to post Save-as-Draft failure ephemeral")
	}
}

// buildSaveAsDraftSplashView returns a minimal modal that replaces the
// in-progress Case creation modal once the draft has been persisted. Its
// only job is to give the user an unambiguous "Saved" confirmation; there
// is no Submit button, only a Close.
func buildSaveAsDraftSplashView(ctx context.Context, caseID int64) slack.ModalViewRequest {
	body := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, i18n.T(ctx, i18n.MsgDraftSavedModalBody, caseID), false, false),
		nil,
		nil,
	)
	return slack.ModalViewRequest{
		Type:  slack.VTModal,
		Title: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgDraftSavedModalTitle), false, false),
		Close: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{body},
		},
	}
}
