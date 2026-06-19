package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	agentcommon "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/casebound"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
	goslack "github.com/slack-go/slack" //nolint:depguard
)

// AgentUseCase is the Slack-side orchestrator for case-bound mention turns.
// It resolves the request (bot user id, case lookup, conversation
// snapshot, etc.) and hands off to the casebound runtime
// (pkg/usecase/agent/casebound) which owns gollem invocation, system
// prompt assembly, and the turn lock lifecycle.
type AgentUseCase struct {
	deps AgentDeps

	// casebound runs the case-bound gollem ReAct loop. It is non-nil
	// whenever the LLM client is configured.
	casebound *casebound.UseCase

	// threadcase runs the thread-mode plan-and-execute agent (materialize on
	// creation, investigate / respond / close on mention). Non-nil whenever
	// the LLM client is configured.
	threadcase *threadcase.UseCase
}

// AgentDeps groups the dependencies AgentUseCase needs. Required fields are
// marked below; optional ones can be left zero to disable the corresponding
// tool or behaviour.
//
// SlackRetriever, when supplied, switches slack__get_messages to a User-token-
// backed read path so public channels can be fetched without bot membership.
type AgentDeps struct {
	Repo     interfaces.Repository    // required
	Registry *model.WorkspaceRegistry // required
	LLM      gollem.LLMClient         // required

	// HistoryRepo and TraceRepo are required: the agent session flow persists
	// gollem.History across mentions and writes a trace for each Execute. Pass
	// agentarchive.NewMemoryHistoryRepository / NewMemoryTraceRepository in tests.
	HistoryRepo gollem.HistoryRepository
	TraceRepo   trace.Repository

	// ActionUC is required: the core__create_action tool routes through it so
	// all Action create paths share the same usecase implementation.
	// ActionStepUC follows the same contract for the core__*_action_step
	// tool family.
	ActionUC     *ActionUseCase
	ActionStepUC *ActionStepUseCase

	// MemoUC backs the Case-scoped memo tools (memo__*) in case-bound mode.
	// Optional: nil means the agent gets no memo tools (e.g. the workspace has
	// not enabled memos, or memos are intentionally withheld).
	MemoUC *MemoUseCase

	// KnowledgeUC backs the workspace-wide knowledge tools. Optional: nil means
	// the agent gets no knowledge tools.
	KnowledgeUC *KnowledgeUseCase

	// CaseUC is required for thread mode: the thread-case orchestrator applies
	// the agent's materialize / close decisions through it so every case
	// mutation funnels through the single CaseUseCase entry point.
	CaseUC *CaseUseCase

	// ThreadcaseBudget overrides the planexec budget for the thread-mode
	// agent. Zero values fall back to DefaultThreadcaseBudget.
	ThreadcaseBudget planexec.BudgetConfig

	// Optional Slack tool clients. SlackService is the Bot-token client;
	// SlackSearch and SlackRetriever sit on the User OAuth Token.
	SlackService   slack.Service
	SlackSearch    slacktool.SearchService
	SlackRetriever slacktool.MessageRetriever

	// Optional integrations.
	NotionTool     notiontool.Client
	GitHubClient   *githubtool.Client
	WebFetchClient *webfetch.Client
	EmbedClient    interfaces.EmbedClient
}

