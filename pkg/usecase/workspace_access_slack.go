package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"
	"github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// ephemeralPoster is the narrow Slack surface a denial notice needs.
type ephemeralPoster interface {
	PostEphemeral(ctx context.Context, channelID, userID, text string) error
}

// authorizeSlackActor decides whether a Slack actor may act on workspaceID.
// It returns (true, nil) when allowed, (false, nil) when denied — after posting
// the denial to the actor as an ephemeral message in channelID (skipped when
// channelID is empty or no poster is wired) — and (false, err) when the
// decision failed.
func authorizeSlackActor(ctx context.Context, access interfaces.WorkspaceAuthorizer,
	poster ephemeralPoster, workspaceID, userID, channelID string) (bool, error) {
	err := access.Authorize(ctx, workspaceID, userID)
	if err == nil {
		return true, nil
	}
	if !isWorkspaceAccessDenied(err) {
		return false, goerr.Wrap(err, "decide workspace access for Slack actor",
			goerr.V("workspace_id", workspaceID), goerr.V("user_id", userID))
	}
	postWorkspaceAccessDenied(ctx, poster, channelID, userID, err)
	return false, nil
}

// postWorkspaceAccessDenied renders err as the 3-part user-facing message
// (reporting it once, benign) and posts it to userID only.
func postWorkspaceAccessDenied(ctx context.Context, poster ephemeralPoster, channelID, userID string, err error) {
	text, _ := prepareUserError(ctx, err, "workspace access denied")
	if text == "" || channelID == "" || poster == nil {
		return
	}
	if postErr := poster.PostEphemeral(ctx, channelID, userID, text); postErr != nil {
		errutil.Handle(ctx, goerr.Wrap(postErr, "failed to post workspace access denial",
			goerr.V("channel_id", channelID), goerr.V("user_id", userID)),
			"failed to post workspace access denial")
	}
}

// buildWorkspaceAccessDeniedModal is the view a denied view_submission
// replaces the modal with; text is the rendered denial message.
func buildWorkspaceAccessDeniedModal(ctx context.Context, text string) slack.ModalViewRequest {
	return slack.ModalViewRequest{
		Type:  slack.VTModal,
		Title: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgWorkspaceAccessDeniedModalTitle), false, false),
		Close: slack.NewTextBlockObject(slack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, text, false, false), nil, nil),
			},
		},
	}
}

// deniedModalFor authorizes a view_submission that answers synchronously. On
// denial it returns the replacement view; on success it returns (nil, nil).
func (uc *SlackUseCases) deniedModalFor(ctx context.Context, workspaceID, userID string) (*slack.ModalViewRequest, error) {
	err := uc.workspaceAccess.Authorize(ctx, workspaceID, userID)
	if err == nil {
		return nil, nil
	}
	if !isWorkspaceAccessDenied(err) {
		return nil, goerr.Wrap(err, "decide workspace access for modal submission",
			goerr.V("workspace_id", workspaceID), goerr.V("user_id", userID))
	}
	text, _ := prepareUserError(ctx, err, "workspace access denied")
	view := buildWorkspaceAccessDeniedModal(ctx, text)
	return &view, nil
}

// NotifyWorkspaceAccessDenied posts the denial, in the user's language, for a
// controller that received model.ErrWorkspaceAccessDenied from a usecase call
// made on behalf of a Slack user.
func (uc *SlackUseCases) NotifyWorkspaceAccessDenied(ctx context.Context, channelID, userID string, cause error) {
	postWorkspaceAccessDenied(uc.contextWithUserLang(ctx, userID), uc.ephemeralPoster(), channelID, userID, cause)
}

// eventActorAllowed is the check the event dispatcher runs right before it
// acts on an event. An event with no human author — a bot post, or one without
// a user — is not checked: accept_bot is where an operator already decided to
// act on those. A decision error is reported and treated as denied. The
// denial is localized to the actor, which costs a locale lookup only when the
// event is refused.
func (uc *SlackUseCases) eventActorAllowed(ctx context.Context, workspaceID, userID, botID, channelID string) bool {
	if botID != "" || userID == "" {
		return true
	}
	err := uc.workspaceAccess.Authorize(ctx, workspaceID, userID)
	switch {
	case err == nil:
		return true
	case isWorkspaceAccessDenied(err):
		postWorkspaceAccessDenied(uc.contextWithUserLang(ctx, userID), uc.ephemeralPoster(), channelID, userID, err)
	default:
		errutil.Handle(ctx, goerr.Wrap(err, "decide workspace access for Slack event",
			goerr.V("workspace_id", workspaceID), goerr.V("user_id", userID)),
			"workspace access decision failed")
	}
	return false
}

// authorizeDraftActor is the check a case-draft interaction runs once it knows
// which workspace the user is acting on. It returns false after telling the
// user, in their language, when the workspace denies them.
func (uc *MentionProposalUseCase) authorizeDraftActor(ctx context.Context, workspaceID, userID, channelID string) (bool, error) {
	err := uc.workspaceAccess.Authorize(ctx, workspaceID, userID)
	switch {
	case err == nil:
		return true, nil
	case isWorkspaceAccessDenied(err):
		var poster ephemeralPoster
		if uc.slackService != nil {
			poster = uc.slackService
		}
		postWorkspaceAccessDenied(contextWithSlackUserLang(ctx, uc.slackService, userID), poster, channelID, userID, err)
		return false, nil
	default:
		return false, goerr.Wrap(err, "decide workspace access for case draft",
			goerr.V("workspace_id", workspaceID), goerr.V("user_id", userID))
	}
}

// ephemeralPoster returns the Slack service as the denial poster, or a nil
// interface when Slack is not wired (a typed nil would pass the nil check).
func (uc *SlackUseCases) ephemeralPoster() ephemeralPoster {
	if uc.slackService == nil {
		return nil
	}
	return uc.slackService
}
