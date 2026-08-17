package proposal

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"sync"
	"text/template"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"

	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// proposalAgentVersion is the strategy state version stamped on every Process
// this agent creates. Bump it only alongside a DecodeState that still reads the
// older shape — a running deployment always has in-flight Processes on it.
const proposalAgentVersion = 1

//go:embed prompts/durable_planner.md
var durablePlannerPromptSrc string

var (
	durablePromptOnce sync.Once
	durablePromptTmpl *template.Template
	durablePromptErr  error
)

// durablePromptInput is what the case-draft persona prompt is rendered from.
//
// Unlike the prompt it replaces this carries no Language field: the language
// directive is planexec's, supplied per run through Input.LanguageLabel.
type durablePromptInput struct {
	Workspaces []plannerPromptWorkspace
	// WorkspaceSwitch marks the turn where the user moved an existing draft to a
	// different workspace, which is answered from the conversation rather than by
	// investigating again.
	WorkspaceSwitch bool
}

// renderDurablePrompt builds the persona prompt for one turn.
func renderDurablePrompt(registry *model.WorkspaceRegistry, wsSwitch bool) (string, error) {
	durablePromptOnce.Do(func() {
		durablePromptTmpl, durablePromptErr = template.New("durable_planner").
			Parse(durablePlannerPromptSrc)
	})
	if durablePromptErr != nil {
		return "", goerr.Wrap(durablePromptErr, "parse the case-draft prompt template")
	}
	var buf bytes.Buffer
	if err := durablePromptTmpl.Execute(&buf, durablePromptInput{
		Workspaces:      workspacePromptEntries(registry),
		WorkspaceSwitch: wsSwitch,
	}); err != nil {
		return "", goerr.Wrap(err, "render the case-draft prompt")
	}
	return buf.String(), nil
}

// Host is the Slack-facing surface a finished case-draft turn needs. Each method
// is called at most once per turn, from the completion handler.
type Host interface {
	// Propose renders the draft into the preview UI the human reviews and submits.
	Propose(ctx context.Context, target Target, payload MaterializePayload) error
	// Ask posts the planner's question and records it on the session; the turn has
	// ended, and the user's answer starts the next one.
	Ask(ctx context.Context, target Target, question QuestionPayload) error
	// ReportFallback tells the user the turn reached no conclusion.
	ReportFallback(ctx context.Context, target Target, reason string) error
}

// Target locates a finished case-draft run's thread and session.
//
// It is rebuilt from the Process metadata rather than captured at spawn, because
// the completion handler runs after the turn — possibly on another instance.
type Target struct {
	SessionID string
	ChannelID string
	ThreadTS  string
	// ActorUserID is the person whose request this draft answers.
	ActorUserID string
	// ProcessingTS and PreviewTS name the message the result replaces, and are
	// mutually exclusive: the "working on it" placeholder a fresh mention posted,
	// or the existing preview a workspace switch updates in place.
	ProcessingTS string
	PreviewTS    string
	// ProposalID is the draft THIS run writes into, carried on the run rather than
	// read back from the Session. The Session's ProposalID is mutable — a later
	// mention repoints it while this run is still going — so reading it here would
	// let one turn's result land in another turn's draft.
	ProposalID model.CaseProposalID
	// ProcessID is the run that produced this outcome. A host that records a
	// question needs it so the turn started by the answer can inherit this run's
	// conversation.
	ProcessID string
}

// Durable runs the case-draft agent on the agentkit runtime.
//
// It coexists with the in-process planner loop: a deployment that has not wired
// this keeps taking UseCase.RunTurn's synchronous path.
type Durable struct {
	repo     interfaces.Repository
	registry *model.WorkspaceRegistry
	host     Host
	locator  agentkernel.Locator

	agent  agentkit.Agent[planexec.Input]
	kernel *agentkit.Kernel
	// probe filters the planner palette down to the toolset ids that resolve to a
	// tool for this run. nil leaves the palette unfiltered.
	probe *agentkernel.ToolSetProbe
}

// NewDurable builds the durable case-draft host. locator is used only to tell a
// re-delivered Slack event from a busy thread; a nil locator makes every delivery
// look fresh, which the idempotency key still covers.
func NewDurable(repo interfaces.Repository, registry *model.WorkspaceRegistry,
	host Host, locator agentkernel.Locator,
) (*Durable, error) {
	if repo == nil {
		return nil, goerr.New("repository is required")
	}
	if registry == nil {
		return nil, goerr.New("workspace registry is required")
	}
	if host == nil {
		return nil, goerr.New("host is required")
	}
	return &Durable{repo: repo, registry: registry, host: host, locator: locator}, nil
}

