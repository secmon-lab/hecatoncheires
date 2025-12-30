package slack

import (
	"context"
	"time"

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
			text:      evt.Text,
			eventTS:   evt.EventTimeStamp,
			createdAt: now,
		}
	case *slackevents.MessageEvent:
		threadTS := ""
		if evt.ThreadTimeStamp != "" && evt.ThreadTimeStamp != evt.TimeStamp {
			threadTS = evt.ThreadTimeStamp
		}
		return &Message{
			id:        evt.TimeStamp,
			channelID: evt.Channel,
			threadTS:  threadTS,
			teamID:    ev.TeamID,
			userID:    evt.User,
			userName:  evt.User, // Default to user ID
			text:      evt.Text,
			eventTS:   evt.EventTimeStamp,
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

func (m *Message) CreatedAt() time.Time {
	return m.createdAt
}

// NewMessageFromData creates a Message from raw data (for repository reconstruction)
func NewMessageFromData(id, channelID, threadTS, teamID, userID, userName, text, eventTS string, createdAt time.Time) *Message {
	return &Message{
		id:        id,
		channelID: channelID,
		threadTS:  threadTS,
		teamID:    teamID,
		userID:    userID,
		userName:  userName,
		text:      text,
		eventTS:   eventTS,
		createdAt: createdAt,
	}
}
