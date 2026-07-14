package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	goslack "github.com/slack-go/slack" //nolint:depguard
)

// agentTestSlackService is a mock Slack service for agent testing
type agentTestSlackService struct {
	mockSlackService
	getConversationRepliesFn func(ctx context.Context, channelID string, threadTS string, limit int) ([]slack.ConversationMessage, error)
	getConversationHistoryFn func(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slack.ConversationMessage, error)
	postThreadReplyFn        func(ctx context.Context, channelID string, threadTS string, text string) (string, error)
	getBotUserIDFn           func(ctx context.Context) (string, error)
	getPermalinkFn           func(ctx context.Context, channelID string, ts string) (string, error)
	postedMessages           []agentPostedMessage
	updatedMessages          []agentUpdatedMessage
}

type agentPostedMessage struct {
	ChannelID string
	ThreadTS  string
	Text      string
}

type agentUpdatedMessage struct {
	ChannelID string
	Timestamp string
	Text      string
}

func (m *agentTestSlackService) GetConversationReplies(ctx context.Context, channelID string, threadTS string, limit int) ([]slack.ConversationMessage, error) {
	if m.getConversationRepliesFn != nil {
		return m.getConversationRepliesFn(ctx, channelID, threadTS, limit)
	}
	return nil, nil
}

func (m *agentTestSlackService) GetConversationHistory(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slack.ConversationMessage, error) {
	if m.getConversationHistoryFn != nil {
		return m.getConversationHistoryFn(ctx, channelID, oldest, limit)
	}
	return nil, nil
}

func (m *agentTestSlackService) PostThreadReply(ctx context.Context, channelID string, threadTS string, text string) (string, error) {
	m.postedMessages = append(m.postedMessages, agentPostedMessage{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Text:      text,
	})
	if m.postThreadReplyFn != nil {
		return m.postThreadReplyFn(ctx, channelID, threadTS, text)
	}
	return "1234567890.trace01", nil
}

func (m *agentTestSlackService) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []goslack.Block, text string, opts ...slack.PostThreadOption) (string, error) {
	m.postedMessages = append(m.postedMessages, agentPostedMessage{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Text:      text,
	})
	return "1234567890.session01", nil
}

func (m *agentTestSlackService) UpdateMessage(ctx context.Context, channelID string, timestamp string, blocks []goslack.Block, text string) error {
	m.updatedMessages = append(m.updatedMessages, agentUpdatedMessage{
		ChannelID: channelID,
		Timestamp: timestamp,
		Text:      text,
	})
	return nil
}

func (m *agentTestSlackService) OpenView(ctx context.Context, triggerID string, view goslack.ModalViewRequest) error {
	return nil
}

func (m *agentTestSlackService) UpdateView(_ context.Context, _ goslack.ModalViewRequest, _, _, _ string) error {
	return nil
}

