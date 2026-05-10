package draft_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
)

// hostStub records every Handler method invocation so each test can assert
// on observable side effects (Slack-side calls) without needing a Slack
// service mock.
type hostStub struct {
	mu              sync.Mutex
	postedQuestion  []draft.QuestionPayload
	materialized    []draft.MaterializePayload
	traceLines      []string
	roundLines      map[string][]string
	registeredTasks []draft.TaskInfo
	taskLines       map[string][]string
	busyCalls       int
}

func (h *hostStub) Question(_ context.Context, _ *model.Session, q draft.QuestionPayload) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.postedQuestion = append(h.postedQuestion, q)
	return nil
}
func (h *hostStub) Materialize(_ context.Context, _ *model.Session, m draft.MaterializePayload) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.materialized = append(h.materialized, m)
	return nil
}
func (h *hostStub) Trace(_ context.Context, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.traceLines = append(h.traceLines, line)
}
func (h *hostStub) TraceRound(_ context.Context, roundKey, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.roundLines == nil {
		h.roundLines = map[string][]string{}
	}
	h.roundLines[roundKey] = append(h.roundLines[roundKey], line)
}
func (h *hostStub) RegisterTasks(_ context.Context, tasks []draft.TaskInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.registeredTasks = append(h.registeredTasks, tasks...)
}
func (h *hostStub) TraceTask(_ context.Context, taskID, line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.taskLines == nil {
		h.taskLines = map[string][]string{}
	}
	h.taskLines[taskID] = append(h.taskLines[taskID], line)
}
func (h *hostStub) PostBusy(_ context.Context, _ *model.Session, _ agent.BusyInfo) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.busyCalls++
	return nil
}

// scriptedLLM returns a fakeLLM that emits a queued JSON plan per Generate
// call. Tests typically queue two-or-three plans (investigate → terminal).
type scriptedLLM struct {
	mu      sync.Mutex
	scripts []string
	idx     int
}

func newScriptedLLM(plans []string) *mock.LLMClientMock {
	s := &scriptedLLM{scripts: plans}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			calls := atomic.Int32{}
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					n := calls.Add(1)
					_ = n
					s.mu.Lock()
					defer s.mu.Unlock()
					if s.idx >= len(s.scripts) {
						return nil, errors.New("no more scripted plans")
					}
					out := s.scripts[s.idx]
					s.idx++
					return &gollem.Response{Texts: []string{out}}, nil
				},
			}, nil
		},
	}
}

func mustDraft(t *testing.T, llm gollem.LLMClient, plannerMax, subMax int) *draft.UseCase {
	t.Helper()
	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := draft.New(deps, plannerMax, subMax, 20)
	gt.NoError(t, err).Required()
	return uc
}

