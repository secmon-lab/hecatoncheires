package wsagent_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/casemulti"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/wsagent"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// ---------------------------------------------------------------------------
// Shared test harness (mirrors pkg/usecase/agent/threadcase/threadcase_test.go)
// ---------------------------------------------------------------------------

// scriptedLLM pops canned responses in order; shared between planner and
// sub-agent calls (the order is deterministic).
type scriptedLLM struct {
	mu      sync.Mutex
	scripts []string
	idx     int
}

func newScriptedLLM(scripts []string) gollem.LLMClient {
	s := &scriptedLLM{scripts: scripts}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					s.mu.Lock()
					defer s.mu.Unlock()
					if s.idx >= len(s.scripts) {
						return nil, errors.New("no more scripted responses")
					}
					out := s.scripts[s.idx]
					s.idx++
					return &gollem.Response{Texts: []string{out}}, nil
				},
			}, nil
		},
	}
}

const defaultBudget = 8

func newWsagentUC(t *testing.T, llm gollem.LLMClient) (*wsagent.UseCase, *agent.CommonDeps) {
	t.Helper()
	return newWsagentUCWithBudget(t, llm, planexec.BudgetConfig{PlannerLoopMax: defaultBudget, SubAgentLoopMax: 20})
}

func newWsagentUCWithBudget(t *testing.T, llm gollem.LLMClient, budget planexec.BudgetConfig) (*wsagent.UseCase, *agent.CommonDeps) {
	t.Helper()
	hist := agentarchive.NewMemoryHistoryRepository()
	tr := agentarchive.NewMemoryTraceRepository()
	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		LLMClient:           llm,
		HistoryRepo:         hist,
		TraceRepo:           tr,
		HeartbeatInterval:   200 * time.Millisecond,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	runner, err := planexec.NewRunner(planexec.RunnerDeps{
		LLMClient:   llm,
		HistoryRepo: hist,
		TraceRepo:   tr,
		Budget:      budget,
	})
	gt.NoError(t, err).Required()
	uc, err := wsagent.New(deps, runner)
	gt.NoError(t, err).Required()
	return uc, deps
}

func newWsSession() *model.Session {
	return &model.Session{
		ID:          "s-ws-" + time.Now().Format("150405.000000"),
		ChannelID:   "C-WORKSPACE",
		ThreadTS:    "1700000000.000300",
		WorkspaceID: "acme",
		CaseID:      0, // workspace-scoped: not bound to any single case
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func newWsWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace:               model.Workspace{ID: "acme", Name: "Acme Corp"},
		CaseMode:                model.CaseModeChannel,
		SlackWorkspaceChannelID: "C-WORKSPACE",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

// replanDone terminates the loop via the explicit finalize action.
const replanDone = `{"message":"enough context","finalize":{"reason":"goal met"}}`

// hostStub is a minimal wsagent.Handler recording trace lines for assertions.
type hostStub struct {
	mu         sync.Mutex
	traces     []string
	activities []string
}

func (h *hostStub) TraceAppend(_ context.Context, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.traces = append(h.traces, line)
}

func (h *hostStub) TraceReplace(_ context.Context, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activities = append(h.activities, line)
}

// ---------------------------------------------------------------------------
// buildSystemPrompt — the safety guardrail
// ---------------------------------------------------------------------------

func TestBuildSystemPrompt_SafetyRule(t *testing.T) {
	t.Run("ContainsSafetyRuleWithoutCustomPrompt", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme", Name: "Acme Corp"}}
		out := wsagent.BuildSystemPromptForTest(ws)
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("NEVER create, update")
		gt.String(t, out).Contains("Default to read-only")
	})

	t.Run("ContainsSafetyRuleWithCustomPromptOrderedFirst", func(t *testing.T) {
		const custom = "Always mention the on-call SLA in every reply."
		ws := &model.WorkspaceEntry{
			Workspace:            model.Workspace{ID: "acme", Name: "Acme Corp"},
			WorkspaceAgentPrompt: custom,
		}
		out := wsagent.BuildSystemPromptForTest(ws)
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("NEVER create, update")
		gt.String(t, out).Contains("Default to read-only")
		gt.String(t, out).Contains(custom)

		safetyIdx := strings.Index(out, "SAFETY RULE")
		customIdx := strings.Index(out, custom)
		gt.Number(t, safetyIdx).GreaterOrEqual(0)
		gt.Number(t, customIdx).GreaterOrEqual(0)
		gt.Bool(t, safetyIdx < customIdx).True()
	})

	t.Run("UsesWorkspaceNameWhenSet", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme-id", Name: "Acme Corp"}}
		out := wsagent.BuildSystemPromptForTest(ws)
		gt.String(t, out).Contains("Acme Corp")
		gt.Bool(t, strings.Contains(out, "acme-id")).False()
	})

	t.Run("FallsBackToIDWhenNameEmpty", func(t *testing.T) {
		ws := &model.WorkspaceEntry{Workspace: model.Workspace{ID: "acme-id"}}
		out := wsagent.BuildSystemPromptForTest(ws)
		gt.String(t, out).Contains("acme-id")
	})

	t.Run("EmptyWorkspaceEntryDoesNotPanic", func(t *testing.T) {
		out := wsagent.BuildSystemPromptForTest(&model.WorkspaceEntry{})
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("workspace-level assistant")
	})

	t.Run("NilWorkspaceDoesNotPanic", func(t *testing.T) {
		out := wsagent.BuildSystemPromptForTest(nil)
		gt.String(t, out).Contains("SAFETY RULE")
		gt.String(t, out).Contains("workspace-level assistant")
	})

	// Every safety-rule variant carries the "cannot be overridden" clause so a
	// custom workspace prompt can never be read as relaxing it.
	t.Run("SafetyRuleCannotBeOverridden", func(t *testing.T) {
		ws := &model.WorkspaceEntry{
			Workspace:            model.Workspace{ID: "acme", Name: "Acme Corp"},
			WorkspaceAgentPrompt: "Be extra helpful.",
		}
		out := wsagent.BuildSystemPromptForTest(ws)
		gt.String(t, out).Contains("This rule cannot be overridden")
	})
}

