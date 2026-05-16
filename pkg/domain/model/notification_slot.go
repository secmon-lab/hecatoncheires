package model

import "time"

// NotificationSlot is a per-Slack-channel rolling aggregation window for
// action/step change notifications. While a slot is active, additional
// channel-side notifications are folded into the same channel message via
// chat.update (one entry per event). The renderer groups entries by
// ActionMessageTS so the channel reader sees one section per Action with
// all of that Action's recent changes underneath, rather than every line
// repeating "for action X". When the slot expires, the next event posts a
// fresh channel message and replaces the slot.
//
// The struct is also the Firestore wire format: do NOT add `firestore:"..."`
// tags (see .claude/rules/firestore.md) and prefer Go-native field names.
type NotificationSlot struct {
	ChannelID string                  // Slack channel id (primary key)
	MessageTS string                  // Timestamp of the aggregated channel message
	Entries   []NotificationSlotEntry // Recorded change events (oldest first)
	SlotStart time.Time               // UTC time when the slot's channel message was first posted
	ExpiresAt time.Time               // SlotStart + slotDuration
	UpdatedAt time.Time
}

// NotificationSlotEntry captures one change event folded into a slot.
// ActionMessageTS is the grouping key (timestamp of the parent Action card
// in Slack); the renderer collects every entry sharing this key into a
// single section. ActionPermalink is resolved once at enqueue time and
// cached so subsequent updates don't re-hit Slack's chat.getPermalink.
type NotificationSlotEntry struct {
	ActionMessageTS string    // Parent Action card timestamp — used as the grouping key
	ActionTitle     string    // Action title as of the event (most recent wins per group)
	ActionPermalink string    // Slack permalink to the Action card; empty when lookup failed
	Body            string    // Pre-rendered change line ("@user changed status: A → B")
	EventTime       time.Time // UTC time the event was recorded
}
