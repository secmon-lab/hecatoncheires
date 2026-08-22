package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	slackmodel "github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/casebound"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
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

	// casebound runs the case-channel mention agent.
	casebound *casebound.UseCase

	// durableWorkspaceAgent runs the workspace-channel cross-case agent and
	// durableThreadcase the thread-mode create / mention agents. Both are filled by
	// RegisterAgents, which runs only when a Kernel is being built.
	durableWorkspaceAgent *wsagent.Durable
	durableThreadcase     *threadcase.Durable
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

	// TagUC backs the workspace-wide tag tools (and tag existence checks). It is
	// required alongside KnowledgeUC for any knowledge tools to be wired.
	TagUC *TagUseCase

	// CaseUC is required for thread mode: the thread-case orchestrator applies
	// the agent's materialize / close decisions through it so every case
	// mutation funnels through the single CaseUseCase entry point.
	CaseUC *CaseUseCase

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

	// JiraTools carries the already-expanded Jira read tools (see
	// pkg/agent/tool/jira / agent.CommonDeps.JiraTools for why this is a
	// plain tool slice rather than a client type). nil/empty means Jira is
	// not configured.
	JiraTools []gollem.Tool
}

// NewAgentUseCase creates a new AgentUseCase from a deps bundle. See AgentDeps.
//
// No agent is built here. Every one of them runs on the agentkit runtime, and
// wiring that needs the process store and the Kernel — both assembled after the
// usecases. RegisterAgents and BindAgentKernel do it in the order agentkit
// requires; until they run, the mention handlers stand down.
func NewAgentUseCase(deps AgentDeps) *AgentUseCase {
	return &AgentUseCase{deps: deps}
}

// RegisterAgents builds the agents that run on the agentkit runtime and
// registers them in reg. Call it before agentkernel.Build — agentkit requires
// every registration to complete before the first Spawn or Serve — and
// BindAgentKernel afterwards.
//
// It is a separate step from NewAgentUseCase because both halves of the wiring
// depend on the other: registering needs this usecase as the completion handler,
// and building the Kernel needs the filled registry.
// taskAgent is the shared per-task sub-agent (agentkernel.RegisterTaskAgent),
// registered by the caller so the Job runtime can be handed the same handle.
// models prices each finished run for its record. It is passed alongside the
// limiter rather than derived from it because the two answer different questions
// — what a run may spend, and what it did spend — and the limiter is an opaque
// function by the time it arrives here.
func (uc *AgentUseCase) RegisterAgents(
	reg *agentkit.Registry,
	limiter agentkit.Limiter,
	models agentkernel.ModelPolicy,
	store agentkit.HistoryStore,
	procRepo agentkit.Repository,
	taskAgent agentkit.Agent[react.Input],
) error {
	locator, err := agentkernel.NewLocator(procRepo)
	if err != nil {
		return goerr.Wrap(err, "build the agent process locator")
	}
	cb, err := casebound.New(uc.deps.Repo, caseboundHost{uc: uc}, locator, models)
	if err != nil {
		return goerr.Wrap(err, "build the case-channel agent")
	}
	if err := cb.Register(reg, limiter, store); err != nil {
		return goerr.Wrap(err, "register the case-channel agent")
	}
	uc.casebound = cb

	if taskAgent.Name() == "" {
		return goerr.New("the task sub-agent must be registered before the plan-execute agents")
	}

	progress := agentProgress{uc: uc}

	wa, err := wsagent.NewDurable(wsagentHost{uc: uc}, locator)
	if err != nil {
		return goerr.Wrap(err, "build the workspace agent")
	}
	if err := wa.Register(reg, taskAgent, progress, limiter, store); err != nil {
		return goerr.Wrap(err, "register the workspace agent")
	}
	uc.durableWorkspaceAgent = wa

	tc, err := threadcase.NewDurable(uc.deps.Repo, uc.deps.Registry, threadcaseHost{uc: uc}, locator, models)
	if err != nil {
		return goerr.Wrap(err, "build the thread-mode agents")
	}
	if err := tc.Register(reg, taskAgent, progress, limiter, store); err != nil {
		return goerr.Wrap(err, "register the thread-mode agents")
	}
	uc.durableThreadcase = tc
	return nil
}

