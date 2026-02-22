package usecase

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack" //nolint:depguard
)

//go:embed prompt/agent_system.md
var agentSystemPromptTmpl string

var agentSystemPrompt = template.Must(template.New("agent_system").Parse(agentSystemPromptTmpl))

// AgentUseCase handles AI agent responses for Slack mentions
type AgentUseCase struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackService slack.Service
	llmClient    gollem.LLMClient
}

// NewAgentUseCase creates a new AgentUseCase instance
func NewAgentUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, slackService slack.Service, llmClient gollem.LLMClient) *AgentUseCase {
	return &AgentUseCase{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		llmClient:    llmClient,
	}
}

// HandleAgentMention processes an app_mention event and responds with an AI agent
func (uc *AgentUseCase) HandleAgentMention(ctx context.Context, msg *slackmodel.Message) error {
	logger := logging.From(ctx)

	// Skip if bot user ID matches the message sender (prevent infinite loop)
	botUserID, err := uc.slackService.GetBotUserID(ctx)
	if err != nil {
		return goerr.Wrap(err, "failed to get bot user ID")
	}
	if msg.UserID() == botUserID {
		logger.Debug("skipping bot's own message", "user_id", msg.UserID())
		return nil
	}

	// Find the case associated with this channel
	foundCase, entry, err := uc.findCaseByChannel(ctx, msg.ChannelID())
	if err != nil {
		return goerr.Wrap(err, "failed to find case by channel")
	}
	if foundCase == nil {
		logger.Debug("no case found for channel, skipping agent response", "channel_id", msg.ChannelID())
		return nil
	}

	// Determine thread TS for replies
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		// Not in a thread, use the message's own timestamp as the thread parent
		threadTS = msg.EventTS()
	}

	// Generate session ID and post session start message
	sessionID := uuid.Must(uuid.NewV7()).String()
	if err := uc.postSessionStart(ctx, msg.ChannelID(), threadTS, sessionID); err != nil {
		logger.Error("failed to post session start", "error", err.Error())
	}

	// Collect context messages
	contextMessages, err := uc.collectContextMessages(ctx, msg)
	if err != nil {
		return goerr.Wrap(err, "failed to collect context messages")
	}

	// Build system prompt
	systemPrompt := uc.buildSystemPrompt(foundCase, entry, contextMessages)

	// Create trace message for intermediate updates (tool executions only)
	traceMsg := uc.newTraceMessage(msg.ChannelID(), threadTS)

	// Create and execute the gollem Agent
	agent := gollem.New(uc.llmClient,
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithToolMiddleware(
			func(next gollem.ToolHandler) gollem.ToolHandler {
				return func(ctx context.Context, req *gollem.ToolExecRequest) (*gollem.ToolExecResponse, error) {
					traceMsg.update(ctx, fmt.Sprintf("🔧 `%s`", req.Tool.Name))
					resp, err := next(ctx, req)
					if resp != nil && resp.Error != nil {
						traceMsg.update(ctx, "❌ Error: "+resp.Error.Error())
					}
					return resp, err
				}
			},
		),
	)

	// Execute the agent with the user's message
	resp, err := agent.Execute(ctx, gollem.Text(msg.Text()))
	if err != nil {
		// Post error message to Slack thread
		errMsg := "⚠️ An error occurred while processing your request. Please try again later."
		if _, postErr := uc.slackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, errMsg); postErr != nil {
			logger.Error("failed to post error message to Slack", "error", postErr.Error())
		}
		return goerr.Wrap(err, "failed to execute agent")
	}

	// Post final response: update trace message if it exists, otherwise post new
	finalText := strings.Join(resp.Texts, "\n")
	if err := traceMsg.finalize(ctx, finalText); err != nil {
		return goerr.Wrap(err, "failed to post final response")
	}

	return nil
}

// Slack interaction constants for agent session actions
const (
	// SlackAgentSessionActionsID is the actionID for the agent session overflow menu.
	// Slack sends this in block_actions callbacks when any menu option is selected.
	SlackAgentSessionActionsID = "hc_agent_session_actions"

	// SlackAgentActionShowSessionInfo is the option value prefix for showing session info modal.
	// Full value format: "show_session_info:{sessionID}"
	SlackAgentActionShowSessionInfo = "show_session_info"
)