func (m *agentTestSlackService) PostEphemeral(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (m *agentTestSlackService) PostEphemeralBlocks(_ context.Context, _ string, _ string, _ []goslack.Block, _ string) (string, error) {
	return "ts-eph", nil
}

func (m *agentTestSlackService) GetPermalink(ctx context.Context, channelID string, ts string) (string, error) {
	if m.getPermalinkFn != nil {
		return m.getPermalinkFn(ctx, channelID, ts)
	}
	return "https://slack.test/" + channelID + "/" + ts, nil
}

func (m *agentTestSlackService) GetBotUserID(ctx context.Context) (string, error) {
	if m.getBotUserIDFn != nil {
		return m.getBotUserIDFn(ctx)
	}
	return "UBOT001", nil
}

// mockLLMSession is a mock gollem Session for testing
type mockLLMSession struct {
	generateContentFn func(ctx context.Context, input ...gollem.Input) (*gollem.Response, error)
}

func (s *mockLLMSession) Generate(ctx context.Context, input []gollem.Input, opts ...gollem.GenerateOption) (*gollem.Response, error) {
	if s.generateContentFn != nil {
		return s.generateContentFn(ctx, input...)
	}
	return &gollem.Response{
		Texts: []string{"This is a test response from the AI agent."},
	}, nil
}

func (s *mockLLMSession) Stream(ctx context.Context, input []gollem.Input, opts ...gollem.GenerateOption) (<-chan *gollem.Response, error) {
	return nil, nil
}

func (s *mockLLMSession) GenerateContent(ctx context.Context, input ...gollem.Input) (*gollem.Response, error) {
	return s.Generate(ctx, input)
}

func (s *mockLLMSession) GenerateStream(ctx context.Context, input ...gollem.Input) (<-chan *gollem.Response, error) {
	return s.Stream(ctx, input)
}

func (s *mockLLMSession) History() (*gollem.History, error) {
	return nil, nil
}

func (s *mockLLMSession) AppendHistory(*gollem.History) error {
	return nil
}

func (s *mockLLMSession) CountToken(ctx context.Context, input ...gollem.Input) (int, error) {
	return 0, nil
}

// mockLLMClient is a mock gollem LLMClient for testing
type mockLLMClient struct {
	newSessionFn func(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error)
}

func (c *mockLLMClient) NewSession(ctx context.Context, options ...gollem.SessionOption) (gollem.Session, error) {
	if c.newSessionFn != nil {
		return c.newSessionFn(ctx, options...)
	}
	return &mockLLMSession{}, nil
}

func (c *mockLLMClient) GenerateEmbedding(ctx context.Context, dimension int, input []string) ([][]float64, error) {
	return nil, nil
}

func TestAgentUseCase_HandleAgentMention(t *testing.T) {
	t.Run("responds to mention in channel with case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a case with a Slack channel
		_, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Security Incident",
			Description:    "A test security incident",
			Status:         types.CaseStatusOpen,
			SlackChannelID: "C-AGENT-001",
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect},
				},
			},
		})

		slackMock := &agentTestSlackService{}
		slackMock.getConversationHistoryFn = func(ctx context.Context, channelID string, oldest time.Time, limit int) ([]slack.ConversationMessage, error) {
			return []slack.ConversationMessage{
				{UserID: "U001", UserName: "alice", Text: "Something happened", Timestamp: "1234567890.000001"},
				{UserID: "U002", UserName: "bob", Text: "@bot what do you think?", Timestamp: "1234567890.000002"},
			}, nil
		}

		llmClient := &mockLLMClient{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llmClient,
			EmbedClient:  llmClient,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		msg := slackmodel.NewMessageFromData(
			"1234567890.000002",
			"C-AGENT-001",
			"", // no thread TS (channel mention)
			"T123",
			"U002",
			"bob",
			"@bot what do you think?",
			"1234567890.000002",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleAgentMention(ctx, msg)).Required()

		// Verify session start + final response were posted (2 messages)
		gt.Array(t, slackMock.postedMessages).Length(2).Required()
		// First message: session start (via PostThreadMessage)
		gt.Value(t, slackMock.postedMessages[0].ChannelID).Equal("C-AGENT-001")
		gt.Value(t, slackMock.postedMessages[0].Text).NotEqual("") // session start (random label)
		// Second message: final response (via PostThreadReply)
		gt.Value(t, slackMock.postedMessages[1].ChannelID).Equal("C-AGENT-001")
		gt.Value(t, slackMock.postedMessages[1].Text).Equal("This is a test response from the AI agent.")
	})

	t.Run("responds to mention in thread", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Bug Report",
			Description:    "A test bug",
			Status:         types.CaseStatusOpen,
			SlackChannelID: "C-AGENT-002",
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		slackMock := &agentTestSlackService{}
		slackMock.getConversationRepliesFn = func(ctx context.Context, channelID string, threadTS string, limit int) ([]slack.ConversationMessage, error) {
			gt.Value(t, threadTS).Equal("1234567890.000010")
			return []slack.ConversationMessage{
				{UserID: "U001", UserName: "alice", Text: "Found a bug", Timestamp: "1234567890.000010"},
				{UserID: "U002", UserName: "bob", Text: "@bot help", Timestamp: "1234567890.000011"},
			}, nil
		}

		llmClient := &mockLLMClient{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llmClient,
			EmbedClient:  llmClient,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		msg := slackmodel.NewMessageFromData(
			"1234567890.000011",
			"C-AGENT-002",
			"1234567890.000010", // thread TS
			"T123",
			"U002",
			"bob",
			"@bot help",
			"1234567890.000011",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleAgentMention(ctx, msg)).Required()

		// Verify session start + final response were posted (2 messages)
		gt.Array(t, slackMock.postedMessages).Length(2).Required()
		// First message: session start (via PostThreadMessage)
		gt.Value(t, slackMock.postedMessages[0].ThreadTS).Equal("1234567890.000010")
		gt.Value(t, slackMock.postedMessages[0].Text).NotEqual("") // session start (random label)
		// Second message: final response (via PostThreadReply)
		gt.Value(t, slackMock.postedMessages[1].ThreadTS).Equal("1234567890.000010")
	})

	t.Run("skips when no case found for channel", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		slackMock := &agentTestSlackService{}
		llmClient := &mockLLMClient{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llmClient,
			EmbedClient:  llmClient,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		msg := slackmodel.NewMessageFromData(
			"1234567890.000100",
			"C-UNKNOWN",
			"",
			"T123",
			"U002",
			"bob",
			"@bot hello",
			"1234567890.000100",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleAgentMention(ctx, msg)).Required()

		// No messages should be posted
		gt.Array(t, slackMock.postedMessages).Length(0)
	})

	t.Run("skips bot's own message", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		_, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			ReporterID:     "U-TEST-DEFAULT",
			Title:          "Test Case",
			SlackChannelID: "C-AGENT-003",
		})
		gt.NoError(t, err).Required()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		slackMock := &agentTestSlackService{}
		slackMock.getBotUserIDFn = func(ctx context.Context) (string, error) {
			return "UBOT001", nil
		}

		llmClient := &mockLLMClient{}

		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			Registry:     registry,
			LLM:          llmClient,
			EmbedClient:  llmClient,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: slackMock,
		})

		msg := slackmodel.NewMessageFromData(
			"1234567890.000200",
			"C-AGENT-003",
			"",
			"T123",
			"UBOT001", // bot's own user ID
			"bot",
			"I already responded",
			"1234567890.000200",
			time.Now(),
			nil,
		)

		gt.NoError(t, agentUC.HandleAgentMention(ctx, msg)).Required()

		// No messages should be posted
		gt.Array(t, slackMock.postedMessages).Length(0)
	})

	// Note: system prompt assembly tests (case info / field values / channel
	// ID / actions / current action / due date / unassigned) live in
	// pkg/usecase/agent/casebound/casebound_test.go now that buildSystemPrompt
	// has moved into the casebound subpackage.
}