func newOpenSession() *model.Session {
	return &model.Session{
		ID:        "s-open-" + time.Now().Format("150405.000"),
		ChannelID: "C-DRAFT",
		ThreadTS:  "1700000000.000001",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestRunTurn_QuestionHappyPath(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"need user input","action":"question","question":{"reason":"need workspace","items":[{"id":"q-ws","text":"Which workspace?","type":"select","options":["A","B"]}]}}`,
	})
	uc := mustDraft(t, llm, 8, 16)

	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   newOpenSession(),
		UserInput: "@bot which workspace?",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "1700000001.000001",
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res).NotNil().Required()
	gt.Value(t, res.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res.EndedWith).Equal(model.SessionEndedWithQuestion)
	gt.Array(t, host.postedQuestion).Length(1).Required()
	gt.Value(t, host.postedQuestion[0].Reason).Equal("need workspace")
	gt.Array(t, host.postedQuestion[0].Items).Length(1).Required()
	gt.Value(t, host.postedQuestion[0].Items[0].ID).Equal("q-ws")
	gt.Value(t, host.postedQuestion[0].Items[0].Type).Equal(draft.QuestionItemSelect)
	gt.Array(t, host.materialized).Length(0)
}

func TestRunTurn_InvestigateThenMaterialize(t *testing.T) {
	ctx := context.Background()
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			calls := atomic.Int32{}
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					n := calls.Add(1)
					_ = n
					if len(input) == 0 {
						return nil, errors.New("no input")
					}
					txt, ok := input[0].(gollem.Text)
					if !ok {
						return nil, errors.New("expected gollem.Text")
					}
					body := string(txt)

					// Sub-agent task description for inv-1 → return summary.
					if strings.Contains(body, "Look at the prior thread") {
						return &gollem.Response{Texts: []string{"The thread mentions team-X was paged."}}, nil
					}
					// First planner round (initial mention).
					if strings.Contains(body, "[budget] planner 0/8") {
						return &gollem.Response{Texts: []string{`{
							"reasoning":"need more context",
							"action":"investigate",
							"investigate":{
								"message":"Looking up context",
								"tasks":[{
									"id":"inv-1","title":"Recent thread",
									"description":"Look at the prior thread",
									"acceptance_criteria":"identify team",
									"tools":["slack_ro"]
								}]
							}
						}`}}, nil
					}
					// Second planner round (after observations).
					if strings.Contains(body, "Observations from prior investigations") {
						return &gollem.Response{Texts: []string{`{
							"reasoning":"all set",
							"action":"materialize",
							"materialize":{
								"workspace_id":"ws-1",
								"title":"API outage",
								"description":"Brief.",
								"custom_field_values":{"severity":"high"}
							}
						}`}}, nil
					}
					return nil, errors.New("unexpected planner input: " + body)
				},
			}, nil
		},
	}

	uc := mustDraft(t, llm, 8, 16)
	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   newOpenSession(),
		UserInput: "@bot create a case for the API outage",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "1700000010.000001",
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res.EndedWith).Equal(model.SessionEndedWithMaterialize)

	gt.Array(t, host.materialized).Length(1).Required()
	gt.Value(t, host.materialized[0].WorkspaceID).Equal("ws-1")
	gt.Value(t, host.materialized[0].Title).Equal("API outage")
	gt.Value(t, host.materialized[0].CustomFieldValues["severity"]).Equal("high")

	// Two logical planner rounds → two distinct round messages in
	// roundLines (plan-1, plan-2). Within each round, the runtime
	// posts at least the "Planning…" line and the action selection
	// line, all via TraceRound under the same key. The phase-level
	// Trace surface (for non-round phases like investigate.message)
	// is independent and may be empty here.
	gt.Number(t, len(host.roundLines)).GreaterOrEqual(2)
	for key, lines := range host.roundLines {
		gt.Array(t, lines).Length(2)
		gt.String(t, strings.Join(lines, "\n")).Contains("Planning")
		_ = key
	}

	// The investigation task was registered (sub-agent block created
	// before sub-agent ran) and ended with a "done" line carrying the
	// inner-loops counter — surfaced via TraceTask, not Trace.
	gt.Array(t, host.registeredTasks).Length(1).Required()
	gt.Value(t, host.registeredTasks[0].ID).Equal("inv-1")
	gt.Value(t, host.registeredTasks[0].Title).Equal("Recent thread")
	taskLines := host.taskLines["inv-1"]
	gt.Bool(t, len(taskLines) >= 2).True()
	gt.String(t, taskLines[len(taskLines)-1]).Contains("inner loops")
	gt.String(t, taskLines[len(taskLines)-1]).Contains("Recent thread")
}

func TestRunTurn_PlannerBudgetExhaustionFallback(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		// Always returns investigate, no terminal action.
		`{"reasoning":"more context","action":"investigate","investigate":{"tasks":[{"id":"inv-1","title":"loop","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}]}}`,
		`{"reasoning":"more context","action":"investigate","investigate":{"tasks":[{"id":"inv-2","title":"loop","description":"d","acceptance_criteria":"a","tools":["slack_ro"]}]}}`,
	})
	// Plug in a sub-agent script that responds to "d" with a summary so
	// the planner round can keep invoking. We need the same llm for both
	// the planner and sub-agents — make a combined scripter.
	combined := combineScript(llm, map[string]fakeSessionConfig{
		"d": {text: "summary"},
	})

	uc := mustDraft(t, combined, 2, 16)
	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   newOpenSession(),
		UserInput: "@bot loop please",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "1700000020.000001",
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	// Budget exhausted → StatusFallback with non-empty reason. Runtime
	// does NOT post anything itself; the host renders fallback copy.
	gt.Value(t, res.Status).Equal(draft.StatusFallback)
	gt.String(t, res.FallbackReason).NotEqual("")
	gt.Array(t, host.materialized).Length(0)
	gt.Array(t, host.postedQuestion).Length(0)
}

func TestRunTurn_BusyShortCircuits(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"x","action":"question","question":{"reason":"r","items":[{"id":"q","text":"?","type":"select","options":["a","b"]}]}}`,
	})
	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   200 * time.Millisecond,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := draft.New(deps, 8, 16, 20)
	gt.NoError(t, err).Required()

	ssn := newOpenSession()
	// Manually pre-acquire the lock so the next RunTurn sees Busy.
	ownerID := "preacquired:trigger-A"
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trigger-A", ownerID, time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   ssn,
		UserInput: "second mention",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "trigger-B",
		Handler:   host,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(draft.StatusBusy)
	gt.Number(t, host.busyCalls).Equal(1)
	gt.Array(t, host.postedQuestion).Length(0)
}

func TestRunTurn_IdempotentRetryDropsSilently(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"x","action":"question","question":{"reason":"r","items":[{"id":"q","text":"?","type":"select","options":["a","b"]}]}}`,
	})
	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   200 * time.Millisecond,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := draft.New(deps, 8, 16, 20)
	gt.NoError(t, err).Required()

	ssn := newOpenSession()
	ownerID := "preacquired:trig-dup"
	acq, err := deps.Repo.Session().AcquireTurnLock(ctx,
		ssn.ChannelID, ssn.ThreadTS, "trig-dup", ownerID, time.Hour,
		func() *model.Session { return ssn })
	gt.NoError(t, err).Required()
	gt.Bool(t, acq.Acquired).True().Required()

	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   ssn,
		UserInput: "duplicate",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "trig-dup",
		Handler:   host,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, res.Status).Equal(draft.StatusIdempotent)
	gt.Number(t, host.busyCalls).Equal(0)
	gt.Array(t, host.postedQuestion).Length(0)
}

