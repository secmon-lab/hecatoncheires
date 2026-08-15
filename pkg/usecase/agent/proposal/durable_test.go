package proposal_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/proposal"
)

const (
	draftChannelID = "C-DRAFT"
	draftThreadTS  = "1700000000.000500"
	draftSessionID = "s-draft-1"
	draftActorID   = "U-REQUESTER"
)

// durableCall records one Host call a finished turn made.
type durableCall struct {
	Kind     string // "propose" | "ask" | "fallback"
	Target   proposal.Target
	Draft    proposal.MaterializePayload
	Question proposal.QuestionPayload
	Reason   string
}

type durableHost struct {
	mu    sync.Mutex
	calls []durableCall
	// askErr, when set, is what Ask returns — the Slack post or the Session write
	// failing, after which the thread must NOT be recorded as waiting on a form that
	// does not exist.
	askErr error
}

func (h *durableHost) Propose(_ context.Context, target proposal.Target, m proposal.MaterializePayload) error {
	h.record(durableCall{Kind: "propose", Target: target, Draft: m})
	return nil
}

func (h *durableHost) Ask(_ context.Context, target proposal.Target, q proposal.QuestionPayload) error {
	h.record(durableCall{Kind: "ask", Target: target, Question: q})
	return h.askErr
}

func (h *durableHost) ReportFallback(_ context.Context, target proposal.Target, reason string) error {
	h.record(durableCall{Kind: "fallback", Target: target, Reason: reason})
	return nil
}

func (h *durableHost) record(c durableCall) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, c)
}

func (h *durableHost) Calls() []durableCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]durableCall, len(h.calls))
	copy(out, h.calls)
	return out
}

// kinds returns the Host calls in order, which is the turn's observable outcome.
func (h *durableHost) kinds() []string {
	calls := h.Calls()
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Kind
	}
	return out
}

