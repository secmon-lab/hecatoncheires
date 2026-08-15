package usecase

import (
	"context"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// HandleWorkspaceAgentMention processes an app_mention that should run the
// workspace-level cross-case agent (wsagent) on behalf of the mentioning user,
// then posts the reply into the mention's thread. The session is case-less
// (CaseID == 0) and tagged SessionKindWorkspaceAgent: the agent operates across
// every case the user can access, not one bound case.
//
// Both hosts of the workspace agent funnel through here:
//   - channel mode — a mention in the configured [slack] workspace_channel.
//   - thread mode — a channel-root mention in the monitored channel (with
//     trigger = "mention"), plus every follow-up mention inside the thread it
//     opened. Those threads never become Cases; the Session's Kind is what the
//     dispatcher reads to keep them out of the case-creation path.
func (uc *AgentUseCase) HandleWorkspaceAgentMention(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry) error {
	if msg == nil || entry == nil {
		return goerr.New("msg and entry are required")
	}
	logger := logging.From(ctx)
	if uc.durableWorkspaceAgent == nil {
		logger.Debug("workspace agent not configured; skipping workspace channel mention")
		return nil
	}

	// A top-level mention anchors its own thread; a threaded mention continues it.
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		threadTS = msg.ID()
	}

	// Skip the bot's own mentions to avoid a self-trigger loop. This runs before
	// the claim so a self-mention leaves no Session behind; GetBotUserID is
	// cached for the process lifetime, so it does not widen the window below.
	botUserID, err := uc.deps.SlackService.GetBotUserID(ctx)
	if err != nil {
		uc.replyUserError(ctx, err, "failed to get bot user ID", msg.ChannelID(), threadTS)
		return nil
	}
	if msg.UserID() == botUserID {
		logger.Debug("skipping bot's own message", "user_id", msg.UserID())
		return nil
	}

	// Claim the thread before any further Slack round-trip. The Slack dispatcher
	// decides whether a later in-thread mention starts a Case by reading this
	// Session's Kind, so every uncached call made before the claim is a window in
	// which that mention sees no Session and routes the thread into case
	// creation. Case-less session (CaseID == 0): workspace-scoped, tied only to
	// the thread.
	session, err := uc.claimSession(ctx, entry.Workspace.ID, 0, msg.ChannelID(), threadTS, model.SessionKindWorkspaceAgent)
	if err != nil {
		return goerr.Wrap(err, "failed to claim workspace-agent session")
	}
	// Another host got here first and this thread belongs to a Case (either bound
	// or still being formed). Running the cross-case agent on that Session would
	// braid two conversations into one history, so leave the thread to the case
	// flow.
	if session.Kind != model.SessionKindWorkspaceAgent {
		logger.Debug("thread is owned by the case flow; skipping workspace agent",
			"channel_id", msg.ChannelID(), "thread_ts", threadTS, "kind", string(session.Kind))
		return nil
	}

	// Locale lookup is an uncached Slack call, so it happens after the claim.
	ctx = contextWithSlackUserLang(ctx, uc.deps.SlackService, msg.UserID())

	// The run outlives this call: it draws its own progress into the thread and
	// its answer is posted by the completion handler, from whichever instance
	// commits the last transition.
	res, runErr := uc.durableWorkspaceAgent.StartTurn(ctx, wsagent.TurnRequest{
		Session:     session,
		Workspace:   entry,
		ActorID:     msg.UserID(),
		MentionText: msg.Text(),
		TriggerTS:   msg.ID(),
	})
	if runErr != nil {
		uc.replyUserError(ctx, runErr, "workspace agent start turn", msg.ChannelID(), threadTS)
		return nil
	}

	switch res.Status {
	case wsagent.StatusStarted, wsagent.StatusIdempotent:
		// Started: the answer arrives from the run's completion handler.
		// Idempotent: a re-delivery of an event already being handled.
		return nil
	case wsagent.StatusBusy:
		busyMsg := i18n.T(ctx, i18n.MsgKeyAgentBusy)
		if _, postErr := uc.deps.SlackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, busyMsg); postErr != nil {
			errutil.Handle(ctx, postErr, "post workspace-agent busy notice")
		}
		return nil
	default:
		return goerr.New("unexpected workspace-agent status", goerr.V("status", int(res.Status)))
	}
}
