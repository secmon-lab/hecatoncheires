package threadcase_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/threadcase"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

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

type hostStub struct {
	mu         sync.Mutex
	traces     []string
	activities []string
	questions  []threadcase.QuestionPayload
	creates    []threadcase.CreatePayload
	// createErr, when set, is returned by Create for the first n calls to
	// exercise the OnFinalize re-plan path; createErrRemaining decrements.
	createErr          error
	createErrRemaining int
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

func (h *hostStub) Question(_ context.Context, _ *model.Session, q threadcase.QuestionPayload) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.questions = append(h.questions, q)
	return nil
}

func (h *hostStub) Create(_ context.Context, ssn *model.Session, p threadcase.CreatePayload) (*model.Case, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.creates = append(h.creates, p)
	if h.createErrRemaining > 0 {
		h.createErrRemaining--
		return nil, h.createErr
	}
	return &model.Case{
		ID:          1,
		Title:       p.Title,
		Description: p.Description,
		FieldValues: p.Fields,
		ReporterID:  ssn.CreatorUserID,
	}, nil
}

func newThreadcaseUC(t *testing.T, llm gollem.LLMClient) (*threadcase.UseCase, *agent.CommonDeps) {
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
		Budget:      planexec.BudgetConfig{PlannerLoopMax: 8, SubAgentLoopMax: 20},
	})
	gt.NoError(t, err).Required()
	uc, err := threadcase.New(deps, runner)
	gt.NoError(t, err).Required()
	return uc, deps
}

func newThreadSession() *model.Session {
	return &model.Session{
		ID:          "s-thread-" + time.Now().Format("150405.000000"),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		WorkspaceID: "support",
		CaseID:      42,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func newThreadWorkspace() *model.WorkspaceEntry {
	set, _ := model.NewActionStatusSet("TRIAGE", []string{"DONE"}, []model.ActionStatusDefinition{
		{ID: "TRIAGE", Name: "Triage"},
		{ID: "DONE", Name: "Done"},
	})
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		CaseStatusSet:         set,
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
			},
		},
	}
}

func newThreadCase() *model.Case {
	return &model.Case{
		ID:             42,
		Title:          "Initial title",
		Status:         types.CaseStatusOpen,
		ReporterID:     "U-REPORTER",
		SlackChannelID: "C-MONITOR",
		SlackThreadTS:  "1700000000.000100",
		BoardStatus:    "TRIAGE",
	}
}

// investigatePlan is the round-1 plan that runs one read-only sub-agent.
const investigatePlan = `{"message":"investigate the thread","tasks":[{"id":"t-1","title":"Review thread","description":"Review the message","acceptance_criteria":"reviewed","tools":["core_ro"]}]}`

const replanDone = `{"message":"enough context","tasks":[]}`

func TestRunTurn_MentionRespond(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"Observed the user's question.",
		replanDone,
		`{"kind":"respond","message":"Here is what I found."}`,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:     newThreadSession(),
		Workspace:   newThreadWorkspace(),
		Case:        newThreadCase(),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		MentionTS:   "1700000001.000001",
		MentionText: "<@bot> any update?",
		TriggerTS:   "1700000001.000001",
		Mode:        threadcase.ModeMention,
		Handler:     host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Decision).NotNil().Required()
	gt.Value(t, res.Decision.Kind).Equal(threadcase.DecisionRespond)
	gt.String(t, res.Decision.Message).Equal("Here is what I found.")
}

func TestRunTurn_Materialize(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"The message reports a login failure on production.",
		replanDone,
		`{"kind":"materialize","title":"Login failure","description":"User reports a login failure on production.","fields":[{"field_id":"severity","value":"high"}]}`,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:   newThreadSession(),
		Workspace: newThreadWorkspace(),
		Case:      newThreadCase(),
		ChannelID: "C-MONITOR",
		ThreadTS:  "1700000000.000100",
		TriggerTS: "1700000000.000100",
		Mode:      threadcase.ModeMaterialize,
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Decision).NotNil().Required()
	gt.Value(t, res.Decision.Kind).Equal(threadcase.DecisionMaterialize)
	gt.String(t, res.Decision.Title).Equal("Login failure")
	gt.Array(t, res.Decision.Fields).Length(1).Required()
	gt.Value(t, res.Decision.Fields[0].FieldID).Equal("severity")
	gt.Value(t, res.Decision.Fields[0].Value).Equal("high")
}

