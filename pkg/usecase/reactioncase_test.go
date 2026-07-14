package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	goslack "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// createDecisionScripts drives one ModeCreate turn to completion: planner →
// sub-agent → replan(finalize) → final create decision. Reused for reaction
// creation, which flows through the same create path.
func createDecisionScripts() []string {
	return []string{
		tcInvestigatePlan,
		"The message reports a login outage.",
		tcReplanDone,
		`{"kind":"materialize","title":"Login outage","description":"Users cannot log in.","fields":[{"field_id":"severity","value":"high"}]}`,
	}
}

func newReactionWorkspaceRegistry() *model.WorkspaceRegistry {
	set, _ := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		ReactionEmoji:         "incident",
		CaseStatusSet:         set,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	})
	return reg
}

func newReactionSetup(t *testing.T, llm gollem.LLMClient) (*usecase.SlackUseCases, *usecase.AgentUseCase, *memory.Memory, *agentTestSlackService) {
	t.Helper()
	repo := memory.New()
	reg := newReactionWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})
	slackUC := usecase.NewSlackUseCases(repo, reg, agentUC, nil, slackMock)
	return slackUC, agentUC, repo, slackMock
}

func reactionEvent(reaction, user, itemUser, channel, ts string) *slackevents.EventsAPIEvent {
	return &slackevents.EventsAPIEvent{
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "reaction_added",
			Data: &slackevents.ReactionAddedEvent{
				Reaction: reaction,
				User:     user,
				ItemUser: itemUser,
				Item:     slackevents.Item{Type: "message", Channel: channel, Timestamp: ts},
			},
		},
	}
}

// countPostsTo returns how many plain PostMessage calls landed on channelID.
func countPostsTo(slackMock *agentTestSlackService, channelID string) int {
	n := 0
	for _, ch := range slackMock.postedChannelIDs {
		if ch == channelID {
			n++
		}
	}
	return n
}

func TestReactionCreateContextPrompt(t *testing.T) {
	out := usecase.RenderReactionCreateInstructionForTest(context.Background(), "1700000000.000100")
	// The anchor timestamp is interpolated and the core guidance is present.
	gt.String(t, out).Contains("1700000000.000100")
	gt.String(t, out).Contains("BEFORE")
	gt.String(t, out).Contains("AFTER")
	gt.String(t, out).Contains("anchor")
}

func TestNormalizeReactionName(t *testing.T) {
	gt.String(t, usecase.NormalizeReactionNameForTest("incident")).Equal("incident")
	// Skin-tone modifiers are dropped so the base emoji matches config.
	gt.String(t, usecase.NormalizeReactionNameForTest("wave::skin-tone-3")).Equal("wave")
}

// B5: an emoji not configured on any workspace is ignored — no case, no posts.
func TestReaction_UnsupportedEmoji_NoOp(t *testing.T) {
	slackUC, _, repo, slackMock := newReactionSetup(t, newScriptedClient([]string{}))
	ctx := context.Background()

	ev := reactionEvent("thumbsup", "U-REACTOR", "U-AUTHOR", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
	async.Wait()

	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c).Nil()
	gt.Array(t, slackMock.postedMessages).Length(0)
	gt.Array(t, slackMock.postedChannelIDs).Length(0)
}