// durableLLM answers with replies[i] on the i-th Generate. An extra call fails
// rather than repeating the last answer.
func durableLLM(replies ...string) gollem.LLMClient {
	var n atomic.Int32
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					i := int(n.Add(1)) - 1
					if i >= len(replies) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					return &gollem.Response{Texts: []string{replies[i]}, InputToken: 5, OutputToken: 3}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

func durableFailingLLM() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return nil, goerr.New("the model is unreachable")
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

func draftWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace: model.Workspace{ID: "risk", Name: "Risk", Description: "Risk management"},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect,
					Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

type durableHarness struct {
	agent   *proposal.Durable
	host    *durableHost
	repo    *memory.Memory
	kernel  *agentkit.Kernel
	locator agentkernel.Locator
}

// newDurableHarness wires the case-draft agent onto a real Kernel with an
// in-process Process store. The tool factory returns nothing: which tools an agent
// gets is pkg/agent/kernel's contract, and these tests are about this host's own —
// spawn, turn lock, redelivery, and what the completion handler delivers.
func newDurableHarness(t *testing.T, llm gollem.LLMClient) *durableHarness {
	t.Helper()

	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(draftWorkspace())

	procRepo := agentprocmemory.New()
	locator, err := agentkernel.NewLocator(procRepo)
	gt.NoError(t, err).Required()

	host := &durableHost{}
	d, err := proposal.NewDurable(repo, registry, host, locator)
	gt.NoError(t, err).Required()

	store := agentarchive.NewMemoryHistoryStore()
	cfg := budget.Config{MaxSteps: 64, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	reg := agentkit.NewRegistry()
	taskAgent, err := agentkernel.RegisterTaskAgent(reg, cfg.Limiter(), store)
	gt.NoError(t, err).Required()
	gt.NoError(t, d.Register(reg, taskAgent, nil, cfg.Limiter(), store)).Required()

	k, err := agentkit.New(procRepo, llm, reg,
		agentkit.WithToolFactory(func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return nil, nil
		}))
	gt.NoError(t, err).Required()
	d.Bind(k)

	return &durableHarness{agent: d, host: host, repo: repo, kernel: k, locator: locator}
}

// session persists the Session a turn locks on and returns it.
func (h *durableHarness) session(t *testing.T, ctx context.Context) *model.Session {
	t.Helper()
	ssn := &model.Session{
		ID:            draftSessionID,
		ChannelID:     draftChannelID,
		ThreadTS:      draftThreadTS,
		CreatorUserID: draftActorID,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	gt.NoError(t, h.repo.Session().Put(ctx, ssn)).Required()
	return ssn
}

func (h *durableHarness) request(ssn *model.Session, triggerTS string) proposal.TurnRequest {
	return proposal.TurnRequest{
		Session:      ssn,
		UserInput:    "@bot draft a case for the failed deploy",
		Trigger:      proposal.TriggerAppMention,
		TriggerTS:    triggerTS,
		ActorUserID:  draftActorID,
		ProcessingTS: "1700000000.000900",
	}
}

// awaitTerminal drives the worker until pid reaches a terminal state.
func (h *durableHarness) awaitTerminal(t *testing.T, pid agentkit.ProcessID) *agentkit.Process {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() {
		served <- agentkernel.Serve(ctx, h.kernel, agentkit.WithPollInterval(5*time.Millisecond))
	}()

	for {
		proc, err := h.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			cancel()
			<-served
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
			return proc
		case <-time.After(3 * time.Millisecond):
		}
	}
}

func (h *durableHarness) spawned(t *testing.T, triggerTS string) agentkit.ProcessID {
	t.Helper()
	pid, err := h.locator.ByTrigger(context.Background(),
		agentkernel.TriggerKey(draftChannelID, draftThreadTS, triggerTS))
	gt.NoError(t, err).Required()
	gt.Value(t, pid).NotEqual(agentkit.ProcessID(""))
	return pid
}

// run spawns a turn and drives it to completion.
func (h *durableHarness) run(t *testing.T, req proposal.TurnRequest) *agentkit.Process {
	t.Helper()
	res, err := h.agent.StartTurn(context.Background(), req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(proposal.StatusStarted)
	return h.awaitTerminal(t, h.spawned(t, req.TriggerTS))
}

const (
	draftPlan     = `{"tasks":[{"id":"t1","title":"Read the thread","description":"read it","acceptance_criteria":"read","tools":["slack_ro"]}]}`
	draftFinalize = `{"finalize":{"reason":"enough is known"}}`
)

// A finished turn must hand the draft to the host — from the completion handler,
// since StartTurn returned long before the model answered — carrying the
// placeholder the result replaces.
func TestDurableDeliversTheDraft(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the deploy failed at 14:00",
		draftFinalize,
		`{"workspace_id":"risk","title":"Failed deploy","description":"The 14:00 deploy failed.","custom_field_values":{"severity":"high"}}`,
	))
	ssn := h.session(t, ctx)

	proc := h.run(t, h.request(ssn, "1700000001.000001"))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("propose")
	gt.String(t, calls[0].Draft.WorkspaceID).Equal("risk")
	gt.String(t, calls[0].Draft.Title).Equal("Failed deploy")
	gt.String(t, calls[0].Draft.Description).Equal("The 14:00 deploy failed.")
	gt.Value(t, calls[0].Draft.CustomFieldValues["severity"]).Equal("high")
	gt.Value(t, calls[0].Draft.IsTest).Equal(false)
	// The placeholder the result replaces survived the durable boundary.
	gt.Value(t, calls[0].Target.ProcessingTS).Equal("1700000000.000900")
	gt.Value(t, calls[0].Target.ChannelID).Equal(draftChannelID)
	gt.Value(t, calls[0].Target.ThreadTS).Equal(draftThreadTS)

	stored, err := h.repo.Session().GetByThread(ctx, draftChannelID, draftThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.LastAction).Equal(model.SessionEndedWithMaterialize)
}