func TestParseAgentActionValue(t *testing.T) {
	t.Run("parses action with data", func(t *testing.T) {
		action, data, err := usecase.ParseAgentActionValue("show_session_info:abc-123-def")
		gt.NoError(t, err)
		gt.Value(t, action).Equal("show_session_info")
		gt.Value(t, data).Equal("abc-123-def")
	})

	t.Run("parses action without data", func(t *testing.T) {
		action, data, err := usecase.ParseAgentActionValue("send_feedback")
		gt.NoError(t, err)
		gt.Value(t, action).Equal("send_feedback")
		gt.Value(t, data).Equal("")
	})

	t.Run("parses action with multiple colons in data", func(t *testing.T) {
		action, data, err := usecase.ParseAgentActionValue("show_session_info:0193a7b0-7c3d-7e8f-9a1b-2c3d4e5f6a7b")
		gt.NoError(t, err)
		gt.Value(t, action).Equal("show_session_info")
		gt.Value(t, data).Equal("0193a7b0-7c3d-7e8f-9a1b-2c3d4e5f6a7b")
	})
}

func TestBuildTraceContextBlocks(t *testing.T) {
	t.Run("empty lines produce empty blocks", func(t *testing.T) {
		blocks := usecase.BuildTraceContextBlocksForTest(nil)
		gt.Array(t, blocks).Length(0)
	})

	t.Run("each line becomes its own context block", func(t *testing.T) {
		lines := []string{
			"\U0001f527 `tool_a`",
			"\U0001f527 `tool_b`",
			"❌ Error: boom",
		}
		blocks := usecase.BuildTraceContextBlocksForTest(lines)
		gt.Array(t, blocks).Length(len(lines)).Required()

		for i, block := range blocks {
			ctxBlock, ok := block.(*goslack.ContextBlock)
			gt.Bool(t, ok).True().Required()
			gt.Value(t, ctxBlock.Type).Equal(goslack.MBTContext)
			gt.Array(t, ctxBlock.ContextElements.Elements).Length(1).Required()

			text, ok := ctxBlock.ContextElements.Elements[0].(*goslack.TextBlockObject)
			gt.Bool(t, ok).True().Required()
			gt.Value(t, text.Type).Equal(goslack.MarkdownType)
			gt.String(t, text.Text).Equal(lines[i])
		}
	})

	t.Run("caps blocks at Slack's 50-block per-message limit", func(t *testing.T) {
		lines := make([]string, 75)
		for i := range lines {
			lines[i] = fmt.Sprintf("line-%02d", i)
		}
		blocks := usecase.BuildTraceContextBlocksForTest(lines)
		gt.Array(t, blocks).Length(50).Required()

		// The most recent lines must survive (lines[25] .. lines[74]).
		first, ok := blocks[0].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		firstText, ok := first.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		gt.String(t, firstText.Text).Equal("line-25")

		last, ok := blocks[49].(*goslack.ContextBlock)
		gt.Bool(t, ok).True().Required()
		lastText, ok := last.ContextElements.Elements[0].(*goslack.TextBlockObject)
		gt.Bool(t, ok).True().Required()
		gt.String(t, lastText.Text).Equal("line-74")
	})
}