// Register registers the case-draft agent and wires this host as its completion
// handler. Call it before building the Kernel, and Bind after.
func (d *Durable) Register(
	reg *agentkit.Registry, taskAgent agentkit.Agent[react.Input],
	progress planexec.Progress, limiter agentkit.Limiter, store agentkit.HistoryStore,
) error {
	if d == nil {
		return goerr.New("durable case-draft agent is nil")
	}
	handle, err := planexec.Register(reg, agentkernel.AgentProposal, proposalAgentVersion,
		taskAgent, progress, limiter,
		planexec.Config[Draft]{
			// The workspace and its field values are checked against the registry
			// inside the regeneration loop, so a bad option id is fed back and the
			// draft re-emitted rather than reaching the human as a broken preview.
			Finalizers: []planexec.Finalizer[Draft]{d.validateAgainstRegistry},
		},
		agentkit.WithHistoryStore[planexec.Output[Draft]](store),
		agentkit.WithOnFinish(d.onFinish),
	)
	if err != nil {
		return goerr.Wrap(err, "register the case-draft agent")
	}
	d.agent = handle
	return nil
}

// Bind hands over the Kernel the registered agent runs on, and the probe that
// tells this host which toolset ids actually resolve to a tool for a given run.
func (d *Durable) Bind(k *agentkit.Kernel, probe *agentkernel.ToolSetProbe) {
	if d != nil {
		d.kernel = k
		d.probe = probe
	}
}

// ready reports whether a turn can be spawned.
func (d *Durable) ready() bool {
	return d != nil && d.kernel != nil && d.agent.Name() != ""
}

// StartTurn spawns one case-draft turn and returns as soon as the run is
// recorded. The draft, or the question, is delivered by the completion handler.
func (d *Durable) StartTurn(ctx context.Context, req TurnRequest) (*Result, error) {
	if !d.ready() {
		return nil, goerr.New("durable case-draft agent is not bound to an agent runtime")
	}
	if err := validateTurnRequest(&req); err != nil {
		return nil, err
	}
	if req.UserInput == "" {
		return nil, goerr.New("UserInput is required (it is the planner's first message)")
	}

	systemPrompt, err := renderDurablePrompt(d.registry, req.Trigger == TriggerWSSwitch)
	if err != nil {
		return nil, err
	}

	// The draft this run writes into is pinned to the run here. It is empty only
	// when the thread has no draft left (submitted or cancelled before the reply
	// arrived), which the completion handler reports as a fallback.
	var proposalID model.CaseProposalID
	if req.ExistingProposal != nil {
		proposalID = req.ExistingProposal.ID
	}

	scope := agentkernel.Scope{
		ChannelID: req.Session.ChannelID,
		ThreadTS:  req.Session.ThreadTS,
		SessionID: req.Session.ID,
		// The requester is the access actor for the whole run. Without one the
		// usecase layer reads the run as a system context and BYPASSES private-case
		// access control, so a `core_ro` read would see cases this person may not.
		ActorUserID: req.ActorUserID,
		Lang:        string(i18n.LangFromContext(ctx)),
		ToolSets:    []string{agentkernel.ToolSetsAll},
		// No WorkspaceID: choosing one is what this run is for.
		ProcessingTS: req.ProcessingTS,
		PreviewTS:    req.PreviewTS,
		ProposalID:   string(proposalID),
	}
	if err := agentkernel.ValidateSpawn(agentkernel.AgentProposal, scope); err != nil {
		return nil, goerr.Wrap(err, "validate the case-draft turn scope")
	}

	// Asking the store first is what tells a re-delivery apart from a busy thread:
	// Spawn resolves an idempotency key silently, returning the existing id without
	// saying that it is existing.
	if d.alreadyStarted(ctx, req) {
		return &Result{Status: StatusIdempotent}, nil
	}

	opts := []agentkit.SpawnOption{
		agentkit.WithSubject(agentkernel.ThreadSubject(req.Session.ID)),
		agentkit.WithMetadata(scope.Metadata()),
	}
	// A synthetic trigger (the workspace switch) has no Slack ts to dedup on, and
	// the turn-lock layer treated an empty trigger key the same way.
	if req.TriggerTS != "" {
		opts = append(opts, agentkit.WithIdempotencyKey(agentkernel.TriggerKey(
			req.Session.ChannelID, req.Session.ThreadTS, req.TriggerTS)))
	}
	opts = append(opts, d.inheritOpts(ctx, req.InheritFrom)...)

	// No workspace and no case yet — choosing one is what this run is for — so the
	// only ids a sub-agent can be given are the thread the request came from.
	taskContext, err := agent.TaskContext{
		SlackChannelID: req.Session.ChannelID,
		SlackThreadTS:  req.Session.ThreadTS,
	}.Render()
	if err != nil {
		return nil, err
	}

	knownToolIDs, err := d.probe.Available(ctx, scope, agent.KnownToolSetIDsProposal)
	if err != nil {
		return nil, goerr.Wrap(err, "resolve the case-draft tool palette",
			goerr.V("session_id", req.Session.ID))
	}

	_, err = d.agent.Spawn(ctx, d.kernel, planexec.Input{
		SystemPrompt:  systemPrompt,
		UserInput:     req.UserInput,
		LanguageLabel: plannerLanguageLabel(ctx),
		KnownToolIDs:  knownToolIDs,
		TaskContext:   taskContext,
		AllowQuestion: true,
		// No direct path: this turn's job is to produce a draft, and the direct path
		// answers in prose. No sub-agent writes either — nothing is committed until
		// a human submits the preview.
		AllowDirect:         false,
		AllowSubAgentWrites: false,
	}, opts...)
	switch {
	case errors.Is(err, agentkit.ErrSubjectBusy):
		return &Result{Status: StatusBusy}, nil
	case err != nil:
		return nil, goerr.Wrap(err, "spawn the case-draft agent",
			goerr.V("session_id", req.Session.ID))
	}
	return &Result{Status: StatusStarted}, nil
}

