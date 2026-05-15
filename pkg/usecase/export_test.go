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

// BuildCaseCreatedTailBlocksForTest is exported for testing
var BuildCaseCreatedTailBlocksForTest = buildCaseCreatedTailBlocks

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

// ShouldBroadcastActionEventForTest exposes shouldBroadcastActionEvent
// so external tests can verify the broadcast set membership.
var ShouldBroadcastActionEventForTest = shouldBroadcastActionEvent

// ShouldBroadcastAnyActionEventForTest exposes shouldBroadcastAnyActionEvent
// so external tests can verify multi-kind broadcast judgement.
var ShouldBroadcastAnyActionEventForTest = shouldBroadcastAnyActionEvent

// ClampPlainTextForTest exposes the unexported clampPlainText helper so
// external tests can verify the Slack input-length contract.
var ClampPlainTextForTest = clampPlainText

// IsLikelySlackUserIDForTest exposes the unexported isLikelySlackUserID
// helper so external tests can verify the user-ID syntactic gate.
var IsLikelySlackUserIDForTest = isLikelySlackUserID

// FilterSlackUserIDsForTest exposes the unexported filterSlackUserIDs
// helper so external tests can verify the slice filtering behaviour.
var FilterSlackUserIDsForTest = filterSlackUserIDs

// SlackPlainTextMaxRunesForTest exposes the clamp ceiling for assertions.
const SlackPlainTextMaxRunesForTest = slackPlainTextMaxRunes

// ClampSuffixMultiLineForTest exposes the multiline truncation sentinel.
const ClampSuffixMultiLineForTest = clampSuffixMultiLine

// ClampSuffixSingleLineForTest exposes the single-line truncation sentinel.
const ClampSuffixSingleLineForTest = clampSuffixSingleLine

// BuildDraftEditModalForTest exposes buildDraftEditModal so external tests
// can assert on the rendered Block Kit payload.
var BuildDraftEditModalForTest = buildDraftEditModal

// Type aliases for testing
type GitHubPullRequest = githubsvc.PullRequest
type GitHubIssue = githubsvc.Issue
type GitHubIssueWithComments = githubsvc.IssueWithComments
type GitHubComment = githubsvc.Comment
type GitHubReview = githubsvc.Review
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
type ConversationMessage = slack.ConversationMessage