var sessionStartMessages = []string{
	"Thinking...",
	"Analyzing...",
	"Processing...",
	"Investigating...",
	"Looking into it...",
	"On it...",
}

// ParseAgentActionValue parses an agent action option value into action type and data.
// Format: "{action}:{data}" (e.g., "show_session_info:uuid-value")
func ParseAgentActionValue(value string) (action string, data string, err error) {
	idx := strings.Index(value, ":")
	if idx < 0 {
		return value, "", nil
	}
	return value[:idx], value[idx+1:], nil
}

// postSessionStart posts a context block message with an overflow menu for agent session actions
func (uc *AgentUseCase) postSessionStart(ctx context.Context, channelID, threadTS, sessionID string) error {
	//nolint:gosec // not for security use
	label := sessionStartMessages[time.Now().UnixNano()%int64(len(sessionStartMessages))]

	overflow := goslack.NewOverflowBlockElement(
		SlackAgentSessionActionsID,
		goslack.NewOptionBlockObject(
			fmt.Sprintf("%s:%s", SlackAgentActionShowSessionInfo, sessionID),
			goslack.NewTextBlockObject(goslack.PlainTextType, "Session Info", false, false),
			nil,
		),
	)

	blocks := []goslack.Block{
		goslack.NewSectionBlock(
			goslack.NewTextBlockObject(goslack.MarkdownType,
				fmt.Sprintf("🤖 %s", label), false, false),
			nil,
			goslack.NewAccessory(overflow),
		),
	}
	_, err := uc.slackService.PostThreadMessage(ctx, channelID, threadTS, blocks, label)
	if err != nil {
		return goerr.Wrap(err, "failed to post session start message")
	}
	return nil
}

// HandleSessionInfoRequest opens a modal displaying the session ID
func (uc *AgentUseCase) HandleSessionInfoRequest(ctx context.Context, triggerID, sessionID string) error {
	view := goslack.ModalViewRequest{
		Type:  goslack.VTModal,
		Title: goslack.NewTextBlockObject(goslack.PlainTextType, "Session Info", false, false),
		Close: goslack.NewTextBlockObject(goslack.PlainTextType, "Close", false, false),
		Blocks: goslack.Blocks{
			BlockSet: []goslack.Block{
				goslack.NewSectionBlock(
					goslack.NewTextBlockObject(goslack.MarkdownType,
						fmt.Sprintf("*Session ID*\n`%s`", sessionID), false, false),
					nil, nil,
				),
			},
		},
	}
	if err := uc.slackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open session info modal")
	}
	return nil
}

// findCaseByChannel searches for a case associated with the given channel ID across all workspaces
func (uc *AgentUseCase) findCaseByChannel(ctx context.Context, channelID string) (*model.Case, *model.WorkspaceEntry, error) {
	if uc.registry == nil {
		return nil, nil, nil
	}

	for _, entry := range uc.registry.List() {
		c, err := uc.repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, channelID)
		if err != nil {
			return nil, nil, goerr.Wrap(err, "failed to look up case by slack channel ID",
				goerr.V("channelID", channelID),
				goerr.V("workspaceID", entry.Workspace.ID),
			)
		}
		if c != nil {
			return c, entry, nil
		}
	}

	return nil, nil, nil
}

// collectContextMessages retrieves conversation context based on whether the mention is in a thread or channel
func (uc *AgentUseCase) collectContextMessages(ctx context.Context, msg *slackmodel.Message) ([]slack.ConversationMessage, error) {
	if msg.ThreadTS() != "" {
		// Thread mention: get thread replies
		return uc.slackService.GetConversationReplies(ctx, msg.ChannelID(), msg.ThreadTS(), 100)
	}

	// Channel mention: get recent messages (last 24 hours)
	oldest := time.Now().Add(-24 * time.Hour)
	return uc.slackService.GetConversationHistory(ctx, msg.ChannelID(), oldest, 100)
}

