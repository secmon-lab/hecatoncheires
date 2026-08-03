package slack_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	libslack "github.com/slack-go/slack"
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
		nil,
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

func TestNewMessage_MessageEventWithFiles(t *testing.T) {
	ctx := context.Background()

	// MessageEvent.UnmarshalJSON normalizes top-level JSON fields (including "files")
	// into Message *slack.Msg. When constructing the struct directly (without JSON),
	// we set Message.Files to simulate the post-unmarshal state.
	// See TestNewMessage_MessageEventWithFilesFromJSON for a JSON-based test that
	// verifies the full unmarshal pipeline.
	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Type:           "message",
				User:           "U123456",
				Text:           "Check this file",
				TimeStamp:      "1234567890.123456",
				Channel:        "C123456",
				EventTimeStamp: "1234567890.123456",
				Message: &libslack.Msg{
					Files: []libslack.File{
						{
							ID:         "F001",
							Name:       "screenshot.png",
							Mimetype:   "image/png",
							Filetype:   "png",
							Size:       102400,
							URLPrivate: "https://files.slack.com/files-pri/T123-F001/screenshot.png",
							Permalink:  "https://workspace.slack.com/files/U123/F001/screenshot.png",
							Thumb480:   "https://files.slack.com/thumb_480.png",
						},
						{
							ID:         "F002",
							Name:       "document.pdf",
							Mimetype:   "application/pdf",
							Filetype:   "pdf",
							Size:       204800,
							URLPrivate: "https://files.slack.com/files-pri/T123-F002/document.pdf",
							Permalink:  "https://workspace.slack.com/files/U123/F002/document.pdf",
						},
					},
				},
			},
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()

	files := msg.Files()
	gt.Array(t, files).Length(2)

	gt.Value(t, files[0].ID()).Equal("F001")
	gt.Value(t, files[0].Name()).Equal("screenshot.png")
	gt.Value(t, files[0].Mimetype()).Equal("image/png")
	gt.Value(t, files[0].Size()).Equal(102400)
	gt.Value(t, files[0].ThumbURL()).Equal("https://files.slack.com/thumb_480.png")

	gt.Value(t, files[1].ID()).Equal("F002")
	gt.Value(t, files[1].Filetype()).Equal("pdf")
	gt.Value(t, files[1].ThumbURL()).Equal("")
}

func TestNewMessage_MessageEventWithFilesFromJSON(t *testing.T) {
	ctx := context.Background()

	// This test verifies that files are correctly extracted when MessageEvent
	// is parsed from real Slack JSON. In actual Slack events, files appear at
	// the top level of the event JSON, not nested inside a "message" sub-object.
	// MessageEvent.UnmarshalJSON normalizes this by unmarshaling the top-level
	// JSON into Message *slack.Msg when no "message" sub-field is present.
	innerEventJSON := `{
		"type": "message",
		"user": "U123456",
		"text": "Check this file",
		"ts": "1234567890.123456",
		"channel": "C123456",
		"event_ts": "1234567890.123456",
		"files": [
			{
				"id": "F001",
				"name": "screenshot.png",
				"mimetype": "image/png",
				"filetype": "png",
				"size": 102400,
				"url_private": "https://files.slack.com/files-pri/T123-F001/screenshot.png",
				"permalink": "https://workspace.slack.com/files/U123/F001/screenshot.png",
				"thumb_480": "https://files.slack.com/thumb_480.png"
			},
			{
				"id": "F002",
				"name": "document.pdf",
				"mimetype": "application/pdf",
				"filetype": "pdf",
				"size": 204800,
				"url_private": "https://files.slack.com/files-pri/T123-F002/document.pdf",
				"permalink": "https://workspace.slack.com/files/U123/F002/document.pdf"
			}
		]
	}`

	var msgEvent slackevents.MessageEvent
	gt.NoError(t, json.Unmarshal([]byte(innerEventJSON), &msgEvent)).Required()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &msgEvent,
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()

	files := msg.Files()
	gt.Array(t, files).Length(2)

	gt.Value(t, files[0].ID()).Equal("F001")
	gt.Value(t, files[0].Name()).Equal("screenshot.png")
	gt.Value(t, files[0].Mimetype()).Equal("image/png")
	gt.Value(t, files[0].Size()).Equal(102400)
	gt.Value(t, files[0].URLPrivate()).Equal("https://files.slack.com/files-pri/T123-F001/screenshot.png")
	gt.Value(t, files[0].Permalink()).Equal("https://workspace.slack.com/files/U123/F001/screenshot.png")
	gt.Value(t, files[0].ThumbURL()).Equal("https://files.slack.com/thumb_480.png")

	gt.Value(t, files[1].ID()).Equal("F002")
	gt.Value(t, files[1].Name()).Equal("document.pdf")
	gt.Value(t, files[1].Filetype()).Equal("pdf")
	gt.Value(t, files[1].ThumbURL()).Equal("")
}

func TestNewMessage_MessageEventWithoutFiles(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &slackevents.MessageEvent{
				Type:           "message",
				User:           "U123456",
				Text:           "No files here",
				TimeStamp:      "1234567890.123456",
				Channel:        "C123456",
				EventTimeStamp: "1234567890.123456",
			},
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()

	// Files should be nil/empty for messages without attachments
	gt.Array(t, msg.Files()).Length(0)
}

func TestNewMessage_AppMentionEventNoFiles(t *testing.T) {
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

	// AppMentionEvent does not support files
	gt.Array(t, msg.Files()).Length(0)
}

