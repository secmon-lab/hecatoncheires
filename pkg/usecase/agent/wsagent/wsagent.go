package wsagent

import (
	"context"
	"fmt"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

// UseCase drives one workspace-agent turn on top of the shared planexec
// runtime. It holds no per-call state (deps + runner are injected once), so it
// is safe for concurrent turns; per-turn isolation comes from the turn lock and
// the per-call planexec inputs.
type UseCase struct {
	deps   *agent.CommonDeps
	runner *planexec.Runner
}

// New constructs the workspace-agent host. Both deps and runner are required.
func New(deps *agent.CommonDeps, runner *planexec.Runner) (*UseCase, error) {
	if deps == nil {
		return nil, goerr.New("common deps are required")
	}
	if runner == nil {
		return nil, goerr.New("planexec runner is required")
	}
	return &UseCase{deps: deps, runner: runner}, nil
}

// RunTurn runs one workspace-agent turn: acquire the per-thread lock, establish
// the mentioning user as the access actor (ctx auth token), run the plan-execute
// loop with the cross-case tool set, and return the reply text. All case
// mutations happen as sub-agent tool calls inside the loop (AllowSubAgentWrites)
// governed by the safety-rule system prompt; planexec performs no side effects.
func (uc *UseCase) RunTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if err := validateRequest(&req); err != nil {
		return nil, err
	}
	if req.Handler == nil {
		req.Handler = HandlerFuncs{}
	}

	handle, err := uc.deps.StartTurn(ctx, req.Session, req.TriggerTS)
	if err != nil {
		return nil, goerr.Wrap(err, "start workspace-agent turn")
	}
	if handle.Idempotent {
		return &Result{Status: StatusIdempotent}, nil
	}
	if !handle.Acquired {
		owner := ""
		if handle.BusyOwner != nil {
			owner = handle.BusyOwner.TurnOwnerID
		}
		return &Result{Status: StatusBusy, BusyOwner: owner}, nil
	}
	defer handle.Release(ctx)

	req.Session = handle.Session

	// Establish the mentioning user as the access actor for the whole turn. This
	// single injection makes every casemulti read (ListCases/GetCase → redaction)
	// and write (loadCaseForWrite/assertCaseWriteAccess) enforce this user's
	// private-case membership. A missing token would grant full access, so this
	// is mandatory, not optional.
	turnCtx := auth.ContextWithToken(handle.Ctx, &auth.Token{Sub: req.ActorID})

	// Per-tool chatter overwrites the single live activity line rather than
	// appending each call to the trace block.
	turnCtx = tool.WithUpdate(turnCtx, func(innerCtx context.Context, message string) {
		req.Handler.TraceReplace(innerCtx, message)
	})

	wsID := req.Workspace.Workspace.ID
	resolver := uc.buildToolResolver(req)
	sink := newHandlerSink(req.Handler)

	result, err := planexec.RunText(turnCtx, uc.runner, planexec.RunRequest{
		HistoryKey: req.Session.ID,
		TraceID:    handle.OwnerID,
		TraceMetadata: trace.TraceMetadata{
			Labels: map[string]string{
				"session_id":   req.Session.ID,
				"workspace_id": wsID,
				"thread_ts":    req.Session.ThreadTS,
				"trigger_ts":   req.TriggerTS,
			},
		},
		UserInput:    req.MentionText,
		SystemPrompt: buildSystemPrompt(req.Workspace),
		ToolResolver: resolver,
		KnownToolIDs: agent.KnownToolSetIDsWorkspaceChannel,
		// Case mutations happen as sub-agent tool calls inside the loop, gated by
		// the safety-rule prompt. planexec itself performs no side effects.
		AllowSubAgentWrites: true,
		// A trivial request (a question, a status lookup) can be answered on
		// round 1 without the full investigation loop.
		AllowDirect: true,
		// v1: no mid-turn clarifying questions (would need resume plumbing across
		// turns). The agent replies or reports within a single turn per mention.
		AllowQuestion: false,
		Sink:          sink,
	})
	if err != nil {
		return nil, goerr.Wrap(err, "workspace-agent planexec run",
			goerr.V("workspace_id", wsID),
			goerr.V("session_id", req.Session.ID))
	}

	switch result.Status {
	case planexec.StatusCompleted:
		return &Result{Status: StatusCompleted, ReplyText: result.Text}, nil
	case planexec.StatusFallbackBudget, planexec.StatusFallbackError:
		return &Result{Status: StatusFallback}, nil
	default:
		return nil, goerr.New("workspace-agent planexec returned unknown status",
			goerr.V("status", result.Status),
			goerr.V("workspace_id", wsID))
	}
}

// buildToolResolver wires the cross-case tool set plus the read-only auxiliary
// tools. OmitCore is set because the case-pinned core action tools are replaced
// by casemulti's cross-case action tools (which take case_id at call time).
func (uc *UseCase) buildToolResolver(req TurnRequest) *agent.ToolSetResolver {
	d := uc.deps
	wsID := req.Workspace.Workspace.ID
	return agent.NewToolSetResolver(agent.ToolSetDeps{
		OmitCore: true,
		CaseMulti: casemulti.Deps{
			WorkspaceID: wsID,
			ActorID:     req.ActorID,
			CaseUC:      d.CaseMultiUC,
			ActionUC:    d.CaseMultiActionUC,
			Schema:      req.Workspace.FieldSchema,
		},
		Slack: slacktool.Deps{
			Bot:       d.SlackBot,
			Search:    d.SlackSearch,
			Retriever: d.SlackRetriever,
		},
		Notion:   notiontool.Deps{Client: d.NotionClient},
		GitHub:   d.GitHubClient,
		WebFetch: d.WebFetchClient,
		Jira:     d.JiraTools,
		Knowledge: knowledgetool.Deps{
			WorkspaceID: wsID,
			Accessor:    d.KnowledgeAccessor,
		},
	})
}

func validateRequest(req *TurnRequest) error {
	if req == nil {
		return goerr.New("request is nil")
	}
	if req.Session == nil {
		return goerr.New("Session is required")
	}
	if req.Workspace == nil {
		return goerr.New("Workspace is required")
	}
	if req.ActorID == "" {
		return goerr.New("ActorID is required (the mentioning user is the access actor)")
	}
	return nil
}

// newHandlerSink adapts the Handler into a planexec.Sink, forwarding planner
// progress as flat trace lines to the thread.
func newHandlerSink(h Handler) planexec.Sink {
	return planexec.SinkFuncs{
		NotifyFn: func(ctx context.Context, line string) {
			h.TraceAppend(ctx, line)
		},
		PlanProposedFn: func(ctx context.Context, info planexec.PlanInfo) {
			label := "🧭 Planning"
			if info.IsReplan {
				label = "🧭 Re-planning"
			}
			if info.Reasoning != "" {
				h.TraceAppend(ctx, fmt.Sprintf("%s — %s", label, info.Reasoning))
			} else {
				h.TraceAppend(ctx, label)
			}
		},
		PhaseStartedFn: func(ctx context.Context, phase int, tasks []planexec.TaskInfo) {
			h.TraceAppend(ctx, fmt.Sprintf("🔎 Investigating (%d task(s))", len(tasks)))
		},
		TaskFinishedFn: func(ctx context.Context, result planexec.TaskResult) {
			h.TraceAppend(ctx, fmt.Sprintf("✓ %s", result.Title))
		},
	}
}
