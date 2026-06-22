package casebound

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	githubtool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/github"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	memotool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/memo"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// UseCase runs a single case-bound agent turn (a Slack mention in a Case
// channel). It owns the gollem invocation, system prompt assembly, and
// session lock lifecycle; the Slack-side host is responsible for fetching
// the conversation, posting trace updates, and posting the final reply.
type UseCase struct {
	deps *agent.CommonDeps
}

// New builds a casebound UseCase wired against the shared agent deps.
func New(deps *agent.CommonDeps) (*UseCase, error) {
	if deps == nil {
		return nil, goerr.New("CommonDeps is required")
	}
	if deps.LLMClient == nil {
		return nil, goerr.New("LLMClient is required")
	}
	if deps.HistoryRepo == nil {
		return nil, goerr.New("HistoryRepo is required")
	}
	if deps.TraceRepo == nil {
		return nil, goerr.New("TraceRepo is required")
	}
	return &UseCase{deps: deps}, nil
}

// TurnRequest collects the inputs the host has already resolved before
// handing control to the casebound runtime. The casebound runtime never
// touches the Slack service directly; everything Slack-related is either
// pre-built (system / delta messages) or delegated via Handler.
type TurnRequest struct {
	// Session is the active turn's Session, with TurnOwnerID etc. already
	// populated by the upstream lock acquire. The runtime updates
	// LastMentionTS / LastAction at the end of the turn and persists.
	Session *model.Session

	// Mention metadata (channel / thread / user / mention text) that drives
	// the prompt body and trace metadata.
	ChannelID   string
	ThreadTS    string
	MentionTS   string
	MentionText string
	BotUserID   string

	// Resolved domain state.
	Workspace     *model.WorkspaceEntry
	Case          *model.Case
	Actions       []*model.Action
	CurrentAction *model.Action

	// Pre-fetched conversation context. SystemMessages is non-empty only
	// for fresh sessions (Session.LastMentionTS == ""). DeltaMessages is
	// the unprocessed-since-last-mention slice for continuing sessions.
	SystemMessages []ConversationMessage
	DeltaMessages  []ConversationMessage

	// TriggerTS is the Slack TS used both as the trace ID and as the lock
	// trigger key (to detect duplicate event delivery).
	TriggerTS string

	// Handler receives progressive trace updates while gollem runs.
	Handler Handler
}

// Result is the outcome of RunTurn. Status discriminates the three terminal
// shapes the host needs to act on.
type Result struct {
	// Status is one of the constants below.
	Status Status
	// FinalText is the agent's final reply (only when Status==StatusCompleted).
	FinalText string
	// BusyOwner exposes the live owner's session when Status==StatusBusy.
	BusyOwner *model.Session
}

// Status values returned by RunTurn.
type Status int

const (
	// StatusCompleted indicates the turn ran end-to-end and FinalText is set.
	StatusCompleted Status = iota
	// StatusBusy indicates another turn was already running on this session;
	// the host should post a busy notification and drop the trigger.
	StatusBusy
	// StatusIdempotent indicates the trigger duplicates a turn already in
	// flight; the host should drop silently with no Slack post.
	StatusIdempotent
)