func TestNewMessageFromData_WithFiles(t *testing.T) {
	createdAt := time.Now()
	files := []slack.File{
		slack.NewFileFromData("F001", "test.png", "image/png", "png", 1024,
			"https://files.slack.com/private", "https://slack.com/permalink", "https://slack.com/thumb"),
	}

	msg := slack.NewMessageFromData(
		"1234567890.123456", "C123456", "", "T123456",
		"U123456", "user", "test", "1234567890.123456",
		createdAt, files,
	)

	gt.Value(t, msg).NotNil().Required()
	gt.Array(t, msg.Files()).Length(1)
	gt.Value(t, msg.Files()[0].ID()).Equal("F001")
	gt.Value(t, msg.Files()[0].Name()).Equal("test.png")
}

// A GitHub Slack integration notification arrives with an empty top-level
// "text": the whole body lives in "attachments", which slack-go exposes only
// through MessageEvent.Message (its custom unmarshaller re-decodes the payload
// there). Reading evt.Text alone made every such PR notification look like an
// empty message to the agent.
func TestNewMessage_MessageEventWithAttachmentsFromJSON(t *testing.T) {
	ctx := context.Background()

	innerEventJSON := `{
		"type": "message",
		"subtype": "bot_message",
		"text": "",
		"ts": "1785673711.954009",
		"channel": "C05CZJVUS5N",
		"event_ts": "1785673711.954009",
		"bot_id": "B01234567",
		"attachments": [
			{
				"color": "36a64f",
				"pretext": "Pull request opened by octocat",
				"title": "#297 Add a Design Doc for coding agent usage observability",
				"title_link": "https://github.com/example/design-doc/pull/297",
				"text": "Adds a Design Doc describing how coding agent usage is observed.",
				"fields": [
					{"title": "Reviewers", "value": "@alice, @bob", "short": true},
					{"title": "Comments", "value": "1", "short": true}
				],
				"footer": "example/design-doc"
			}
		]
	}`

	var msgEvent slackevents.MessageEvent
	gt.NoError(t, json.Unmarshal([]byte(innerEventJSON), &msgEvent)).Required()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &msgEvent,
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()

	gt.Value(t, msg.Text()).Equal(strings.Join([]string{
		"Pull request opened by octocat",
		"#297 Add a Design Doc for coding agent usage observability",
		"https://github.com/example/design-doc/pull/297",
		"Adds a Design Doc describing how coding agent usage is observed.",
		"Reviewers: @alice, @bob",
		"Comments: 1",
		"example/design-doc",
	}, "\n"))
}

func TestNewMessage_MessageEventWithBlocksFromJSON(t *testing.T) {
	ctx := context.Background()

	innerEventJSON := `{
		"type": "message",
		"subtype": "bot_message",
		"text": "",
		"ts": "1785673711.954010",
		"channel": "C05CZJVUS5N",
		"event_ts": "1785673711.954010",
		"blocks": [
			{"type": "header", "text": {"type": "plain_text", "text": "Deploy finished"}},
			{"type": "section", "text": {"type": "mrkdwn", "text": "*service*: api"}},
			{"type": "divider"},
			{"type": "context", "elements": [{"type": "mrkdwn", "text": "took 42s"}]}
		]
	}`

	var msgEvent slackevents.MessageEvent
	gt.NoError(t, json.Unmarshal([]byte(innerEventJSON), &msgEvent)).Required()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &msgEvent,
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()
	gt.Value(t, msg.Text()).Equal("Deploy finished\n*service*: api\ntook 42s")
}

// A human message carries the same content in "text" and in the rich_text
// blocks Slack derives it from; it must not be doubled.
func TestNewMessage_MessageEventHumanTextNotDuplicated(t *testing.T) {
	ctx := context.Background()

	innerEventJSON := `{
		"type": "message",
		"user": "U058S2H241M",
		"text": "<@U023SNFKMB6> please review this",
		"ts": "1785466360.561719",
		"channel": "C05CZJVUS5N",
		"event_ts": "1785466360.561719",
		"blocks": [
			{
				"type": "rich_text",
				"elements": [
					{
						"type": "rich_text_section",
						"elements": [
							{"type": "user", "user_id": "U023SNFKMB6"},
							{"type": "text", "text": " please review this"}
						]
					}
				]
			}
		]
	}`

	var msgEvent slackevents.MessageEvent
	gt.NoError(t, json.Unmarshal([]byte(innerEventJSON), &msgEvent)).Required()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: &msgEvent,
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()
	gt.Value(t, msg.Text()).Equal("<@U023SNFKMB6> please review this")
}

func TestNewMessage_AppMentionEventWithAttachments(t *testing.T) {
	ctx := context.Background()

	event := &slackevents.EventsAPIEvent{
		Type:   slackevents.CallbackEvent,
		TeamID: "T123456",
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: &slackevents.AppMentionEvent{
				Type:           "app_mention",
				User:           "U123456",
				Text:           "",
				TimeStamp:      "1234567890.123456",
				Channel:        "C123456",
				EventTimeStamp: "1234567890.123456",
				Attachments: []libslack.Attachment{{
					Title: "Alert fired",
					Text:  "cpu usage above threshold",
				}},
			},
		},
	}

	msg := slack.NewMessage(ctx, event)
	gt.Value(t, msg).NotNil().Required()
	gt.Value(t, msg.Text()).Equal("Alert fired\ncpu usage above threshold")
}