// inheritOpts returns the option that continues prevID's conversation in the turn
// about to be spawned, or nothing when there is nothing to continue.
//
// It checks the issuing run first because the Kernel REFUSES a Spawn whose issuer
// committed no conversation. Passing the option blindly would turn "the run that
// asked never got far enough to record anything" into "the answer fails outright",
// which is strictly worse than answering from a fresh conversation.
func (d *Durable) inheritOpts(ctx context.Context, prevID string) []agentkit.SpawnOption {
	if prevID == "" {
		return nil
	}
	proc, err := d.kernel.GetProcess(ctx, agentkit.ProcessID(prevID))
	if err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "read the run whose conversation this turn continues",
			goerr.V("from", prevID)), "read the run whose conversation this turn continues")
		return nil
	}
	if proc == nil || proc.HistoryRef == "" {
		return nil
	}
	return []agentkit.SpawnOption{agentkit.WithInheritedHistory(agentkit.ProcessID(prevID))}
}

// validateAgainstRegistry is the draft's finalizer: it checks that the proposed
// workspace is one this deployment actually has. Registration happens once at
// startup, so it reads the registry rather than closing over one workspace.
//
// The FIELD VALUES are deliberately not checked here. The host already drops a
// field id outside the schema and a value it cannot coerce, leaving the human to
// fill it in the review modal — and that tolerance is the behaviour being
// preserved. Rejecting here instead would spend regeneration rounds on a draft
// the human could have fixed with one click. A workspace that does not exist is
// different: there is no preview to render at all.
func (d *Durable) validateAgainstRegistry(_ context.Context, _ map[string]string, out *Draft) error {
	if out == nil {
		return goerr.New("the draft is empty")
	}
	if _, err := d.registry.Get(out.WorkspaceID); err != nil {
		return goerr.Wrap(err, "the proposed workspace is not registered",
			goerr.V("workspace_id", out.WorkspaceID))
	}
	return nil
}

// alreadyStarted reports whether a previous delivery of this Slack event already
// started a run.
func (d *Durable) alreadyStarted(ctx context.Context, req TurnRequest) bool {
	if d.locator == nil || req.TriggerTS == "" {
		return false
	}
	key := agentkernel.TriggerKey(req.Session.ChannelID, req.Session.ThreadTS, req.TriggerTS)
	pid, err := d.locator.ByTrigger(ctx, key)
	if err != nil {
		// Not knowing means treating it as a fresh delivery: dropping a real mention
		// is worse than the duplicate a wrong guess would cause, and the idempotency
		// key still stops a second run from being created.
		errutil.Handle(ctx, goerr.Wrap(err, "look up the run for this trigger"),
			"look up the run for this trigger")
		return false
	}
	return pid != ""
}

