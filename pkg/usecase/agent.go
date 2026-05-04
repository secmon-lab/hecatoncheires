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
	"github.com/m-mizutani/gollem/trace"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack" //nolint:depguard
)

//go:embed prompts/agent_system.md
var agentSystemPromptTmpl string

var agentSystemPrompt = template.Must(template.New("agent_system").Parse(agentSystemPromptTmpl))

// AgentUseCase handles AI agent responses for Slack mentions
type AgentUseCase struct {
	repo         interfaces.Repository
	registry     *model.WorkspaceRegistry
	slackService slack.Service
	slackSearch  slacktool.SearchService
	notionTool   notiontool.Client
	llmClient    gollem.LLMClient
	historyRepo  gollem.HistoryRepository
	traceRepo    trace.Repository
}

// NewAgentUseCase creates a new AgentUseCase instance.
//
// slackSearch and notionTool are optional; pass nil to omit the corresponding
// agent tools (slack__search_messages, notion__search, notion__get_page).
//
// historyRepo and traceRepo are required: the agent session flow persists
// gollem.History across mentions and writes a trace for each Execute. Pass
// agentarchive.NewMemoryHistoryRepository / NewMemoryTraceRepository in tests.
func NewAgentUseCase(repo interfaces.Repository, registry *model.WorkspaceRegistry, slackService slack.Service, slackSearch slacktool.SearchService, notionTool notiontool.Client, llmClient gollem.LLMClient, historyRepo gollem.HistoryRepository, traceRepo trace.Repository) *AgentUseCase {
	return &AgentUseCase{
		repo:         repo,
		registry:     registry,
		slackService: slackService,
		slackSearch:  slackSearch,
		notionTool:   notionTool,
		llmClient:    llmClient,
		historyRepo:  historyRepo,
		traceRepo:    traceRepo,
	}
}

