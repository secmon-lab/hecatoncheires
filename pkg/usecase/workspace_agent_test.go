package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
)

// workspaceChannelRegistry returns a channel-mode workspace registry whose
// workspace channel is "C-WORKSPACE" (the shared cross-case channel wsagent
// listens on), mirroring newThreadWorkspaceRegistry's role for thread mode.
func workspaceChannelRegistry() *model.WorkspaceRegistry {
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace:               model.Workspace{ID: "ws-1", Name: "Workspace"},
		SlackWorkspaceChannelID: "C-WORKSPACE",
	})
	return reg
}

// wsMentionScript is the two-call scripted planner response for the
// AllowDirect fast path wsagent always enables: round 1 answers "direct"
// (no investigation), and the direct ReAct loop's own Generate call returns
// the plain-text reply. See pkg/usecase/agent/planexec/direct.go /
// planexec_test.go's direct-mode integration test for the same two-call shape.
func wsMentionScript(reply string) gollem.LLMClient {
	return newScriptedClient([]string{
		`{"message":"answering directly","direct":{}}`,
		reply,
	})
}

func TestAgentUseCase_HandleWorkspaceChannelMention(t *testing.T) {
	t.Run("happy path: direct reply posted to the mention's own thread", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := workspaceChannelRegistry()
		slackMock := &agentTestSlackService{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          wsMentionScript("Here is the direct reply."),
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		// Top-level mention (no ThreadTS): the mention anchors its own thread.
		const mentionTS = "1700300000.000001"
		msg := slackmodel.NewMessageFromData(
			mentionTS,
			"C-WORKSPACE",
			"",
			"T1",
			"U-ASKER",
			"alice",
			"@bot what's open right now?",
			mentionTS,
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleWorkspaceChannelMention(ctx, msg, entry)).Required()

		// Two Slack posts: the trace banner (planner round announced via
		// Sink.PlanProposed → Handler.TraceAppend) and the final reply.
		gt.Array(t, slackMock.postedMessages).Length(2).Required()
		gt.Value(t, slackMock.postedMessages[0].ChannelID).Equal("C-WORKSPACE")
		gt.Value(t, slackMock.postedMessages[0].ThreadTS).Equal(mentionTS)
		gt.String(t, slackMock.postedMessages[0].Text).Contains("answering directly")

		gt.Value(t, slackMock.postedMessages[1].ChannelID).Equal("C-WORKSPACE")
		gt.Value(t, slackMock.postedMessages[1].ThreadTS).Equal(mentionTS)
		gt.Value(t, slackMock.postedMessages[1].Text).Equal("Here is the direct reply.")
	})

	t.Run("threaded mention replies in the same thread", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := workspaceChannelRegistry()
		slackMock := &agentTestSlackService{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          wsMentionScript("Reply in thread."),
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		const threadTS = "1700300010.000001"
		msg := slackmodel.NewMessageFromData(
			"1700300011.000001",
			"C-WORKSPACE",
			threadTS,
			"T1",
			"U-ASKER",
			"alice",
			"@bot follow up question",
			"1700300011.000001",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleWorkspaceChannelMention(ctx, msg, entry)).Required()

		gt.Array(t, slackMock.postedMessages).Length(2).Required()
		gt.Value(t, slackMock.postedMessages[0].ThreadTS).Equal(threadTS)
		gt.Value(t, slackMock.postedMessages[1].ThreadTS).Equal(threadTS)
		gt.Value(t, slackMock.postedMessages[1].Text).Equal("Reply in thread.")

		// The case-less session is bound to the thread, not to any case.
		ssn, err := repo.Session().GetByThread(ctx, "C-WORKSPACE", threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.CaseID).Equal(int64(0))
		gt.Value(t, ssn.WorkspaceID).Equal("ws-1")
	})

	t.Run("bot's own mention is skipped: no LLM call, no Slack post", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := workspaceChannelRegistry()
		slackMock := &agentTestSlackService{}

		llm := &mockLLMClient{
			newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
				t.Fatal("planner must not run for the bot's own mention")
				return nil, nil
			},
		}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llm,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		// agentTestSlackService.GetBotUserID defaults to "UBOT001"; author the
		// mention as that same user to exercise the self-mention guard.
		msg := slackmodel.NewMessageFromData(
			"1700300020.000001",
			"C-WORKSPACE",
			"",
			"T1",
			"UBOT001",
			"bot",
			"@bot self mention",
			"1700300020.000001",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleWorkspaceChannelMention(ctx, msg, entry)).Required()

		gt.Array(t, slackMock.postedMessages).Length(0)

		// No session should have been created for a self-mention that never
		// reached the planner.
		ssn, err := repo.Session().GetByThread(ctx, "C-WORKSPACE", "1700300020.000001")
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("workspace agent not configured (LLM nil): no-op, no Slack post", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := workspaceChannelRegistry()
		slackMock := &agentTestSlackService{}

		// LLM: nil means NewAgentUseCase never builds a workspaceAgent (see
		// NewAgentUseCase's `if deps.LLM != nil` gate), so
		// HandleWorkspaceChannelMention must take its very first guard clause
		// and return before touching Slack at all (not even GetBotUserID).
		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		msg := slackmodel.NewMessageFromData(
			"1700300030.000001",
			"C-WORKSPACE",
			"",
			"T1",
			"U-ASKER",
			"alice",
			"@bot are you there?",
			"1700300030.000001",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleWorkspaceChannelMention(ctx, msg, entry)).Required()
		gt.Array(t, slackMock.postedMessages).Length(0)
	})
}