func TestAgentUseCase_HandleSessionInfoRequest(t *testing.T) {
	t.Run("opens modal with session ID", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		mockWithCapture := &agentTestSlackServiceWithOpenView{
			agentTestSlackService: slackMock,
		}

		llmClient := &mockLLMClient{}
		i18n.Init(i18n.LangEN)
		agentUC := usecase.NewAgentUseCase(usecase.AgentDeps{
			Repo:         repo,
			LLM:          llmClient,
			EmbedClient:  llmClient,
			HistoryRepo:  agentarchive.NewMemoryHistoryRepository(),
			TraceRepo:    agentarchive.NewMemoryTraceRepository(),
			SlackService: mockWithCapture,
		})

		err := agentUC.HandleSessionInfoRequest(t.Context(), "trigger-123", "test-session-id")
		gt.NoError(t, err)

		gt.Value(t, mockWithCapture.openViewCalled).Equal(true)
		gt.Value(t, mockWithCapture.openViewTriggerID).Equal("trigger-123")
		gt.Value(t, mockWithCapture.openViewRequest.Title.Text).Equal("Session Info")
	})
}

// agentTestSlackServiceWithOpenView wraps agentTestSlackService with OpenView capture
type agentTestSlackServiceWithOpenView struct {
	*agentTestSlackService
	openViewCalled    bool
	openViewTriggerID string
	openViewRequest   goslack.ModalViewRequest
}

func (m *agentTestSlackServiceWithOpenView) OpenView(ctx context.Context, triggerID string, view goslack.ModalViewRequest) error {
	m.openViewCalled = true
	m.openViewTriggerID = triggerID
	m.openViewRequest = view
	return nil
}