// A draft naming a workspace this deployment does not have must be fed back and
// regenerated: there is no preview to render for a workspace that is not there.
func TestDurableRegeneratesAnUnknownWorkspace(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the deploy failed",
		draftFinalize,
		`{"workspace_id":"nope","title":"Failed deploy","description":"It failed."}`,
		`{"workspace_id":"risk","title":"Failed deploy","description":"It failed."}`,
	))
	ssn := h.session(t, ctx)

	proc := h.run(t, h.request(ssn, "1700000001.000002"))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("propose")
	gt.String(t, calls[0].Draft.WorkspaceID).Equal("risk")
}

// A field value outside the schema is NOT regenerated: the host already drops it
// and lets the human fill it in the review modal, and that tolerance is what this
// path preserves.
func TestDurableLeavesUnknownFieldValuesToTheHost(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the deploy failed",
		draftFinalize,
		`{"workspace_id":"risk","title":"Failed deploy","description":"It failed.","custom_field_values":{"severity":"critical"}}`,
	))
	ssn := h.session(t, ctx)

	h.run(t, h.request(ssn, "1700000001.000003"))

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("propose")
	gt.Value(t, calls[0].Draft.CustomFieldValues["severity"]).Equal("critical")
}

// A planner question ends the turn: the host posts the form, and the session
// records that it is waiting so a later event resumes rather than restarts.
func TestDurableQuestionEndsTheTurn(t *testing.T) {
	ctx := context.Background()
	// The opening round may not ask — it has nothing to ask about yet — so the
	// question comes on the replan round.
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the thread does not say which environment",
		`{"question":{"reason":"which environment?","items":[{"id":"env","text":"Which environment?","type":"select","options":["staging","production"]}]}}`,
	))
	ssn := h.session(t, ctx)

	proc := h.run(t, h.request(ssn, "1700000001.000004"))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("ask")
	gt.String(t, calls[0].Question.Reason).Equal("which environment?")
	gt.Array(t, calls[0].Question.Items).Length(1).Required()
	gt.String(t, calls[0].Question.Items[0].ID).Equal("env")
	gt.Value(t, calls[0].Question.Items[0].Type).Equal(proposal.QuestionItemSelect)
	gt.Array(t, calls[0].Question.Items[0].Options).Equal([]string{"staging", "production"})

	stored, err := h.repo.Session().GetByThread(ctx, draftChannelID, draftThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.LastAction).Equal(model.SessionEndedWithQuestion)
}

// The turn an answer starts must continue the conversation of the run that asked.
//
// A resume is a NEW Process — its own budget, its own record — and agentkit does
// not carry a conversation across Processes on its own: the subject only serialises
// them. Without the inherited history the answering turn begins from nothing and
// sees only the answer text, with no record of the original request, the
// investigation behind the question, or the question itself.
func TestDurableResumeInheritsTheAskingRunsConversation(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the thread does not say which environment",
		`{"question":{"reason":"which environment?","items":[{"id":"env","text":"Which environment?","type":"select","options":["staging","production"]}]}}`,
		draftPlan,
		"the user says production",
		draftFinalize,
		`{"workspace_id":"risk","title":"Deploy failure","description":"Production deploy failed."}`,
	))
	ssn := h.session(t, ctx)

	asked := h.run(t, h.request(ssn, "1700000001.000051"))
	gt.Value(t, asked.Status).Equal(agentkit.ProcessSucceeded)
	gt.Value(t, asked.HistoryRef).NotEqual(agentkit.HistoryRef(""))

	// The answer turn names the asking run.
	resume := h.request(ssn, "1700000001.000052")
	resume.UserInput = "production"
	resume.Trigger = proposal.TriggerThreadReply
	resume.InheritFrom = string(asked.ID)
	answered := h.run(t, resume)
	gt.Value(t, answered.Status).Equal(agentkit.ProcessSucceeded)

	// The new Process records where its conversation came from, pinned to the
	// version the asking run committed.
	gt.Value(t, answered.InheritedHistory).NotNil().Required()
	gt.Value(t, answered.InheritedHistory.Process).Equal(asked.ID)
	gt.Value(t, answered.InheritedHistory.Ref).Equal(asked.HistoryRef)
}