// B6: our own reaction, or a reaction on our own message, never triggers.
func TestReaction_BotLoopGuards_NoOp(t *testing.T) {
	ctx := context.Background()

	t.Run("bot is the reactor", func(t *testing.T) {
		slackUC, _, repo, _ := newReactionSetup(t, newScriptedClient([]string{}))
		ev := reactionEvent("incident", "UBOT001", "U-AUTHOR", "C-MONITOR", "1700000000.000100")
		gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
		async.Wait()
		c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	t.Run("reacted message was authored by the bot", func(t *testing.T) {
		slackUC, _, repo, _ := newReactionSetup(t, newScriptedClient([]string{}))
		ev := reactionEvent("incident", "U-REACTOR", "UBOT001", "C-MONITOR", "1700000000.000100")
		gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
		async.Wait()
		c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})
}

// B1: a reaction inside the monitored channel turns the reacted message's thread
// into a case directly (same-channel path), with the reactor as reporter.
func TestReaction_SameChannel_CreatesCase(t *testing.T) {
	slackUC, _, repo, _ := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	ev := reactionEvent("incident", "U-REACTOR", "U-AUTHOR", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
	async.Wait()

	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.SlackChannelID).Equal("C-MONITOR")
	gt.Value(t, c.SlackThreadTS).Equal("1700000000.000100")
	// Reporter is the person who reacted, not the message author.
	gt.Value(t, c.ReporterID).Equal("U-REACTOR")
}

// B3: reacting on a reply binds the case to the thread root (parent), not the
// reply's own ts.
func TestReaction_SameChannel_ReplyNormalizesToThreadRoot(t *testing.T) {
	slackUC, _, repo, slackMock := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	// conversations.replies for the reacted reply returns the parent first.
	slackMock.getConversationRepliesFn = func(_ context.Context, _ string, _ string, _ int) ([]slack.ConversationMessage, error) {
		return []slack.ConversationMessage{
			{Timestamp: "1700000000.000100", UserID: "U-AUTHOR", Text: "root", ThreadTS: "1700000000.000100"},
			{Timestamp: "1700000000.000200", UserID: "U-REPLY", Text: "reply", ThreadTS: "1700000000.000100"},
		}, nil
	}

	// React on the reply (ts .000200).
	ev := reactionEvent("incident", "U-REACTOR", "U-REPLY", "C-MONITOR", "1700000000.000200")
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
	async.Wait()

	// The case binds to the thread root, not the reply.
	atReply, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000200")
	gt.NoError(t, err).Required()
	gt.Value(t, atReply).Nil()
	atRoot, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, atRoot).NotNil().Required()
	gt.Value(t, atRoot.SlackThreadTS).Equal("1700000000.000100")
}

// B16: a second reaction on a different message in the same monitored-channel
// thread is a no-op — the thread is already a case.
func TestReaction_SameChannel_SecondReactionSameThreadNoOp(t *testing.T) {
	slackUC, _, repo, _ := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	// First reaction creates the case (thread root .000100).
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-REACTOR", "U-AUTHOR", "C-MONITOR", "1700000000.000100"))).Required()
	async.Wait()
	c1, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c1).NotNil().Required()

	// Second reaction on another message in the same thread resolves to the same
	// root; the case already exists so nothing new happens. The scripted LLM has
	// no more responses, so any second create turn would error — a no-op proves
	// the dedup.
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-OTHER", "U-AUTHOR", "C-MONITOR", "1700000000.000100"))).Required()
	async.Wait()

	c2, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Value(t, c2.ID).Equal(c1.ID)
}

