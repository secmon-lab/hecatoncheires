package slack

import (
	"context"
	"time"

	libslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// Message represents a Slack message domain model
type Message struct {
	id        string
	channelID string
	threadTS  string
	teamID    string
	userID    string
	userName  string
	text      string
	eventTS   string
	files     []File
	createdAt time.Time
}

// NewMessage creates a new Message from a Slack Events API event
func NewMessage(ctx context.Context, ev *slackevents.EventsAPIEvent) *Message {
	if ev.Type != slackevents.CallbackEvent {
		return nil
	}

	innerEvent := ev.InnerEvent
	now := time.Now()

	switch evt := innerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		return &Message{
			id:        evt.TimeStamp,
			channelID: evt.Channel,
			threadTS:  evt.ThreadTimeStamp,
			teamID:    ev.TeamID,
			userID:    evt.User,
			userName:  evt.User, // Default to user ID, will be updated later if needed
			text:      MessageBody(evt.Text, evt.Blocks, evt.Attachments),
			eventTS:   evt.EventTimeStamp,
			createdAt: now,
		}
	case *slackevents.MessageEvent:
		threadTS := ""
		if evt.ThreadTimeStamp != "" && evt.ThreadTimeStamp != evt.TimeStamp {
			threadTS = evt.ThreadTimeStamp
		}
		// MessageEvent carries `blocks` at the top level but NOT `attachments`;
		// slack-go's custom unmarshaller re-decodes the whole payload into
		// evt.Message, which is where the attachments (and the files below)
		// land. An integration such as the GitHub Slack app puts the entire
		// notification there and leaves evt.Text empty.
		var (
			files       []File
			attachments []libslack.Attachment
		)
		if evt.Message != nil {
			for _, f := range evt.Message.Files {
				files = append(files, NewFileFromSlack(f))
			}
			attachments = evt.Message.Attachments
		}
		return &Message{
			id:        evt.TimeStamp,
			channelID: evt.Channel,
			threadTS:  threadTS,
			teamID:    ev.TeamID,
			userID:    evt.User,
			userName:  evt.User, // Default to user ID
			text:      MessageBody(evt.Text, evt.Blocks, attachments),
			eventTS:   evt.EventTimeStamp,
			files:     files,
			createdAt: now,
		}
	default:
		return nil
	}
}

// Getters to maintain immutability
func (m *Message) ID() string {
	return m.id
}

func (m *Message) ChannelID() string {
	return m.channelID
}

func (m *Message) ThreadTS() string {
	return m.threadTS
}

func (m *Message) TeamID() string {
	return m.teamID
}

func (m *Message) UserID() string {
	return m.userID
}

func (m *Message) UserName() string {
	return m.userName
}

func (m *Message) Text() string {
	return m.text
}

func (m *Message) EventTS() string {
	return m.eventTS
}

func (m *Message) Files() []File {
	return m.files
}

func (m *Message) CreatedAt() time.Time {
	return m.createdAt
}

// NewMessageFromData creates a Message from raw data (for repository reconstruction)
func NewMessageFromData(id, channelID, threadTS, teamID, userID, userName, text, eventTS string, createdAt time.Time, files []File) *Message {
	return &Message{
		id:        id,
		channelID: channelID,
		threadTS:  threadTS,
		teamID:    teamID,
		userID:    userID,
		userName:  userName,
		text:      text,
		eventTS:   eventTS,
		files:     files,
		createdAt: createdAt,
	}
}