// HandleAgentMention processes an app_mention event and responds with an AI agent
func (uc *AgentUseCase) HandleAgentMention(ctx context.Context, msg *slackmodel.Message) error {
	logger := logging.From(ctx)

	// Detect user's language from Slack locale
	ctx = contextWithSlackUserLang(ctx, uc.slackService, msg.UserID())

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

	// Determine thread parent TS. Slack stores thread replies under their
	// parent's `ts`; for a top-level mention we treat the mention itself as
	// the parent so subsequent replies hang off it.
	threadTS := msg.ThreadTS()
	if threadTS == "" {
		threadTS = msg.ID()
	}

	// Look up (or create) the AgentSession that ties this thread to the Case.
	session, err := uc.loadOrCreateSession(ctx, entry.Workspace.ID, foundCase.ID, msg.ChannelID(), threadTS)
	if err != nil {
		return goerr.Wrap(err, "failed to load or create agent session")
	}

	// Post the per-mention session start banner using the AgentSession.ID so
	// the overflow menu surfaces the persistent identifier.
	if err := uc.postSessionStart(ctx, msg.ChannelID(), threadTS, session.ID); err != nil {
		logger.Error("failed to post session start", "error", err.Error())
	}

	// Fetch case context (actions, knowledge) every turn — these may have
	// been mutated since the previous mention by direct GraphQL/UI edits.
	actions, err := uc.repo.Action().GetByCase(ctx, entry.Workspace.ID, foundCase.ID)
	if err != nil {
		return goerr.Wrap(err, "failed to get actions for case")
	}
	knowledges, err := uc.repo.Knowledge().ListByCaseID(ctx, entry.Workspace.ID, foundCase.ID)
	if err != nil {
		return goerr.Wrap(err, "failed to get knowledge for case")
	}

	// Build prompt + user input. For a fresh session we drop the full thread
	// into the system prompt's Conversation Context section. For continuing
	// sessions the gollem History already holds prior turns; we only need to
	// surface unprocessed messages (everything in the thread newer than the
	// previous mention TS, excluding the bot's own posts) as user input.
	systemMessages, deltaMessages, err := uc.partitionConversation(ctx, msg, session, botUserID)
	if err != nil {
		return goerr.Wrap(err, "failed to partition conversation")
	}
	systemPrompt := uc.buildSystemPrompt(foundCase, entry, actions, knowledges, systemMessages)
	userInput := buildAgentUserInput(deltaMessages, msg)

	// Slack-side trace banner (per-mention; not persisted).
	traceMsg := uc.newTraceMessage(msg.ChannelID(), threadTS)
	ctx = tool.WithUpdate(ctx, func(innerCtx context.Context, message string) {
		traceMsg.update(innerCtx, message)
	})

	// Configure the gollem trace recorder for the durable trace artifact.
	actionIDStr := ""
	if session.ActionID != 0 {
		actionIDStr = fmt.Sprintf("%d", session.ActionID)
	}
	recorder := trace.New(
		trace.WithRepository(uc.traceRepo),
		trace.WithTraceID(msg.ID()),
		trace.WithMetadata(trace.TraceMetadata{
			Labels: map[string]string{
				agentSessionLabel:        session.ID,
				agentWorkspaceIDLabel:    entry.Workspace.ID,
				agentCaseIDLabel:         fmt.Sprintf("%d", foundCase.ID),
				agentThreadTSLabel:       threadTS,
				agentActionIDLabel:       actionIDStr,
				agentTriggerMentionLabel: msg.ID(),
			},
		}),
	)

	// Build core tools (action / knowledge) for this case.
	coreTools := core.New(core.Deps{
		Repo:        uc.repo,
		WorkspaceID: entry.Workspace.ID,
		CaseID:      foundCase.ID,
		StatusSet:   entry.ActionStatusSet,
		LLMClient:   uc.llmClient,
	})

	// Slack and Notion tools are independent packages. Each gates its own tools
	// on whether the relevant client/service is configured (nil → no tools).
	// Mention flow uses the read-only Slack tool set (no post_message — the
	// trace UI handles outbound messages).
	slackTools := slacktool.NewReadOnly(slacktool.Deps{
		Bot:    uc.slackService,
		Search: uc.slackSearch,
	})
	notionTools := notiontool.New(notiontool.Deps{Client: uc.notionTool})

	allTools := make([]gollem.Tool, 0, len(coreTools)+len(slackTools)+len(notionTools))
	allTools = append(allTools, coreTools...)
	allTools = append(allTools, slackTools...)
	allTools = append(allTools, notionTools...)

	// Note: gollem's WithHistoryRepository follows a load-mutate-overwrite
	// pattern (Load at session start, Save after every LLM turn). Two
	// concurrent mentions on the same thread would therefore race
	// last-writer-wins on the persisted History. We accept that trade-off
	// because Slack mentions on a single thread are effectively serial
	// (humans typing) and adding GCS generation preconditions here would
	// require deeper changes inside gollem itself.
	agent := gollem.New(uc.llmClient,
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithTools(allTools...),
		gollem.WithHistoryRepository(uc.historyRepo, session.ID),
		gollem.WithTrace(recorder),
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

	resp, execErr := agent.Execute(ctx, gollem.Text(userInput))

	// Persist the trace regardless of Execute outcome — partial traces are
	// the most useful diagnostic for failures.
	if finishErr := recorder.Finish(ctx); finishErr != nil {
		errutil.Handle(ctx, finishErr, "failed to persist agent trace")
	}

	if execErr != nil {
		errMsg := "⚠️ " + i18n.T(ctx, i18n.MsgAgentError)
		if _, postErr := uc.slackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, errMsg); postErr != nil {
			logger.Error("failed to post error message to Slack", "error", postErr.Error())
		}
		return goerr.Wrap(execErr, "failed to execute agent")
	}

	// Update the session record with the just-processed mention TS so the
	// next mention only ingests messages strictly after this one.
	session.LastMentionTS = msg.ID()
	session.UpdatedAt = time.Now().UTC()
	if err := uc.repo.AgentSession().Put(ctx, session); err != nil {
		errutil.Handle(ctx, err, "failed to update agent session lastMentionTS")
	}

	finalText := strings.Join(resp.Texts, "\n")
	if err := traceMsg.finalize(ctx, finalText); err != nil {
		return goerr.Wrap(err, "failed to post final response")
	}

	return nil
}

// Trace metadata labels keyed off the SessionIDLabel exported by agentarchive.
const (
	agentSessionLabel        = "session_id"
	agentWorkspaceIDLabel    = "workspace_id"
	agentCaseIDLabel         = "case_id"
	agentThreadTSLabel       = "thread_ts"
	agentActionIDLabel       = "action_id"
	agentTriggerMentionLabel = "trigger_mention_ts"
)

