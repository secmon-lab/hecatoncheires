package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
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

// TestSlackUseCases_ThreadModeCreationInitiation verifies the instant-trigger
// thread-mode routing rules: a channel-root post initiates creation, and — the
// recovery path — an @mention inside a thread that has no case yet ALSO
// initiates creation (both are gated so a bot-authored trigger needs
// accept_bot). A plain reply inside a case-less thread carries no creation
// semantics and is ignored (no create turn runs, the planner is never invoked,
// and no session is created).
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

	mentionEvent := func(user, botID, ts, threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            user,
					BotID:           botID,
					Text:            "<@UBOT001> please make this a case",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  ts,
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

	t.Run("mention in a case-less thread initiates creation (recovery path)", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		threadTS := "1700000000.000100" // a thread with no case bound
		// A human @mention inside a case-less thread starts a create turn even in
		// instant mode: the planner runs and a session is created for the thread
		// root. No case is committed here because the scripted planner errors on
		// Generate, which the create flow handles gracefully.
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", "1700000009.000001", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(true)

		// The session is bound to the thread root (threadTS), not the mention's ts.
		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil()

		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	t.Run("bot-authored mention in a case-less thread is ignored when accept_bot is off", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		threadTS := "1700000000.000200"
		// A bot-authored @mention inside a case-less thread must NOT start a case
		// when accept_bot is off — same gate as a bot-authored channel-root post.
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("", "B-FORMBOT", "1700000011.000001", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("bot-authored mention in a case-less thread initiates creation when accept_bot is on", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(true)

		threadTS := "1700000000.000300"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("", "B-FORMBOT", "1700000012.000001", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(true)

		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil()
	})

	t.Run("bot's own mention in a case-less thread is ignored even with accept_bot on", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(true)

		threadTS := "1700000000.000400"
		// A mention authored by our own bot user must never self-trigger a case.
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("UBOT001", "B-SELF", "1700000013.000001", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

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

	// Instant mode is deliberately unchanged by the mention-mode root-mention
	// rework: the accompanying `message` event is what creates the Case, so
	// acting on the app_mention here too would double-handle the same post.
	t.Run("channel-root mention is ignored in instant mode", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked := wire(false)

		rootTS := "1700000050.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", rootTS, ""))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
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

	t.Run("human file_share root post initiates case creation", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked := wire(false)

		// A human filing an intake request by uploading a screenshot/document
		// posts a "file_share" subtype message at the channel root; it must start
		// a case (the flag gates bot posts only, not human file shares).
		ev := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:           "message",
					SubType:        "file_share",
					User:           "U-ASKER",
					Text:           "please review this",
					TimeStamp:      "1700000040.000001",
					Channel:        channel,
					EventTimeStamp: "1700000040.000001",
				},
			},
			TeamID: "T1",
		}
		gt.NoError(t, uc.HandleSlackEvent(ctx, ev)).Required()
		async.Wait()

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

// wsAgentPromptMarker appears only in the workspace agent's system prompt
// (pkg/usecase/agent/wsagent/prompts/system.md), never in the thread-mode case
// planner's. Recording the system prompt is therefore the precise way to tell
// which agent a dispatch decision actually reached — both paths invoke the LLM,
// so "the planner ran" alone cannot distinguish them.
const wsAgentPromptMarker = "workspace-level assistant"

// promptRecorder captures every system prompt handed to the probe LLM.
type promptRecorder struct {
	mu      sync.Mutex
	prompts []string
}

func (p *promptRecorder) record(opts ...gollem.SessionOption) {
	cfg := gollem.NewSessionConfig(opts...)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prompts = append(p.prompts, cfg.SystemPrompt())
}

// sawWorkspaceAgent reports whether any recorded system prompt belongs to the
// workspace agent.
func (p *promptRecorder) sawWorkspaceAgent() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.prompts {
		if strings.Contains(s, wsAgentPromptMarker) {
			return true
		}
	}
	return false
}

// allWorkspaceAgent reports whether EVERY recorded system prompt belongs to the
// workspace agent — i.e. no turn slipped into the case planner. It is false when
// nothing was recorded at all.
// workspaceAgentPromptCount is how many prompts carried the workspace agent's
// persona. A plan-execute run also creates sessions for the sub-agents it spawns,
// whose prompts are the per-task instructions rather than the persona — so
// "every prompt is the workspace agent" is no longer the right shape. Counting
// the persona prompts is: one per turn that reached the workspace agent.
func (p *promptRecorder) workspaceAgentPromptCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, s := range p.prompts {
		if strings.Contains(s, wsAgentPromptMarker) {
			n++
		}
	}
	return n
}

// TestSlackUseCases_ThreadModeMentionTrigger exercises the mention-trigger sub-mode
// ([slack] trigger = "mention"): a channel-root @mention runs the workspace
// agent, an @mention in a case-less thread starts a Case, a plain post never
// starts one, and bot-authored mentions are gated by accept_bot.
func TestSlackUseCases_ThreadModeMentionTrigger(t *testing.T) {
	const channel = "C-MONITOR"

	// wire builds a mention-trigger thread-mode workspace and a probe LLM that
	// records whether a planner was invoked and which system prompt it got.
	// acceptBot toggles the workspace's [slack] accept_bot (which gates
	// bot-authored mentions).
	wire := func(acceptBot bool) (*usecase.SlackUseCases, *memory.Memory, *atomic.Bool, *promptRecorder) {
		repo := memory.New()
		reg := newThreadWorkspaceRegistry()
		if e, err := reg.Get("support"); err == nil {
			e.CaseTrigger = model.CaseTriggerMention
			e.AcceptBot = acceptBot
		}
		slackMock := &agentTestSlackService{}
		caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

		var llmInvoked atomic.Bool
		prompts := &promptRecorder{}
		probe := &mockLLMClient{
			newSessionFn: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
				llmInvoked.Store(true)
				prompts.record(opts...)
				return &mockLLMSession{
					generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
						return nil, errors.New("planner generate not scripted in dispatch test")
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
		// The agents run on the durable runtime, so the worker is what makes the
		// probe fire at all. Subtests that assert NO agent ran are unaffected:
		// nothing is spawned, so the worker has nothing to claim.
		startAgentRuntime(t, agentRuntimeDeps{
			UC: agentUC, Repo: repo, Registry: reg, LLM: probe,
		})
		return usecase.NewSlackUseCases(repo, reg, agentUC, nil, slackMock), repo, &llmInvoked, prompts
	}

	// waitForLLM blocks until the probe has seen a session created, which is the
	// signal that the worker picked the run up. The dispatch decision under test
	// happens synchronously; the agent it dispatched to does not.
	waitForLLM := func(t *testing.T, invoked *atomic.Bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !invoked.Load() {
			if time.Now().After(deadline) {
				gt.Bool(t, invoked.Load()).True().Required()
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	mentionEvent := func(user, botID, ts, threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            user,
					BotID:           botID,
					Text:            "<@UBOT001> please make this a case",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  ts,
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
					Text:            "just a plain post, no mention",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  ts,
				},
			},
			TeamID: "T1",
		}
	}

	// A channel-root mention no longer starts a Case: it opens a workspace-agent
	// conversation in its own thread. Cases in mention mode come from an
	// in-thread mention instead.
	t.Run("channel-root mention runs the workspace agent instead of creating a case", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, prompts := wire(false)

		rootTS := "1700000010.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", rootTS, ""))).Required()
		async.Wait()
		waitForLLM(t, llmInvoked)

		gt.Bool(t, prompts.sawWorkspaceAgent()).True()

		// The session anchoring the mention's thread is tagged as workspace-agent
		// owned, which is what keeps follow-up mentions off the creation path.
		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)

		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	t.Run("mention in a case-less thread initiates creation", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, prompts := wire(false)

		threadTS := "1700000000.000100"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", "1700000020.000009", threadTS))).Required()
		async.Wait()

		waitForLLM(t, llmInvoked)
		gt.Bool(t, prompts.sawWorkspaceAgent()).False()

		// The case is bound to the thread root, not the mention's own ts.
		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.Kind).Equal(model.SessionKindCase)
	})

	// The follow-up half of the root-mention change: inside the thread the
	// workspace agent opened, a mention continues that conversation and must
	// never start a Case.
	t.Run("mention inside a workspace-agent thread does not initiate creation", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, prompts := wire(false)

		threadTS := "1700000080.000001"
		gt.NoError(t, repo.Session().Put(ctx, &model.Session{
			ID:          "ws-agent-session",
			ChannelID:   channel,
			ThreadTS:    threadTS,
			WorkspaceID: "support",
			Kind:        model.SessionKindWorkspaceAgent,
		})).Required()

		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", "1700000080.000002", threadTS))).Required()
		async.Wait()
		waitForLLM(t, llmInvoked)

		gt.Bool(t, prompts.sawWorkspaceAgent()).True()

		ssn, err := repo.Session().GetByThread(ctx, channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.ID).Equal("ws-agent-session")
		gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)

		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	// A Session written before Kind existed decodes with the zero value, which
	// must keep meaning "case thread" so existing in-flight creations still work.
	t.Run("mention in a thread with a legacy case-kind session still initiates creation", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, prompts := wire(false)

		threadTS := "1700000090.000001"
		gt.NoError(t, repo.Session().Put(ctx, &model.Session{
			ID:          "legacy-session",
			ChannelID:   channel,
			ThreadTS:    threadTS,
			WorkspaceID: "support",
		})).Required()

		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("U-ASKER", "", "1700000090.000002", threadTS))).Required()
		async.Wait()

		waitForLLM(t, llmInvoked)
		gt.Bool(t, prompts.sawWorkspaceAgent()).False()
	})

	t.Run("plain channel-root post does not initiate creation", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, _ := wire(false)

		rootTS := "1700000030.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, messageEvent(rootTS, ""))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)
		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	t.Run("plain thread reply does not initiate creation", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked, _ := wire(false)

		gt.NoError(t, uc.HandleSlackEvent(ctx, messageEvent("1700000040.000002", "1700000040.000001"))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)
	})

	t.Run("bot-authored root mention is ignored when accept_bot is off", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, _ := wire(false)

		rootTS := "1700000050.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("", "B-FORMBOT", rootTS, ""))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)
		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).Nil()
	})

	// An app has no Slack user identity, so it cannot be the workspace agent's
	// access actor — running the agent unattributed would bypass private-case
	// scoping entirely. accept_bot keeps its documented meaning instead: the
	// bot-authored root mention files a Case.
	t.Run("bot-authored root mention initiates creation when accept_bot is on", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, prompts := wire(true)

		rootTS := "1700000060.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("", "B-FORMBOT", rootTS, ""))).Required()
		async.Wait()

		waitForLLM(t, llmInvoked)
		gt.Bool(t, prompts.sawWorkspaceAgent()).False()

		ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.Kind).Equal(model.SessionKindCase)
	})

	// Inside a thread the workspace agent owns, a bot-authored mention is
	// dropped outright: it cannot run the agent (no user identity) and turning
	// the thread into a Case would contradict what the Session records.
	t.Run("bot-authored mention inside a workspace-agent thread is dropped", func(t *testing.T) {
		ctx := context.Background()
		uc, repo, llmInvoked, _ := wire(true)

		threadTS := "1700000110.000001"
		gt.NoError(t, repo.Session().Put(ctx, &model.Session{
			ID:          "ws-agent-session-bot",
			ChannelID:   channel,
			ThreadTS:    threadTS,
			WorkspaceID: "support",
			Kind:        model.SessionKindWorkspaceAgent,
		})).Required()

		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("", "B-FORMBOT", "1700000110.000002", threadTS))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)

		c, err := repo.Case().GetBySlackThread(ctx, "support", channel, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, c).Nil()
	})

	t.Run("bot's own mention is ignored even with accept_bot on", func(t *testing.T) {
		ctx := context.Background()
		uc, _, llmInvoked, _ := wire(true)

		// A mention authored by our own bot user must never self-trigger.
		rootTS := "1700000070.000001"
		gt.NoError(t, uc.HandleSlackEvent(ctx, mentionEvent("UBOT001", "B-SELF", rootTS, ""))).Required()
		async.Wait()

		gt.Value(t, llmInvoked.Load()).Equal(false)
	})
}