// NewAgentUseCase creates a new AgentUseCase from a deps bundle. See AgentDeps.
func NewAgentUseCase(deps AgentDeps) *AgentUseCase {
	uc := &AgentUseCase{deps: deps}
	if deps.LLM != nil {
		commonDeps := &agentcommon.CommonDeps{
			Repo:                deps.Repo,
			Registry:            deps.Registry,
			LLMClient:           deps.LLM,
			HistoryRepo:         deps.HistoryRepo,
			TraceRepo:           deps.TraceRepo,
			SlackBot:            deps.SlackService,
			SlackSearch:         deps.SlackSearch,
			SlackRetriever:      deps.SlackRetriever,
			NotionClient:        deps.NotionTool,
			GitHubClient:        deps.GitHubClient,
			WebFetchClient:      deps.WebFetchClient,
			ActionUC:            NewActionToolAdapter(deps.ActionUC),
			ActionStepUC:        NewActionStepToolAdapter(deps.ActionStepUC),
			CaseUC:              NewCaseToolAdapter(deps.CaseUC),
			MemoUC:              NewMemoToolAdapter(deps.MemoUC),
			KnowledgeAccessor:   NewKnowledgeToolAccessor(deps.KnowledgeUC),
			KnowledgeMutator:    NewKnowledgeToolMutator(deps.KnowledgeUC),
			HeartbeatInterval:   agentcommon.DefaultHeartbeatInterval,
			HeartbeatStaleAfter: agentcommon.DefaultHeartbeatStaleAfter,
		}
		cb, err := casebound.New(commonDeps)
		if err != nil {
			// casebound.New only fails on missing deps that we already
			// guarded above, so reaching here means a wiring bug. Surface
			// to Sentry and leave casebound nil; HandleAgentMention will
			// short-circuit.
			errutil.Handle(context.Background(), goerr.Wrap(err, "failed to build casebound usecase"), "failed to build casebound usecase")
		} else {
			uc.casebound = cb
		}

		// Build the thread-mode agent. It reuses the same backend deps and a
		// dedicated planexec runner.
		budget := deps.ThreadcaseBudget
		if budget.PlannerLoopMax <= 0 || budget.SubAgentLoopMax <= 0 {
			budget = DefaultThreadcaseBudget
		}
		runner, runnerErr := planexec.NewRunner(planexec.RunnerDeps{
			LLMClient:   deps.LLM,
			HistoryRepo: deps.HistoryRepo,
			TraceRepo:   deps.TraceRepo,
			Budget:      budget,
		})
		if runnerErr != nil {
			errutil.Handle(context.Background(), goerr.Wrap(runnerErr, "failed to build threadcase planexec runner"), "failed to build threadcase planexec runner")
		} else if tc, tcErr := threadcase.New(commonDeps, runner); tcErr != nil {
			errutil.Handle(context.Background(), goerr.Wrap(tcErr, "failed to build threadcase usecase"), "failed to build threadcase usecase")
		} else {
			uc.threadcase = tc
		}
	}
	return uc
}

// DefaultThreadcaseBudget is the planexec budget used for thread-mode agent
// turns when AgentDeps.ThreadcaseBudget is unset. Conservative bounds keep a
// mention turn responsive while allowing a couple of investigation rounds.
var DefaultThreadcaseBudget = planexec.BudgetConfig{
	PlannerLoopMax:  8,
	SubAgentLoopMax: 20,
}

// HandleAgentMention processes an app_mention event and responds with an AI agent
func (uc *AgentUseCase) HandleAgentMention(ctx context.Context, msg *slackmodel.Message) error {
	logger := logging.From(ctx)
	if uc.casebound == nil {
		logger.Debug("casebound usecase not configured; skipping agent mention")
		return nil
	}

	// Detect user's language from Slack locale
	ctx = contextWithSlackUserLang(ctx, uc.deps.SlackService, msg.UserID())

	// Skip if bot user ID matches the message sender (prevent infinite loop)
	botUserID, err := uc.deps.SlackService.GetBotUserID(ctx)
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

	// Look up (or create) the Session that ties this thread to the Case.
	session, err := uc.loadOrCreateSession(ctx, entry.Workspace.ID, foundCase.ID, msg.ChannelID(), threadTS)
	if err != nil {
		return goerr.Wrap(err, "failed to load or create agent session")
	}

	// Post the per-mention session start banner using the Session.ID so
	// the overflow menu surfaces the persistent identifier.
	if err := uc.postSessionStart(ctx, msg.ChannelID(), threadTS, session.ID); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to post session start",
			goerr.V("session_id", session.ID),
			goerr.V("channel_id", msg.ChannelID()),
			goerr.V("thread_ts", threadTS),
		), "failed to post session start")
	}

	// Fetch case context (actions) every turn — these may have been mutated
	// since the previous mention by direct GraphQL/UI edits. Archived
	// actions are excluded so the agent's working set matches what the
	// channel sees.
	actions, err := uc.deps.Repo.Action().GetByCase(ctx, entry.Workspace.ID, foundCase.ID, interfaces.ActionListOptions{})
	if err != nil {
		return goerr.Wrap(err, "failed to get actions for case")
	}

	// Build delta vs. system snapshot of the conversation. The casebound
	// runtime takes pre-fetched messages (Slack-independent shape).
	systemMessages, deltaMessages, err := uc.partitionConversation(ctx, msg, session, botUserID)
	if err != nil {
		return goerr.Wrap(err, "failed to partition conversation")
	}

	// When this thread is bound to a specific Action, surface that action's
	// detail instead of the case-wide action list.
	var currentAction *model.Action
	if session.ActionID != 0 {
		for _, a := range actions {
			if a.ID == session.ActionID {
				currentAction = a
				break
			}
		}
	}

	// Slack-side trace banner (per-mention; not persisted).
	traceMsg := uc.newTraceMessage(msg.ChannelID(), threadTS)

	req := casebound.TurnRequest{
		Session:        session,
		ChannelID:      msg.ChannelID(),
		ThreadTS:       threadTS,
		MentionTS:      msg.ID(),
		MentionText:    msg.Text(),
		BotUserID:      botUserID,
		Workspace:      entry,
		Case:           foundCase,
		Actions:        actions,
		CurrentAction:  currentAction,
		SystemMessages: toCaseboundMessages(systemMessages),
		DeltaMessages:  toCaseboundMessages(deltaMessages),
		TriggerTS:      msg.ID(),
		Handler: casebound.HandlerFuncs{
			TraceAppendFn:  traceMsg.appendLine,
			TraceReplaceFn: traceMsg.replaceLine,
		},
	}

	result, runErr := uc.casebound.RunTurn(ctx, req)
	if runErr != nil {
		errMsg := "⚠️ " + i18n.T(ctx, i18n.MsgAgentError)
		if _, postErr := uc.deps.SlackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, errMsg); postErr != nil {
			errutil.Handle(ctx, postErr, "post agent error reply")
		}
		return goerr.Wrap(runErr, "casebound run turn")
	}
	switch result.Status {
	case casebound.StatusBusy:
		busyMsg := i18n.T(ctx, i18n.MsgKeyAgentBusy)
		if _, postErr := uc.deps.SlackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, busyMsg); postErr != nil {
			errutil.Handle(ctx, postErr, "post busy notice")
		}
		return nil
	case casebound.StatusIdempotent:
		return nil
	case casebound.StatusCompleted:
		if err := traceMsg.finalize(ctx, result.FinalText); err != nil {
			return goerr.Wrap(err, "failed to post final response")
		}
		return nil
	default:
		return goerr.New("unexpected casebound status", goerr.V("status", int(result.Status)))
	}
}

