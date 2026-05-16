package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack/slackevents"
)

// SlackUseCases handles Slack-related business logic
type SlackUseCases struct {
	repo            interfaces.Repository
	registry        *model.WorkspaceRegistry
	agent           *AgentUseCase
	slackService    slacksvc.Service
	mentionProposal *MentionProposalUseCase
}

// NewSlackUseCases creates a new SlackUseCases instance. agent and
// mentionProposal are mandatory — Slack mention dispatch requires both.
func NewSlackUseCases(repo interfaces.Repository, registry *model.WorkspaceRegistry, agent *AgentUseCase, mentionProposal *MentionProposalUseCase, slackService slacksvc.Service) *SlackUseCases {
	return &SlackUseCases{
		repo:            repo,
		registry:        registry,
		agent:           agent,
		mentionProposal: mentionProposal,
		slackService:    slackService,
	}
}

// contextWithUserLang fetches the Slack user's locale and embeds the detected language into the context.
// If the locale cannot be fetched, the context is returned unchanged (defaultLang will be used).
func (uc *SlackUseCases) contextWithUserLang(ctx context.Context, userID string) context.Context {
	return contextWithSlackUserLang(ctx, uc.slackService, userID)
}

// contextWithSlackUserLang fetches the Slack user's locale and embeds the detected language into the context.
func contextWithSlackUserLang(ctx context.Context, svc slacksvc.Service, userID string) context.Context {
	if svc == nil || userID == "" {
		return ctx
	}
	user, err := svc.GetUserInfo(ctx, userID)
	if err != nil || user == nil {
		if err != nil {
			// Locale fetch failures are normal-flow: bot users, deleted users,
			// transient Slack API hiccups. Tag as benign so the operator sees
			// it in logs but Sentry stays quiet.
			errutil.Handle(ctx, goerr.Wrap(err, "failed to get user locale for i18n",
				goerr.V("user_id", userID),
				goerr.T(errutil.TagBenign),
			), "failed to get user locale for i18n")
		}
		return ctx
	}
	if lang := i18n.DetectLang(user.Locale); lang != "" {
		return i18n.ContextWithLang(ctx, lang)
	}
	return ctx
}

// HandleSlackEvent processes Slack Events API events
func (uc *SlackUseCases) HandleSlackEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error {
	// Handle member_joined_channel / member_left_channel events
	switch event.InnerEvent.Type {
	case "member_joined_channel", "member_left_channel":
		return uc.handleMembershipEvent(ctx, event)
	}

	// Convert event to domain model
	msg := slack.NewMessage(ctx, event)
	if msg == nil {
		// Unsupported event type: slack.NewMessage returned nil. This is an
		// unexpected path — surface to Sentry so we notice schema drift or
		// new event types we forgot to wire.
		errutil.Handle(ctx, goerr.New("unsupported slack event type",
			goerr.V("type", event.Type),
			goerr.V("innerType", event.InnerEvent.Type),
		), "unsupported slack event type")
		return nil
	}

	// Save the message
	if err := uc.HandleSlackMessage(ctx, msg); err != nil {
		return goerr.Wrap(err, "failed to handle slack message")
	}

	// Dispatch app_mention events:
	//   - In a channel already bound to a Case → existing AgentUseCase flow
	//   - Otherwise → MentionProposalUseCase (Slack-mention case-draft flow)
	//
	// Both branches require Slack/LLM to be wired (agent and mentionProposal are
	// constructed together with slackService in usecase.New). When Slack is
	// not wired (e.g. unit tests that only exercise message storage), skip
	// dispatch entirely.
	if appMention, ok := event.InnerEvent.Data.(*slackevents.AppMentionEvent); ok {
		if uc.isCaseBoundChannel(ctx, appMention.Channel) {
			if uc.agent == nil {
				return nil
			}
			if err := uc.agent.HandleAgentMention(ctx, msg); err != nil {
				errutil.Handle(ctx, goerr.Wrap(err, "failed to handle agent mention",
					goerr.V("channel_id", msg.ChannelID()),
					goerr.V("message_id", msg.ID()),
					goerr.V("user_id", msg.UserID()),
				), "failed to handle agent mention")
			}
			return nil
		}

		if uc.mentionProposal == nil {
			return nil
		}
		ctx = uc.contextWithUserLang(ctx, appMention.User)
		if err := uc.mentionProposal.HandleAppMention(ctx, appMention); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to handle mention draft",
				goerr.V("channel_id", appMention.Channel),
				goerr.V("user_id", appMention.User),
				goerr.V("event_ts", appMention.EventTimeStamp),
			), "failed to handle mention draft")
		}
		return nil
	}

	// Thread-reply route: F1-F8 filters (see docs/slack-mention-case-draft.md
	// §dispatcher) decide whether a `message` event should resume an open-mode
	// draft turn. App-mention duplicates land here too — F5 drops them so
	// only the app_mention path runs.
	if msgEv, ok := event.InnerEvent.Data.(*slackevents.MessageEvent); ok {
		if uc.mentionProposal == nil {
			return nil
		}
		if !uc.shouldResumeOnReply(ctx, msgEv) {
			return nil
		}
		ctx = uc.contextWithUserLang(ctx, msgEv.User)
		if err := uc.mentionProposal.HandleThreadReply(ctx, msgEv); err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to handle thread reply resume",
				goerr.V("channel_id", msgEv.Channel),
				goerr.V("thread_ts", msgEv.ThreadTimeStamp),
				goerr.V("message_ts", msgEv.TimeStamp),
				goerr.V("user_id", msgEv.User),
			), "failed to handle thread reply resume")
		}
	}

	return nil
}

