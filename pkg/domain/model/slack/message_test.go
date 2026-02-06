package slack_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
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

	gt.Value(t, msg).NotNil().Required()

	gt.Value(t, msg.ID()).Equal("1234567890.123456")

	gt.Value(t, msg.ChannelID()).Equal("C123456")

	gt.Value(t, msg.TeamID()).Equal("T123456")

	gt.Value(t, msg.UserID()).Equal("U123456")

	gt.Value(t, msg.UserName()).Equal("U123456")

	gt.Value(t, msg.Text()).Equal("Hello, world!")

	gt.Value(t, msg.EventTS()).Equal("1234567890.123456")

	gt.Value(t, msg.ThreadTS()).Equal("")

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

	gt.Value(t, msg).NotNil().Required()

	gt.Value(t, msg.ThreadTS()).Equal("1234567890.123456")
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

	gt.Value(t, msg).NotNil().Required()

	gt.Value(t, msg.Text()).Equal("<@UBOT123> help")
}

func TestNewMessage_UnsupportedEvent(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type: slackevents.URLVerification,
	}

	msg := slack.NewMessage(ctx, event)

	gt.Value(t, msg).Nil()
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

	gt.Value(t, msg).NotNil().Required()

	gt.Value(t, msg.ID()).Equal("1234567890.123456")

	gt.Value(t, msg.ChannelID()).Equal("C123456")

	gt.Value(t, msg.ThreadTS()).Equal("1234567890.123455")

	gt.Value(t, msg.TeamID()).Equal("T123456")

	gt.Value(t, msg.UserID()).Equal("U123456")

	gt.Value(t, msg.UserName()).Equal("john_doe")

	gt.Value(t, msg.Text()).Equal("Test message")

	gt.Value(t, msg.EventTS()).Equal("1234567890.123456")

	gt.Value(t, msg.CreatedAt()).Equal(createdAt)
}