// TestLifecycle_ThreadModeWorkspaceAgentThread drives the whole root-mention
// flow through the public entry point, with no hand-seeded state: the root
// mention opens the thread, a follow-up mention inside it continues the same
// conversation, and no Case is ever created.
//
// Pre-seeding the Session with Put (as the per-branch dispatch tests do) cannot
// catch the ordering bug this guards: the Session that marks the thread as
// workspace-agent owned has to be written by the root turn itself, early enough
// that the follow-up's lookup sees it.
func TestLifecycle_ThreadModeWorkspaceAgentThread(t *testing.T) {
	ctx := context.Background()
	const channel = "C-MONITOR"

	repo := memory.New()
	reg := newThreadWorkspaceRegistry()
	e, err := reg.Get("support")
	gt.NoError(t, err).Required()
	e.CaseTrigger = model.CaseTriggerMention

	slackMock := &agentTestSlackService{}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	// Two scripted direct-reply turns (planner "direct" round + the reply).
	var turn atomic.Int32
	prompts := &promptRecorder{}
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			prompts.record(opts...)
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					switch turn.Add(1) {
					case 1, 3:
						return &gollem.Response{Texts: []string{`{"message":"answering directly","direct":{}}`}}, nil
					case 2:
						return &gollem.Response{Texts: []string{"Nothing is on fire."}}, nil
					case 4:
						return &gollem.Response{Texts: []string{"Still nothing."}}, nil
					}
					return nil, errors.New("unexpected extra LLM call")
				},
			}, nil
		},
	}

	agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     reg,
		LLM:          llm,
		HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:    agentarchive.NewMemoryTraceRepository(),
		SlackService: slackMock,
		CaseUC:       caseUC,
	})
	startAgentRuntime(t, agentRuntimeDeps{
		UC: agentUC, Repo: repo, Registry: reg, LLM: llm,
	})
	uc := usecase.NewSlackUseCases(repo, reg, agentUC, nil, slackMock)

	mention := func(ts, threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            "U-ASKER",
					Text:            "<@UBOT001> anything on fire?",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         channel,
					EventTimeStamp:  ts,
				},
			},
			TeamID: "T1",
		}
	}

	// 1. Channel-root mention opens the workspace-agent thread.
	const rootTS = "1700000200.000001"
	gt.NoError(t, uc.HandleSlackEvent(ctx, mention(rootTS, ""))).Required()
	async.Wait()

	// The answer arrives from the worker; the session is claimed synchronously.
	posts := waitForPosts(t, slackMock, 2)
	gt.Array(t, posts).Length(2).Required()
	gt.Value(t, posts[1].ThreadTS).Equal(rootTS)
	gt.Value(t, posts[1].Text).Equal("Nothing is on fire.")

	ssn, err := repo.Session().GetByThread(ctx, channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)
	firstSessionID := ssn.ID

	// 2. Follow-up mention inside that thread continues the same conversation.
	gt.NoError(t, uc.HandleSlackEvent(ctx, mention("1700000200.000002", rootTS))).Required()
	async.Wait()

	posts = waitForPosts(t, slackMock, 4)
	gt.Array(t, posts).Length(4).Required()
	gt.Value(t, posts[3].ThreadTS).Equal(rootTS)
	gt.Value(t, posts[3].Text).Equal("Still nothing.")

	// 3. Both turns went to the workspace agent — two persona prompts, one per
	// turn — on one Session, and nothing in this thread ever became a Case.
	gt.Bool(t, prompts.sawWorkspaceAgent()).True()
	gt.Number(t, prompts.workspaceAgentPromptCount()).Equal(2)

	ssn, err = repo.Session().GetByThread(ctx, channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, ssn).NotNil().Required()
	gt.Value(t, ssn.ID).Equal(firstSessionID)
	gt.Value(t, ssn.Kind).Equal(model.SessionKindWorkspaceAgent)

	c, err := repo.Case().GetBySlackThread(ctx, "support", channel, rootTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).Nil()
}

