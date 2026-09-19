package usecase_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// An answer from a user the workspace's policy denies is not processed: the
// form is left as it is, no turn resumes, and the user alone is told why.
func TestHandleThreadCaseQuestionSubmit_WorkspaceAccessDenied(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")
	llm := newScriptedClient(nil)
	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:            repo,
		Registry:        reg,
		LLM:             llm,
		HistoryRepo:     agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:       agentarchive.NewMemoryTraceRepository(),
		SlackService:    slackMock,
		CaseUC:          caseUC,
		WorkspaceAccess: denyingAccess(t, reg, "support"),
	})
	startAgentRuntime(t, agentRuntimeDeps{UC: agentUC, Repo: repo, Registry: reg, LLM: llm})

	const channel = "C-MONITOR"
	const rootTS = "1700000000.000800"
	cb := &goslack.InteractionCallback{
		Type:    goslack.InteractionTypeBlockActions,
		User:    goslack.User{ID: "U-DENIED"},
		Channel: goslack.Channel{GroupConversation: goslack.GroupConversation{Conversation: goslack.Conversation{ID: channel}}},
		Message: goslack.Message{Msg: goslack.Msg{Timestamp: "1700000000.000900", ThreadTimestamp: rootTS}},
		ActionCallback: goslack.ActionCallbacks{
			BlockActions: []*goslack.BlockAction{{ActionID: usecase.ActionIDThreadCreateQuestionSubmit, Value: channel + ":" + rootTS}},
		},
	}
	gt.NoError(t, agentUC.HandleThreadCaseQuestionSubmit(ctx, cb, cb.ActionCallback.BlockActions[0])).Required()
	async.Wait()

	gt.Array(t, slackMock.updates()).Length(0)
	eph := slackMock.ephemerals()
	gt.Array(t, eph).Length(1).Required()
	gt.Value(t, eph[0].ChannelID).Equal(channel)
	gt.Value(t, eph[0].UserID).Equal("U-DENIED")
}

// TestBuildThreadCreateQuestionBlocks_Fallback locks the notification
// fallback of the thread-mode question form to the i18n layer: English is
// the historical hardcoded string, and a Japanese-locale context must yield
// the Japanese translation (compared against the same i18n source the
// production code reads, not hardcoded here).
func TestBuildThreadCreateQuestionBlocks_Fallback(t *testing.T) {
	items := []model.PendingQuestionItem{
		{ID: "severity", Text: "How severe is it?", Type: "select", Options: []string{"high", "low"}},
	}

	t.Run("default context yields English fallback", func(t *testing.T) {
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(context.Background(), "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal("We need a bit more info to create this case.")
	})

	t.Run("Japanese context yields localized fallback", func(t *testing.T) {
		jaCtx := i18n.ContextWithLang(context.Background(), i18n.LangJA)
		blocks, fallback := usecase.BuildThreadCreateQuestionBlocksForTest(jaCtx, "need severity", items, "C-CASE:1700000000.000100", "U123")
		gt.Number(t, len(blocks)).GreaterOrEqual(1)
		gt.Value(t, fallback).Equal(i18n.T(jaCtx, i18n.MsgThreadCaseQuestionFallback))
		gt.Value(t, fallback).NotEqual("We need a bit more info to create this case.")
	})
}

func TestCaseThreadValueCodec(t *testing.T) {
	t.Run("round-trips channel and thread ts", func(t *testing.T) {
		v := usecase.EncodeCaseThreadValueForTest("C-MONITOR", "1700000000.000100")
		ch, ts, ok := usecase.ParseCaseThreadValueForTest(v)
		gt.Bool(t, ok).True()
		gt.String(t, ch).Equal("C-MONITOR")
		gt.String(t, ts).Equal("1700000000.000100")
	})

	t.Run("a bare thread ts (no channel) is not parseable", func(t *testing.T) {
		// No colon → the submit handler rejects it as malformed rather than
		// splitting on the ts's dot.
		_, _, ok := usecase.ParseCaseThreadValueForTest("1700000000.000100")
		gt.Bool(t, ok).False()
	})

	t.Run("empty and separator-only values are rejected", func(t *testing.T) {
		for _, v := range []string{"", ":", "C-ONLY:", ":1700000000.0001"} {
			_, _, ok := usecase.ParseCaseThreadValueForTest(v)
			gt.Bool(t, ok).False()
		}
	})
}