// onFinish delivers a finished turn's draft or question.
func (d *Durable) onFinish(ctx context.Context, pid agentkit.ProcessID,
	res agentkit.FinishResult[planexec.Output[Draft]],
) error {
	proc, err := d.kernel.GetProcess(ctx, pid)
	if err != nil {
		return goerr.Wrap(err, "read the finished run", goerr.V("process", pid))
	}
	sc := agentkernel.ScopeFrom(proc.Metadata)
	target := Target{
		SessionID:    sc.SessionID,
		ChannelID:    sc.ChannelID,
		ThreadTS:     sc.ThreadTS,
		ActorUserID:  sc.ActorUserID,
		ProcessingTS: sc.ProcessingTS,
		PreviewTS:    sc.PreviewTS,
		ProposalID:   model.CaseProposalID(sc.ProposalID),
		ProcessID:    string(pid),
	}

	switch {
	case res.Status == agentkit.ProcessSucceeded && res.Output != nil:
		out := *res.Output
		switch out.Kind {
		case planexec.OutputFinal:
			d.deliver(ctx, target, out.Data)
		case planexec.OutputQuestion:
			d.ask(ctx, target, out.Question)
		default:
			d.reportFallback(ctx, target, fallbackReason(out.FallbackReason))
			d.endSession(ctx, target, model.SessionEndedWithMaterialize)
		}
	case res.Status == agentkit.ProcessFailed:
		d.reportFallback(ctx, target, failureError(res.Failure).Error())
		d.endSession(ctx, target, model.SessionEndedWithMaterialize)
	case res.Status == agentkit.ProcessCancelled:
		// No user-facing message: someone stopped this deliberately. The outcome is
		// still stamped, because "how did the last turn end" must have an answer for
		// every way a turn can end — a thread left unstamped reads as one whose turn
		// never finished, and anything waiting on that answer waits forever.
		d.endSession(ctx, target, model.SessionEndedWithMaterialize)
	}
	return nil
}

// deliver hands a finished draft to the preview UI.
func (d *Durable) deliver(ctx context.Context, target Target, draft *Draft) {
	if draft == nil {
		d.reportFallback(ctx, target, "the turn finished with no draft")
		d.endSession(ctx, target, model.SessionEndedWithMaterialize)
		return
	}
	if err := d.host.Propose(ctx, target, MaterializePayload{
		WorkspaceID:       draft.WorkspaceID,
		Title:             draft.Title,
		Description:       draft.Description,
		CustomFieldValues: draft.CustomFieldValues,
		IsTest:            draft.IsTest,
	}); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "render the case draft"), "render the case draft")
		d.reportFallback(ctx, target, err.Error())
	}
	d.endSession(ctx, target, model.SessionEndedWithMaterialize)
}

// ask posts the planner's question and records that the session is waiting.
func (d *Durable) ask(ctx context.Context, target Target, q *planexec.Question) {
	if q == nil {
		d.reportFallback(ctx, target, "the turn ended on a question with nothing to ask")
		return
	}
	payload := QuestionPayload{Reason: q.Reason, Items: make([]QuestionItem, len(q.Items))}
	for i, it := range q.Items {
		payload.Items[i] = QuestionItem{
			ID:      it.ID,
			Text:    it.Text,
			Type:    QuestionItemType(it.Type),
			Options: it.Options,
		}
	}
	// The question state is confirmed only once the form is actually posted AND
	// recorded. Stamping it after a failed Ask would leave the thread claiming to be
	// waiting on a form that does not exist — the submit handler would read the
	// missing PendingQuestion as stale and drop the answer — while the "working on
	// it" placeholder stayed up forever. A failure is a turn that reached no
	// conclusion, which is what fallback already means.
	if err := d.host.Ask(ctx, target, payload); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "post the case-draft question"),
			"post the case-draft question")
		d.reportFallback(ctx, target, err.Error())
		d.endSession(ctx, target, model.SessionEndedWithMaterialize)
		return
	}
	d.endSession(ctx, target, model.SessionEndedWithQuestion)
}

func (d *Durable) reportFallback(ctx context.Context, target Target, reason string) {
	if err := d.host.ReportFallback(ctx, target, reason); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "report the case-draft fallback"),
			"report the case-draft fallback")
	}
}

// endSession stamps how the turn ended. The host reads it to decide whether a
// later event resumes a pending question or starts a fresh turn.
//
// It is a one-field repository call rather than a read-modify-write. This runs
// after the turn, and agentkit released the thread's subject at the terminal
// commit — so by now a later turn may be running and writing the same row. A full
// write from here would restore this turn's stale copy of whatever that turn
// recorded, the mention cursor included.
func (d *Durable) endSession(ctx context.Context, target Target, ended model.SessionEndReason) {
	if target.ChannelID == "" || target.ThreadTS == "" {
		return
	}
	if err := d.repo.Session().StampLastAction(ctx, target.ChannelID, target.ThreadTS, ended); err != nil {
		errutil.Handle(ctx, goerr.Wrap(err, "stamp the session outcome",
			goerr.V("session_id", target.SessionID)), "stamp the session outcome")
	}
}

// fallbackReason renders a turn that reached no conclusion.
func fallbackReason(reason string) string {
	if reason == "" {
		return "the turn reached no conclusion"
	}
	return reason
}

func failureError(f *agentkit.Failure) error {
	if f == nil {
		return goerr.New("the run failed")
	}
	if f.Message == "" {
		return goerr.New("the run failed", goerr.V("code", string(f.Code)))
	}
	return goerr.New(f.Message, goerr.V("code", string(f.Code)))
}
