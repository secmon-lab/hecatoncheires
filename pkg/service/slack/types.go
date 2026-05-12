package slack

import (
	"context"
	"time"

	"github.com/slack-go/slack"
)

// Service provides interface to Slack API for Source operations
type Service interface {
	// ListJoinedChannels retrieves the list of channels the bot has joined.
	// If teamID is non-empty, only channels in that workspace are returned (for org-level apps).
	// If teamID is empty, behaves the same as before (single-workspace mode).
	ListJoinedChannels(ctx context.Context, teamID string) ([]Channel, error)

	// GetChannelNames retrieves channel names for the given IDs (with caching)
	// Used for displaying channel names in the UI
	GetChannelNames(ctx context.Context, ids []string) (map[string]string, error)

	// GetUserInfo retrieves user information for the given user ID
	GetUserInfo(ctx context.Context, userID string) (*User, error)

	// ListUsers retrieves all non-deleted, non-bot users.
	// For org-level apps, team_id is required per Slack API spec.
	// For WS-level apps, pass empty string (behaves as before).
	ListUsers(ctx context.Context, teamID string) ([]*User, error)

	// CreateChannel creates a new Slack channel for a case.
	// The channel name is automatically generated from caseID, caseName, and the given prefix.
	// If isPrivate is true, the channel is created as a private channel.
	// If teamID is non-empty, the channel is created in the specified workspace (for org-level apps).
	// Returns the channel ID on success.
	CreateChannel(ctx context.Context, caseID int64, caseName string, prefix string, isPrivate bool, teamID string) (string, error)

	// GetConversationMembers retrieves the list of user IDs in the given channel
	// Handles Slack API pagination internally
	GetConversationMembers(ctx context.Context, channelID string) ([]string, error)

	// GetChannelInfo retrieves a channel's identity, topic, purpose, and a
	// few orientation hints (privacy, member count, archive flag). Backed
	// by Slack's `conversations.info` endpoint; the same scopes that gate
	// `channels:read` / `groups:read` cover this call. Used by the draft
	// agent to give the planner enough channel-level context to pick the
	// right workspace and frame the case correctly.
	GetChannelInfo(ctx context.Context, channelID string) (*ChannelInfo, error)

	// RenameChannel renames an existing Slack channel for a case
	// The channel name is automatically generated from caseID, caseName, and the given prefix
	RenameChannel(ctx context.Context, channelID string, caseID int64, caseName string, prefix string) error

	// InviteUsersToChannel invites users to a Slack channel
	// Silently skips if userIDs is empty
	InviteUsersToChannel(ctx context.Context, channelID string, userIDs []string) error

	// AddBookmark adds a link bookmark to a Slack channel
	AddBookmark(ctx context.Context, channelID, title, link string) error

	// GetTeamURL retrieves the Slack workspace URL (e.g., "https://ubie.enterprise.slack.com/")
	// The result is cached for the lifetime of the service instance.
	GetTeamURL(ctx context.Context) (string, error)

	// PostMessage posts a Block Kit message to a channel and returns the message timestamp.
	// The text parameter is used as a fallback for notifications.
	PostMessage(ctx context.Context, channelID string, blocks []slack.Block, text string) (string, error)

	// UpdateMessage updates an existing Block Kit message identified by channel and timestamp.
	UpdateMessage(ctx context.Context, channelID string, timestamp string, blocks []slack.Block, text string) error

	// GetConversationReplies retrieves messages from a thread.
	GetConversationReplies(ctx context.Context, channelID string, threadTS string, limit int) ([]ConversationMessage, error)

	// GetConversationHistory retrieves channel messages from the specified time.
	GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]ConversationMessage, error)

	// PostThreadReply posts a text message as a thread reply and returns the message timestamp.
	PostThreadReply(ctx context.Context, channelID string, threadTS string, text string) (string, error)

	// PostThreadMessage posts a Block Kit message as a thread reply and returns the message timestamp.
	// Accepts optional PostThreadOption values (e.g. WithBroadcastToChannel) to tweak posting behavior.
	PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slack.Block, text string, opts ...PostThreadOption) (string, error)

	// GetBotUserID retrieves the bot's own user ID via auth.test API.
	// The result is cached permanently (sync.Once).
	GetBotUserID(ctx context.Context) (string, error)

	// OpenView opens a modal view in Slack using the provided trigger ID.
	OpenView(ctx context.Context, triggerID string, view slack.ModalViewRequest) error

	// ListUserGroups retrieves all user groups in the workspace.
	// If teamID is non-empty, only groups in that workspace are returned (for org-level apps).
	// If teamID is empty, behaves the same as before (single-workspace mode).
	ListUserGroups(ctx context.Context, teamID string) ([]UserGroup, error)

	// GetUserGroupMembers retrieves the member user IDs of a user group.
	GetUserGroupMembers(ctx context.Context, groupID string) ([]string, error)

	// ListTeams returns all workspace team IDs accessible by the bot token.
	// For org-level apps, this returns all workspaces in the enterprise.
	// For WS-level apps, this returns a single workspace.
	ListTeams(ctx context.Context) ([]Team, error)

	// PostEphemeral posts an ephemeral message visible only to the specified user in a channel.
	// Uses chat.postEphemeral API. The message does not persist across reloads or sessions.
	PostEphemeral(ctx context.Context, channelID string, userID string, text string) error

	// PostEphemeralBlocks posts an ephemeral Block Kit message visible only to the specified user.
	// Returns the message timestamp for later updates via response_url or chat.update.
	PostEphemeralBlocks(ctx context.Context, channelID string, userID string, blocks []slack.Block, text string) (string, error)

	// GetPermalink retrieves the public permalink to a specific message.
	GetPermalink(ctx context.Context, channelID string, messageTS string) (string, error)
}