// RunTurn executes one case-bound mention turn. It acquires the per-thread
// turn lock (Phase A, §5.3), runs the gollem ReAct loop, persists the
// updated Session.LastMentionTS, and returns the gollem final text for the
// host to post to Slack.
func (uc *UseCase) RunTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if err := validateRequest(&req); err != nil {
		return nil, err
	}
	if req.Handler == nil {
		req.Handler = HandlerFunc(func(context.Context, string) {})
	}

	handle, err := uc.deps.StartTurn(ctx, req.Session, req.TriggerTS)
	if err != nil {
		return nil, goerr.Wrap(err, "start casebound turn")
	}
	if handle.Idempotent {
		return &Result{Status: StatusIdempotent}, nil
	}
	if !handle.Acquired {
		return &Result{Status: StatusBusy, BusyOwner: handle.BusyOwner}, nil
	}
	defer handle.Release(ctx)

	turnCtx := handle.Ctx
	req.Session = handle.Session

	// Build trace recorder for the durable trace artifact.
	actionIDStr := ""
	if req.Session.ActionID != 0 {
		actionIDStr = fmt.Sprintf("%d", req.Session.ActionID)
	}
	wsID := ""
	caseID := ""
	if req.Workspace != nil {
		wsID = req.Workspace.Workspace.ID
	}
	if req.Case != nil {
		caseID = fmt.Sprintf("%d", req.Case.ID)
	}
	recorder := trace.New(
		trace.WithRepository(uc.deps.TraceRepo),
		trace.WithTraceID(handle.OwnerID),
		trace.WithMetadata(trace.TraceMetadata{
			Labels: map[string]string{
				labelSessionID:        req.Session.ID,
				labelWorkspaceID:      wsID,
				labelCaseID:           caseID,
				labelThreadTS:         req.Session.ThreadTS,
				labelActionID:         actionIDStr,
				labelTriggerMentionTS: req.MentionTS,
			},
		}),
	)
	defer func() {
		// Use the parent ctx (not turnCtx) so the final trace flush
		// still runs even when the turn was cancelled by lock loss /
		// heartbeat staleness — that's exactly when the trace is most
		// valuable for debugging.
		if err := recorder.Finish(ctx); err != nil {
			errutil.Handle(ctx, err, "casebound: persist agent trace")
		}
	}()

	systemPrompt := buildSystemPrompt(req.Case, req.Workspace, req.ChannelID, time.Now().UTC(), req.CurrentAction, req.Actions, req.SystemMessages)
	userInput := buildUserInput(req.DeltaMessages, req.MentionText, req.MentionTS)

	// Per-tool activity is ephemeral: route it through TraceReplace so each
	// "Searching…/Fetching…" line overwrites the previous one in place rather
	// than piling up in the thread.
	turnCtx = tool.WithUpdate(turnCtx, func(innerCtx context.Context, message string) {
		req.Handler.TraceReplace(innerCtx, message)
	})

	allTools := uc.buildTools(req)
	gollemAgent := gollem.New(uc.deps.LLMClient,
		gollem.WithSystemPrompt(systemPrompt),
		gollem.WithTools(allTools...),
		gollem.WithHistoryRepository(uc.deps.HistoryRepo, req.Session.ID),
		gollem.WithTrace(recorder),
		gollem.WithToolMiddleware(
			func(next gollem.ToolHandler) gollem.ToolHandler {
				return func(ctx context.Context, tr *gollem.ToolExecRequest) (*gollem.ToolExecResponse, error) {
					// The "running this tool" notice is transient activity; the
					// error is a milestone worth keeping in the history.
					req.Handler.TraceReplace(ctx, fmt.Sprintf("🔧 `%s`", tr.Tool.Name))
					resp, err := next(ctx, tr)
					if resp != nil && resp.Error != nil {
						req.Handler.TraceAppend(ctx, "❌ Error: "+resp.Error.Error())
					}
					return resp, err
				}
			},
		),
	)

	resp, execErr := gollemAgent.Execute(turnCtx, gollem.Text(userInput))
	if execErr != nil {
		return nil, goerr.Wrap(execErr, "execute casebound agent")
	}

	// Persist the just-processed mention TS so the next mention starts its
	// delta scan strictly after this one.
	req.Session.LastMentionTS = req.MentionTS
	req.Session.LastAction = model.SessionEndedWithCaseBoundReply
	req.Session.UpdatedAt = time.Now().UTC()
	if err := uc.deps.Repo.Session().Put(turnCtx, req.Session); err != nil {
		errutil.Handle(turnCtx, err, "casebound: persist session lastMentionTS")
	}

	finalText := strings.Join(resp.Texts, "\n")
	return &Result{Status: StatusCompleted, FinalText: finalText}, nil
}

