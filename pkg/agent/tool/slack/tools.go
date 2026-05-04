// Package slacktool contains gollem tools that let the AI agent interact with
// Slack: workspace-wide message search, bulk message fetch, and posting to a
// case channel.
//
// Tools and the underlying clients live here (rather than in pkg/service/slack)
// because their entire surface — including the User-token-backed search.messages
// API — exists only for the agent. The bot-token-backed slack.Service from
// pkg/service/slack is reused for chat.* / conversations.* operations.
package slacktool

import (
	"github.com/m-mizutani/gollem"
	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// Deps groups the dependencies needed to register Slack-backed agent tools.
type Deps struct {
	// Bot is the Slack bot-token client. Required for the bulk message-fetch
	// tool (slack__get_messages) and for the post-message tool. nil disables both.
	Bot slackservice.Service

	// Search is the Slack User-token-backed search client. Construct via
	// NewSearchClient with a User OAuth Token holding the search:read scope.
	// nil disables the workspace-wide message-search tool.
	Search SearchService

	// ChannelID is the Slack channel the case is bound to and is used by the
	// post-message tool. Empty disables it.
	ChannelID string
}

// NewReadOnly returns the read-only Slack tools (search, bulk get) — both safe
// to expose during interactive mention flows where the agent should observe
// rather than write.
func NewReadOnly(deps Deps) []gollem.Tool {
	var tools []gollem.Tool
	if deps.Search != nil {
		tools = append(tools, &searchMessagesTool{search: deps.Search})
	}
	if deps.Bot != nil {
		tools = append(tools, &getMessagesTool{slack: deps.Bot})
	}
	return tools
}

// NewForAssist extends NewReadOnly with the post-message tool, which needs the
// case-bound channel. Used by the assist flow where the agent is expected to
// produce Slack output.
func NewForAssist(deps Deps) []gollem.Tool {
	tools := NewReadOnly(deps)
	if deps.Bot != nil && deps.ChannelID != "" {
		tools = append(tools, &postMessageTool{slack: deps.Bot, channelID: deps.ChannelID})
	}
	return tools
}
