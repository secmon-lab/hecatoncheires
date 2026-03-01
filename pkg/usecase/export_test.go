package usecase

import (
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	githubsvc "github.com/secmon-lab/hecatoncheires/pkg/service/github"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// BuildThreadedMarkdown is exported for testing
var BuildThreadedMarkdown = buildThreadedMarkdown

// BuildSlackSourceURLs is exported for testing
var BuildSlackSourceURLs = buildSlackSourceURLs

// BuildAgentSystemPrompt is exported for testing
var BuildAgentSystemPrompt = (*AgentUseCase).buildSystemPrompt

// BuildAssistSystemPrompt is exported for testing
var BuildAssistSystemPrompt = (*AssistUseCase).buildAssistSystemPrompt

// AssistPromptData is exported for testing template rendering
type AssistPromptData = assistPromptData
type AssistPromptAction = assistPromptAction
type AssistPromptMessage = assistPromptMessage
type AssistPromptAssistLog = assistPromptAssistLog
type AssistPromptMemory = assistPromptMemory

// BuildPRMarkdown is exported for testing
var BuildPRMarkdown = buildPRMarkdown

// BuildIssueMarkdown is exported for testing
var BuildIssueMarkdown = buildIssueMarkdown

// BuildUpdatedDiscussionMarkdown is exported for testing
var BuildUpdatedDiscussionMarkdown = buildUpdatedDiscussionMarkdown

// TestErrAccessDenied is exported for testing
var TestErrAccessDenied = ErrAccessDenied

// Type aliases for testing
type GitHubPullRequest = githubsvc.PullRequest
type GitHubIssue = githubsvc.Issue
type GitHubIssueWithComments = githubsvc.IssueWithComments
type GitHubComment = githubsvc.Comment
type GitHubReview = githubsvc.Review
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
type ConversationMessage = slack.ConversationMessage
