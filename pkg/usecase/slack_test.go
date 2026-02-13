package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	slackevents "github.com/slack-go/slack/slackevents"
)

func TestSlackUseCases_HandleSlackEvent(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	t.Run("handles message event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:           "message",
					User:           "U123",
					Text:           "Hello, world!",
					TimeStamp:      "1234567890.123456",
					Channel:        "C123",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)

		if len(messages) > 0 {
			msg := messages[0]
			gt.Value(t, msg.UserID()).Equal("U123")
			gt.Value(t, msg.Text()).Equal("Hello, world!")
		}
	})

	t.Run("handles app_mention event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:           "app_mention",
					User:           "U456",
					Text:           "@bot help",
					TimeStamp:      "1234567890.654321",
					Channel:        "C456",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T456",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C456",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)
	})

	t.Run("ignores unsupported event types", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "some_other_event",
				Data: map[string]interface{}{
					"type": "some_other_event",
				},
			},
			TeamID: "T789",
		}

		// Should not error, just skip
		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})
}

func TestSlackUseCases_HandleSlackMessage(t *testing.T) {
	t.Run("stores message successfully", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"1234567890.123456",
			"C123",
			"",
			"T123",
			"U123",
			"testuser",
			"Test message",
			"1234567890.123456",
			time.Now(),
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1).Required()

		gt.Value(t, messages[0].ID()).Equal("1234567890.123456")
	})

	t.Run("returns error for nil message", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		err := uc.Slack.HandleSlackMessage(ctx, nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("saves to case sub-collection when channel is mapped", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a case with SlackChannelID
		created, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			Title:          "Test Case",
			SlackChannelID: "C-MAPPED",
		})
		gt.NoError(t, err).Required()

		// Set up registry with the workspace
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		uc := usecase.New(repo, registry)

		msg := slack.NewMessageFromData(
			"mapped-msg-001",
			"C-MAPPED",
			"",
			"T123",
			"U123",
			"alice",
			"Hello from mapped channel",
			"ev1",
			time.Now(),
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-MAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)

		// Verify message was also saved to case sub-collection
		caseMsgs, _, err := repo.CaseMessage().List(ctx, "ws-test", created.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, caseMsgs).Length(1)
		gt.Value(t, caseMsgs[0].Text()).Equal("Hello from mapped channel")
	})

	t.Run("does not save to case sub-collection when channel is not mapped", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"unmapped-msg-001",
			"C-UNMAPPED",
			"",
			"T123",
			"U123",
			"bob",
			"Hello from unmapped channel",
			"ev1",
			time.Now(),
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-UNMAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)
	})
}

func TestSlackUseCases_CleanupOldMessages(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	newTime := now.Add(-1 * time.Hour)

	// Create old and new messages
	oldMsg := slack.NewMessageFromData(
		"old.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"Old message",
		"old.123456",
		oldTime,
	)

	newMsg := slack.NewMessageFromData(
		"new.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"New message",
		"new.123456",
		newTime,
	)

	gt.NoError(t, repo.Slack().PutMessage(ctx, oldMsg)).Required()
	gt.NoError(t, repo.Slack().PutMessage(ctx, newMsg)).Required()

	// Cleanup messages older than 24 hours
	cutoffTime := now.Add(-24 * time.Hour)
	gt.NoError(t, uc.Slack.CleanupOldMessages(ctx, cutoffTime)).Required()

	// Verify only new message remains
	messages, _, err := repo.Slack().ListMessages(
		ctx,
		"C123",
		time.Time{},
		now.Add(1*time.Hour),
		10,
		"",
	)
	gt.NoError(t, err).Required()

	gt.Array(t, messages).Length(1).Required()

	gt.Value(t, messages[0].ID()).Equal("new.123456")
}
