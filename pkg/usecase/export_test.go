package usecase

import (
	githubsvc "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
)

// NewSlackDraftHandlerForTest builds a slackDraftHandler with the
// minimum dependencies required by the per-task trace tests. The Slack
// service is the only side-effect surface; the rest is plumbing.
func NewSlackDraftHandlerForTest(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slack.Service,
	channelID, threadTS string,
) draft.Handler {
	return newSlackDraftHandler(
		repo, registry, slackService,
		channelID, threadTS, "1700000000.000001", "U-test",
		nil, model.CaseDraftID("draft-test"), "",
	)
}

// BuildTraceContextBlocksForTest is exported for testing
var BuildTraceContextBlocksForTest = buildTraceContextBlocks

// BuildDraftUserInputForTest exposes the unexported buildDraftUserInput
// so tests in the external usecase_test package can assert on the
// planner's first-turn prompt content.
var BuildDraftUserInputForTest = buildDraftUserInput

// BuildAssistSystemPrompt is exported for testing
var BuildAssistSystemPrompt = (*AssistUseCase).buildAssistSystemPrompt

// AssistPromptData is exported for testing template rendering
type AssistPromptData = assistPromptData
type AssistPromptAction = assistPromptAction
type AssistPromptMessage = assistPromptMessage
type AssistPromptAssistLog = assistPromptAssistLog

// TestErrAccessDenied is exported for testing
var TestErrAccessDenied = ErrAccessDenied

// NewWelcomeRendererForTest is exported for testing
var NewWelcomeRendererForTest = newWelcomeRenderer

// BuildWelcomeFieldsForTest is exported for testing
var BuildWelcomeFieldsForTest = buildWelcomeFields

// WelcomeContextForTest is exported for testing
type WelcomeContextForTest = welcomeContext

// WelcomeRendererRenderForTest invokes the (unexported) Render method on a
// welcomeRenderer for tests in the external package.
func WelcomeRendererRenderForTest(r *welcomeRenderer, ctx welcomeContext) ([]string, error) {
	return r.Render(ctx)
}

// Type aliases for testing
type GitHubPullRequest = githubsvc.PullRequest
type GitHubIssue = githubsvc.Issue
type GitHubIssueWithComments = githubsvc.IssueWithComments
type GitHubComment = githubsvc.Comment
type GitHubReview = githubsvc.Review
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
type ConversationMessage = slack.ConversationMessage
