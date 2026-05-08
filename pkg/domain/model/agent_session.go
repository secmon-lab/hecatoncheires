package model

import "time"

// AgentSession represents an ongoing agent conversation bound to a Slack thread.
//
// One session per (workspaceID, caseID, threadTS). The ID is a UUIDv7 generated
// on first creation and drives the storage paths for History and Trace
// artifacts in the agent archive layer.
type AgentSession struct {
	ID            string
	WorkspaceID   string
	CaseID        int64
	ThreadTS      string
	ChannelID     string
	ActionID      int64
	LastMentionTS string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
