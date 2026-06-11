package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	slackevents "github.com/slack-go/slack/slackevents"
)

func TestSlackUseCases_HandleSlackEvent(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	t.Run("handles message event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:           "message",
					User:           "U123",
					Text:           "Hello, world!",
					TimeStamp:      "1234567890.123456",
					Channel:        "C123",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)

		if len(messages) > 0 {
			msg := messages[0]
			gt.Value(t, msg.UserID()).Equal("U123")
			gt.Value(t, msg.Text()).Equal("Hello, world!")
		}
	})

	t.Run("handles app_mention event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:           "app_mention",
					User:           "U456",
					Text:           "@bot help",
					TimeStamp:      "1234567890.654321",
					Channel:        "C456",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T456",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C456",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)
	})

	t.Run("ignores unsupported event types", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "some_other_event",
				Data: map[string]interface{}{
					"type": "some_other_event",
				},
			},
			TeamID: "T789",
		}

		// Should not error, just skip
		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})
}

// TestSlackUseCases_ThreadModeCreationInitiation verifies the thread-mode
// routing rule: case creation is initiated ONLY by a channel-root post.
// A mention or a reply inside a thread that has no case yet must be ignored —
// no create/resume turn runs (the LLM planner is never invoked, and no session
// is created for the thread).
func TestSlackUseCases_ThreadModeCreationInitiation(t *testing.T) {
	const channel = "C-MONITOR"

	// wire builds a SlackUseCases backed by a thread-mode workspace and a probe
	// LLM that records whether the agent planner was ever invoked.
	// createFromBotPosts toggles the workspace's [slack] accept_bot.
	wire := func(createFromBotPosts bool) (*usecase.SlackUseCases, *memory.Memory, *atomic.Bool) {
		repo := memory.New()
		reg := newThreadWorkspaceRegistry()
		if createFromBotPosts {
			if e, err := reg.Get("support"); err == nil {
				e.AcceptBot = true
			}
		}
		slackMock := &agentTestSlackService{}
		caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

		var llmInvoked atomic.Bool
		probe := &mockLLMClient{
			newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
				llmInvoked.Store(true)
				return &mockLLMSession{
					generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
						return nil, errors.New("planner must not run for ignored events")
					},
				}, nil
			},
		}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     reg,
			LLM:          probe,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
			CaseUC:       caseUC,
		})
		slackUC := usecase.NewSlackUseCases(repo, reg, agentUC, nil, slackMock)
		return slackUC, repo, &llmInvoked
	}

	mentionEvent := func(threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            "U-ASKER",
					Text:            "<@UBOT001> please help",
					TimeStamp:       "1700000009.000001",
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  "1700000009",
				},
			},
			TeamID: "T1",
		}
	}

	messageEvent := func(ts, threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:            "message",
					User:            "U-ASKER",
					Text:            "some text",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  ts,
				},
			},
			TeamID: "T1",
		}
	}

	t.Run("mention in a case-less thread is ignored", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		threadTS := "1700000000.000100" // a thread with no case bound
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent(threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()

		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("reply in a case-less thread is ignored", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		threadTS := "1700000000.000100"
		gt.NoError(t, uc.HandleSlackEvent(ctx, messageEvent("1700000005.000001", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("channel-root post initiates case creation", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked := wire(false)

		// A top-level post (no thread_ts): the message's own ts is the thread.
		rootTS := "1700000010.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, messageEvent(rootTS, ""))).Required()
		async.Wait()

		// The create turn was initiated: the planner was invoked. (It errors out
		// on Generate here, which the create flow handles gracefully — the point
		// is only that root posts reach creation while threaded events do not.)
		gt.Value(t, llmInvoked.Load()).Equal(true)
	})

	// botFormRootEvent is a channel-root post authored by an integration bot
	// (an intake-form app) rather than a human: no SubType / "bot_message", an
	// empty User, a BotID, and the human requester named in the body.
	botFormRootEvent := func(ts, text string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:           "message",
					SubType:        "bot_message",
					BotID:          "B-FORMBOT",
					Text:           text,
					TimeStamp:      ts,
					Channel:        channel,
					EventTimeStamp: ts,
				},
			},
			TeamID: "T1",
		}
	}

	t.Run("bot root post is ignored when accept_bot is off (default)", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		// Default off: a bot-authored channel-root post must NOT start a case, so
		// the channel is not flooded with a case per bot notification.
		rootTS := "1700000015.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, botFormRootEvent(rootTS, "RISK NAVIGATOR request\nReporter: <@U06KHSXQW4V|ahyan>"))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("bot-relayed form root post initiates case creation when opted in", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked := wire(true)

		// With the workspace opted in, an intake form posted by a bot at the
		// channel root initiates creation, attributing to the body's requester.
		rootTS := "1700000020.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, botFormRootEvent(rootTS, "RISK NAVIGATOR request\nReporter: <@U06KHSXQW4V|ahyan>"))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(true)
	})

	t.Run("opted-in bot root post with no body mention still initiates creation (empty reporter)", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked := wire(true)

		// A bot post with no human mention in the body still initiates creation:
		// a thread-mode case is allowed to have no reporter, so the reporter
		// simply stays empty and the create turn runs.
		rootTS := "1700000030.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, botFormRootEvent(rootTS, "automated heartbeat, no requester"))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(true)
	})
}