// sessionLookupFailureRepo makes Session().GetByThread fail while leaving every
// other repository intact, so a test can exercise the dispatcher's behaviour
// when it cannot tell a workspace-agent thread from a case-forming one.
type sessionLookupFailureRepo struct {
	interfaces.Repository
	err error
}

func (r *sessionLookupFailureRepo) Session() interfaces.SessionRepository {
	return &failingSessionRepo{SessionRepository: r.Repository.Session(), err: r.err}
}

type failingSessionRepo struct {
	interfaces.SessionRepository
	err error
}

func (r *failingSessionRepo) GetByThread(_ context.Context, _, _ string) (*model.Session, error) {
	return nil, r.err
}

// When the Session lookup fails the dispatcher cannot distinguish a
// workspace-agent thread from a thread whose Case is still forming. Creating a
// stray Case in someone's agent conversation is worse than not answering, so
// the mention must be dropped rather than guessed at.
func TestSlackUseCases_ThreadModeMentionSessionLookupFailure(t *testing.T) {
	ctx := context.Background()
	const channel = "C-MONITOR"

	base := memory.New()
	reg := newThreadWorkspaceRegistry()
	if e, err := reg.Get("support"); err == nil {
		e.CaseTrigger = model.CaseTriggerMention
	}
	slackMock := &agentTestSlackService{}
	repo := &sessionLookupFailureRepo{Repository: base, err: errors.New("session backend unavailable")}
	caseUC := usecase.NewCaseUseCase(repo, reg, slackMock, nil, "https://app.test")

	var llmInvoked atomic.Bool
	probe := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			llmInvoked.Store(true)
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, _ ...gollem.Input) (*gollem.Response, error) {
					return nil, errors.New("planner generate not scripted in dispatch test")
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
	uc := usecase.NewSlackUseCases(repo, reg, agentUC, nil, slackMock)

	threadTS := "1700000100.000001"
	ev := &slackevents.EventsAPIEvent{
		Type: slackevents.CallbackEvent,
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: string(slackevents.AppMention),
			Data: &slackevents.AppMentionEvent{
				Type:            "app_mention",
				User:            "U-ASKER",
				Text:            "<@UBOT001> please make this a case",
				TimeStamp:       "1700000100.000002",
				ThreadTimeStamp: threadTS,
				Channel:         channel,
				EventTimeStamp:  "1700000100.000002",
			},
		},
		TeamID: "T1",
	}
	gt.NoError(t, uc.HandleSlackEvent(ctx, ev)).Required()
	async.Wait()

	gt.Value(t, llmInvoked.Load()).Equal(false)

	c, err := base.Case().GetBySlackThread(ctx, "support", channel, threadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, c).Nil()
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

