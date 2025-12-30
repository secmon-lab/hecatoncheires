package usecase

import (
	"context"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	"github.com/slack-go/slack/slackevents"
)

// SlackUseCases handles Slack-related business logic
type SlackUseCases struct {
	repo interfaces.Repository
}

// NewSlackUseCases creates a new SlackUseCases instance
func NewSlackUseCases(repo interfaces.Repository) *SlackUseCases {
	return &SlackUseCases{
		repo: repo,
	}
}

// HandleSlackEvent processes Slack Events API events
func (uc *SlackUseCases) HandleSlackEvent(ctx context.Context, event *slackevents.EventsAPIEvent) error {
	logger := logging.From(ctx)

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

	return nil
}

// HandleSlackMessage saves a Slack message
func (uc *SlackUseCases) HandleSlackMessage(ctx context.Context, msg *slack.Message) error {
	if msg == nil {
		return goerr.New("message is nil")
	}

	logger := logging.From(ctx)

	// Business rules could be applied here (e.g., filtering bot messages)
	// For now, just save the message

	if err := uc.repo.Slack().PutMessage(ctx, msg); err != nil {
		return goerr.Wrap(err, "failed to save slack message", goerr.V("messageID", msg.ID()), goerr.V("channelID", msg.ChannelID()))
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