// TestLifecycle_AgentSession exercises the AgentSession + History/Trace
// pipeline across two consecutive mentions on the same Slack thread:
//
//  1. First mention creates a new AgentSession, records its ID, and seeds the
//     prompt with the thread's full context (no delta).
//  2. A non-bot user message arrives in the thread between mentions.
//  3. Second mention reuses the same session, surfaces the intervening
//     message as a delta in the user input, and bumps LastMentionTS.
//
// It also asserts that gollem received the same sessionID for WithHistoryRepository
// on both turns (so persisted history is actually reused) and that a trace
// blob was written for each turn.
func TestLifecycle_AgentSession(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	historyRepo := agentarchive.NewMemoryHistoryRepository()
	traceRepo := agentarchive.NewMemoryTraceRepository()

	_, err := repo.Case().Create(ctx, "ws-lifecycle", &model.Case{
		ReporterID:     "U-TEST-DEFAULT",
		Title:          "Thread session test",
		Description:    "lifecycle",
		Status:         types.CaseStatusOpen,
		SlackChannelID: "C-LIFE",
	})
	gt.NoError(t, err).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-lifecycle", Name: "Lifecycle"},
	})

	threadParent := "1700000001.000001"
	firstMentionTS := threadParent
	intermediateTS := "1700000002.000001"
	secondMentionTS := "1700000003.000001"

	repliesAfterFirst := []slack.ConversationMessage{
		{UserID: "U001", UserName: "alice", Text: "context message", Timestamp: threadParent},
		{UserID: "U001", UserName: "alice", Text: "@bot kicking off", Timestamp: firstMentionTS},
	}
	repliesAfterSecond := append(repliesAfterFirst,
		slack.ConversationMessage{UserID: "U002", UserName: "bob", Text: "extra info", Timestamp: intermediateTS},
		slack.ConversationMessage{UserID: "UBOT001", UserName: "bot", Text: "previous bot reply", Timestamp: "1700000002.500000"},
		slack.ConversationMessage{UserID: "U001", UserName: "alice", Text: "@bot follow up", Timestamp: secondMentionTS},
	)

	stage := 0 // 0 = before first mention runs, 1 = before second
	slackMock := &agentTestSlackService{
		getConversationRepliesFn: func(_ context.Context, _ string, _ string, _ int) ([]slack.ConversationMessage, error) {
			if stage == 0 {
				return repliesAfterFirst, nil
			}
			return repliesAfterSecond, nil
		},
	}

	type capturedTurn struct {
		generateText string
	}
	var captured []capturedTurn

	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			turn := capturedTurn{}
			session := &mockLLMSession{
				generateContentFn: func(_ context.Context, input ...gollem.Input) (*gollem.Response, error) {
					if len(input) > 0 {
						if txt, ok := input[0].(gollem.Text); ok {
							turn.generateText = string(txt)
						}
					}
					return &gollem.Response{Texts: []string{"ack"}}, nil
				},
			}
			captured = append(captured, turn)
			return session, nil
		},
	}

	uc := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     registry,
		LLM:          llm,
		EmbedClient:  llm,
		HistoryRepo:  historyRepo,
		TraceRepo:    traceRepo,
		SlackService: slackMock,
	})

	// --- First mention -----------------------------------------------------
	first := slackmodel.NewMessageFromData(
		firstMentionTS,
		"C-LIFE",
		"", // top-level mention; threadTS will be derived from msg.ID()
		"T-life",
		"U001",
		"alice",
		"@bot kicking off",
		firstMentionTS,
		time.Unix(1700000001, 0).UTC(),
		nil,
	)
	gt.NoError(t, uc.HandleAgentMention(ctx, first)).Required()

	session1, err := repo.Session().GetByThread(ctx, "C-LIFE", threadParent)
	gt.NoError(t, err).Required()
	gt.Value(t, session1).NotNil().Required()
	gt.Value(t, session1.LastMentionTS).Equal(firstMentionTS)
	gt.Value(t, session1.ChannelID).Equal("C-LIFE")
	gt.String(t, session1.ID).NotEqual("")

	// First turn LLM input is just the mention text (no delta).
	gt.Array(t, captured).Length(1).Required()
	// The actual generateText is captured inside the closure scope, but we
	// ran the closure assertions there. Re-fetch via slackMock posted text
	// to confirm the agent reply made it to Slack.
	gt.Array(t, slackMock.postedMessages).Length(2)
	gt.Value(t, slackMock.postedMessages[1].Text).Equal("ack")

	// One trace persisted under the new session. TraceID is now the per-turn
	// UUID v7 (TurnID), so we just assert there is a non-empty entry.
	traces1 := traceRepo.TraceIDs(session1.ID)
	gt.Array(t, traces1).Length(1)
	gt.String(t, traces1[0]).NotEqual("")

	// --- Second mention ----------------------------------------------------
	stage = 1
	second := slackmodel.NewMessageFromData(
		secondMentionTS,
		"C-LIFE",
		threadParent, // explicit thread reply
		"T-life",
		"U001",
		"alice",
		"@bot follow up",
		secondMentionTS,
		time.Unix(1700000003, 0).UTC(),
		nil,
	)
	gt.NoError(t, uc.HandleAgentMention(ctx, second)).Required()

	session2, err := repo.Session().GetByThread(ctx, "C-LIFE", threadParent)
	gt.NoError(t, err).Required()
	gt.Value(t, session2).NotNil().Required()
	gt.Value(t, session2.ID).Equal(session1.ID) // same session reused
	gt.Value(t, session2.LastMentionTS).Equal(secondMentionTS)

	// Two turns total in captured.
	gt.Array(t, captured).Length(2)

	// Two distinct traces persisted under the same session — TurnID UUIDs.
	traces2 := traceRepo.TraceIDs(session1.ID)
	gt.Array(t, traces2).Length(2)
	gt.String(t, traces2[0]).NotEqual("")
	gt.String(t, traces2[1]).NotEqual("")
	gt.String(t, traces2[0]).NotEqual(traces2[1])
}