// A resume whose asking run committed no conversation must still start.
//
// agentkit REFUSES a Spawn that names an issuer with no history, so passing the
// option blindly would turn a question the asking run never got to record into an
// answer that fails outright. Starting fresh is the correct degradation.
func TestDurableResumeStartsFreshWhenThereIsNothingToInherit(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"read the thread",
		draftFinalize,
		`{"workspace_id":"risk","title":"Deploy failure","description":"It failed."}`,
	))
	ssn := h.session(t, ctx)

	resume := h.request(ssn, "1700000001.000061")
	// A process id that never existed stands in for one that recorded nothing:
	// both are "no conversation to continue".
	resume.InheritFrom = "01900000-0000-7000-8000-000000000000"

	proc := h.run(t, resume)
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	gt.Value(t, proc.InheritedHistory).Nil()
	gt.Array(t, h.host.kinds()).Equal([]string{"propose"})
}

// A question the host could not deliver must NOT leave the thread recorded as
// waiting on one.
//
// Whether the Slack post failed or the Session write after it did, there is no form
// to answer. Recording post_question anyway leaves the "working on it" placeholder
// up forever and makes the submit handler read the missing PendingQuestion as
// stale. A turn that could not ask reached no conclusion, so it must fall back —
// which is also what takes the placeholder down and unlocks the draft.
func TestDurableQuestionDeliveryFailureFallsBack(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(
		draftPlan,
		"the thread does not say which environment",
		`{"question":{"reason":"which environment?","items":[{"id":"env","text":"Which environment?","type":"select","options":["staging","production"]}]}}`,
	))
	h.host.askErr = goerr.New("slack refused the form")
	ssn := h.session(t, ctx)

	proc := h.run(t, h.request(ssn, "1700000001.000041"))
	// The run itself succeeded — delivery is the host's half, after the turn.
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	gt.Array(t, h.host.kinds()).Equal([]string{"ask", "fallback"})

	stored, err := h.repo.Session().GetByThread(ctx, draftChannelID, draftThreadTS)
	gt.NoError(t, err).Required()
	gt.Value(t, stored).NotNil().Required()
	gt.Value(t, stored.LastAction).NotEqual(model.SessionEndedWithQuestion)
	gt.Value(t, stored.PendingQuestion).Nil()
}

// A run the model never answers must tell the user, so nobody is left watching a
// "working on it" placeholder forever.
func TestDurableReportsAFailedRun(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableFailingLLM())
	ssn := h.session(t, ctx)

	proc := h.run(t, h.request(ssn, "1700000001.000005"))
	gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)

	calls := h.host.Calls()
	gt.Array(t, calls).Length(1).Required()
	gt.Value(t, calls[0].Kind).Equal("fallback")
	gt.String(t, calls[0].Reason).NotEqual("")
	gt.Value(t, calls[0].Target.ProcessingTS).Equal("1700000000.000900")
}

// The run must carry the requester as its access actor. Without one the usecase
// layer reads the run as a system context and bypasses private-case access
// control, so a cross-case read would see cases this person may not.
func TestDurableRecordsTheActorAndScope(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(draftPlan))
	ssn := h.session(t, ctx)

	_, err := h.agent.StartTurn(ctx, h.request(ssn, "1700000001.000006"))
	gt.NoError(t, err).Required()

	proc, err := h.kernel.GetProcess(ctx, h.spawned(t, "1700000001.000006"))
	gt.NoError(t, err).Required()

	sc := agentkernel.ScopeFrom(proc.Metadata)
	gt.Value(t, sc.ActorUserID).Equal(draftActorID)
	gt.Value(t, sc.SessionID).Equal(draftSessionID)
	gt.Value(t, sc.ChannelID).Equal(draftChannelID)
	// Choosing the workspace is what this run is for, so it names none.
	gt.Value(t, sc.WorkspaceID).Equal("")
	gt.Value(t, sc.CaseID).Equal(int64(0))
	gt.Value(t, proc.Agent).Equal(agentkernel.AgentProposal)
}