// B1/FR-2/FR-5: a reaction outside the monitored channel posts a placeholder
// root in the monitored channel that is replaced in place by the shared case
// summary, posts the reaction-specific origin (reporter + exact source link) as
// a reply under it, binds the case there, and posts a back-link in the source
// thread. The root summary carries no reaction-specific info — that lives only
// in the origin reply.
func TestReaction_CrossChannel_CreatesCaseAndBacklink(t *testing.T) {
	slackUC, _, repo, slackMock := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	const srcPermalink = "https://slack.test/C-GENERAL/1700000000.000100"

	ev := reactionEvent("incident", "U-REACTOR", "U-AUTHOR", "C-GENERAL", "1700000000.000100")
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, ev)).Required()
	async.Wait()

	// Exactly one placeholder root reached the monitored channel, and its text is
	// a neutral placeholder — no source permalink, no "flag" preamble.
	gt.Number(t, countPostsTo(slackMock, "C-MONITOR")).Equal(1)
	for _, txt := range slackMock.postedTexts {
		gt.Bool(t, strings.Contains(txt, srcPermalink)).False()
		gt.Bool(t, strings.Contains(txt, "flag")).False()
	}

	// The placeholder root (default PostMessage ts) was replaced in place with the
	// shared summary: it carries the case URL but NOT the reporter mention or the
	// source permalink (those belong to the origin reply, not the common summary).
	var summary *agentUpdatedMessage
	for i := range slackMock.updatedMessages {
		if slackMock.updatedMessages[i].ChannelID == "C-MONITOR" && slackMock.updatedMessages[i].Timestamp == "1234567890.123456" {
			summary = &slackMock.updatedMessages[i]
		}
	}
	gt.Value(t, summary).NotNil().Required()
	gt.Bool(t, strings.Contains(summary.Text, "https://app.test/ws/support/cases/")).True()
	gt.Bool(t, strings.Contains(summary.Text, "<@U-REACTOR>")).False()
	gt.Bool(t, strings.Contains(summary.Text, srcPermalink)).False()

	// The origin reply was posted under the summary root (monitored channel,
	// thread = placeholder ts) carrying the reporter mention and the exact source
	// message permalink.
	var origin *agentPostedMessage
	for i := range slackMock.postedMessages {
		if slackMock.postedMessages[i].ChannelID == "C-MONITOR" && slackMock.postedMessages[i].ThreadTS == "1234567890.123456" {
			origin = &slackMock.postedMessages[i]
		}
	}
	gt.Value(t, origin).NotNil().Required()
	gt.Bool(t, strings.Contains(origin.Text, "<@U-REACTOR>")).True()
	gt.Bool(t, strings.Contains(origin.Text, srcPermalink)).True()

	// The case is bound to the placeholder thread in the monitored channel.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1234567890.123456")
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.ReporterID).Equal("U-REACTOR")

	// The source message reference was persisted on the session so a resume turn
	// could still link the exact message.
	ssn, err := repo.Session().GetByThread(ctx, "C-MONITOR", "1234567890.123456")
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.ReactionSourceChannelID).Equal("C-GENERAL")
	gt.Value(t, ssn.ReactionSourceMessageTS).Equal("1700000000.000100")

	// A back-link message was posted to the source thread carrying the case URL.
	foundBacklink := false
	for _, m := range slackMock.postedMessages {
		if m.ChannelID == "C-GENERAL" && strings.Contains(m.Text, "https://app.test/ws/support/cases/") {
			foundBacklink = true
		}
	}
	gt.Bool(t, foundBacklink).True()

	// The claim persists after success (a re-reaction must not create a second case).
	claimed, err := repo.ReactionClaim().Claim(ctx, "support", "C-GENERAL", "1700000000.000100")
	gt.NoError(t, err).Required()
	gt.Bool(t, claimed).False()
}

// B7: two reactions on the same out-of-channel message create only one case; the
// second is deduped by the claim before any placeholder root is posted.
func TestReaction_CrossChannel_DedupeOnSourceMessage(t *testing.T) {
	slackUC, _, _, slackMock := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-A", "U-AUTHOR", "C-GENERAL", "1700000000.000100"))).Required()
	async.Wait()
	// A different user reacts on the same message.
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-B", "U-AUTHOR", "C-GENERAL", "1700000000.000100"))).Required()
	async.Wait()

	// Exactly one placeholder root reached the monitored channel.
	gt.Number(t, countPostsTo(slackMock, "C-MONITOR")).Equal(1)
}

// A failed placeholder-root post must not leave the reactor with no response: an
// error is posted to their source thread, and the claim is released so a retry
// works.
func TestReaction_CrossChannel_PlaceholderPostFailureNotifiesReactor(t *testing.T) {
	slackUC, _, repo, slackMock := newReactionSetup(t, newScriptedClient([]string{}))
	ctx := context.Background()

	slackMock.postMessageFn = func(_ context.Context, _ string, _ []goslack.Block, _ string) (string, error) {
		return "", errors.New("channel_not_found")
	}

	const srcChannel = "C-GENERAL"
	const srcTS = "1700000000.000100"
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-REACTOR", "U-AUTHOR", srcChannel, srcTS))).Required()
	async.Wait()

	// No case was created.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", "1234567890.123456")
	gt.NoError(t, err).Required()
	gt.Value(t, c).Nil()

	// An error reply was posted back in the reactor's source thread.
	foundError := false
	for _, m := range slackMock.postedMessages {
		if m.ChannelID == srcChannel && strings.Contains(m.Text, "⚠️") {
			foundError = true
		}
	}
	gt.Bool(t, foundError).True()

	// The claim was released, so the source message can be reacted again.
	claimed, err := repo.ReactionClaim().Claim(ctx, "support", srcChannel, srcTS)
	gt.NoError(t, err).Required()
	gt.Bool(t, claimed).True()
}

