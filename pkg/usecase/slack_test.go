package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	slackevents "github.com/slack-go/slack/slackevents"
)

func TestSlackUseCases_HandleSlackEvent(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo)
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

		if err := uc.Slack.HandleSlackEvent(ctx, event); err != nil {
			t.Fatalf("failed to handle slack event: %v", err)
		}

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(messages))
		}

		if len(messages) > 0 {
			msg := messages[0]
			if msg.UserID() != "U123" {
				t.Errorf("expected user ID U123, got %s", msg.UserID())
			}
			if msg.Text() != "Hello, world!" {
				t.Errorf("expected text 'Hello, world!', got %s", msg.Text())
			}
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

		if err := uc.Slack.HandleSlackEvent(ctx, event); err != nil {
			t.Fatalf("failed to handle slack event: %v", err)
		}

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C456",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(messages))
		}
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
		if err := uc.Slack.HandleSlackEvent(ctx, event); err != nil {
			t.Fatalf("failed to handle unsupported event: %v", err)
		}
	})
}

func TestSlackUseCases_HandleSlackMessage(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo)
	ctx := context.Background()

	t.Run("stores message successfully", func(t *testing.T) {
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

		if err := uc.Slack.HandleSlackMessage(ctx, msg); err != nil {
			t.Fatalf("failed to handle slack message: %v", err)
		}

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}

		if messages[0].ID() != "1234567890.123456" {
			t.Errorf("expected message ID 1234567890.123456, got %s", messages[0].ID())
		}
	})

	t.Run("returns error for nil message", func(t *testing.T) {
		if err := uc.Slack.HandleSlackMessage(ctx, nil); err == nil {
			t.Error("expected error for nil message, got nil")
		}
	})
}

func TestSlackUseCases_CleanupOldMessages(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo)
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

	if err := repo.Slack().PutMessage(ctx, oldMsg); err != nil {
		t.Fatalf("failed to put old message: %v", err)
	}
	if err := repo.Slack().PutMessage(ctx, newMsg); err != nil {
		t.Fatalf("failed to put new message: %v", err)
	}

	// Cleanup messages older than 24 hours
	cutoffTime := now.Add(-24 * time.Hour)
	if err := uc.Slack.CleanupOldMessages(ctx, cutoffTime); err != nil {
		t.Fatalf("failed to cleanup old messages: %v", err)
	}

	// Verify only new message remains
	messages, _, err := repo.Slack().ListMessages(
		ctx,
		"C123",
		time.Time{},
		now.Add(1*time.Hour),
		10,
		"",
	)
	if err != nil {
		t.Fatalf("failed to list messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message after cleanup, got %d", len(messages))
	}

	if messages[0].ID() != "new.123456" {
		t.Errorf("expected new message to remain, got %s", messages[0].ID())
	}
}