// PostThreadOptions captures the resolved configuration produced by applying
// PostThreadOption values. It is exposed so test fakes / callers can apply the
// options themselves and observe what was requested (Broadcast, etc).
type PostThreadOptions struct {
	Broadcast bool
}

// PostThreadOption tweaks how a threaded reply is posted via PostThreadMessage.
// Use the With* constructors below; do not construct values manually.
type PostThreadOption func(*PostThreadOptions)

// ApplyPostThreadOptions walks the given options and returns the resulting
// configuration. Used internally by the client and by test fakes that need
// to observe what options were requested.
func ApplyPostThreadOptions(opts ...PostThreadOption) PostThreadOptions {
	var cfg PostThreadOptions
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithBroadcastToChannel makes the threaded reply also surface in the
// parent channel ("Also sent to #channel" label), keeping the message
// inside the thread while raising its visibility to all members.
func WithBroadcastToChannel() PostThreadOption {
	return func(o *PostThreadOptions) { o.Broadcast = true }
}

// UserGroup represents a Slack user group
type UserGroup struct {
	ID     string
	Handle string
	Name   string
}

// Channel represents a Slack channel
type Channel struct {
	ID   string
	Name string
}

// ChannelInfo is the rich channel descriptor returned by GetChannelInfo. It
// trims the slack-go Channel down to the fields callers in this codebase
// actually need (mostly LLM-context oriented), so consumers don't take a
// transitive dep on the slack-go shape.
type ChannelInfo struct {
	ID         string
	Name       string
	Topic      string
	Purpose    string
	IsPrivate  bool
	IsArchived bool
	IsShared   bool // shared / connected / org-shared (any cross-workspace flavour)
	NumMembers int
	Creator    string    // Slack user ID of the channel creator
	CreatedAt  time.Time // Channel creation time (UTC)
}

// ConversationMessage represents a message retrieved from Slack conversation history
type ConversationMessage struct {
	UserID    string
	UserName  string
	Text      string
	Timestamp string
	ThreadTS  string
}

// Team represents a Slack workspace (team)
type Team struct {
	ID   string
	Name string
}

// User represents a Slack user
type User struct {
	ID       string
	Name     string
	RealName string
	Email    string
	ImageURL string
	Locale   string
}