// FR-6/D2 (the core of the design): a cross-channel reaction that raises a
// question posts the form in the reactor's source thread, and the answer resumes
// creation by reconstructing (uiThread = the Submit callback's location,
// caseThread = the Submit button value) with no Session marker. This is the most
// fragile path — the value codec or the callback-location parse breaking here
// would silently drop the reactor's answer.
func TestReaction_CrossChannel_QuestionResume(t *testing.T) {
	scripts := []string{
		// Turn 1 (create): plan -> sub-agent -> ask a question.
		tcInvestigatePlan,
		"Need the severity.",
		`{"message":"need info","question":{"reason":"What severity?","items":[{"id":"q-sev","text":"Severity?","type":"select","options":["high","low"]}]}}`,
		// Turn 2 (resume after submit): plan -> sub-agent -> replan done -> create.
		tcInvestigatePlan,
		"Reporter said high.",
		tcReplanDone,
		`{"title":"Login outage","description":"Users cannot log in.","fields":[{"field_id":"severity","value":"high"}]}`,
	}
	slackUC, agentUC, repo, slackMock := newReactionSetup(t, newScriptedClient(scripts))
	ctx := context.Background()

	const srcChannel = "C-GENERAL"
	const srcTS = "1700000000.000100"
	const placeholderTS = "1234567890.123456" // the default PostMessage ts = the placeholder root ts
	const srcPermalink = "https://slack.test/C-GENERAL/1700000000.000100"

	// A reaction outside the monitored channel makes the create agent ask a question.
	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-REACTOR", "U-AUTHOR", srcChannel, srcTS))).Required()
	async.Wait()

	// No case yet. The session sits on the case thread (monitored channel,
	// placeholder ts), and the question form was posted in the reactor's source
	// thread. The source message reference is already persisted so the resume turn
	// can still link the exact message.
	noCase, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", placeholderTS)
	gt.NoError(t, err).Required()
	gt.Value(t, noCase).Nil()
	ssn, err := repo.Session().GetByThread(ctx, "C-MONITOR", placeholderTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.ReactionSourceChannelID).Equal(srcChannel)
	gt.Value(t, ssn.ReactionSourceMessageTS).Equal(srcTS)
	gt.Value(t, ssn.PendingQuestion).NotNil().Required()
	gt.Value(t, ssn.PendingQuestion.PostedChannelID).Equal(srcChannel)
	formTS := ssn.PendingQuestion.PostedMessageTS

	// The reactor answers in the source thread. The Submit callback's own location
	// is the source (UI) thread; the button value carries the case thread.
	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U-REACTOR"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: srcChannel}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: formTS, ThreadTimestamp: srcTS}},
		BlockActionState: &goslack.BlockActionStates{
			Values: map[string]map[string]goslack.BlockAction{
				usecase.BlockIDDraftQuestionItemPrefix + "q-sev": {
					usecase.ActionIDDraftQuestionChoice: {SelectedOption: goslack.OptionBlockObject{Value: "high"}},
				},
			},
		},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: "C-MONITOR:" + placeholderTS}},
		},
	}
	gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	// Resume reconstructed both threads and committed the case in the monitored
	// channel with the reactor as reporter.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", placeholderTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).NotNil().Required()
	gt.Value(t, c.Title).Equal("Login outage")
	gt.Value(t, c.ReporterID).Equal("U-REACTOR")

	// The placeholder root was replaced in place with the shared summary.
	var summary *agentUpdatedMessage
	for i := range slackMock.updatedMessages {
		if slackMock.updatedMessages[i].ChannelID == "C-MONITOR" && slackMock.updatedMessages[i].Timestamp == placeholderTS {
			summary = &slackMock.updatedMessages[i]
		}
	}
	gt.Value(t, summary).NotNil().Required()
	gt.Bool(t, strings.Contains(summary.Text, "https://app.test/ws/support/cases/")).True()

	// Even after a question/resume, the origin reply under the summary root links
	// the exact source message (proving the reference survived via the session).
	var origin *agentPostedMessage
	for i := range slackMock.postedMessages {
		if slackMock.postedMessages[i].ChannelID == "C-MONITOR" && slackMock.postedMessages[i].ThreadTS == placeholderTS {
			origin = &slackMock.postedMessages[i]
		}
	}
	gt.Value(t, origin).NotNil().Required()
	gt.Bool(t, strings.Contains(origin.Text, "<@U-REACTOR>")).True()
	gt.Bool(t, strings.Contains(origin.Text, srcPermalink)).True()

	// The completion back-link was posted back in the source thread.
	foundBacklink := false
	for _, m := range slackMock.postedMessages {
		if m.ChannelID == srcChannel && strings.Contains(m.Text, "https://app.test/ws/support/cases/") {
			foundBacklink = true
		}
	}
	gt.Bool(t, foundBacklink).True()
}