// buildTools assembles the gollem tool slice for a case-bound turn. The
// case-bound mode uses the *full* mutating action tool set so the agent can
// create / update / archive Actions on behalf of the user.
func (uc *UseCase) buildTools(req TurnRequest) []gollem.Tool {
	d := uc.deps
	var statusSet *model.ActionStatusSet
	if req.Workspace != nil {
		statusSet = req.Workspace.ActionStatusSet
	}
	wsID := ""
	caseID := int64(0)
	if req.Workspace != nil {
		wsID = req.Workspace.Workspace.ID
	}
	if req.Case != nil {
		caseID = req.Case.ID
	}
	// Action tools exist only where Actions exist: channel-mode cases. A
	// thread-mode case (bound to a Slack thread) tracks progress via its board
	// status and has no Actions, so the usecase boundary rejects action writes
	// there (ErrCaseThreadModeNoActions). Withhold the whole core toolset for
	// thread-mode rather than offer tools that can only error — mirroring the
	// Job runtime's exclusion. case_ref read tools live in the core toolset too,
	// so thread-mode forgoes them along with the action tools.
	var coreTools []gollem.Tool
	if req.Case == nil || !req.Case.IsThreadBound() {
		coreTools = core.New(core.Deps{
			Repo:         d.Repo,
			WorkspaceID:  wsID,
			CaseID:       caseID,
			StatusSet:    statusSet,
			ActionUC:     d.ActionUC,
			ActionStepUC: d.ActionStepUC,
			CaseRefUC:    d.CaseRefUC,
		})
	}
	slackTools := slacktool.NewReadOnly(slacktool.Deps{
		Bot:       d.SlackBot,
		Search:    d.SlackSearch,
		Retriever: d.SlackRetriever,
	})
	notionTools := notiontool.New(notiontool.Deps{Client: d.NotionClient})
	githubTools := githubtool.New(d.GitHubClient)
	webfetchTools := webfetch.New(d.WebFetchClient)

	// Case-editing tools (title / description / assignees / custom fields and,
	// for thread-mode workspaces, board status). Only wired when a CaseUC is
	// configured; the schema / status set come from the workspace so the tool
	// specs and the system prompt advertise the same field ids and status ids.
	var caseTools []gollem.Tool
	if d.CaseUC != nil {
		var fieldSchema *config.FieldSchema
		var caseStatusSet *model.ActionStatusSet
		if req.Workspace != nil {
			fieldSchema = req.Workspace.FieldSchema
			caseStatusSet = req.Workspace.CaseStatusSet
		}
		caseTools = casewriter.New(casewriter.Deps{
			CaseUC:      d.CaseUC,
			WorkspaceID: wsID,
			CaseID:      caseID,
			Schema:      fieldSchema,
			StatusSet:   caseStatusSet,
		})
	}

	// Case-scoped memo tools (memo__list/get/create/update/archive). Wired only
	// when a MemoUC is configured and the workspace has a memo schema; the schema
	// drives the field coercion in the create/update tools.
	var memoTools []gollem.Tool
	if d.MemoUC != nil && req.Workspace != nil && req.Workspace.MemoConfig.Enabled() {
		memoTools = memotool.New(memotool.Deps{
			Repo:        d.Repo,
			WorkspaceID: wsID,
			CaseID:      caseID,
			MemoUC:      d.MemoUC,
			Schema:      req.Workspace.MemoConfig.FieldSchema,
		})
	}

	// Workspace-wide knowledge tools (knowledge__*). Read tools are always
	// offered when an accessor is configured; the write tools (create/update)
	// are withheld while processing a PRIVATE case, because shared knowledge is
	// visible to the whole workspace and a private case's contents must not leak
	// into it through an agent write.
	var knowledgeTools []gollem.Tool
	if d.KnowledgeAccessor != nil {
		kdeps := knowledgetool.Deps{WorkspaceID: wsID, Accessor: d.KnowledgeAccessor}
		if d.KnowledgeMutator != nil && req.Case != nil && !req.Case.IsPrivate {
			kdeps.Mutator = d.KnowledgeMutator
			knowledgeTools = knowledgetool.New(kdeps)
		} else {
			knowledgeTools = knowledgetool.NewReadOnly(kdeps)
		}
	}

	all := make([]gollem.Tool, 0, len(coreTools)+len(slackTools)+len(notionTools)+len(githubTools)+len(webfetchTools)+len(caseTools)+len(memoTools)+len(knowledgeTools))
	all = append(all, coreTools...)
	all = append(all, slackTools...)
	all = append(all, notionTools...)
	all = append(all, githubTools...)
	all = append(all, webfetchTools...)
	all = append(all, caseTools...)
	all = append(all, memoTools...)
	all = append(all, knowledgeTools...)
	return all
}

// validateRequest enforces the minimum invariants RunTurn needs.
func validateRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	if req.MentionTS == "" {
		return goerr.New("MentionTS is required")
	}
	if req.TriggerTS == "" {
		return goerr.New("TriggerTS is required")
	}
	if req.Case == nil {
		return goerr.New("Case is required")
	}
	if req.Workspace == nil {
		return goerr.New("Workspace is required")
	}
	return nil
}

// buildUserInput assembles the user-facing text passed to gollem. Unprocessed
// thread messages are prepended in chronological order with a header so the
// agent can distinguish them from the new prompt. The current mention text
// is always appended last.
func buildUserInput(delta []ConversationMessage, mentionText, mentionTS string) string {
	if len(delta) == 0 {
		return mentionText
	}
	var b strings.Builder
	b.WriteString("# Unprocessed thread messages since last mention\n")
	for _, m := range delta {
		// Skip the current mention itself if it appears in the delta.
		if m.Timestamp == mentionTS {
			continue
		}
		name := m.UserName
		if name == "" {
			name = m.UserID
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.Timestamp, name, m.Text)
	}
	b.WriteString("\n# Current mention\n")
	b.WriteString(mentionText)
	return b.String()
}

// Trace metadata labels keyed off the SessionIDLabel exported by agentarchive.
// Kept verbatim from the legacy agent.go so existing trace consumers keep
// working without re-indexing.
const (
	labelSessionID        = "session_id"
	labelWorkspaceID      = "workspace_id"
	labelCaseID           = "case_id"
	labelThreadTS         = "thread_ts"
	labelActionID         = "action_id"
	labelTriggerMentionTS = "trigger_mention_ts"
)