// A second mention while a turn is live must be refused, not queued onto the same
// thread.
func TestDurableRefusesASecondTurnOnTheSameThread(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(draftPlan))
	ssn := h.session(t, ctx)

	first, err := h.agent.StartTurn(ctx, h.request(ssn, "1700000001.000007"))
	gt.NoError(t, err).Required()
	gt.Value(t, first.Status).Equal(proposal.StatusStarted)

	second, err := h.agent.StartTurn(ctx, h.request(ssn, "1700000001.000008"))
	gt.NoError(t, err).Required()
	gt.Value(t, second.Status).Equal(proposal.StatusBusy)
	gt.Array(t, h.host.Calls()).Length(0)
}

// A re-delivered Slack event must be dropped silently.
func TestDurableDropsARedeliveredTrigger(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(draftPlan))
	ssn := h.session(t, ctx)

	req := h.request(ssn, "1700000001.000009")
	first, err := h.agent.StartTurn(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, first.Status).Equal(proposal.StatusStarted)

	again, err := h.agent.StartTurn(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, again.Status).Equal(proposal.StatusIdempotent)
	gt.Array(t, h.host.Calls()).Length(0)
}

// The workspace-switch turn is synthetic and has no Slack ts to dedup on, so it
// must still start rather than be mistaken for a duplicate.
func TestDurableStartsATriggerlessWorkspaceSwitch(t *testing.T) {
	ctx := context.Background()
	h := newDurableHarness(t, durableLLM(draftPlan))
	ssn := h.session(t, ctx)

	req := h.request(ssn, "")
	req.Trigger = proposal.TriggerWSSwitch
	req.ProcessingTS = ""
	req.PreviewTS = "1700000000.000700"
	res, err := h.agent.StartTurn(ctx, req)
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(proposal.StatusStarted)
}

func TestNewDurableRejectsMissingDependencies(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(draftWorkspace())

	_, err := proposal.NewDurable(nil, registry, &durableHost{}, nil)
	gt.Error(t, err).Required()

	_, err = proposal.NewDurable(memory.New(), nil, &durableHost{}, nil)
	gt.Error(t, err).Required()

	_, err = proposal.NewDurable(memory.New(), registry, nil, nil)
	gt.Error(t, err).Required()
}

// An unbound host must say so rather than panicking on a nil Kernel.
func TestDurableStartTurnRefusesWhenUnbound(t *testing.T) {
	registry := model.NewWorkspaceRegistry()
	registry.Register(draftWorkspace())
	d, err := proposal.NewDurable(memory.New(), registry, &durableHost{}, nil)
	gt.NoError(t, err).Required()

	_, err = d.StartTurn(context.Background(), proposal.TurnRequest{
		Session:   &model.Session{ID: draftSessionID, ChannelID: draftChannelID, ThreadTS: draftThreadTS},
		UserInput: "draft a case",
	})
	gt.Error(t, err).Required()
}

// Draft.Validate is what stops a shapeless proposal reaching the human.
func TestDraftValidate(t *testing.T) {
	ok := proposal.Draft{WorkspaceID: "risk", Title: "T", Description: "D"}
	gt.NoError(t, ok.Validate())

	for name, d := range map[string]proposal.Draft{
		"no workspace":   {Title: "T", Description: "D"},
		"no title":       {WorkspaceID: "risk", Description: "D"},
		"blank title":    {WorkspaceID: "risk", Title: "   ", Description: "D"},
		"no description": {WorkspaceID: "risk", Title: "T"},
	} {
		t.Run(name, func(t *testing.T) {
			gt.Error(t, d.Validate())
		})
	}
}