func TestRunTurn_Close(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"The thread says the issue is resolved.",
		replanDone,
		`{"kind":"close","message":"Resolved per the thread.","close_status":"DONE"}`,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:     newThreadSession(),
		Workspace:   newThreadWorkspace(),
		Case:        newThreadCase(),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		MentionTS:   "1700000002.000001",
		MentionText: "<@bot> looks resolved, please close",
		TriggerTS:   "1700000002.000001",
		Mode:        threadcase.ModeMention,
		Handler:     host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Decision).NotNil().Required()
	gt.Value(t, res.Decision.Kind).Equal(threadcase.DecisionClose)
	gt.Value(t, res.Decision.CloseStatus).Equal("DONE")
}

func TestRunTurn_Question(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"Need to know the environment.",
		`{"message":"need more info","question":{"reason":"Which environment?","items":[{"id":"q1","text":"Which environment?","type":"select","options":["prod","staging"]}]}}`,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:     newThreadSession(),
		Workspace:   newThreadWorkspace(),
		Case:        newThreadCase(),
		ChannelID:   "C-MONITOR",
		ThreadTS:    "1700000000.000100",
		MentionTS:   "1700000003.000001",
		MentionText: "<@bot> help",
		TriggerTS:   "1700000003.000001",
		Mode:        threadcase.ModeMention,
		Handler:     host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusQuestion)
	gt.Value(t, res.Decision).Nil()
	host.mu.Lock()
	defer host.mu.Unlock()
	gt.Array(t, host.questions).Length(1).Required()
	gt.String(t, host.questions[0].Reason).Equal("Which environment?")
	gt.Array(t, host.questions[0].Items).Length(1).Required()
	gt.Value(t, host.questions[0].Items[0].Type).Equal(threadcase.QuestionItemSelect)
}

func TestRunTurn_Busy(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{replanDone})
	uc, deps := newThreadcaseUC(t, llm)

	ssn := newThreadSession()
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trigger-A", "preacquired:A", time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:   ssn,
		Workspace: newThreadWorkspace(),
		Case:      newThreadCase(),
		ChannelID: ssn.ChannelID,
		ThreadTS:  ssn.ThreadTS,
		TriggerTS: "trigger-B",
		Mode:      threadcase.ModeMention,
		Handler:   &hostStub{},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusBusy)
}

func TestRunTurn_Idempotent(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{replanDone})
	uc, deps := newThreadcaseUC(t, llm)

	ssn := newThreadSession()
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trig-dup", "preacquired:dup", time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:   ssn,
		Workspace: newThreadWorkspace(),
		Case:      newThreadCase(),
		ChannelID: ssn.ChannelID,
		ThreadTS:  ssn.ThreadTS,
		TriggerTS: "trig-dup",
		Mode:      threadcase.ModeMention,
		Handler:   &hostStub{},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusIdempotent)
}

func TestBuildSystemPrompt_ThreadContext(t *testing.T) {
	ws := newThreadWorkspace()
	c := newThreadCase()
	c.Title = "Payment outage"

	prompt := threadcase.BuildSystemPromptForTest(c, ws, threadcase.ModeMention)
	gt.String(t, prompt).Contains("Payment outage")
	gt.String(t, prompt).Contains("severity")
	gt.String(t, prompt).Contains("DONE")
	gt.String(t, prompt).Contains("CANNOT create or manage Actions")
}

func TestBuildSystemPrompt_CreateMode_WorkspacePrompt(t *testing.T) {
	ws := createTestWorkspace()
	ws.CaseCreatePrompt = "Always fill the severity field for security cases."

	// ModeCreate (no case yet) renders the schema and appends the workspace
	// instructions.
	prompt := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate)
	gt.String(t, prompt).Contains("NO case exists yet")
	gt.String(t, prompt).Contains("severity")
	gt.String(t, prompt).Contains("Workspace-specific instructions")
	gt.String(t, prompt).Contains("Always fill the severity field")

	// Empty CaseCreatePrompt → no workspace-specific section.
	ws.CaseCreatePrompt = ""
	bare := threadcase.BuildSystemPromptForTest(nil, ws, threadcase.ModeCreate)
	gt.Bool(t, strings.Contains(bare, "Workspace-specific instructions")).False()
}

func TestParseDecision_RejectsUnknownKind(t *testing.T) {
	_, err := threadcase.ParseDecisionForTest([]byte(`{"kind":"explode"}`))
	gt.Error(t, err)

	d, err := threadcase.ParseDecisionForTest([]byte(`{"kind":"respond","message":"hi"}`))
	gt.NoError(t, err).Required()
	gt.Value(t, d.Kind).Equal(threadcase.DecisionRespond)
}