// toCaseboundMessages converts the Slack-service ConversationMessage shape
// into the Slack-independent shape consumed by the casebound runtime.
func toCaseboundMessages(in []slack.ConversationMessage) []casebound.ConversationMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]casebound.ConversationMessage, len(in))
	for i, m := range in {
		out[i] = casebound.ConversationMessage{
			UserID:    m.UserID,
			UserName:  m.UserName,
			Text:      m.Text,
			Timestamp: m.Timestamp,
		}
	}
	return out
}

// loadOrCreateSession returns the Session for the given thread, creating
// (but not yet persisting) a fresh one when none exists. Persistence happens
// at the end of HandleAgentMention so we only commit a session that
// successfully started a turn.
func (uc *AgentUseCase) loadOrCreateSession(ctx context.Context, workspaceID string, caseID int64, channelID, threadTS string) (*model.Session, error) {
	existing, err := uc.deps.Repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get session")
	}
	if existing != nil {
		return existing, nil
	}

	// New session: detect Action linkage by matching the thread parent TS
	// against any registered action notification message. Most threads
	// have no associated action — tag ErrNotFound as benign so the lookup
	// is visible at Info level without paging Sentry, while real backend
	// failures still alert as ERROR.
	var actionID int64
	if action, err := uc.deps.Repo.Action().GetBySlackMessageTS(ctx, workspaceID, threadTS); err == nil && action != nil {
		actionID = action.ID
	} else if err != nil {
		if isRepoNotFound(err) {
			err = goerr.Wrap(err, "no action linked to thread", goerr.T(errutil.TagBenign))
		}
		errutil.Handle(ctx, err, "failed to look up action by thread TS for new session")
	}

	now := time.Now().UTC()
	return &model.Session{
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
func (uc *AgentUseCase) partitionConversation(ctx context.Context, msg *slackmodel.Message, session *model.Session, botUserID string) ([]slack.ConversationMessage, []slack.ConversationMessage, error) {
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
	replies, err := uc.deps.SlackService.GetConversationReplies(ctx, msg.ChannelID(), session.ThreadTS, 1000)
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
	_, err := uc.deps.SlackService.PostThreadMessage(ctx, channelID, threadTS, blocks, label)
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
	if err := uc.deps.SlackService.OpenView(ctx, triggerID, view); err != nil {
		return goerr.Wrap(err, "failed to open session info modal")
	}
	return nil
}

// findCaseByChannel searches for a case associated with the given channel ID across all workspaces
func (uc *AgentUseCase) findCaseByChannel(ctx context.Context, channelID string) (*model.Case, *model.WorkspaceEntry, error) {
	if uc.deps.Registry == nil {
		return nil, nil, nil
	}

	for _, entry := range uc.deps.Registry.List() {
		c, err := uc.deps.Repo.Case().GetBySlackChannelID(ctx, entry.Workspace.ID, channelID)
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
		return uc.deps.SlackService.GetConversationReplies(ctx, msg.ChannelID(), msg.ThreadTS(), 100)
	}

	// Channel mention: get recent messages (last 24 hours)
	oldest := time.Now().Add(-24 * time.Hour)
	return uc.deps.SlackService.GetConversationHistory(ctx, msg.ChannelID(), oldest, 100)
}

// traceMessage manages a single updatable Slack message for showing agent
// progress using context blocks. It distinguishes two kinds of progress:
//
//   - lines: the persistent milestone history (planner rounds, task results,
//     errors). Appended via appendLine; these accumulate and stay visible.
//   - liveLine: a single transient activity line (the tool the agent is
//     running right now). Overwritten via replaceLine so per-tool chatter
//     ("Searching…", "Fetching…") never piles up in the thread.
//
// The live line is always rendered last, after the milestone history.
type traceMessage struct {
	slackService slack.Service
	channelID    string
	threadTS     string
	messageTS    string
	lines        []string
	liveLine     string
	mu           sync.Mutex
}

// newTraceMessage creates a new traceMessage for posting agent progress updates
func (uc *AgentUseCase) newTraceMessage(channelID, threadTS string) *traceMessage {
	return &traceMessage{
		slackService: uc.deps.SlackService,
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

// buildContextBlocks renders the milestone history followed by the transient
// live line. When a live line is present, one block slot is reserved for it so
// a long milestone history never pushes the in-place line out of the message.
func (tm *traceMessage) buildContextBlocks() []goslack.Block {
	if tm.liveLine == "" {
		return buildTraceContextBlocks(tm.lines)
	}
	lines := tm.lines
	if len(lines) > maxTraceBlocks-1 {
		lines = lines[len(lines)-(maxTraceBlocks-1):]
	}
	blocks := buildTraceContextBlocks(lines)
	return append(blocks, goslack.NewContextBlock("",
		goslack.NewTextBlockObject(goslack.MarkdownType, tm.liveLine, false, false),
	))
}

// fallbackText renders the plain-text notification fallback. It mirrors the
// same window buildContextBlocks renders (most recent maxTraceBlocks lines,
// live line last) so the fallback stays consistent with the visible blocks and
// never exceeds Slack's 4000-char text-field limit, which an unbounded
// milestone history would otherwise blow past with a msg_too_long error.
func (tm *traceMessage) fallbackText() string {
	lines := tm.lines
	if tm.liveLine == "" {
		if len(lines) > maxTraceBlocks {
			lines = lines[len(lines)-maxTraceBlocks:]
		}
		return strings.Join(lines, "\n")
	}
	if len(lines) > maxTraceBlocks-1 {
		lines = lines[len(lines)-(maxTraceBlocks-1):]
	}
	all := make([]string, 0, len(lines)+1)
	all = append(all, lines...)
	all = append(all, tm.liveLine)
	return strings.Join(all, "\n")
}

// appendLine appends a milestone to the persistent history and clears the
// transient live line, then re-renders the Slack message. Use this for
// progress that must remain visible (planner milestones, task results, errors).
func (tm *traceMessage) appendLine(ctx context.Context, line string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.lines = append(tm.lines, line)
	tm.liveLine = ""
	tm.flush(ctx)
}

// replaceLine overwrites the single transient live line in place, without
// growing the milestone history, then re-renders the Slack message. Use this
// for ephemeral per-tool activity ("Searching…", "Fetching…") that should not
// accumulate. An empty line clears the live line.
func (tm *traceMessage) replaceLine(ctx context.Context, line string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.liveLine = line
	tm.flush(ctx)
}

// flush renders the current trace state and posts (first call) or updates
// (subsequent calls) the Slack message. Callers MUST hold tm.mu.
func (tm *traceMessage) flush(ctx context.Context) {
	blocks := tm.buildContextBlocks()
	fallback := tm.fallbackText()

	if tm.messageTS == "" {
		ts, err := tm.slackService.PostThreadMessage(ctx, tm.channelID, tm.threadTS, blocks, fallback)
		if err != nil {
			errutil.Handle(ctx, goerr.Wrap(err, "failed to post trace message",
				goerr.V("channel_id", tm.channelID),
				goerr.V("thread_ts", tm.threadTS),
			), "failed to post trace message")
			return
		}
		tm.messageTS = ts
		return
	}
	if err := tm.slackService.UpdateMessage(ctx, tm.channelID, tm.messageTS, blocks, fallback); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "failed to update trace message",
			goerr.V("channel_id", tm.channelID),
			goerr.V("message_ts", tm.messageTS),
		), "failed to update trace message")
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
