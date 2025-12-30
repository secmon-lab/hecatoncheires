package slack_test

import (
	"context"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/slack-go/slack/slackevents"
)

func TestNewMessage_MessageEvent(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Type:           "message",
				User:           "U123456",
				Text:           "Hello, world!",
				TimeStamp:      "1234567890.123456",
				Channel:        "C123456",
				EventTimeStamp: "1234567890.123456",
			},
		},
	}

	msg := slack.NewMessage(ctx, event)

	if msg == nil {
		t.Fatal("expected message to be created, got nil")
	}

	if msg.ID() != "1234567890.123456" {
		t.Errorf("expected ID to be %q, got %q", "1234567890.123456", msg.ID())
	}

	if msg.ChannelID() != "C123456" {
		t.Errorf("expected ChannelID to be %q, got %q", "C123456", msg.ChannelID())
	}

	if msg.TeamID() != "T123456" {
		t.Errorf("expected TeamID to be %q, got %q", "T123456", msg.TeamID())
	}

	if msg.UserID() != "U123456" {
		t.Errorf("expected UserID to be %q, got %q", "U123456", msg.UserID())
	}

	if msg.UserName() != "U123456" {
		t.Errorf("expected UserName to be %q (default to UserID), got %q", "U123456", msg.UserName())
	}

	if msg.Text() != "Hello, world!" {
		t.Errorf("expected Text to be %q, got %q", "Hello, world!", msg.Text())
	}

	if msg.EventTS() != "1234567890.123456" {
		t.Errorf("expected EventTS to be %q, got %q", "1234567890.123456", msg.EventTS())
	}

	if msg.ThreadTS() != "" {
		t.Errorf("expected ThreadTS to be empty for root message, got %q", msg.ThreadTS())
	}

	// Check that CreatedAt is recent (within 1 second)
	if time.Since(msg.CreatedAt()) > time.Second {
		t.Errorf("expected CreatedAt to be recent, but it was %v ago", time.Since(msg.CreatedAt()))
	}
	if msg.CreatedAt().After(now.Add(time.Second)) {
		t.Errorf("expected CreatedAt to be before %v, got %v", now.Add(time.Second), msg.CreatedAt())
	}
}

func TestNewMessage_ThreadMessage(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Type:            "message",
				User:            "U123456",
				Text:            "Thread reply",
				TimeStamp:       "1234567890.123457",
				ThreadTimeStamp: "1234567890.123456",
				Channel:         "C123456",
				EventTimeStamp:  "1234567890.123457",
			},
		},
	}

	msg := slack.NewMessage(ctx, event)

	if msg == nil {
		t.Fatal("expected message to be created, got nil")
	}

	if msg.ThreadTS() != "1234567890.123456" {
		t.Errorf("expected ThreadTS to be %q, got %q", "1234567890.123456", msg.ThreadTS())
	}
}

func TestNewMessage_AppMentionEvent(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{
				Type:           "app_mention",
				User:           "U123456",
				Text:           "<@UBOT123> help",
				TimeStamp:      "1234567890.123456",
				Channel:        "C123456",
				EventTimeStamp: "1234567890.123456",
			},
		},
	}

	msg := slack.NewMessage(ctx, event)

	if msg == nil {
		t.Fatal("expected message to be created, got nil")
	}

	if msg.Text() != "<@UBOT123> help" {
		t.Errorf("expected Text to be %q, got %q", "<@UBOT123> help", msg.Text())
	}
}

func TestNewMessage_UnsupportedEvent(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type: slackevents.URLVerification,
	}

	msg := slack.NewMessage(ctx, event)

	if msg != nil {
		t.Errorf("expected nil for unsupported event type, got %+v", msg)
	}
}

func TestNewMessageFromData(t *testing.T) {
	createdAt := time.Now()

	msg := slack.NewMessageFromData(
		"1234567890.123456",
		"C123456",
		"1234567890.123455",
		"T123456",
		"U123456",
		"john_doe",
		"Test message",
		"1234567890.123456",
		createdAt,
	)

	if msg == nil {
		t.Fatal("expected message to be created, got nil")
	}

	if msg.ID() != "1234567890.123456" {
		t.Errorf("expected ID to be %q, got %q", "1234567890.123456", msg.ID())
	}

	if msg.ChannelID() != "C123456" {
		t.Errorf("expected ChannelID to be %q, got %q", "C123456", msg.ChannelID())
	}

	if msg.ThreadTS() != "1234567890.123455" {
		t.Errorf("expected ThreadTS to be %q, got %q", "1234567890.123455", msg.ThreadTS())
	}

	if msg.TeamID() != "T123456" {
		t.Errorf("expected TeamID to be %q, got %q", "T123456", msg.TeamID())
	}

	if msg.UserID() != "U123456" {
		t.Errorf("expected UserID to be %q, got %q", "U123456", msg.UserID())
	}

	if msg.UserName() != "john_doe" {
		t.Errorf("expected UserName to be %q, got %q", "john_doe", msg.UserName())
	}

	if msg.Text() != "Test message" {
		t.Errorf("expected Text to be %q, got %q", "Test message", msg.Text())
	}

	if msg.EventTS() != "1234567890.123456" {
		t.Errorf("expected EventTS to be %q, got %q", "1234567890.123456", msg.EventTS())
	}

	if !msg.CreatedAt().Equal(createdAt) {
		t.Errorf("expected CreatedAt to be %v, got %v", createdAt, msg.CreatedAt())
	}
}
