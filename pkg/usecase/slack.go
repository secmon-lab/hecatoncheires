package usecase

import (
	"context"
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
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	agent        *AgentUseCase
	slackService slacksvc.Service
	mentionDraft *MentionDraftUseCase
}

// NewSlackUseCases creates a new SlackUseCases instance. agent and
// mentionDraft are mandatory — Slack mention dispatch requires both.
func NewSlackUseCases(repo interfaces.Repository, registry *model.WorkspaceRegistry, agent *AgentUseCase, mentionDraft *MentionDraftUseCase, slackService slacksvc.Service) *SlackUseCases {
	return &SlackUseCases{
		repo:         repo,
		registry:     registry,
		agent:        agent,
		mentionDraft: mentionDraft,
		slackService: slackService,
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
			logging.From(ctx).Warn("failed to get user locale for i18n", "error", err, "user_id", userID)
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
	logger := logging.From(ctx)

	// Handle member_joined_channel / member_left_channel events
	switch event.InnerEvent.Type {
	case "member_joined_channel", "member_left_channel":
		return uc.handleMembershipEvent(ctx, event)
	}

	// Convert event to domain model
	msg := slack.NewMessage(ctx, event)
	if msg == nil {
		// Unsupported event type, log warning but don't return error
		logger.Warn("unsupported slack event type", "type", event.Type, "innerType", event.InnerEvent.Type)
		return nil
	}

	// Save the message
	if err := uc.HandleSlackMessage(ctx, msg); err != nil {
		return goerr.Wrap(err, "failed to handle slack message")
	}

	// Dispatch app_mention events:
	//   - In a channel already bound to a Case → existing AgentUseCase flow
	//   - Otherwise → MentionDraftUseCase (Slack-mention case-draft flow)
	//
	// Both branches require Slack/LLM to be wired (agent and mentionDraft are
	// constructed together with slackService in usecase.New). When Slack is
	// not wired (e.g. unit tests that only exercise message storage), skip
	// dispatch entirely.
	if appMention, ok := event.InnerEvent.Data.(*slackevents.AppMentionEvent); ok {
		if uc.isCaseBoundChannel(ctx, appMention.Channel) {
			if uc.agent == nil {
				return nil
			}
			if err := uc.agent.HandleAgentMention(ctx, msg); err != nil {
				logger.Error("failed to handle agent mention", "error", err.Error())
			}
			return nil
		}

		if uc.mentionDraft == nil {
			return nil
		}
		if err := uc.mentionDraft.HandleAppMention(ctx, appMention); err != nil {
			logger.Error("failed to handle mention draft", "error", err.Error())
		}
	}

	return nil
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

	logger := logging.From(ctx)

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
			break
		}
	}

	logger.Info("slack message saved",
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