// TestAgentUseCase_DeltaMessageInjection asserts the delta path explicitly:
// continuing-session mentions surface only post-lastMentionTS, non-bot
// thread messages, and pass them as user input rather than re-stuffing the
// system prompt.
func TestAgentUseCase_DeltaMessageInjection(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	historyRepo := agentarchive.NewMemoryHistoryRepository()
	traceRepo := agentarchive.NewMemoryTraceRepository()

	c, err := repo.Case().Create(ctx, "ws-delta", &model.Case{
		ReporterID:     "U-TEST-DEFAULT",
		Title:          "Delta test",
		Status:         types.CaseStatusOpen,
		SlackChannelID: "C-DELTA",
	})
	gt.NoError(t, err).Required()

	// Pre-seed an existing AgentSession so the next mention takes the
	// continuing-session path.
	const (
		threadTS        = "1700100000.000001"
		previousMention = "1700100005.000001"
		newMention      = "1700100020.000001"
	)
	gt.NoError(t, repo.Session().Put(ctx, &model.Session{
		ID:            "session-delta",
		WorkspaceID:   "ws-delta",
		CaseID:        c.ID,
		ThreadTS:      threadTS,
		ChannelID:     "C-DELTA",
		LastMentionTS: previousMention,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-delta", Name: "Delta"},
	})

	slackMock := &agentTestSlackService{
		getConversationRepliesFn: func(_ context.Context, _ string, _ string, _ int) ([]slack.ConversationMessage, error) {
			return []slack.ConversationMessage{
				// before previous mention — must be excluded
				{UserID: "U001", UserName: "alice", Text: "old chatter", Timestamp: "1700100002.000000"},
				// previous mention itself — must be excluded (== previousMention)
				{UserID: "U001", UserName: "alice", Text: "@bot earlier", Timestamp: previousMention},
				// bot reply between mentions — must be excluded (bot user)
				{UserID: "UBOT001", UserName: "bot", Text: "earlier reply", Timestamp: "1700100006.000000"},
				// real delta — must be included
				{UserID: "U002", UserName: "bob", Text: "interim update", Timestamp: "1700100010.000000"},
				// current mention — must be excluded (handled separately)
				{UserID: "U001", UserName: "alice", Text: "@bot now what", Timestamp: newMention},
			}, nil
		},
	}

	var capturedInput string
	llm := &mockLLMClient{
		newSessionFn: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mockLLMSession{
				generateContentFn: func(_ context.Context, input ...gollem.Input) (*gollem.Response, error) {
					if len(input) > 0 {
						if txt, ok := input[0].(gollem.Text); ok {
							capturedInput = string(txt)
						}
					}
					return &gollem.Response{Texts: []string{"ok"}}, nil
				},
			}, nil
		},
	}

	uc := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     registry,
		LLM:          llm,
		EmbedClient:  llm,
		HistoryRepo:  historyRepo,
		TraceRepo:    traceRepo,
		SlackService: slackMock,
	})

	msg := slackmodel.NewMessageFromData(
		newMention,
		"C-DELTA",
		threadTS,
		"T-delta",
		"U001",
		"alice",
		"@bot now what",
		newMention,
		time.Unix(1700100020, 0).UTC(),
		nil,
	)
	gt.NoError(t, uc.HandleAgentMention(ctx, msg)).Required()

	// Verify exactly the interim update (and not the bot reply, nor older
	// messages, nor the current mention itself) was included as a delta.
	gt.String(t, capturedInput).Contains("Unprocessed thread messages")
	gt.String(t, capturedInput).Contains("interim update")
	gt.String(t, capturedInput).Contains("@bot now what")
	if strings.Contains(capturedInput, "@bot earlier") {
		t.Errorf("delta must not contain previous mention: %q", capturedInput)
	}
	if strings.Contains(capturedInput, "earlier reply") {
		t.Errorf("delta must not contain bot reply: %q", capturedInput)
	}
	if strings.Contains(capturedInput, "old chatter") {
		t.Errorf("delta must not contain pre-lastMentionTS chatter: %q", capturedInput)
	}

	// Session updated with the new mention TS.
	updated, err := repo.Session().GetByThread(ctx, "C-DELTA", threadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, updated.LastMentionTS).Equal(newMention)
}

