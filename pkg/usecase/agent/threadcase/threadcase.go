package threadcase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/core"
	knowledgetool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/knowledge"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// UseCase runs one thread-mode agent turn on top of the shared planexec
// runtime. It owns the per-thread turn lock and the planexec wiring; the
// host applies the returned Decision (post reply / update fields / close)
// through the case usecase.
type UseCase struct {
	deps   *agent.CommonDeps
	runner *planexec.Runner
}

// New builds a threadcase UseCase. Both deps and runner are required.
func New(deps *agent.CommonDeps, runner *planexec.Runner) (*UseCase, error) {
	if deps == nil {
		return nil, goerr.New("CommonDeps is required")
	}
	if runner == nil {
		return nil, goerr.New("planexec runner is required")
	}
	return &UseCase{deps: deps, runner: runner}, nil
}

// TurnRequest collects the inputs resolved by the host before handing control
// to the threadcase runtime.
type TurnRequest struct {
	Session   *model.Session
	Workspace *model.WorkspaceEntry
	Case      *model.Case

	ChannelID   string
	ThreadTS    string
	MentionTS   string
	MentionText string

	SystemMessages []ConversationMessage
	DeltaMessages  []ConversationMessage

	// TriggerTS is the Slack TS used as both the trace ID seed and the lock
	// trigger key (duplicate-event dedup).
	TriggerTS string

	// Mode selects the turn purpose (materialize on creation vs mention).
	Mode Mode

	Handler Handler
}

// Status discriminates the terminal shapes RunTurn returns.
type Status int

const (
	// StatusCompleted means the turn finished and Decision is populated.
	StatusCompleted Status = iota
	// StatusBusy means another turn was running; BusyOwner is set.
	StatusBusy
	// StatusIdempotent means the trigger duplicates a live turn; drop silently.
	StatusIdempotent
	// StatusQuestion means the planner asked the user a question; the turn
	// ended and will resume on the next mention. Decision is nil.
	StatusQuestion
	// StatusFallback means the planner exhausted its budget or errored before
	// reaching a decision. Decision is nil.
	StatusFallback
)

// Result is the outcome of RunTurn.
type Result struct {
	Status    Status
	Decision  *Decision
	BusyOwner *model.Session
	// Case is the newly created case for a ModeCreate turn that committed it
	// (via Handler.Create inside the planner loop). Nil for other modes and
	// for non-completed statuses.
	Case *model.Case
}

