package threadcase

import (
	"context"
	"fmt"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/google/uuid"
	"github.com/m-mizutani/goerr/v2"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casewriter"
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

	// Per-mode wiring. A mention turn lets its sub-agent close / transition the
	// case via the case__update_case_status tool (case_status_write toolset +
	// AllowSubAgentWrites), so status changes are a tool-driven side effect
	// inside the loop, not a host-applied terminal decision. A create turn has
	// no case yet, so it stays observation-only and materializes the new case
	// from the structured final output.
	isCreate := req.Mode == ModeCreate
	knownToolIDs := agent.KnownToolSetIDsNoCore
	allowWrites := false
	if req.Mode == ModeMention {
		knownToolIDs = agent.KnownToolSetIDsThreadWrite
		allowWrites = true
	}

	systemPrompt := buildSystemPrompt(req.Case, req.Workspace, req.Mode)

	// Record a mention turn (ModeMention) as a JobRunLog + JobRunEvent trail so
	// the case agent page lists it alongside Job runs. ModeCreate runs before a
	// case exists (it materialises the case) and is a creation-time flow, not a
	// post-creation mention, so it is intentionally excluded. The run gets its
	// own fresh per-turn JobID and is tagged EventType=mention; its TraceID is
	// shared with the planexec archive recorder (both keyed on handle.OwnerID)
	// so the two trace sinks correlate. Opening the log is observability, not
	// part of the turn's success contract — a failure here is non-fatal.
	var runErr error
	var traceHandler trace.Handler
	if !isCreate {
		rec, recOpenErr := runtrace.Open(turnCtx, runtrace.OpenParams{
			Repo:         uc.deps.Repo,
			WorkspaceID:  wsID,
			CaseID:       req.Case.ID,
			JobID:        uuid.Must(uuid.NewV7()).String(),
			RunID:        uuid.Must(uuid.NewV7()).String(),
			TraceID:      handle.OwnerID,
			EventType:    model.EventTypeMention,
			ExecutorKind: model.ExecutorKindPlanexec,
			SystemPrompt: systemPrompt,
			StartedAt:    time.Now().UTC(),
		})
		if recOpenErr != nil {
			errutil.Handle(turnCtx, recOpenErr, "threadcase: open mention run trace")
		}
		if rec != nil {
			traceHandler = rec.Handler()
			// Use the parent ctx (not turnCtx) so the terminal log write still
			// runs when the turn was cancelled by lock loss.
			defer func() { rec.Finish(ctx, runErr) }()
		}
	}

	baseReq := planexec.RunRequest{
		HistoryKey: req.Session.ID,
		TraceID:    handle.OwnerID,
		// TraceHandler streams the per-call JobRunEvent timeline; planexec
		// combines it with its own archive recorder (trace.Multi) and wires the
		// result into every agent it drives. Nil for ModeCreate.
		TraceHandler: traceHandler,
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
		SystemPrompt: systemPrompt,
		ToolResolver: resolver,
		KnownToolIDs: knownToolIDs,
		// Mention turns let the sub-agent close / transition the case via
		// case__update_case_status; create turns stay observation-only (no case
		// yet — the host materializes it from the final output).
		AllowSubAgentWrites: allowWrites,
		AllowQuestion:       true,
		// Direct mode answers a trivial mention without the investigation loop,
		// replying in plain text. Disabled for ModeCreate: a create turn must
		// materialize a Case, which the direct path deliberately never does.
		AllowDirect: !isCreate,
		OnQuestion:  onQuestion,
		Sink:        sink,
	}

	// Persist the just-processed mention TS so the next turn's delta scan starts
	// strictly after this one. Set before dispatching so it is stamped onto the
	// session the completion branches persist.
	if req.MentionTS != "" {
		req.Session.LastMentionTS = req.MentionTS
	}

	// ModeCreate materializes a NEW case from the validated structured final
	// output (Run[CreateDecision]); every other mode resolves to a mention
	// Decision (respond / materialize) via Run[Decision]. Close is neither — it
	// is a sub-agent tool side effect inside the loop.
	if isCreate {
		return uc.runCreateTurn(turnCtx, req, baseReq, &runErr)
	}
	return uc.runMentionTurn(turnCtx, req, baseReq, &runErr)
}