// BindAgentKernel hands the built Kernel to every agent RegisterAgents
// registered. Until it is called, a mention turn cannot be spawned.
//
// probe travels with the Kernel because the two must be built from the same
// ToolDeps: it is what a plan-execute host filters its planner palette with, and
// a probe answering about a different wiring than the tool factory uses would
// re-create the very mismatch it exists to prevent.
func (uc *AgentUseCase) BindAgentKernel(k *agentkit.Kernel, probe *agentkernel.ToolSetProbe) {
	if uc.casebound != nil {
		uc.casebound.Bind(k)
	}
	uc.durableWorkspaceAgent.Bind(k, probe)
	uc.durableThreadcase.Bind(k, probe)
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
		// Was silent before. The Slack client classified this (token/auth) at
		// the origin; surface it in the mention's thread. return nil so the async
		// dispatcher does not re-Handle it.
		threadTS := msg.ThreadTS()
		if threadTS == "" {
			threadTS = msg.ID()
		}
		uc.replyUserError(ctx, err, "failed to get bot user ID", msg.ChannelID(), threadTS)
		return nil
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

	// Claim (and therefore persist) the Session that ties this thread to the
	// Case. It is claimed rather than merely loaded because its ID is the turn
	// lock's subject: a Session that exists only in memory cannot serialise a
	// concurrent mention arriving on another instance.
	session, err := uc.claimSession(ctx, entry.Workspace.ID, foundCase.ID, msg.ChannelID(), threadTS, model.SessionKindCase)
	if err != nil {
		return goerr.Wrap(err, "failed to claim agent session")
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

	req := casebound.TurnRequest{
		Session:        session,
		ChannelID:      msg.ChannelID(),
		ThreadTS:       threadTS,
		MentionTS:      msg.ID(),
		MentionText:    msg.Text(),
		MentionUserID:  msg.UserID(),
		BotUserID:      botUserID,
		Workspace:      entry,
		Case:           foundCase,
		Actions:        actions,
		CurrentAction:  currentAction,
		SystemMessages: toCaseboundMessages(systemMessages),
		DeltaMessages:  toCaseboundMessages(deltaMessages),
		TriggerTS:      msg.ID(),
	}

	// StartTurn returns as soon as the run is recorded; the LLM calls, the tool
	// calls and the reply all happen afterwards on the agent worker, and the
	// reply is posted from the completion handler (see caseboundHost).
	result, runErr := uc.casebound.StartTurn(ctx, req)
	if runErr != nil {
		// replyUserError reports and posts the 3-part message; return nil so the
		// async dispatcher does not re-Handle (double report) the same error.
		uc.replyUserError(ctx, runErr, "casebound start turn", msg.ChannelID(), threadTS)
		return nil
	}
	switch result.Status {
	case casebound.StatusBusy:
		busyMsg := i18n.T(ctx, i18n.MsgKeyAgentBusy)
		if _, postErr := uc.deps.SlackService.PostThreadReply(ctx, msg.ChannelID(), threadTS, busyMsg); postErr != nil {
			errutil.Handle(ctx, postErr, "post busy notice")
		}
		return nil
	case casebound.StatusDuplicate:
		// A re-delivery of an event whose run already exists. Dropping it
		// silently is the point: the original run posts the only reply.
		logger.Debug("dropping a duplicate mention delivery",
			"process", string(result.ProcessID), "trigger_ts", msg.ID())
		return nil
	case casebound.StatusStarted:
		return nil
	default:
		return goerr.New("unexpected casebound status", goerr.V("status", int(result.Status)))
	}
}

// caseboundHost is the Slack side of a finished case-channel turn. It is a
// separate type rather than methods on AgentUseCase because the Host interface
// needs exported method names, and Reply / ReportFailure are far too generic to
// sit on the usecase's own surface.
type caseboundHost struct {
	uc *AgentUseCase
}

// Reply posts the agent's answer as a thread reply.
func (h caseboundHost) Reply(ctx context.Context, channelID, threadTS, text string) error {
	if text == "" {
		return nil
	}
	if _, err := h.uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, text); err != nil {
		return goerr.Wrap(err, "post the agent reply",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	return nil
}

// ReportFailure tells the user the turn could not finish. reason crossed the
// durable process boundary as a string, so it is wrapped the same way a
// planexec fallback reason is: the user sees the "couldn't finish this turn"
// classification with the reason as the technical note.
func (h caseboundHost) ReportFailure(ctx context.Context, channelID, threadTS, reason string) error {
	h.uc.replyUserError(ctx, fallbackReasonError(reason), "casebound agent turn", channelID, threadTS)
	return nil
}

// wsagentHost is the Slack side of a finished workspace-agent turn. It mirrors
// caseboundHost; the two are separate types because each host package declares
// its own Host interface, and one adapter satisfying both would tie their
// contracts together.
type wsagentHost struct {
	uc *AgentUseCase
}

// Reply posts the agent's answer as a thread reply.
func (h wsagentHost) Reply(ctx context.Context, channelID, threadTS, text string) error {
	if text == "" {
		return nil
	}
	if _, err := h.uc.deps.SlackService.PostThreadReply(ctx, channelID, threadTS, text); err != nil {
		return goerr.Wrap(err, "post the workspace-agent reply",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	return nil
}

// ReportFailure tells the user the turn could not finish.
func (h wsagentHost) ReportFailure(ctx context.Context, channelID, threadTS, reason string) error {
	h.uc.replyUserError(ctx, fallbackReasonError(reason), "workspace agent turn", channelID, threadTS)
	return nil
}

// agentProgress draws a durable run's milestone lines into a single Slack
// message.
//
// Unlike traceMessage it holds no state: the message id and the lines so far
// live in the run's checkpointed state, because a run's transitions can be
// claimed by a different instance and an in-process accumulator there would
// start a second message instead of updating the first.
type agentProgress struct {
	uc *AgentUseCase
}

// Render posts the lines as one message, or updates the message already posted.
func (p agentProgress) Render(ctx context.Context, target planexec.ProgressTarget,
	messageTS string, lines []string,
) (string, error) {
	blocks := buildTraceContextBlocks(lines)
	fallback := traceFallbackText(lines)
	if messageTS == "" {
		ts, err := p.uc.deps.SlackService.PostThreadMessage(ctx, target.ChannelID, target.ThreadTS, blocks, fallback)
		if err != nil {
			return "", goerr.Wrap(err, "post the agent progress message",
				goerr.V("channel_id", target.ChannelID), goerr.V("thread_ts", target.ThreadTS))
		}
		return ts, nil
	}
	if err := p.uc.deps.SlackService.UpdateMessage(ctx, target.ChannelID, messageTS, blocks, fallback); err != nil {
		return messageTS, goerr.Wrap(err, "update the agent progress message",
			goerr.V("channel_id", target.ChannelID), goerr.V("message_ts", messageTS))
	}
	return messageTS, nil
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

// claimSession is loadOrCreateSession plus immediate, atomic persistence: it
// returns the stored Session for the thread, creating it in the same operation
// when none exists yet (see interfaces.SessionRepository.Claim).
//
// A host uses this instead of loadOrCreateSession when the Session's Kind is
// load-bearing for routing OTHER events — the Slack dispatcher reads it to
// decide whether an in-thread mention starts a Case. Deferring the write to
// AcquireTurnLock would leave the thread unowned for the length of the host's
// setup work, and a concurrent mention arriving in that gap would route it the
// other way. The returned Session may therefore carry a different Kind than
// requested: that means another host claimed the thread first, and the caller
// must respect that decision rather than proceed.
func (uc *AgentUseCase) claimSession(ctx context.Context, workspaceID string, caseID int64, channelID, threadTS string, kind model.SessionKind) (*model.Session, error) {
	claimed, err := uc.deps.Repo.Session().Claim(ctx, channelID, threadTS, func() *model.Session {
		return uc.newSession(ctx, workspaceID, caseID, channelID, threadTS, kind)
	})
	if err != nil {
		return nil, goerr.Wrap(err, "failed to claim session",
			goerr.V("channel_id", channelID), goerr.V("thread_ts", threadTS))
	}
	return claimed, nil
}

// loadOrCreateSession returns the Session for the given thread, creating
// (but not yet persisting) a fresh one when none exists. Persistence happens
// at the end of HandleAgentMention so we only commit a session that
// successfully started a turn.
//
// kind is applied ONLY to a freshly created Session. An existing one is
// returned untouched: a thread's owner is decided by whoever started it, and
// letting a later caller rewrite it would make the dispatcher's routing
// (case creation vs. workspace agent) depend on event ordering.
func (uc *AgentUseCase) loadOrCreateSession(ctx context.Context, workspaceID string, caseID int64, channelID, threadTS string, kind model.SessionKind) (*model.Session, error) {
	existing, err := uc.deps.Repo.Session().GetByThread(ctx, channelID, threadTS)
	if err != nil {
		return nil, goerr.Wrap(err, "failed to get session")
	}
	if existing != nil {
		return existing, nil
	}
	return uc.newSession(ctx, workspaceID, caseID, channelID, threadTS, kind), nil
}

// newSession builds a fresh, unpersisted Session for the thread. Shared by
// loadOrCreateSession and claimSession so both produce identical records.
func (uc *AgentUseCase) newSession(ctx context.Context, workspaceID string, caseID int64, channelID, threadTS string, kind model.SessionKind) *model.Session {
	// Detect Action linkage by matching the thread parent TS against any
	// registered action notification message. Most threads have no associated
	// action — tag ErrNotFound as benign so the lookup is visible at Info level
	// without paging Sentry, while real backend failures still alert as ERROR.
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
		Kind:        kind,
		ThreadTS:    threadTS,
		ChannelID:   channelID,
		ActionID:    actionID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
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

// traceFallbackText renders the plain-text notification fallback for a milestone
// history, windowed to the same most-recent maxTraceBlocks lines
// buildTraceContextBlocks renders. Keeping the two in step is what stops an
// unbounded history from blowing past Slack's 4000-char text-field limit
// (msg_too_long) while the blocks themselves stay within their own cap.
func traceFallbackText(lines []string) string {
	if len(lines) > maxTraceBlocks {
		lines = lines[len(lines)-maxTraceBlocks:]
	}
	return strings.Join(lines, "\n")
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
		return traceFallbackText(lines)
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