// shouldResumeOnReply applies the F1-F8 filter chain. Returns true when the
// caller must invoke HandleThreadReply.
func (uc *SlackUseCases) shouldResumeOnReply(ctx context.Context, ev *slackevents.MessageEvent) bool {
	logger := logging.From(ctx)

	// F1: SubType set — bot_message / message_changed / channel_join / etc.
	if ev.SubType != "" {
		return false
	}
	// F3: BotID set — defensive against bot posts that don't set SubType.
	if ev.BotID != "" {
		return false
	}
	// F4: top-level post or thread-parent post.
	if ev.ThreadTimeStamp == "" || ev.ThreadTimeStamp == ev.TimeStamp {
		return false
	}
	// F2 / F5: need bot user id.
	botUserID, err := uc.slackService.GetBotUserID(ctx)
	if err != nil {
		errutil.Handle(ctx, err, "thread-reply filter: get bot user id failed")
		return false
	}
	if ev.User == botUserID {
		return false
	}
	if strings.Contains(ev.Text, "<@"+botUserID+">") {
		return false
	}
	// F6: Session must exist for this thread.
	session, err := uc.repo.Session().GetByThread(ctx, ev.Channel, ev.ThreadTimeStamp)
	if err != nil {
		errutil.Handle(ctx, err, "thread-reply filter: session lookup failed")
		return false
	}
	if session == nil {
		return false
	}
	// F7: case-bound sessions only resume on @mention.
	if session.IsCaseBound() {
		return false
	}
	// F8: open-mode session must have ended on post_question.
	if !session.ResumeOnReply() {
		return false
	}
	logger.Debug("thread reply will resume open-mode draft turn",
		"channel_id", ev.Channel,
		"thread_ts", ev.ThreadTimeStamp,
		"session_id", session.ID,
	)
	return true
}

// isCaseBoundChannel reports whether the given channel ID is associated with
// a Case in any registered workspace.
func (uc *SlackUseCases) isCaseBoundChannel(ctx context.Context, channelID string) bool {
	if channelID == "" || uc.registry == nil {
		return false
	}
	for _, entry := range uc.registry.List() {
		c, err := uc.repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, channelID)
		if err != nil {
			errutil.Handle(ctx, err, "failed to look up case-bound channel during mention dispatch")
			continue
		}
		if c != nil {
			return true
		}
	}
	return false
}