// runCreateTurn drives a ModeCreate turn: run the structured loop with a
// field-validation finalizer, then commit the validated case via Handler.Create.
// runErr is set (for the deferred run-trace Finish) on every terminal failure.
// A create turn never uses the direct fast path (AllowDirect=false), so a
// completed turn always yields a validated CreateDecision in res.Data.
//
// The split between the two failure kinds is deliberate:
//   - Field-validation errors (a non-RFC3339 due_date, a missing required field,
//     an option outside the schema) are the model's fault and the model can fix
//     them, so they run INSIDE planexec's final-output regeneration loop via the
//     finalizer: a rejection is fed back to the planner and the output
//     regenerated. This is the whole point of the change — such an error used to
//     kill the turn with no feedback.
//   - A persistence error from Handler.Create is an infrastructure failure the
//     model cannot fix by re-emitting the same JSON, so it stays OUT of the loop:
//     Create runs once after the turn, and a failure falls back rather than
//     wasting a regeneration cycle re-asking the model to "fix" a write conflict.
func (uc *UseCase) runCreateTurn(ctx context.Context, req TurnRequest, baseReq planexec.RunRequest, runErr *error) (*Result, error) {
	// The finalizer validates the proposed fields against the workspace schema
	// inside the regeneration loop (no side effects) and captures the enriched
	// field values from the accepted attempt. planexec stops on the first
	// accepting finalizer, so validatedFields holds exactly that attempt's result.
	var validatedFields map[string]model.FieldValue
	res, err := planexec.Run[CreateDecision](ctx, uc.runner, baseReq,
		func(d *CreateDecision) error {
			fields, verr := validateCreateDecision(req.Workspace, d)
			if verr != nil {
				return verr
			}
			validatedFields = fields
			return nil
		})
	if err != nil {
		*runErr = goerr.Wrap(err, "run threadcase planexec (create)")
		return nil, *runErr
	}

	switch res.Status {
	case planexec.StatusCompleted:
		if res.EndedWithQuestion {
			uc.persistSession(ctx, req.Session, model.SessionEndedWithQuestion)
			return &Result{Status: StatusQuestion}, nil
		}
		if res.Data == nil {
			*runErr = goerr.New("threadcase create completed without a decision")
			return nil, *runErr
		}
		// The finalizer already validated the fields in-loop; commit the case
		// once, after the turn. A persistence failure is surfaced (recorded FAILED
		// via runErr) and the turn falls back — it is NOT fed back to the model,
		// which cannot repair an infrastructure error.
		c, cerr := req.Handler.Create(ctx, req.Session, CreatePayload{
			Title:       res.Data.Title,
			Description: res.Data.Description,
			Fields:      validatedFields,
		})
		if cerr != nil {
			*runErr = goerr.Wrap(cerr, "create case")
			uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
			return &Result{Status: StatusFallback}, nil
		}
		uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusCompleted, Case: c}, nil
	case planexec.StatusFallbackBudget, planexec.StatusFallbackError:
		*runErr = goerr.New("threadcase planexec ended without a decision",
			goerr.V("status", int(res.Status)),
			goerr.V("reason", res.FallbackReason))
		uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusFallback}, nil
	default:
		*runErr = goerr.New("threadcase planexec returned unknown status",
			goerr.V("status", int(res.Status)))
		return nil, *runErr
	}
}

// runMentionTurn drives a ModeMention turn: run the structured loop and return
// the terminal Decision (respond / materialize) for the host to apply. A direct
// fast-path reply is surfaced as a respond Decision. Closing / status changes
// happened inside the loop via the sub-agent's case__update_case_status tool and
// are NOT represented here.
func (uc *UseCase) runMentionTurn(ctx context.Context, req TurnRequest, baseReq planexec.RunRequest, runErr *error) (*Result, error) {
	res, err := planexec.Run[Decision](ctx, uc.runner, baseReq)
	if err != nil {
		*runErr = goerr.Wrap(err, "run threadcase planexec (mention)")
		return nil, *runErr
	}

	switch res.Status {
	case planexec.StatusCompleted:
		if res.EndedWithQuestion {
			uc.persistSession(ctx, req.Session, model.SessionEndedWithQuestion)
			return &Result{Status: StatusQuestion}, nil
		}
		if res.Direct {
			// Direct path produced a plain-text reply (no structured Decision).
			// Treat it as a respond decision so the host posts it as the thread
			// reply, exactly as it would a parsed respond Decision.
			uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
			return &Result{Status: StatusCompleted, Decision: &Decision{
				Kind:    DecisionRespond,
				Message: res.Text,
			}}, nil
		}
		if res.Data == nil {
			*runErr = goerr.New("threadcase mention completed without a decision")
			return nil, *runErr
		}
		uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusCompleted, Decision: res.Data}, nil
	case planexec.StatusFallbackBudget, planexec.StatusFallbackError:
		*runErr = goerr.New("threadcase planexec ended without a decision",
			goerr.V("status", int(res.Status)),
			goerr.V("reason", res.FallbackReason))
		uc.persistSession(ctx, req.Session, model.SessionEndedWithCaseBoundReply)
		return &Result{Status: StatusFallback}, nil
	default:
		*runErr = goerr.New("threadcase planexec returned unknown status",
			goerr.V("status", int(res.Status)))
		return nil, *runErr
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

// buildToolResolver composes the sub-agent tool resolver. Thread-mode
// workspaces manage no Actions, so the core (action) toolset is omitted entirely
// — investigation reads Slack / Notion / GitHub / the web. The sub-agent's ONE
// write capability is the case status-change tool (case_status_write): it is
// wired only when a concrete case exists (mention / materialize turns), letting
// the sub-agent close / transition that case as the investigation's conclusion.
// Content materialization (title / description / fields) stays with the host, so
// case__update_case is never wired here.
func (uc *UseCase) buildToolResolver(req TurnRequest) *agent.ToolSetResolver {
	d := uc.deps
	wsID := ""
	if req.Workspace != nil {
		wsID = req.Workspace.Workspace.ID
	}
	// The status-change tool is scoped to the case under investigation. A create
	// turn has no case yet (req.Case == nil), so CaseStatus stays zero and the
	// resolver builds no status tool for it.
	var caseStatus casewriter.Deps
	if req.Case != nil {
		caseStatus = casewriter.Deps{
			CaseUC:      d.CaseUC,
			WorkspaceID: wsID,
			CaseID:      req.Case.ID,
			StatusSet:   req.Workspace.CaseStatusSet,
		}
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
		Notion:     notiontool.Deps{Client: d.NotionClient},
		GitHub:     d.GitHubClient,
		WebFetch:   d.WebFetchClient,
		Jira:       d.JiraTools,
		CaseStatus: caseStatus,
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