// A hard failure during cross-channel creation replaces the lingering
// "Creating a case…" placeholder with an honest failure note and releases the
// claim so a retry can work. An empty script makes the create turn error out.
func TestReaction_CrossChannel_FallbackReplacesPlaceholder(t *testing.T) {
	slackUC, _, repo, slackMock := newReactionSetup(t, newScriptedClient([]string{}))
	ctx := context.Background()

	const srcChannel = "C-GENERAL"
	const srcTS = "1700000000.000100"
	const placeholderTS = "1234567890.123456"

	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-REACTOR", "U-AUTHOR", srcChannel, srcTS))).Required()
	async.Wait()

	// No case was created.
	c, err := repo.Case().GetBySlackThread(ctx, "support", "C-MONITOR", placeholderTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).Nil()

	// The placeholder root was replaced with a failure note (not left as "Creating…").
	var replaced *agentUpdatedMessage
	for i := range slackMock.updatedMessages {
		if slackMock.updatedMessages[i].ChannelID == "C-MONITOR" && slackMock.updatedMessages[i].Timestamp == placeholderTS {
			replaced = &slackMock.updatedMessages[i]
		}
	}
	gt.Value(t, replaced).NotNil().Required()
	gt.Bool(t, strings.Contains(replaced.Text, "⚠️")).True()

	// The claim was released so the source message can be reacted again.
	claimed, err := repo.ReactionClaim().Claim(ctx, "support", srcChannel, srcTS)
	gt.NoError(t, err).Required()
	gt.Bool(t, claimed).True()
}

// When the source permalink cannot be resolved, the origin reply degrades to a
// reporter-only line (MsgReactionCaseOriginNoLink) with no broken link markup.
func TestReaction_CrossChannel_OriginReplyDegradesWithoutPermalink(t *testing.T) {
	slackUC, _, _, slackMock := newReactionSetup(t, newScriptedClient(createDecisionScripts()))
	ctx := context.Background()

	// The permalink lookup yields nothing (e.g. the bot lost access to the source).
	slackMock.getPermalinkFn = func(_ context.Context, _ string, _ string) (string, error) {
		return "", nil
	}

	gt.NoError(t, slackUC.HandleSlackEvent(ctx, reactionEvent("incident", "U-REACTOR", "U-AUTHOR", "C-GENERAL", "1700000000.000100"))).Required()
	async.Wait()

	// The origin reply still names the reporter but carries no link markup.
	var origin *agentPostedMessage
	for i := range slackMock.postedMessages {
		if slackMock.postedMessages[i].ChannelID == "C-MONITOR" && slackMock.postedMessages[i].ThreadTS == "1234567890.123456" {
			origin = &slackMock.postedMessages[i]
		}
	}
	gt.Value(t, origin).NotNil().Required()
	gt.Bool(t, strings.Contains(origin.Text, "<@U-REACTOR>")).True()
	gt.Bool(t, strings.Contains(origin.Text, "|source message>")).False()
	gt.Bool(t, strings.Contains(origin.Text, "|元メッセージ>")).False()
}
