package usecase_test

import (
	"context"
	"errors"
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

// threadWorkspaceAgentRegistry returns a thread-mode workspace registry whose
// monitored channel is "C-CASES". A channel-root mention there runs the
// workspace agent (trigger = "mention"), and the thread it opens must never
// become a Case.
func threadWorkspaceAgentRegistry() *model.WorkspaceRegistry {
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "ws-1", Name: "Workspace"},
		CaseMode:              model.CaseModeThread,
		CaseTrigger:           model.CaseTriggerMention,
		SlackMonitorChannelID: "C-CASES",
	})
	return reg
}

func TestAgentUseCase_HandleWorkspaceAgentMention(t *testing.T) {
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

		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

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

		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

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
		gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)
	})

	// In thread mode the same handler runs on a channel-root mention in the
	// monitored channel. The Session it persists must carry
	// SessionKindWorkspaceAgent — that marker is what stops the Slack dispatcher
	// from treating a follow-up mention in this thread as a case-creation
	// trigger.
	t.Run("thread mode: root mention tags the session as workspace-agent owned", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := threadWorkspaceAgentRegistry()
		slackMock := &agentTestSlackService{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          wsMentionScript("Nothing is on fire."),
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		const mentionTS = "1700300040.000001"
		msg := slackmodel.NewMessageFromData(
			mentionTS, "C-CASES", "", "T1", "U-ASKER", "alice",
			"@bot anything on fire?", mentionTS, time.Now(), nil,
		)

		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

		gt.Array(t, slackMock.postedMessages).Length(2).Required()
		gt.Value(t, slackMock.postedMessages[1].ChannelID).Equal("C-CASES")
		gt.Value(t, slackMock.postedMessages[1].ThreadTS).Equal(mentionTS)
		gt.Value(t, slackMock.postedMessages[1].Text).Equal("Nothing is on fire.")

		ssn, err := repo.Session().GetByThread(ctx, "C-CASES", mentionTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)
		gt.Value(t, ssn.CaseID).Equal(int64(0))

		// The mention must not have produced a Case: that is the whole point of
		// routing a root mention here instead of to the creation agent.
		c, err := repo.Case().GetBySlackThread(ctx, "ws-1", "C-CASES", mentionTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	// A thread's owner is decided by whoever claimed it first. When the case flow
	// got there first, the workspace agent must stand down rather than run on
	// that Session — otherwise the two conversations share one history, and a
	// concurrent claim (the dispatcher's lookup and this handler are separate
	// reads) would decide which agent answers.
	t.Run("case-owned thread: workspace agent stands down without touching the session", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := threadWorkspaceAgentRegistry()
		slackMock := &agentTestSlackService{}

		llm := &mockLLMClient{
			newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
				t.Error("workspace agent must not run on a case-owned thread")
				return nil, errors.New("planner must not run")
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

		const threadTS = "1700300050.000001"
		gt.NoError(t, repo.Session().Put(ctx, &model.Session{
			ID:          "pre-existing",
			ChannelID:   "C-CASES",
			ThreadTS:    threadTS,
			WorkspaceID: "ws-1",
			Kind:        model.SessionKindCase,
		})).Required()

		msg := slackmodel.NewMessageFromData(
			"1700300051.000001", "C-CASES", threadTS, "T1", "U-ASKER", "alice",
			"@bot still there?", "1700300051.000001", time.Now(), nil,
		)
		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

		gt.Array(t, slackMock.postedMessages).Length(0)

		ssn, err := repo.Session().GetByThread(ctx, "C-CASES", threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.ID).Equal("pre-existing")
		gt.Value(t, ssn.Kind).Equal(model.SessionKindCase)
	})

	// The claim has to land before the agent does any work, so a mention arriving
	// in the same thread a moment later already sees an owned thread. Asserting
	// it only after RunTurn would pass even if the Session were written last.
	t.Run("thread is claimed before the agent posts anything", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		registry := threadWorkspaceAgentRegistry()

		const mentionTS = "1700300060.000001"
		var kindAtFirstPost model.SessionKind
		var sessionExistedAtFirstPost bool
		firstPostSeen := false
		slackMock := &agentTestSlackService{}
		// The first thread post is the progress trace, which the handler emits
		// after the claim. Sample the stored Session exactly then.
		slackMock.postThreadReplyFn = func(_ context.Context, channelID, threadTS, _ string) (string, error) {
			if !firstPostSeen {
				firstPostSeen = true
				if ssn, err := repo.Session().GetByThread(ctx, channelID, threadTS); err == nil && ssn != nil {
					sessionExistedAtFirstPost = true
					kindAtFirstPost = ssn.Kind
				}
			}
			return "1700300060.trace01", nil
		}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          wsMentionScript("Claimed already."),
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		entry, err := registry.Get("ws-1")
		gt.NoError(t, err).Required()

		msg := slackmodel.NewMessageFromData(
			mentionTS, "C-CASES", "", "T1", "U-ASKER", "alice",
			"@bot anything on fire?", mentionTS, time.Now(), nil,
		)
		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

		gt.Bool(t, sessionExistedAtFirstPost).True().Required()
		gt.Value(t, kindAtFirstPost).Equal(model.SessionKindWorkspaceAgent)
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

		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()

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
		// HandleWorkspaceAgentMention must take its very first guard clause
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

		gt.NoError(t, agentUC.HandleWorkspaceAgentMention(ctx, msg, entry)).Required()
		gt.Array(t, slackMock.postedMessages).Length(0)
	})
}
