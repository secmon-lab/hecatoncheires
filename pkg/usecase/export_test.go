package usecase

import (
	"context"
	"time"

	githubsvc "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

// NewSlackDraftHandlerForTest builds a slackDraftHandler with the
// minimum dependencies required by the per-task trace tests. The Slack
// service is the only side-effect surface; the rest is plumbing.
func NewSlackDraftHandlerForTest(
	repo interfaces.Repository,
	registry *model.WorkspaceRegistry,
	slackService slack.Service,
	channelID, threadTS string,
) proposal.Handler {
	return newSlackDraftHandler(
		repo, registry, slackService,
		channelID, threadTS, "1700000000.000001", "U-test",
		nil, model.CaseProposalID("draft-test"), "", "",
	)
}

// BuildTraceContextBlocksForTest is exported for testing
var BuildTraceContextBlocksForTest = buildTraceContextBlocks

// TraceMessageForTest is the test-facing alias for the unexported
// traceMessage so external tests can exercise the append/replace rendering
// contract through the helpers below.
type TraceMessageForTest = traceMessage

// NewTraceMessageForTest builds a traceMessage wired to the given Slack
// service, mirroring newTraceMessage but injectable from external tests.
func NewTraceMessageForTest(svc slack.Service, channelID, threadTS string) *TraceMessageForTest {
	return &traceMessage{slackService: svc, channelID: channelID, threadTS: threadTS}
}

// TraceMessageAppendForTest invokes the unexported appendLine (milestone
// history) on a traceMessage.
func TraceMessageAppendForTest(tm *TraceMessageForTest, ctx context.Context, line string) {
	tm.appendLine(ctx, line)
}

// TraceMessageReplaceForTest invokes the unexported replaceLine (transient
// live line) on a traceMessage.
func TraceMessageReplaceForTest(tm *TraceMessageForTest, ctx context.Context, line string) {
	tm.replaceLine(ctx, line)
}

// MaxTraceBlocksForTest exposes the per-message block ceiling for assertions.
const MaxTraceBlocksForTest = maxTraceBlocks

// BuildCaseCreatedTailBlocksForTest is exported for testing
var BuildCaseCreatedTailBlocksForTest = buildCaseCreatedTailBlocks

// BuildPreviewBlocksForTest exposes buildPreviewBlocks so tests can assert
// on the localized notification fallback of the draft preview.
var BuildPreviewBlocksForTest = buildPreviewBlocks

// BuildProposalQuestionBlocksForTest exposes buildProposalQuestionBlocks so
// tests can assert on the localized notification fallback of the question form.
var BuildProposalQuestionBlocksForTest = buildProposalQuestionBlocks

// BuildThreadCreateQuestionBlocksForTest exposes buildThreadCreateQuestionBlocks
// so tests can assert on the localized notification fallback of the
// thread-mode question form.
var BuildThreadCreateQuestionBlocksForTest = buildThreadCreateQuestionBlocks

// BuildProposalUserInputForTest exposes the unexported buildProposalUserInput
// so tests in the external usecase_test package can assert on the
// planner's first-turn prompt content.
var BuildProposalUserInputForTest = buildProposalUserInput

// FirstSlackUserMentionForTest exposes firstSlackUserMention so tests can
// verify reporter extraction from a bot-relayed intake post.
var FirstSlackUserMentionForTest = firstSlackUserMention

// RenderReactionCreateInstructionForTest exposes renderReactionCreateInstruction
// so the reaction create-context prompt template can be golden-tested.
var RenderReactionCreateInstructionForTest = renderReactionCreateInstruction

// NormalizeReactionNameForTest exposes normalizeReactionName for skin-tone tests.
var NormalizeReactionNameForTest = normalizeReactionName

// EncodeCaseThreadValueForTest / ParseCaseThreadValueForTest expose the question
// Submit button value codec so its contract (and the legacy fallback) can be
// tested directly.
var EncodeCaseThreadValueForTest = encodeCaseThreadValue
var ParseCaseThreadValueForTest = parseCaseThreadValue

// BuildAssistSystemPrompt is exported for testing
var BuildAssistSystemPrompt = (*AssistUseCase).buildAssistSystemPrompt

// AssistPromptData is exported for testing template rendering
type AssistPromptData = assistPromptData
type AssistPromptAction = assistPromptAction
type AssistPromptMessage = assistPromptMessage
type AssistPromptAssistLog = assistPromptAssistLog

// TestErrAccessDenied is exported for testing
var TestErrAccessDenied = ErrAccessDenied

// Case write access-control helpers exposed for testing (see case_access.go).
var (
	AssertCaseWriteAccessForTest = assertCaseWriteAccess
	TokenActorForTest            = tokenActor
	LoadCaseForWriteForTest      = loadCaseForWrite
	ActorForAccessForTest        = actorForAccess
)

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

// ClampSlackOptionDescriptionForTest exposes the unexported
// clampSlackOptionDescription helper so external tests can verify the
// 75-rune option-description contract.
var ClampSlackOptionDescriptionForTest = clampSlackOptionDescription

// SlackOptionDescriptionMaxRunesForTest exposes the 75-rune ceiling for
// assertions.
const SlackOptionDescriptionMaxRunesForTest = slackOptionDescriptionMaxRunes

// BuildFieldOptionsForTest exposes buildFieldOptions so external tests can
// assert that long option descriptions are clamped before being handed to
// Slack.
var BuildFieldOptionsForTest = buildFieldOptions

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

// Mention-draft Edit modal block / action IDs exposed for tests that
// build the view_submission callback directly (e.g. the reporter
// recording test for HandleEditSubmit).
const (
	BlockIDDraftEditTitleForTest        = blockIDDraftEditTitle
	ActionIDDraftEditTitleForTest       = actionIDDraftEditTitle
	BlockIDDraftEditDescriptionForTest  = blockIDDraftEditDescription
	ActionIDDraftEditDescriptionForTest = actionIDDraftEditDescription
	BlockIDDraftEditTestForTest         = blockIDDraftEditTest
	ActionIDDraftEditTestForTest        = actionIDDraftEditTest
	CaseOptionValueTestForTest          = caseOptionValueTest
)

// NotificationSlotCoordinatorForTest is the test-only alias for the
// unexported notificationSlotCoordinator. External tests treat values of
// this type as opaque and exercise behaviour through the *ForTest helpers
// below.
type NotificationSlotCoordinatorForTest = notificationSlotCoordinator

// NewNotificationSlotCoordinatorForTest constructs a coordinator with an
// injectable clock so the test can drive slot expiry deterministically.
func NewNotificationSlotCoordinatorForTest(
	repo interfaces.NotificationSlotRepository,
	slackService slack.Service,
	slotDuration time.Duration,
	now func() time.Time,
) *NotificationSlotCoordinatorForTest {
	return newNotificationSlotCoordinator(repo, slackService, slotDuration, now)
}

// SlotEntryForTest is the test-facing alias for the coordinator's input
// struct so external tests can describe events without reaching into the
// unexported notification_slot.go internals.
type SlotEntryForTest = slotEntry

// EnqueueChannelLineForTest invokes the unexported enqueueChannelLine.
func EnqueueChannelLineForTest(c *NotificationSlotCoordinatorForTest, ctx context.Context, channelID string, entry SlotEntryForTest) {
	c.enqueueChannelLine(ctx, channelID, entry)
}

// NotificationSlotCoordinatorEnabledForTest exposes the enabled() probe.
func NotificationSlotCoordinatorEnabledForTest(c *NotificationSlotCoordinatorForTest) bool {
	return c.enabled()
}

// BuildSlotBlocksForTest exposes the Block Kit renderer for unit tests.
var BuildSlotBlocksForTest = buildSlotBlocks

// User-facing error classification / rendering seams (see uierr.go).
var (
	ClassifyUserErrorForTest    = classifyUserError
	UnexpectedUserFacingForTest = unexpectedUserFacing
	PrepareUserErrorForTest     = prepareUserError
	FallbackReasonErrorForTest  = fallbackReasonError
)

// Type aliases for testing
type GitHubPullRequest = githubsvc.PullRequest
type GitHubIssue = githubsvc.Issue
type GitHubIssueWithComments = githubsvc.IssueWithComments
type GitHubComment = githubsvc.Comment
type GitHubReview = githubsvc.Review
type SlackMessage = slackmodel.Message
type SlackChannel = model.SlackChannel
type ConversationMessage = slack.ConversationMessage