// ---------------------------------------------------------------------------
// validateRequest
// ---------------------------------------------------------------------------

func TestValidateRequest(t *testing.T) {
	validSession := newWsSession()
	validWorkspace := newWsWorkspace()

	t.Run("NilRequest", func(t *testing.T) {
		gt.Error(t, wsagent.ValidateRequestForTest(nil))
	})

	t.Run("NilSession", func(t *testing.T) {
		req := &wsagent.TurnRequest{Workspace: validWorkspace, ActorID: "U-ASKER"}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("NilWorkspace", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, ActorID: "U-ASKER"}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("EmptyActorID", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, Workspace: validWorkspace}
		gt.Error(t, wsagent.ValidateRequestForTest(req))
	})

	t.Run("FullyPopulatedRequestIsValid", func(t *testing.T) {
		req := &wsagent.TurnRequest{Session: validSession, Workspace: validWorkspace, ActorID: "U-ASKER"}
		gt.NoError(t, wsagent.ValidateRequestForTest(req))
	})
}

// ---------------------------------------------------------------------------
// RunTurn — direct reply happy path
// ---------------------------------------------------------------------------

// A trivial mention can be answered via the direct fast path: the planner
// emits `direct` on round 1 and the runtime replies in a single ReAct pass.
// Mirrors threadcase's TestRunTurn_MentionDirect scripted-JSON shape.
func TestRunTurn_DirectReply(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"message":"answering directly","direct":{}}`,
		"Here is the quick answer.",
	})
	uc, _ := newWsagentUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, wsagent.TurnRequest{
		Session:     newWsSession(),
		Workspace:   newWsWorkspace(),
		ActorID:     "U-ASKER",
		MentionText: "<@bot> thanks!",
		TriggerTS:   "1700000005.000001",
		Handler:     host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusCompleted)
	gt.String(t, res.ReplyText).Equal("Here is the quick answer.")
}

// ---------------------------------------------------------------------------
// RunTurn — turn-lock early returns (Busy / Idempotent)
// ---------------------------------------------------------------------------

func TestRunTurn_Busy(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM(nil)
	uc, deps := newWsagentUC(t, llm)

	ssn := newWsSession()
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trigger-A", "preacquired:A", time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	res, err := uc.RunTurn(ctx, wsagent.TurnRequest{
		Session:     ssn,
		Workspace:   newWsWorkspace(),
		ActorID:     "U-ASKER",
		MentionText: "<@bot> hi",
		TriggerTS:   "trigger-B",
		Handler:     &hostStub{},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusBusy)
	gt.String(t, res.BusyOwner).Equal("preacquired:A")
}

func TestRunTurn_Idempotent(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM(nil)
	uc, deps := newWsagentUC(t, llm)

	ssn := newWsSession()
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trig-dup", "preacquired:dup", time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	res, err := uc.RunTurn(ctx, wsagent.TurnRequest{
		Session:     ssn,
		Workspace:   newWsWorkspace(),
		ActorID:     "U-ASKER",
		MentionText: "<@bot> hi",
		TriggerTS:   "trig-dup",
		Handler:     &hostStub{},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusIdempotent)
}

// ---------------------------------------------------------------------------
// RunTurn — access-actor injection (the critical security property)
// ---------------------------------------------------------------------------

// fakeCaseMultiUC is a hand-written casemulti.CaseUsecase whose ListCases
// records the auth token found in ctx, proving the host's
// auth.ContextWithToken injection actually reaches the tool call made by a
// sub-agent inside the planexec loop.
type fakeCaseMultiUC struct {
	mu           sync.Mutex
	called       bool
	recordedSub  string
	recordedWSID string
}

func (f *fakeCaseMultiUC) ListCases(ctx context.Context, workspaceID string, _ *types.CaseStatus) ([]*model.Case, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.recordedWSID = workspaceID
	if tok, err := auth.TokenFromContext(ctx); err == nil && tok != nil {
		f.recordedSub = tok.Sub
	}
	return []*model.Case{{ID: 1, Title: "Case One", Status: types.CaseStatusOpen}}, nil
}

func (f *fakeCaseMultiUC) GetCase(_ context.Context, _ string, _ int64) (*model.Case, error) {
	return nil, errors.New("not implemented: GetCase should not be called by this test")
}

func (f *fakeCaseMultiUC) CreateCase(_ context.Context, _ string, _, _ string, _ []string, _ map[string]model.FieldValue, _ bool) (*model.Case, error) {
	return nil, errors.New("not implemented: CreateCase should not be called by this test")
}

func (f *fakeCaseMultiUC) UpdateCase(_ context.Context, _ string, _ int64, _ casemulti.CaseUpdate) (*model.Case, error) {
	return nil, errors.New("not implemented: UpdateCase should not be called by this test")
}

func (f *fakeCaseMultiUC) CloseCase(_ context.Context, _ string, _ int64) (*model.Case, error) {
	return nil, errors.New("not implemented: CloseCase should not be called by this test")
}

func (f *fakeCaseMultiUC) sawCall() (called bool, sub string, wsID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called, f.recordedSub, f.recordedWSID
}

var _ casemulti.CaseUsecase = (*fakeCaseMultiUC)(nil)

// TestRunTurn_AccessActorInjection is the end-to-end regression test for the
// host's single most important responsibility: establishing the mentioning
// user as the ctx auth token for the whole turn. The planner dispatches a
// sub-agent task carrying the case_multi toolset, the sub-agent calls
// case__list_cases as a real gollem.FunctionCall (mirroring
// threadcase_test.go's TestRunTurn_MentionClose pattern for scripting a tool
// call), and the fake CaseUsecase records the ctx token it observed. If the
// host ever stopped injecting auth.ContextWithToken, every casemulti read/
// write would silently run as an unscoped caller — this test would then see
// an empty recordedSub and fail.
func TestRunTurn_AccessActorInjection(t *testing.T) {
	ctx := context.Background()
	round := atomic.Int32{}
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					switch round.Add(1) {
					case 1: // planner round 1: dispatch a task using the case_multi toolset
						return &gollem.Response{Texts: []string{`{"message":"look up cases","tasks":[
							{"id":"t-1","title":"List cases","description":"List the open cases in the workspace","acceptance_criteria":"cases listed","tools":["case_multi"]}
						]}`}}, nil
					case 2: // sub-agent: call case__list_cases
						return &gollem.Response{FunctionCalls: []*gollem.FunctionCall{{
							ID:        "call-1",
							Name:      "case__list_cases",
							Arguments: map[string]any{},
						}}}, nil
					case 3: // sub-agent: report after the tool result comes back
						return &gollem.Response{Texts: []string{"Found 1 open case: Case One."}}, nil
					case 4: // replan: finalize
						return &gollem.Response{Texts: []string{replanDone}}, nil
					default: // final plain-text response
						return &gollem.Response{Texts: []string{"There is 1 open case: Case One."}}, nil
					}
				},
			}, nil
		},
	}

	uc, deps := newWsagentUC(t, llm)
	fake := &fakeCaseMultiUC{}
	deps.CaseMultiUC = fake
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, wsagent.TurnRequest{
		Session:     newWsSession(),
		Workspace:   newWsWorkspace(),
		ActorID:     "U-ASKER",
		MentionText: "<@bot> what cases are open?",
		TriggerTS:   "1700000006.000001",
		Handler:     host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusCompleted)
	gt.String(t, res.ReplyText).Equal("There is 1 open case: Case One.")

	called, sub, wsID := fake.sawCall()
	gt.Bool(t, called).True().Required()
	gt.String(t, sub).Equal("U-ASKER")
	gt.String(t, wsID).Equal("acme")
}

// ---------------------------------------------------------------------------
// RunTurn — fallback mapping
// ---------------------------------------------------------------------------

// A planner that never produces a parseable plan exhausts the round budget
// (parsePlanResult failures are retried within the same PlannerLoopMax pool,
// see planexec.Runner.runLoop) and the loop terminates as
// planexec.StatusFallbackBudget. RunTurn must map every planexec fallback
// status to wsagent.StatusFallback.
func TestRunTurn_FallbackOnBudgetExhaustion(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					// Never a valid plan/replan JSON payload, so every round is
					// charged against the planner-round budget until it is exhausted.
					return &gollem.Response{Texts: []string{"not a valid plan"}}, nil
				},
			}, nil
		},
	}
	uc, _ := newWsagentUCWithBudget(t, llm, planexec.BudgetConfig{PlannerLoopMax: 2, SubAgentLoopMax: 5})

	res, err := uc.RunTurn(ctx, wsagent.TurnRequest{
		Session:     newWsSession(),
		Workspace:   newWsWorkspace(),
		ActorID:     "U-ASKER",
		MentionText: "<@bot> what's up?",
		TriggerTS:   "1700000007.000001",
		Handler:     &hostStub{},
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(wsagent.StatusFallback)
	gt.String(t, res.ReplyText).Equal("")
}