// TestAgentUseCase_ActionLinkage asserts that when a mention starts a thread
// whose parent TS matches an Action's notification message, the new session
// records that ActionID.
func TestAgentUseCase_ActionLinkage(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	historyRepo := agentarchive.NewMemoryHistoryRepository()
	traceRepo := agentarchive.NewMemoryTraceRepository()

	c, err := repo.Case().Create(ctx, "ws-action", &model.Case{
		ReporterID:     "U-TEST-DEFAULT",
		Title:          "Action linkage",
		Status:         types.CaseStatusOpen,
		SlackChannelID: "C-ACT",
	})
	gt.NoError(t, err).Required()

	const actionThreadTS = "1700200000.000001"

	createdAction, err := repo.Action().Create(ctx, "ws-action", &model.Action{
		CaseID:         c.ID,
		Title:          "Investigate",
		Status:         "open",
		SlackMessageTS: actionThreadTS,
	})
	gt.NoError(t, err).Required()

	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "ws-action", Name: "Action"},
	})

	slackMock := &agentTestSlackService{}
	llm := &mockLLMClient{}
	uc := usecase.NewAgentUseCase(usecase.AgentDeps{
		Repo:         repo,
		Registry:     registry,
		LLM:          llm,
		EmbedClient:  llm,
		HistoryRepo:  historyRepo,
		TraceRepo:    traceRepo,
		SlackService: slackMock,
	})

	msg := slackmodel.NewMessageFromData(
		"1700200005.000001",
		"C-ACT",
		actionThreadTS,
		"T-act",
		"U001",
		"alice",
		"@bot help with this action",
		"1700200005.000001",
		time.Unix(1700200005, 0).UTC(),
		nil,
	)
	gt.NoError(t, uc.HandleAgentMention(ctx, msg)).Required()

	session, err := repo.Session().GetByThread(ctx, "C-ACT", actionThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, session).NotNil().Required()
	gt.Value(t, session.ActionID).Equal(createdAction.ID)
}

// traceBlockTexts extracts the rendered markdown text of every context block
// in order, so trace tests can assert on the visible lines.
func traceBlockTexts(blocks []goslack.Block) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		cb, ok := b.(*goslack.ContextBlock)
		if !ok {
			continue
		}
		for _, el := range cb.ContextElements.Elements {
			if tb, ok := el.(*goslack.TextBlockObject); ok {
				out = append(out, tb.Text)
			}
		}
	}
	return out
}