// buildSystemPrompt constructs the system prompt with case information and conversation context
// promptField represents a case field for template rendering
type promptField struct {
	Name  string
	Value any
}

// promptMessage represents a conversation message for template rendering
type promptMessage struct {
	Timestamp   string
	DisplayName string
	Text        string
}

// agentPromptData holds all data for the agent system prompt template
type agentPromptData struct {
	Case     *model.Case
	Fields   []promptField
	Messages []promptMessage
}

func (uc *AgentUseCase) buildSystemPrompt(c *model.Case, entry *model.WorkspaceEntry, messages []slack.ConversationMessage) string {
	data := agentPromptData{
		Case: c,
	}

	// Build field values with schema names
	if entry != nil && entry.FieldSchema != nil && len(c.FieldValues) > 0 {
		fieldNames := make(map[string]string)
		for _, fd := range entry.FieldSchema.Fields {
			fieldNames[fd.ID] = fd.Name
		}

		for fieldID, fv := range c.FieldValues {
			name := fieldNames[fieldID]
			if name == "" {
				name = fieldID
			}
			data.Fields = append(data.Fields, promptField{Name: name, Value: fv.Value})
		}
	}

	// Build conversation messages
	for _, msg := range messages {
		displayName := msg.UserName
		if displayName == "" {
			displayName = msg.UserID
		}
		data.Messages = append(data.Messages, promptMessage{
			Timestamp:   msg.Timestamp,
			DisplayName: displayName,
			Text:        msg.Text,
		})
	}

	var buf bytes.Buffer
	if err := agentSystemPrompt.Execute(&buf, data); err != nil {
		// Template execution should not fail with valid data; log and return fallback
		return fmt.Sprintf("You are an AI assistant. Case: %s", c.Title)
	}

	return buf.String()
}

// traceMessage manages a single updatable Slack message for showing agent progress using context blocks
type traceMessage struct {
	slackService slack.Service
	channelID    string
	threadTS     string
	messageTS    string
	lines        []string
	mu           sync.Mutex
}

// newTraceMessage creates a new traceMessage for posting agent progress updates
func (uc *AgentUseCase) newTraceMessage(channelID, threadTS string) *traceMessage {
	return &traceMessage{
		slackService: uc.slackService,
		channelID:    channelID,
		threadTS:     threadTS,
	}
}

// buildContextBlocks builds context blocks from the accumulated trace lines
func (tm *traceMessage) buildContextBlocks() []goslack.Block {
	text := strings.Join(tm.lines, " | ")
	return []goslack.Block{
		goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, text, false, false),
		),
	}
}

// update adds a line to the trace message and posts/updates in Slack as a context block
func (tm *traceMessage) update(ctx context.Context, line string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	logger := logging.From(ctx)
	tm.lines = append(tm.lines, line)
	blocks := tm.buildContextBlocks()
	fallback := strings.Join(tm.lines, " | ")

	if tm.messageTS == "" {
		ts, err := tm.slackService.PostThreadMessage(ctx, tm.channelID, tm.threadTS, blocks, fallback)
		if err != nil {
			logger.Error("failed to post trace message", "error", err.Error())
			return
		}
		tm.messageTS = ts
	} else {
		if err := tm.slackService.UpdateMessage(ctx, tm.channelID, tm.messageTS, blocks, fallback); err != nil {
			logger.Error("failed to update trace message", "error", err.Error())
		}
	}
}

// finalize replaces the trace message content with the final response,
// or posts a new message if no trace was ever posted
func (tm *traceMessage) finalize(ctx context.Context, text string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.messageTS != "" {
		// Update existing trace message with final response (plain text, not context block)
		if err := tm.slackService.UpdateMessage(ctx, tm.channelID, tm.messageTS, nil, text); err != nil {
			return goerr.Wrap(err, "failed to update trace message with final response")
		}
		return nil
	}

	// No trace message was posted, post new message
	if _, err := tm.slackService.PostThreadReply(ctx, tm.channelID, tm.threadTS, text); err != nil {
		return goerr.Wrap(err, "failed to post final response")
	}
	return nil
}