// RunTurn executes one thread-mode turn: acquire the per-thread lock, run the
// planexec loop with read-only sub-agent tools, and return the parsed
// terminal Decision for the host to apply.
func (uc *UseCase) RunTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if err := validateRequest(&req); err != nil {
		return nil, err
	}
	if req.Handler == nil {
		req.Handler = HandlerFuncs{}
	}

	handle, err := uc.deps.StartTurn(ctx, req.Session, req.TriggerTS)
	if err != nil {
		return nil, goerr.Wrap(err, "start threadcase turn")
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

	// Individual sub-agent tool calls are ephemeral activity: overwrite the
	// single live line rather than appending each one to the trace block.
	turnCtx = tool.WithUpdate(turnCtx, func(innerCtx context.Context, message string) {
		req.Handler.TraceReplace(innerCtx, message)
	})

	resolver := uc.buildToolResolver(req)
	sink := newHandlerSink(req.Handler)

	wsID := ""
	caseID := ""
	if req.Workspace != nil {
		wsID = req.Workspace.Workspace.ID
	}
	if req.Case != nil {
		caseID = fmt.Sprintf("%d", req.Case.ID)
	}

	onQuestion := func(qctx context.Context, q planexec.Question) (planexec.QuestionResult, error) {
		payload := QuestionPayload{Reason: q.Reason}
		payload.Items = make([]QuestionItem, len(q.Items))
		for i, it := range q.Items {
			payload.Items[i] = QuestionItem{
				ID:      it.ID,
				Text:    it.Text,
				Type:    QuestionItemType(it.Type),
				Options: it.Options,
			}
		}
		if qerr := req.Handler.Question(qctx, req.Session, payload); qerr != nil {
			return planexec.QuestionResult{}, goerr.Wrap(qerr, "post threadcase question")
		}
		// End the turn; the user resumes by mentioning the bot again. The
		// conversation history (keyed on Session.ID) carries the context.
		return planexec.QuestionResult{Terminate: true}, nil
	}

	// ModeCreate commits the new case inside the planner loop via OnFinalize so
	// that both validation failures and persistence failures fold back into a
	// re-plan. Other modes return a parsed Decision for the host to apply.
	isCreate := req.Mode == ModeCreate
	finalSchema := decisionSchema()
	var createdCase *model.Case
	var onFinalize func(context.Context, json.RawMessage) error
	if isCreate {
		finalSchema = createDecisionSchema()
		onFinalize = func(fctx context.Context, raw json.RawMessage) error {
			dec, perr := parseCreateDecision(raw)
			if perr != nil {
				return perr
			}
			fields, verr := validateCreateDecision(req.Workspace, dec)
			if verr != nil {
				return verr
			}
			c, cerr := req.Handler.Create(fctx, req.Session, CreatePayload{
				Title:       dec.Title,
				Description: dec.Description,
				Fields:      fields,
			})
			if cerr != nil {
				return goerr.Wrap(cerr, "create case")
			}
			createdCase = c
			return nil
		}
	}

	runResult, runErr := uc.runner.Run(turnCtx, planexec.RunRequest{
		HistoryKey: req.Session.ID,
		TraceID:    handle.OwnerID,
		TraceMetadata: trace.TraceMetadata{
			Labels: map[string]string{
				"session_id":   req.Session.ID,
				"workspace_id": wsID,
				"case_id":      caseID,
				"thread_ts":    req.Session.ThreadTS,
				"trigger_ts":   req.TriggerTS,
			},
		},
		UserInput:    buildUserInput(req.SystemMessages, req.DeltaMessages, req.MentionText, req.MentionTS),
		SystemPrompt: buildSystemPrompt(req.Case, req.Workspace, req.Mode),
		ToolResolver: resolver,
		// Thread-mode workspaces manage no Actions, so the planner must not be
		// offered the core (action) toolset; OmitCore below withholds the tools.
		KnownToolIDs:  agent.KnownToolSetIDsNoCore,
		AllowQuestion: true,
		// Direct mode answers a trivial mention without the investigation
		// loop, replying in plain text. It is disabled for ModeCreate: a
		// create turn must commit a Case (a side-effecting terminal action),
		// which the direct path deliberately never does.
		AllowDirect:       !isCreate,
		OnQuestion:        onQuestion,
		FinalOutputSchema: finalSchema,
		OnFinalize:        onFinalize,
		Sink:              sink,
	})
	if runErr != nil {
		return nil, goerr.Wrap(runErr, "run threadcase planexec")
	}

	// Persist the just-processed mention TS so the next turn's delta scan
	// starts strictly after this one.
	if req.MentionTS != "" {
		req.Session.LastMentionTS = req.MentionTS
	}

	switch runResult.Status {
	case planexec.StatusCompleted:
		if runResult.EndedWithQuestion {
			uc.persistSession(turnCtx, req.Session, model.SessionEndedWithQuestion)
			return &Result{Status: StatusQuestion}, nil
		}
		if isCreate {
			// The case was committed inside OnFinalize; createdCase is set.
			uc.persistSession(turnCtx, req.Session, model.SessionEndedWithCaseBoundReply)
			return &Result{Status: StatusCompleted, Case: createdCase}, nil
		}
		if runResult.Direct {
			// Direct path produced a plain-text reply (no structured Decision).
			// Treat it as a respond decision so the host posts it as the
			// thread reply, exactly as it would a parsed respond Decision.
			uc.persistSession(turnCtx, req.Session, model.SessionEndedWithCaseBoundReply)
			return &Result{Status: StatusCompleted, Decision: &Decision{
				Kind:    DecisionRespond,
				Message: runResult.FinalText,
			}}, nil
		}
		decision, perr := parseDecision(runResult.FinalRaw)
		if perr != nil {
			return nil, goerr.Wrap(perr, "parse threadcase decision")
		}
		uc.persistSession(turnCtx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusCompleted, Decision: decision}, nil
	case planexec.StatusFallbackBudget, planexec.StatusFallbackError:
		uc.persistSession(turnCtx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusFallback}, nil
	default:
		return nil, goerr.New("threadcase planexec returned unknown status",
			goerr.V("status", runResult.Status))
	}
}

// persistSession stamps the session end reason + mention TS and persists.
func (uc *UseCase) persistSession(ctx context.Context, ssn *model.Session, ended model.SessionEndReason) {
	ssn.LastAction = ended
	ssn.UpdatedAt = time.Now().UTC()
	if err := uc.deps.Repo.Session().Put(ctx, ssn); err != nil {
		errutil.Handle(ctx, err, "threadcase: persist session")
	}
}

// buildToolResolver composes the read-only sub-agent tool resolver. Thread-mode
// workspaces manage no Actions, so the core (action) toolset is omitted entirely
// — investigation reads Slack / Notion / GitHub / the web, and Case writes happen
// via the returned Decision, never the action tools.
func (uc *UseCase) buildToolResolver(req TurnRequest) *agent.ToolSetResolver {
	d := uc.deps
	wsID := ""
	if req.Workspace != nil {
		wsID = req.Workspace.Workspace.ID
	}
	return agent.NewToolSetResolver(agent.ToolSetDeps{
		OmitCore: true,
		Core: core.Deps{
			Repo:        d.Repo,
			WorkspaceID: wsID,
			CaseRefUC:   d.CaseRefUC,
		},
		Slack: slacktool.Deps{
			Bot:       d.SlackBot,
			Search:    d.SlackSearch,
			Retriever: d.SlackRetriever,
		},
		Notion:   notiontool.Deps{Client: d.NotionClient},
		GitHub:   d.GitHubClient,
		WebFetch: d.WebFetchClient,
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
	// ModeCreate runs before any case exists, so Case is intentionally nil
	// there; every other mode operates on an existing case.
	if req.Case == nil && req.Mode != ModeCreate {
		return goerr.New("Case is required")
	}
	if req.Workspace == nil {
		return goerr.New("Workspace is required")
	}
	if req.TriggerTS == "" {
		return goerr.New("TriggerTS is required")
	}
	return nil
}
