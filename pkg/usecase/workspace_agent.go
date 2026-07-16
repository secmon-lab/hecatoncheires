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

// HandleWorkspaceChannelMention processes an app_mention in a channel-mode
// workspace's configured workspace channel. It runs the workspace-level
// cross-case agent (wsagent) on behalf of the mentioning user, then posts the
// reply into the mention's thread. The session is case-less (CaseID == 0): the
// agent operates across every case the user can access, not one bound case.
func (uc *AgentUseCase) HandleWorkspaceChannelMention(ctx context.Context, msg *slackmodel.Message, entry *model.WorkspaceEntry) error {
	if msg == nil || entry == nil {
		return goerr.New("msg and entry are required")
	}
	logger := logging.From(ctx)
	if uc.workspaceAgent == nil {
		logger.Debug("workspace agent not configured; skipping workspace channel mention")
		return nil
	}

	ctx = contextWithSlackUserLang(ctx, uc.deps.SlackService, msg.UserID())

	// Skip the bot's own mentions to avoid a self-trigger loop.
	botUserID, err := uc.deps.SlackService.GetBotUserID(ctx)
	if err != nil {
		threadTS := msg.ThreadTS()
		if threadTS == "" {
			threadTS = msg.ID()
		}
		uc.replyUserError(ctx, err, "failed to get bot user ID", msg.ChannelID(), threadTS)
		return nil
	}
	if msg.UserID() == botUserID {
		logger.Debug("skipping bot's own message", "user_id", msg.UserID())
		return nil
	}

	// A top-level mention anchors its own thread; a threaded mention continues it.
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		threadTS = msg.ID()
	}

	// Case-less session (CaseID == 0): workspace-scoped, tied only to the thread.
	session, err := uc.loadOrCreateSession(ctx, entry.Workspace.ID, 0, msg.ChannelID(), threadTS)
	if err != nil {
		return goerr.Wrap(err, "failed to load or create workspace-agent session")
	}

	// Slack-side progress trace (per-mention; not persisted). finalize posts the
	// agent's final reply in place of the last transient line.
	traceMsg := uc.newTraceMessage(msg.ChannelID(), threadTS)

	res, runErr := uc.workspaceAgent.RunTurn(ctx, wsagent.TurnRequest{
		Session:     session,
		Workspace:   entry,
		ActorID:     msg.UserID(),
		MentionText: msg.Text(),
		TriggerTS:   msg.ID(),
		Handler: wsagent.HandlerFuncs{
			TraceAppendFn:  traceMsg.appendLine,
			TraceReplaceFn: traceMsg.replaceLine,
		},
	})
	if runErr != nil {
		uc.replyUserError(ctx, runErr, "workspace agent run turn", msg.ChannelID(), threadTS)
		return nil
	}

	switch res.Status {
	case wsagent.StatusBusy:
		busyMsg := i18n.T(ctx, i18n.MsgKeyAgentBusy)
		if _, postErr := uc.deps.SlackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, busyMsg); postErr != nil {
			errutil.Handle(ctx, postErr, "post workspace-agent busy notice")
		}
		return nil
	case wsagent.StatusIdempotent:
		return nil
	case wsagent.StatusCompleted:
		if err := traceMsg.finalize(ctx, res.ReplyText); err != nil {
			return goerr.Wrap(err, "failed to post workspace-agent reply")
		}
		return nil
	case wsagent.StatusFallback:
		fallback := i18n.T(ctx, i18n.MsgWorkspaceAgentFallback)
		if err := traceMsg.finalize(ctx, fallback); err != nil {
			return goerr.Wrap(err, "failed to post workspace-agent fallback")
		}
		return nil
	default:
		return goerr.New("unexpected workspace-agent status", goerr.V("status", int(res.Status)))
	}
}