// handleMembershipEvent processes member_joined_channel / member_left_channel events.
// It syncs the full channel member list from Slack API for the affected case (if any).
func (uc *SlackUseCases) handleMembershipEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error {
	logger := logging.From(ctx)

	// Extract channel ID from the event data
	var channelID string
	switch data := event.InnerEvent.Data.(type) {
	case *slackevents.MemberJoinedChannelEvent:
		channelID = data.Channel
	case *slackevents.MemberLeftChannelEvent:
		channelID = data.Channel
	default:
		return nil
	}

	if channelID == "" || uc.registry == nil || uc.slackService == nil {
		return nil
	}

	// Search all workspaces for a case with this channel
	for _, entry := range uc.registry.List() {
		c, err := uc.repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, channelID)
		if err != nil {
			errutil.Handle(ctx, err, "failed to look up case by slack channel ID for membership sync")
			continue
		}
		if c == nil {
			continue
		}

		// Found the case; sync members from Slack API
		members, err := uc.slackService.GetConversationMembers(ctx, channelID)
		if err != nil {
			errutil.Handle(ctx, err, "failed to get channel members for membership sync")
			return nil // Don't propagate error; next event or manual sync will fix it
		}

		c.ChannelUserIDs = filterHumanUsers(ctx, uc.repo, members)
		if _, err := uc.repo.Case().Update(ctx, entry.Workspace.ID, c); err != nil {
			errutil.Handle(ctx, err, "failed to update case channel user IDs")
			return nil
		}

		logger.Info("synced channel members for case",
			"channel_id", channelID,
			"case_id", c.ID,
			"workspace_id", entry.Workspace.ID,
			"member_count", len(members),
		)
		return nil
	}

	return nil
}

// HandleSlackMessage saves a Slack message
func (uc *SlackUseCases) HandleSlackMessage(ctx context.Context, msg *slack.Message) error {
	if msg == nil {
		return goerr.New("message is nil")
	}

	// Drop our own bot's posts. These are the action change-notification
	// context blocks we emit ourselves; the ActionEvent feed already
	// records the same change, so re-ingesting them would double-count.
	// Other bots (e.g., CI integrations) still pass through.
	if uc.slackService != nil {
		if botUserID, err := uc.slackService.GetBotUserID(ctx); err == nil && botUserID != "" && msg.UserID() == botUserID {
			logging.From(ctx).Debug("skipping bot's own message", "user_id", msg.UserID())
			return nil
		}
	}

	// Save to channel-level collection (backward compatible)
	if err := uc.repo.Slack().PutMessage(ctx, msg); err != nil {
		return goerr.Wrap(err, "failed to save slack message", goerr.V("messageID", msg.ID()), goerr.V("channelID", msg.ChannelID()))
	}

	// Also save to case sub-collection if channel belongs to a case
	if uc.registry == nil {
		return nil
	}
	for _, entry := range uc.registry.List() {
		c, err := uc.repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, msg.ChannelID())
		if err != nil {
			return goerr.Wrap(err, "failed to look up case by slack channel ID",
				goerr.V("channelID", msg.ChannelID()),
				goerr.V("workspaceID", entry.Workspace.ID),
			)
		}
		if c != nil {
			if err := uc.repo.CaseMessage().Put(ctx, entry.Workspace.ID, c.ID, msg); err != nil {
				return goerr.Wrap(err, "failed to save message to case sub-collection",
					goerr.V("channelID", msg.ChannelID()),
					goerr.V("workspaceID", entry.Workspace.ID),
					goerr.V("caseID", c.ID),
				)
			}

			// If this message is a reply in an action's thread, also persist
			// it under the action's slack_messages sub-collection so the WebUI
			// can display the conversation.
			if msg.ThreadTS() != "" && msg.ThreadTS() != msg.ID() {
				action, actionErr := uc.repo.Action().GetBySlackMessageTS(ctx, entry.Workspace.ID, msg.ThreadTS())
				if actionErr == nil && action != nil && action.CaseID == c.ID {
					if err := uc.repo.ActionMessage().Put(ctx, entry.Workspace.ID, action.ID, msg); err != nil {
						return goerr.Wrap(err, "failed to save message to action sub-collection",
							goerr.V("channelID", msg.ChannelID()),
							goerr.V("workspaceID", entry.Workspace.ID),
							goerr.V("actionID", action.ID),
						)
					}
				}
			}
			break
		}
	}

	logging.From(ctx).Info("slack message saved",
		"messageID", msg.ID(),
		"channelID", msg.ChannelID(),
		"userID", msg.UserID(),
	)

	return nil
}

// CleanupOldMessages deletes messages older than the specified time
func (uc *SlackUseCases) CleanupOldMessages(ctx context.Context, before time.Time) error {
	logger := logging.From(ctx)

	deleted, err := uc.repo.Slack().PruneMessages(ctx, "", before)
	if err != nil {
		return goerr.Wrap(err, "failed to cleanup old messages", goerr.V("before", before))
	}

	logger.Info("old slack messages deleted", "count", deleted, "before", before)

	return nil
}