func TestSlackUseCases_HandleSlackMessage(t *testing.T) {
	t.Run("stores message successfully", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"1234567890.123456",
			"C123",
			"",
			"T123",
			"U123",
			"testuser",
			"Test message",
			"1234567890.123456",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1).Required()

		gt.Value(t, messages[0].ID()).Equal("1234567890.123456")
	})

	t.Run("returns error for nil message", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		err := uc.Slack.HandleSlackMessage(ctx, nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("saves to case sub-collection when channel is mapped", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a case with SlackChannelID
		created, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Test Case",
			SlackChannelID: "C-MAPPED",
		})
		gt.NoError(t, err).Required()

		// Set up registry with the workspace
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		uc := usecase.New(repo, registry)

		msg := slack.NewMessageFromData(
			"mapped-msg-001",
			"C-MAPPED",
			"",
			"T123",
			"U123",
			"alice",
			"Hello from mapped channel",
			"ev1",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-MAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)

		// Verify message was also saved to case sub-collection
		caseMsgs, _, err := repo.CaseMessage().List(ctx, "ws-test", created.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, caseMsgs).Length(1)
		gt.Value(t, caseMsgs[0].Text()).Equal("Hello from mapped channel")
	})

	t.Run("thread mode: saves reply to the thread's case sub-collection", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// A thread-mode case bound to (monitored channel, thread ts).
		threadTS := "1700000000.000100"
		created, err := repo.Case().Create(ctx, "support", &model.Case{
			ReporterID:     "U-REPORTER",
			Title:          "Thread case",
			SlackChannelID: "C-MONITOR",
			SlackThreadTS:  threadTS,
			BoardStatus:    "TRIAGE",
		})
		gt.NoError(t, err).Required()

		set, err := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
			{ID: "TRIAGE", Name: "Triage"},
			{ID: "DONE", Name: "Done"},
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace:             model.Workspace{ID: "support", Name: "Support"},
			CaseMode:              model.CaseModeThread,
			SlackMonitorChannelID: "C-MONITOR",
			CaseStatusSet:         set,
		})
		uc := usecase.New(repo, registry)

		// A reply in the case thread (thread_ts points at the case's thread).
		reply := slack.NewMessageFromData(
			"1700000005.000001",
			"C-MONITOR",
			threadTS,
			"T1",
			"U-ASKER",
			"bob",
			"Any update on this?",
			"1700000005.000001",
			time.Now(),
			nil,
		)
		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, reply)).Required()

		caseMsgs, _, err := repo.CaseMessage().List(ctx, "support", created.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, caseMsgs).Length(1).Required()
		gt.Value(t, caseMsgs[0].Text()).Equal("Any update on this?")
	})

	t.Run("saves to action sub-collection when message is in action thread", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		caseRec, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Test Case",
			SlackChannelID: "C-ACTION",
		})
		gt.NoError(t, err).Required()

		// Create an action with a Slack message TS — the thread anchor.
		actionRec, err := repo.Action().Create(ctx, "ws-test", &model.Action{
			CaseID:         caseRec.ID,
			Title:          "Test Action",
			Status:         types.ActionStatusTodo,
			SlackMessageTS: "1700000000.000001",
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})
		uc := usecase.New(repo, registry)

		// A reply in the action thread.
		reply := slack.NewMessageFromData(
			"reply-msg-001",
			"C-ACTION",
			"1700000000.000001",
			"T123", "U123", "alice", "Working on it",
			"ev1",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, reply)).Required()

		// Should be persisted under the action.
		actionMsgs, _, err := repo.ActionMessage().List(ctx, "ws-test", actionRec.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, actionMsgs).Length(1)
		gt.Value(t, actionMsgs[0].Text()).Equal("Working on it")
		gt.Value(t, actionMsgs[0].ThreadTS()).Equal("1700000000.000001")

		// Also still saved at the case level (case channel collection).
		caseMsgs, _, err := repo.CaseMessage().List(ctx, "ws-test", caseRec.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, caseMsgs).Length(1)
	})

	t.Run("non-thread message in case channel is NOT saved to action sub-collection", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		caseRec, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Test Case",
			SlackChannelID: "C-ACTION-2",
		})
		gt.NoError(t, err).Required()
		actionRec, err := repo.Action().Create(ctx, "ws-test", &model.Action{
			CaseID:         caseRec.ID,
			Title:          "Test Action",
			Status:         types.ActionStatusTodo,
			SlackMessageTS: "1700000000.000002",
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})
		uc := usecase.New(repo, registry)

		// Top-level message (no ThreadTS) in the case channel.
		topLevel := slack.NewMessageFromData(
			"top-msg-001",
			"C-ACTION-2",
			"",
			"T123", "U123", "alice", "Top-level",
			"ev1",
			time.Now(),
			nil,
		)
		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, topLevel)).Required()

		actionMsgs, _, err := repo.ActionMessage().List(ctx, "ws-test", actionRec.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, actionMsgs).Length(0)
	})

	t.Run("does not save to case sub-collection when channel is not mapped", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"unmapped-msg-001",
			"C-UNMAPPED",
			"",
			"T123",
			"U123",
			"bob",
			"Hello from unmapped channel",
			"ev1",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-UNMAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)
	})
}

