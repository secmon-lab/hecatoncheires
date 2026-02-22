package usecase_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
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

func (m *agentTestSlackService) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []goslack.Block, text string) (string, error) {
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

func (s *mockLLMSession) GenerateContent(ctx context.Context, input ...gollem.Input) (*gollem.Response, error) {
	if s.generateContentFn != nil {
		return s.generateContentFn(ctx, input...)
	}
	return &gollem.Response{
		Texts: []string{"This is a test response from the AI agent."},
	}, nil
}

func (s *mockLLMSession) GenerateStream(ctx context.Context, input ...gollem.Input) (<-chan *gollem.Response, error) {
	return nil, nil
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

		agentUC := usecase.NewAgentUseCase(repo, registry, slackMock, llmClient)

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

		agentUC := usecase.NewAgentUseCase(repo, registry, slackMock, llmClient)

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

		agentUC := usecase.NewAgentUseCase(repo, registry, slackMock, llmClient)

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

		agentUC := usecase.NewAgentUseCase(repo, registry, slackMock, llmClient)

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

	t.Run("system prompt includes case info and field values", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		llmClient := &mockLLMClient{}

		entry := &model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{ID: "severity", Name: "Severity Level", Type: types.FieldTypeSelect},
				},
			},
		}

		c := &model.Case{
			Title:       "Important Case",
			Description: "This is very important",
			Status:      types.CaseStatusOpen,
			FieldValues: map[string]model.FieldValue{
				"severity": {
					FieldID: "severity",
					Type:    types.FieldTypeSelect,
					Value:   "high",
				},
			},
		}

		messages := []usecase.ConversationMessage{
			{UserID: "U001", UserName: "alice", Text: "Hello", Timestamp: "1234567890.000001"},
		}

		agentUC := usecase.NewAgentUseCase(repo, nil, slackMock, llmClient)
		prompt := usecase.BuildAgentSystemPrompt(agentUC, c, entry, nil, nil, messages)

		gt.Value(t, strings.Contains(prompt, "Important Case")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "This is very important")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Severity Level")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "high")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "alice: Hello")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Slack's mrkdwn format")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Do NOT use Markdown headers")).Equal(true)
	})
}

func TestAgentSystemPrompt_ActionsAndKnowledges(t *testing.T) {
	t.Run("prompt includes actions when present", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		llmClient := &mockLLMClient{}

		entry := &model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		}
		c := &model.Case{
			Title:  "Test Case",
			Status: types.CaseStatusOpen,
		}
		actions := []*model.Action{
			{
				ID:          1,
				Title:       "Investigate the issue",
				Status:      types.ActionStatusInProgress,
				AssigneeIDs: []string{"U001", "U002"},
			},
			{
				ID:     2,
				Title:  "Write report",
				Status: types.ActionStatusTodo,
			},
		}

		agentUC := usecase.NewAgentUseCase(repo, nil, slackMock, llmClient)
		prompt := usecase.BuildAgentSystemPrompt(agentUC, c, entry, actions, nil, nil)

		gt.Value(t, strings.Contains(prompt, "## Actions")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Investigate the issue")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "Write report")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "U001, U002")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "IN_PROGRESS")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "TODO")).Equal(true)
	})

	t.Run("prompt includes knowledge when present", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		llmClient := &mockLLMClient{}

		entry := &model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		}
		c := &model.Case{
			Title:  "Test Case",
			Status: types.CaseStatusOpen,
		}
		knowledges := []*model.Knowledge{
			{
				ID:    model.KnowledgeID("knowledge-001"),
				Title: "How to handle this type of incident",
			},
		}

		agentUC := usecase.NewAgentUseCase(repo, nil, slackMock, llmClient)
		prompt := usecase.BuildAgentSystemPrompt(agentUC, c, entry, nil, knowledges, nil)

		gt.Value(t, strings.Contains(prompt, "## Knowledge")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "knowledge-001")).Equal(true)
		gt.Value(t, strings.Contains(prompt, "How to handle this type of incident")).Equal(true)
	})

	t.Run("actions and knowledge sections absent when empty", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		llmClient := &mockLLMClient{}

		entry := &model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		}
		c := &model.Case{
			Title:  "Test Case",
			Status: types.CaseStatusOpen,
		}

		agentUC := usecase.NewAgentUseCase(repo, nil, slackMock, llmClient)
		prompt := usecase.BuildAgentSystemPrompt(agentUC, c, entry, nil, nil, nil)

		gt.Value(t, strings.Contains(prompt, "## Actions")).Equal(false)
		gt.Value(t, strings.Contains(prompt, "## Knowledge")).Equal(false)
	})
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

func TestAgentUseCase_HandleSessionInfoRequest(t *testing.T) {
	t.Run("opens modal with session ID", func(t *testing.T) {
		repo := memory.New()
		slackMock := &agentTestSlackService{}
		mockWithCapture := &agentTestSlackServiceWithOpenView{
			agentTestSlackService: slackMock,
		}

		llmClient := &mockLLMClient{}
		agentUC := usecase.NewAgentUseCase(repo, nil, mockWithCapture, llmClient)

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