func TestBuildUserInput_FallsBackWhenEmpty(t *testing.T) {
	got := threadcase.BuildUserInputForTest(nil, nil, "", "")
	gt.String(t, got).NotEqual("")
}

// createWorkspaceEntry is the workspace used by the ModeCreate tests: it has a
// required select (severity) and a required text (summary).
func createTestWorkspace() *model.WorkspaceEntry {
	return &model.WorkspaceEntry{
		Workspace:             model.Workspace{ID: "support", Name: "Support"},
		CaseMode:              model.CaseModeThread,
		SlackMonitorChannelID: "C-MONITOR",
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{ID: "severity", Name: "Severity", Type: types.FieldTypeSelect, Required: true, Options: []config.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}},
				{ID: "summary", Name: "Summary", Type: types.FieldTypeText, Required: true},
			},
		},
	}
}

func createTestSession() *model.Session {
	return &model.Session{
		ID:            "s-create-" + time.Now().Format("150405.000000"),
		ChannelID:     "C-MONITOR",
		ThreadTS:      "1700000000.000200",
		WorkspaceID:   "support",
		CreatorUserID: "U-REPORTER",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
}

const validCreateDecision = `{"title":"Login failure","description":"A user cannot log in to production.","fields":[{"field_id":"severity","value":"high"},{"field_id":"summary","value":"login broken"}]}`

// ModeCreate: investigate → terminate → final create decision is valid → the
// case is committed via Handler.Create and returned on the Result.
func TestRunTurn_Create_Success(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"The reporter cannot log in to production.",
		replanDone,
		validCreateDecision,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:        createTestSession(),
		Workspace:      createTestWorkspace(),
		ChannelID:      "C-MONITOR",
		ThreadTS:       "1700000000.000200",
		TriggerTS:      "1700000000.000200",
		Mode:           threadcase.ModeCreate,
		SystemMessages: []threadcase.ConversationMessage{{Timestamp: "1700000000.000200", UserID: "U-REPORTER", Text: "I cannot log in"}},
		Handler:        host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Case).NotNil().Required()
	gt.String(t, res.Case.Title).Equal("Login failure")
	gt.Array(t, host.creates).Length(1).Required()
	gt.Value(t, host.creates[0].Fields["severity"].Value).Equal("high")
}

// ModeCreate: the first create decision uses a disallowed option, so OnFinalize
// rejects it and the planner re-emits a valid decision. Handler.Create is only
// called once (for the valid decision).
func TestRunTurn_Create_ValidationRetry(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"The reporter cannot log in.",
		replanDone,
		`{"title":"Login failure","description":"d","fields":[{"field_id":"severity","value":"critical"}]}`, // invalid option + missing summary
		replanDone,
		validCreateDecision,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:        createTestSession(),
		Workspace:      createTestWorkspace(),
		ChannelID:      "C-MONITOR",
		ThreadTS:       "1700000000.000200",
		TriggerTS:      "1700000000.000200",
		Mode:           threadcase.ModeCreate,
		SystemMessages: []threadcase.ConversationMessage{{Timestamp: "1700000000.000200", UserID: "U-REPORTER", Text: "I cannot log in"}},
		Handler:        host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Case).NotNil().Required()
	// Create called once: the invalid decision was rejected before Create.
	gt.Array(t, host.creates).Length(1).Required()
}

// ModeCreate: validation passes but Handler.Create (persistence) fails the
// first time; the planner re-emits and the second Create succeeds.
func TestRunTurn_Create_GenerationRetry(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		investigatePlan,
		"The reporter cannot log in.",
		replanDone,
		validCreateDecision,
		replanDone,
		validCreateDecision,
	})
	uc, _ := newThreadcaseUC(t, llm)
	host := &hostStub{createErr: errors.New("write conflict"), createErrRemaining: 1}

	res, err := uc.RunTurn(ctx, threadcase.TurnRequest{
		Session:        createTestSession(),
		Workspace:      createTestWorkspace(),
		ChannelID:      "C-MONITOR",
		ThreadTS:       "1700000000.000200",
		TriggerTS:      "1700000000.000200",
		Mode:           threadcase.ModeCreate,
		SystemMessages: []threadcase.ConversationMessage{{Timestamp: "1700000000.000200", UserID: "U-REPORTER", Text: "I cannot log in"}},
		Handler:        host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(threadcase.StatusCompleted)
	gt.Value(t, res.Case).NotNil().Required()
	// Create attempted twice: first failed (persistence), second succeeded.
	gt.Array(t, host.creates).Length(2).Required()
}