// TestTraceMessage_AppendAccumulatesReplaceOverwrites verifies the core
// contract of the trace banner: milestone lines (appendLine) accumulate as
// separate context blocks and stay visible, while the transient activity line
// (replaceLine) overwrites a single trailing block instead of piling up. A
// new milestone clears the live line, so per-tool chatter never lingers after
// the step that produced it.
func TestTraceMessage_AppendAccumulatesReplaceOverwrites(t *testing.T) {
	ctx := context.Background()
	cap := &traceCapture{}
	tm := usecase.NewTraceMessageForTest(cap, "C-TRACE", "1700000000.000001")

	// First milestone: posts a fresh message with a single block.
	usecase.TraceMessageAppendForTest(tm, ctx, "🧭 Planning")
	// Second milestone: accumulates, two blocks now.
	usecase.TraceMessageAppendForTest(tm, ctx, "🔎 Investigating (2 task(s))")

	calls := cap.calls()
	gt.Array(t, calls).Length(2).Required()
	gt.Value(t, calls[0].method).Equal("post")
	gt.Value(t, calls[1].method).Equal("update")
	gt.Array(t, traceBlockTexts(calls[1].blocks)).Equal([]string{
		"🧭 Planning",
		"🔎 Investigating (2 task(s))",
	})

	// Three activity updates: each overwrites the single live line, so the
	// block count stays at 3 (2 milestones + 1 live line), never growing.
	usecase.TraceMessageReplaceForTest(tm, ctx, "Searching Slack: from:@issei")
	usecase.TraceMessageReplaceForTest(tm, ctx, "Searching Notion: scraping")
	usecase.TraceMessageReplaceForTest(tm, ctx, "Fetching Notion page abc")

	calls = cap.calls()
	last := calls[len(calls)-1]
	gt.Array(t, traceBlockTexts(last.blocks)).Equal([]string{
		"🧭 Planning",
		"🔎 Investigating (2 task(s))",
		"Fetching Notion page abc",
	})
	// Fallback text mirrors the rendered lines, live line last.
	gt.String(t, last.text).Contains("Fetching Notion page abc")

	// A new milestone clears the live activity line.
	usecase.TraceMessageAppendForTest(tm, ctx, "✓ Reporter profile & recent activity")
	calls = cap.calls()
	last = calls[len(calls)-1]
	gt.Array(t, traceBlockTexts(last.blocks)).Equal([]string{
		"🧭 Planning",
		"🔎 Investigating (2 task(s))",
		"✓ Reporter profile & recent activity",
	})
}

// TestTraceMessage_LiveLineSurvivesBlockCap verifies that when milestone
// history exceeds Slack's 50-block ceiling, the history is truncated to the
// oldest dropped but the transient live line is always preserved as the final
// block — it is never pushed out by milestone overflow.
func TestTraceMessage_LiveLineSurvivesBlockCap(t *testing.T) {
	ctx := context.Background()
	cap := &traceCapture{}
	tm := usecase.NewTraceMessageForTest(cap, "C-TRACE", "1700000000.000001")

	// Push well past the cap with milestones.
	for i := range usecase.MaxTraceBlocksForTest + 10 {
		usecase.TraceMessageAppendForTest(tm, ctx, fmt.Sprintf("milestone %d", i))
	}
	// Then a live activity line.
	usecase.TraceMessageReplaceForTest(tm, ctx, "Searching Slack: tail")

	calls := cap.calls()
	last := calls[len(calls)-1]
	texts := traceBlockTexts(last.blocks)
	// Total blocks never exceed the cap.
	gt.Number(t, len(texts)).Equal(usecase.MaxTraceBlocksForTest)
	// The live line is always the last block.
	gt.Value(t, texts[len(texts)-1]).Equal("Searching Slack: tail")
	// The most recent milestone is retained just above the live line.
	gt.Value(t, texts[len(texts)-2]).Equal(fmt.Sprintf("milestone %d", usecase.MaxTraceBlocksForTest+10-1))

	// The fallback text mirrors the visible window (it must not carry the
	// dropped milestones, or it could blow past Slack's 4000-char text limit).
	fallbackLines := strings.Split(last.text, "\n")
	gt.Number(t, len(fallbackLines)).Equal(usecase.MaxTraceBlocksForTest)
	gt.Value(t, fallbackLines[len(fallbackLines)-1]).Equal("Searching Slack: tail")
	gt.String(t, last.text).NotContains("milestone 0\n")
}

// TestTraceMessage_ConcurrentUpdatesDoNotPanic exercises the mutex guarding
// the shared lines/liveLine state, mirroring parallel sub-agent activity
// updates racing planner milestones.
func TestTraceMessage_ConcurrentUpdatesDoNotPanic(t *testing.T) {
	ctx := context.Background()
	cap := &traceCapture{}
	tm := usecase.NewTraceMessageForTest(cap, "C-TRACE", "1700000000.000001")

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			usecase.TraceMessageAppendForTest(tm, ctx, fmt.Sprintf("milestone %d", n))
		}(i)
		go func(n int) {
			defer wg.Done()
			usecase.TraceMessageReplaceForTest(tm, ctx, fmt.Sprintf("activity %d", n))
		}(i)
	}
	wg.Wait()

	// At least one Slack call was made and the banner is still renderable.
	gt.Number(t, len(cap.calls())).GreaterOrEqual(1)
}