// TestSlackUseCases_WorkspaceChannelMentionRouting verifies that an
// app_mention landing in a channel-mode workspace's configured workspace
// channel ([slack] workspace_channel) is routed to the cross-case workspace
// agent (AgentUseCase.HandleWorkspaceChannelMention), not to the mention
// draft-proposal flow (MentionProposalUseCase.HandleAppMention) — see the
// dispatch order in HandleSlackEvent. mentionProposal is passed as nil: if the
// dispatcher regressed to that branch it would return nil silently and never
// touch Slack, so any observed workspace-agent-shaped Slack traffic proves the
// correct branch ran.
func TestSlackUseCases_WorkspaceChannelMentionRouting(t *testing.T) {
	const workspaceChannel = "C-WORKSPACE"

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:               model.Workspace{ID: "ws-channel", Name: "Channel WS"},
		SlackWorkspaceChannelID: workspaceChannel,
	})

	mentionEvent := func(ts, threadTS string) *slackevents.EventsAPIEvent {
		return &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:            "app_mention",
					User:            "U-ASKER",
					Text:            "<@UBOT001> what's open right now?",
					TimeStamp:       ts,
					ThreadTimeStamp: threadTS,
					Channel:         workspaceChannel,
					EventTimeStamp:  ts,
				},
			},
			TeamID: "T1",
		}
	}

	t.Run("routes to the workspace agent, not the mention draft flow", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		slackMock := &agentTestSlackService{}

		llm := wsMentionScript("Here is the cross-case answer.")
		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llm,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})
		startAgentRuntime(t, agentRuntimeDeps{
			UC: agentUC, Repo: repo, Registry: registry, LLM: llm,
		})

		// mentionProposal is nil: a regression that routed this event there
		// instead would return nil with zero Slack calls, which the assertions
		// below rule out.
		slackUC := usecase.NewSlackUseCases(repo, registry, agentUC, nil, slackMock)

		const mentionTS = "1700400000.000001"
		gt.NoError(t, slackUC.HandleSlackEvent(ctx, mentionEvent(mentionTS, ""))).Required()

		// The answer arrives from the worker: the handler returns once the run is
		// recorded.
		posts := waitForPosts(t, slackMock, 2)
		gt.Array(t, posts).Length(2).Required()
		gt.Value(t, posts[0].ChannelID).Equal(workspaceChannel)
		gt.Value(t, posts[0].ThreadTS).Equal(mentionTS)
		gt.Value(t, posts[1].ChannelID).Equal(workspaceChannel)
		gt.Value(t, posts[1].ThreadTS).Equal(mentionTS)
		gt.Value(t, posts[1].Text).Equal("Here is the cross-case answer.")

		// A case-less session was created for the thread, tied to the
		// workspace but bound to no Case (CaseID == 0).
		ssn, err := repo.Session().GetByThread(ctx, workspaceChannel, mentionTS)
		gt.NoError(t, err).Required()
		gt.Value(t, ssn).NotNil().Required()
		gt.Value(t, ssn.WorkspaceID).Equal("ws-channel")
		gt.Value(t, ssn.CaseID).Equal(int64(0))
	})

	t.Run("agent nil: no-op even when the channel matches", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		slackMock := &agentTestSlackService{}

		// No AgentUseCase (agent nil): HandleSlackEvent's workspace-channel
		// branch must short-circuit before touching Slack.
		slackUC := usecase.NewSlackUseCases(repo, registry, nil, nil, slackMock)

		gt.NoError(t, slackUC.HandleSlackEvent(ctx, mentionEvent("1700400010.000001", ""))).Required()
		gt.Array(t, slackMock.postedMessages).Length(0)
	})
}
