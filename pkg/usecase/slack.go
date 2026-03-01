package usecase

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
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
}

// NewSlackUseCases creates a new SlackUseCases instance
func NewSlackUseCases(repo interfaces.Repository, registry *model.WorkspaceRegistry, agent *AgentUseCase, slackService slacksvc.Service) *SlackUseCases {
	return &SlackUseCases{
		repo:         repo,
		registry:     registry,
		agent:        agent,
		slackService: slackService,
	}
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

	// Delegate app_mention events to AI agent
	if _, ok := event.InnerEvent.Data.(*slackevents.AppMentionEvent); ok && uc.agent != nil {
		if err := uc.agent.HandleAgentMention(ctx, msg); err != nil {
			logger.Error("failed to handle agent mention", "error", err.Error())
			// Don't return error; the message was saved successfully
		}
	}

	return nil
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