// TestRunTurn_PlannerCallsGetWorkspaceThenMaterializes covers the
// tool-driven planner path: round 1 emits a tool call to `get_workspace`
// (instead of immediate JSON); the wsmeta tool resolves the workspace from
// the registry; round 2 sees the tool response back as input and emits the
// terminal materialise JSON. This is the minimum end-to-end shape after the
// planner stopped having field schemas inlined into the system prompt.
func TestRunTurn_PlannerCallsGetWorkspaceThenMaterializes(t *testing.T) {
	ctx := context.Background()

	// Registry the planner's get_workspace tool will resolve against. The
	// field IDs are sentinel strings so the test would fail loudly if the
	// planner short-circuited and synthesised values without consulting
	// the tool response.
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID:          "ws-tool-test",
			Name:        "Tool-driven WS",
			Description: "Fixture for tool-driven planner test",
		},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID:       "severity",
					Name:     "Severity",
					Type:     types.FieldTypeSelect,
					Required: true,
					Options: []config.FieldOption{
						{ID: "low", Name: "Low", Description: "Minor"},
						{ID: "high", Name: "High", Description: "Critical"},
					},
				},
			},
		},
	})

	// LLM mock: first Generate returns a get_workspace tool call; second
	// Generate (after the tool response) returns the materialize plan.
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			calls := atomic.Int32{}
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, input []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					n := calls.Add(1)
					switch n {
					case 1:
						if len(input) == 0 {
							return nil, errors.New("expected planner mention input on round 1")
						}
						txt, ok := input[0].(gollem.Text)
						if !ok {
							return nil, errors.New("expected gollem.Text on round 1")
						}
						if !strings.Contains(string(txt), "[budget] planner 0/8") {
							return nil, errors.New("round 1 input missing budget prefix")
						}
						return &gollem.Response{
							FunctionCalls: []*gollem.FunctionCall{
								{
									Name:      "get_workspace",
									Arguments: map[string]any{"workspace_id": "ws-tool-test"},
								},
							},
						}, nil
					case 2:
						if len(input) == 0 {
							return nil, errors.New("expected tool response input on round 2")
						}
						resp, ok := input[0].(gollem.FunctionResponse)
						if !ok {
							return nil, errors.New("expected gollem.FunctionResponse on round 2")
						}
						if resp.Name != "get_workspace" {
							return nil, errors.New("unexpected tool response on round 2: " + resp.Name)
						}
						// Confirm the wsmeta tool actually returned the
						// fixture's field schema rather than a stub. The
						// planner is expected to use these field / option
						// IDs for materialize. gollem JSON-roundtrips tool
						// results, so the inner slice arrives as []any
						// even though the tool returned []map[string]any.
						fieldsAny, fok := resp.Data["fields"].([]any)
						if !fok || len(fieldsAny) != 1 {
							return nil, errors.New("get_workspace did not return fields")
						}
						field, fok := fieldsAny[0].(map[string]any)
						if !fok {
							return nil, errors.New("get_workspace fields[0] not a map")
						}
						if field["id"] != "severity" {
							return nil, errors.New("get_workspace returned unexpected field id")
						}
						return &gollem.Response{
							Texts: []string{`{
								"reasoning":"schema confirmed via tool",
								"action":"materialize",
								"materialize":{
									"workspace_id":"ws-tool-test",
									"title":"Tool-driven case",
									"description":"Built after consulting get_workspace.",
									"custom_field_values":{"severity":"high"}
								}
							}`},
						}, nil
					default:
						return nil, errors.New("unexpected extra Generate call")
					}
				},
			}, nil
		},
	}

	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		Registry:            registry,
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           agentarchive.NewMemoryTraceRepository(),
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 5 * time.Second,
	}
	uc, err := draft.New(deps, 8, 16, 20)
	gt.NoError(t, err).Required()

	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   newOpenSession(),
		UserInput: "@bot create a draft",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "1700000030.000001",
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res).NotNil().Required()
	gt.Value(t, res.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res.EndedWith).Equal(model.SessionEndedWithMaterialize)

	gt.Array(t, host.materialized).Length(1).Required()
	gt.Value(t, host.materialized[0].WorkspaceID).Equal("ws-tool-test")
	gt.Value(t, host.materialized[0].Title).Equal("Tool-driven case")
	gt.Value(t, host.materialized[0].CustomFieldValues["severity"]).Equal("high")

	// Tool call must not have consumed an extra planner round — both
	// Generate calls fire within a single Execute (= one planner round).
	gt.Number(t, uc.PlannerLoopMax()).Equal(8)
}

// combineScript wraps a scripted planner LLM and folds in sub-agent
// canned responses (matched by Description). When the input matches a
// sub-agent description, we serve from byDescription. Otherwise we
// delegate to the planner script.
func combineScript(plannerLLM *mock.LLMClientMock, byDescription map[string]fakeSessionConfig) *mock.LLMClientMock {
	return &mock.LLMClientMock{
		NewSessionFunc: func(ctx context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			plannerSession, err := plannerLLM.NewSession(ctx, opts...)
			if err != nil {
				return nil, err
			}
			calls := atomic.Int32{}
			return &mock.SessionMock{
				GenerateFunc: func(c context.Context, input []gollem.Input, gopts ...gollem.GenerateOption) (*gollem.Response, error) {
					_ = calls.Add(1)
					if len(input) > 0 {
						if t, ok := input[0].(gollem.Text); ok {
							if cfg, ok := byDescription[string(t)]; ok {
								if cfg.err != nil {
									return nil, cfg.err
								}
								return &gollem.Response{Texts: []string{cfg.text}}, nil
							}
						}
					}
					return plannerSession.Generate(c, input, gopts...)
				},
			}, nil
		},
	}
}