func TestSlackUseCases_HandleMembershipEvent(t *testing.T) {
	t.Run("member_joined_channel syncs channel members to case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-%d", time.Now().UnixNano())

		// Create a case with a Slack channel
		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Membership Test Case",
			SlackChannelID: "C-MEMBERSHIP-JOIN",
		})
		gt.NoError(t, err).Required()

		// Seed SlackUser cache so filterHumanUsers can work
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U100", Name: "alice"},
			{ID: "U200", Name: "bob"},
		})).Required()

		// Set up registry
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		// Mock slack service that returns channel members
		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				if channelID == "C-MEMBERSHIP-JOIN" {
					return []string{"U100", "U200", "UBOT999"}, nil // UBOT999 is not in SlackUser cache
				}
				return nil, nil
			},
		}

		uc := usecase.New(repo, registry,
			usecase.WithSlackService(slackSvc),
			usecase.WithLLMClient(stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))),
			usecase.WithEmbedClient(&mockLLMClient{}),
			usecase.WithHistoryRepository(agentarchive.NewMemoryHistoryRepository()),
			usecase.WithTraceRepository(agentarchive.NewMemoryTraceRepository()),
		)

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-MEMBERSHIP-JOIN",
					User:    "U200",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify case was updated with filtered human user IDs
		updated, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.ChannelUserIDs).Length(2) // U100, U200 (UBOT999 filtered out)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U100")).Equal(true)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U200")).Equal(true)
	})

	t.Run("member_left_channel syncs channel members to case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-left-%d", time.Now().UnixNano())

		// Create a case with a Slack channel and initial members
		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Membership Leave Test",
			SlackChannelID: "C-MEMBERSHIP-LEFT",
			ChannelUserIDs: []string{"U100", "U200"},
		})
		gt.NoError(t, err).Required()

		// Seed SlackUser cache
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U100", Name: "alice"},
		})).Required()

		// After U200 leaves, only U100 remains in the channel
		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				return []string{"U100"}, nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		uc := usecase.New(repo, registry,
			usecase.WithSlackService(slackSvc),
			usecase.WithLLMClient(stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))),
			usecase.WithEmbedClient(&mockLLMClient{}),
			usecase.WithHistoryRepository(agentarchive.NewMemoryHistoryRepository()),
			usecase.WithTraceRepository(agentarchive.NewMemoryTraceRepository()),
		)

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_left_channel",
				Data: &slackevents.MemberLeftChannelEvent{
					Channel: "C-MEMBERSHIP-LEFT",
					User:    "U200",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify case now only has U100
		updated, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.ChannelUserIDs).Length(1)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U100")).Equal(true)
	})

	t.Run("no-op when channel has no associated case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-noop-%d", time.Now().UnixNano())

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				t.Error("GetConversationMembers should not be called for unrelated channel")
				return nil, nil
			},
		}

		uc := usecase.New(repo, registry,
			usecase.WithSlackService(slackSvc),
			usecase.WithLLMClient(stubPlannerLLM(stubMaterializePlannerJSON("ws-1"))),
			usecase.WithEmbedClient(&mockLLMClient{}),
			usecase.WithHistoryRepository(agentarchive.NewMemoryHistoryRepository()),
			usecase.WithTraceRepository(agentarchive.NewMemoryTraceRepository()),
		)

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-UNRELATED",
					User:    "U999",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})

	t.Run("no-op when slack service is nil", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-nil-slack", Name: "Test"},
		})

		// No WithSlackService option => slackService is nil
		uc := usecase.New(repo, registry)

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-ANY",
					User:    "U100",
				},
			},
			TeamID: "T123",
		}

		// Should not error even without slack service
		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})
}

func TestSlackUseCases_CleanupOldMessages(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	newTime := now.Add(-1 * time.Hour)

	// Create old and new messages
	oldMsg := slack.NewMessageFromData(
		"old.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"Old message",
		"old.123456",
		oldTime,
		nil,
	)

	newMsg := slack.NewMessageFromData(
		"new.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"New message",
		"new.123456",
		newTime,
		nil,
	)

	gt.NoError(t, repo.Slack().PutMessage(ctx, oldMsg)).Required()
	gt.NoError(t, repo.Slack().PutMessage(ctx, newMsg)).Required()

	// Cleanup messages older than 24 hours
	cutoffTime := now.Add(-24 * time.Hour)
	gt.NoError(t, uc.Slack.CleanupOldMessages(ctx, cutoffTime)).Required()

	// Verify only new message remains
	messages, _, err := repo.Slack().ListMessages(
		ctx,
		"C123",
		time.Time{},
		now.Add(1*time.Hour),
		10,
		"",
	)
	gt.NoError(t, err).Required()

	gt.Array(t, messages).Length(1).Required()

	gt.Value(t, messages[0].ID()).Equal("new.123456")
}