// loadOrCreateSession returns the AgentSession for the given thread, creating
// (but not yet persisting) a fresh one when none exists. Persistence happens
// at the end of HandleAgentMention so we only commit a session that
// successfully started a turn.
func (uc *AgentUseCase) loadOrCreateSession(ctx context.Context, workspaceID string, caseID int64, channelID, threadTS string) (*model.AgentSession, error) {
	existing, err := uc.repo.AgentSession().Get(ctx, workspaceID, caseID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get agent session")
	}
	if existing != nil {
		return existing, nil
	}

	// New session: detect Action linkage by matching the thread parent TS
	// against any registered action notification message.
	var actionID int64
	if action, err := uc.repo.Action().GetBySlackMessageTS(ctx, workspaceID, threadTS); err == nil && action != nil {
		actionID = action.ID
	} else if err != nil {
		errutil.Handle(ctx, err, "failed to look up action by thread TS for new agent session")
	}

	now := time.Now().UTC()
	return &model.AgentSession{
		ID:          uuid.Must(uuid.NewV7()).String(),
		WorkspaceID: workspaceID,
		CaseID:      caseID,
		ThreadTS:    threadTS,
		ChannelID:   channelID,
		ActionID:    actionID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// partitionConversation splits the messages around this mention into the two
// buckets the agent needs:
//
//   - systemMessages — the conversation snapshot to inline into the system
//     prompt (only on a fresh session, where the gollem history is empty).
//   - deltaMessages — unprocessed thread messages newer than the previous
//     mention TS, excluding the bot's own posts. These are folded into the
//     user input alongside the current mention text on continuing sessions.
//
// The current mention itself is intentionally not included in either bucket;
// buildAgentUserInput appends it last.
func (uc *AgentUseCase) partitionConversation(ctx context.Context, msg *slackmodel.Message, session *model.AgentSession, botUserID string) ([]slack.ConversationMessage, []slack.ConversationMessage, error) {
	if session.LastMentionTS == "" {
		// Fresh session: existing behavior — inline thread/channel context
		// into the system prompt.
		ctxMsgs, err := uc.collectContextMessages(ctx, msg)
		if err != nil {
			return nil, nil, err
		}
		return ctxMsgs, nil, nil
	}

	// Continuing session: fetch all replies on the thread and surface the
	// ones we haven't seen yet (excluding our own posts and the current
	// mention message itself). The limit is set to Slack's per-call maximum
	// (1000) so a long quiet stretch between mentions doesn't silently drop
	// "unprocessed" messages — pagination would only matter beyond that.
	replies, err := uc.slackService.GetConversationReplies(ctx, msg.ChannelID(), session.ThreadTS, 1000)
	if err != nil {
		return nil, nil, goerr.Wrap(err, "failed to fetch thread replies")
	}
	delta := make([]slack.ConversationMessage, 0, len(replies))
	for _, m := range replies {
		if m.UserID == botUserID {
			continue
		}
		if m.Timestamp == msg.ID() {
			continue // current mention is appended explicitly later
		}
		if compareSlackTS(m.Timestamp, session.LastMentionTS) <= 0 {
			continue
		}
		delta = append(delta, m)
	}
	return nil, delta, nil
}

// buildAgentUserInput assembles the user-facing text passed to gollem.
// Unprocessed thread messages are prepended in chronological order with a
// header so the agent can distinguish them from the new prompt. The current
// mention text is always appended last.
func buildAgentUserInput(delta []slack.ConversationMessage, msg *slackmodel.Message) string {
	if len(delta) == 0 {
		return msg.Text()
	}
	var b strings.Builder
	b.WriteString("# Unprocessed thread messages since last mention\n")
	for _, m := range delta {
		name := m.UserName
		if name == "" {
			name = m.UserID
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.Timestamp, name, m.Text)
	}
	b.WriteString("\n# Current mention\n")
	b.WriteString(msg.Text())
	return b.String()
}

// compareSlackTS compares two Slack timestamps lexicographically. Slack TS
// values are fixed-width "<seconds>.<microseconds>" strings, so string
// ordering matches chronological ordering.
func compareSlackTS(a, b string) int {
	switch {
	case a == b:
		return 0
	case a < b:
		return -1
	default:
		return 1
	}
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

var sessionStartMessageKeys = []i18n.MsgKey{
	i18n.MsgAgentThinking,
	i18n.MsgAgentAnalyzing,
	i18n.MsgAgentProcessing,
	i18n.MsgAgentInvestigating,
	i18n.MsgAgentLookingInto,
	i18n.MsgAgentOnIt,
}

// ParseAgentActionValue parses an agent action option value into action type and data.
// Format: "{action}:{data}" (e.g., "show_session_info:uuid-value")
func ParseAgentActionValue(value string) (action string, data string, err error) {
	before, after, found := strings.Cut(value, ":")
	if !found {
		return value, "", nil
	}
	return before, after, nil
}

// postSessionStart posts a section block message with an overflow menu for agent session actions
func (uc *AgentUseCase) postSessionStart(ctx context.Context, channelID, threadTS, sessionID string) error {
	//nolint:gosec // not for security use
	key := sessionStartMessageKeys[time.Now().UnixNano()%int64(len(sessionStartMessageKeys))]
	label := i18n.T(ctx, key)

	overflow := goslack.NewOverflowBlockElement(
		SlackAgentSessionActionsID,
		goslack.NewOptionBlockObject(
			fmt.Sprintf("%s:%s", SlackAgentActionShowSessionInfo, sessionID),
			goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgAgentSessionInfo), false, false),
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
		Title: goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgAgentSessionInfo), false, false),
		Close: goslack.NewTextBlockObject(goslack.PlainTextType, i18n.T(ctx, i18n.MsgModalCreateCaseCancel), false, false),
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

// promptAction represents an action for template rendering
type promptAction struct {
	ID          int64
	Title       string
	Status      string
	StatusEmoji string
	Assignees   string
}

// promptKnowledge represents a knowledge entry for template rendering
type promptKnowledge struct {
	ID    string
	Title string
}

// agentPromptData holds all data for the agent system prompt template
type agentPromptData struct {
	Case       *model.Case
	Fields     []promptField
	Actions    []promptAction
	Knowledges []promptKnowledge
	Messages   []promptMessage
}

func (uc *AgentUseCase) buildSystemPrompt(c *model.Case, entry *model.WorkspaceEntry, actions []*model.Action, knowledges []*model.Knowledge, messages []slack.ConversationMessage) string {
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

	// Build action list
	statusSet := model.DefaultActionStatusSet()
	if entry != nil && entry.ActionStatusSet != nil {
		statusSet = entry.ActionStatusSet
	}
	for _, a := range actions {
		data.Actions = append(data.Actions, promptAction{
			ID:          a.ID,
			Title:       a.Title,
			Status:      a.Status.String(),
			StatusEmoji: statusSet.Emoji(string(a.Status)),
			Assignees:   a.AssigneeID,
		})
	}

	// Build knowledge list
	for _, k := range knowledges {
		data.Knowledges = append(data.Knowledges, promptKnowledge{
			ID:    string(k.ID),
			Title: k.Title,
		})
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

// maxTraceBlocks caps the number of context blocks emitted per trace message.
// Slack rejects messages with more than 50 blocks (`invalid_blocks`), so when a
// long-running agent produces more lines we keep only the most recent ones.
const maxTraceBlocks = 50

// buildTraceContextBlocks renders one context block per trace line so progress
// reads as a vertical list instead of a single ever-growing one-liner. When the
// line count exceeds Slack's 50-block message limit, only the most recent lines
// are rendered.
func buildTraceContextBlocks(lines []string) []goslack.Block {
	if len(lines) > maxTraceBlocks {
		lines = lines[len(lines)-maxTraceBlocks:]
	}
	blocks := make([]goslack.Block, 0, len(lines))
	for _, line := range lines {
		blocks = append(blocks, goslack.NewContextBlock("",
			goslack.NewTextBlockObject(goslack.MarkdownType, line, false, false),
		))
	}
	return blocks
}

func (tm *traceMessage) buildContextBlocks() []goslack.Block {
	return buildTraceContextBlocks(tm.lines)
}

// update adds a line to the trace message and posts/updates in Slack as a context block
func (tm *traceMessage) update(ctx context.Context, line string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	logger := logging.From(ctx)
	tm.lines = append(tm.lines, line)
	blocks := tm.buildContextBlocks()
	fallback := strings.Join(tm.lines, "\n")

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

// finalize posts the final response as a new thread reply,
// leaving the trace context block intact in Slack
func (tm *traceMessage) finalize(ctx context.Context, text string) error {
	if _, err := tm.slackService.PostThreadReply(ctx, tm.channelID, tm.threadTS, text); err != nil {
		return goerr.Wrap(err, "failed to post final response")
	}
	return nil
}
