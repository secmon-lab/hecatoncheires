package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// BuildThreadedMarkdown is exported for testing
var BuildThreadedMarkdown = buildThreadedMarkdown

// BuildSlackSourceURLs is exported for testing
var BuildSlackSourceURLs = buildSlackSourceURLs

// BuildAgentSystemPrompt is exported for testing
var BuildAgentSystemPrompt = (*AgentUseCase).buildSystemPrompt

// Type aliases for testing
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
type ConversationMessage = slack.ConversationMessage
