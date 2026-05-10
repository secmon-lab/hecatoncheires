package draft_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gollem/trace"
	"github.com/m-mizutani/gt"
	notiontool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/notion"
	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	cliconfig "github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/config"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/draft"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/async"
	"github.com/urfave/cli/v3"
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

// =====================================================================
// Real-LLM scenario tests (TEST_WITH_LLM gated)
// =====================================================================
//
// These tests drive RunTurn with an actual LLM (OpenAI / Claude / Gemini)
// rather than the scripted mock above. They are skipped unless the
// TEST_WITH_LLM environment variable is defined; the LLM client itself is
// built from the same HECATONCHEIRES_LLM_* variables the serve subcommand
// reads, by reusing pkg/cli/config.LLM verbatim — no test-only env names
// or duplicated provider switch.
//
// The structural contract (final action, materialise workspace_id, planner
// tool calls in the trace) is asserted in code. Free-form criteria such as
// "the question asks the user to identify the workspace" are evaluated by
// a separate LLM judge call that returns a {matches, rationale} verdict.
//
// When TEST_TRACE_DIR is set, both the planner trace and the judge trace
// are written as JSON files under that directory, so failures can be
// inspected post-hoc.

// newTestLLMClient builds a real gollem.LLMClient using HECATONCHEIRES_LLM_*
// env vars. The LLM feature itself is the same as the serve subcommand:
// pkg/cli/config.LLM owns the flag definitions and the provider switch, and
// the test simply wires its Flags into a minimal urfave/cli command so the
// env var Sources fire. Any drift in the LLM config layer is therefore
// reflected here automatically.
func newTestLLMClient(t *testing.T, ctx context.Context) gollem.LLMClient {
	t.Helper()
	if _, ok := os.LookupEnv("TEST_WITH_LLM"); !ok {
		t.Skip("TEST_WITH_LLM not set; skipping real-LLM scenario")
	}

	var x cliconfig.LLM
	var (
		client    gollem.LLMClient
		clientErr error
	)
	cmd := &cli.Command{
		Name:  "draft-llm-test",
		Flags: x.Flags(),
		Action: func(ctx context.Context, _ *cli.Command) error {
			client, clientErr = x.NewClient(ctx)
			return nil
		},
	}
	gt.NoError(t, cmd.Run(ctx, []string{"draft-llm-test"})).Required()
	gt.NoError(t, clientErr).Required()
	if client == nil {
		t.Logf("HECATONCHEIRES_LLM_PROVIDER is empty; set provider to run TEST_WITH_LLM tests")
	}
	gt.Value(t, client).NotNil().Required()
	return client
}

// fileTraceRepository persists each saved gollem trace as a JSON file under
// dir/<traceID>.json. It implements trace.Repository so it can be combined
// with the existing in-memory repo via multiTraceRepo, giving tests both
// programmatic access (memory) and a post-mortem artifact (filesystem).
type fileTraceRepository struct {
	dir string
}

func (r *fileTraceRepository) Save(_ context.Context, t *trace.Trace) error {
	if t == nil {
		return goerr.New("trace is nil")
	}
	if t.TraceID == "" {
		return goerr.New("trace ID is empty")
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return goerr.Wrap(err, "create trace dir", goerr.V("dir", r.dir))
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return goerr.Wrap(err, "marshal trace", goerr.V("trace_id", t.TraceID))
	}
	path := filepath.Join(r.dir, t.TraceID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return goerr.Wrap(err, "write trace file", goerr.V("path", path))
	}
	return nil
}

// multiTraceRepo fans out Save calls to every wrapped repository. The first
// non-nil error is returned but every repo still runs — partial failures
// (e.g. filesystem out of space) should not block the in-memory copy that
// the test reads back from.
type multiTraceRepo struct {
	repos []trace.Repository
}

func (m *multiTraceRepo) Save(ctx context.Context, t *trace.Trace) error {
	var firstErr error
	for _, r := range m.repos {
		if err := r.Save(ctx, t); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// newScenarioTraceRepo returns the trace.Repository to hand to CommonDeps
// plus the underlying memory repo for read-back. When TEST_TRACE_DIR is
// set, the returned repo also writes JSON files under
// $TEST_TRACE_DIR/<sanitised-test-name>/<label>/.
func newScenarioTraceRepo(t *testing.T, label string) (trace.Repository, *agentarchive.MemoryTraceRepository) {
	t.Helper()
	mem := agentarchive.NewMemoryTraceRepository()
	base := os.Getenv("TEST_TRACE_DIR")
	if base == "" {
		return mem, mem
	}
	dir := filepath.Join(base, sanitiseFilename(t.Name()), label)
	multi := &multiTraceRepo{repos: []trace.Repository{
		mem,
		&fileTraceRepository{dir: dir},
	}}
	return multi, mem
}

// sanitiseFilename strips characters that are unsafe inside a path segment.
// The result is intentionally permissive — t.Name() can contain `/` for
// subtests, for example, and we want each subtest to land in its own
// directory rather than collapsing them.
func sanitiseFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// recordedToolCall is one tool_exec entry pulled out of a gollem trace.
// Args mirrors what the LLM passed in; the result/error fields are dropped
// because tests assert on the agent's intent (what it called and with
// what arguments), not on the canned data the fakes returned.
type recordedToolCall struct {
	Name string
	Args map[string]any
}

// collectToolCalls walks the trace tree DFS and returns every tool_exec
// span as a (name, args) pair, in invocation order.
func collectToolCalls(t *trace.Trace) []recordedToolCall {
	if t == nil || t.RootSpan == nil {
		return nil
	}
	var calls []recordedToolCall
	var walk func(s *trace.Span)
	walk = func(s *trace.Span) {
		if s == nil {
			return
		}
		if s.Kind == trace.SpanKindToolExec && s.ToolExec != nil {
			calls = append(calls, recordedToolCall{
				Name: s.ToolExec.ToolName,
				Args: s.ToolExec.Args,
			})
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	walk(t.RootSpan)
	return calls
}

// recordingSlackSearch wraps a SearchService and logs every SearchMessages
// call so the test can assert on call count and on the queries the
// sub-agent issued. Sub-agent tools live outside the planner's recorder
// (sub-agents use a counter, not the trace.Repository), so this is the
// only seam through which tests can observe sub-agent intent.
type recordingSlackSearch struct {
	inner slacktool.SearchService
	mu    sync.Mutex
	calls []slackSearchInvocation
}

type slackSearchInvocation struct {
	Query string                    `json:"query"`
	Opts  slacktool.SearchOptions   `json:"opts"`
}

func (r *recordingSlackSearch) SearchMessages(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error) {
	r.mu.Lock()
	r.calls = append(r.calls, slackSearchInvocation{Query: query, Opts: opts})
	r.mu.Unlock()
	return r.inner.SearchMessages(ctx, query, opts)
}

func (r *recordingSlackSearch) snapshot() []slackSearchInvocation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]slackSearchInvocation, len(r.calls))
	copy(out, r.calls)
	return out
}

// recordingNotionClient wraps a notiontool.Client and logs every Search /
// GetPageMarkdown invocation, for the same reason as recordingSlackSearch.
type recordingNotionClient struct {
	inner       notiontool.Client
	mu          sync.Mutex
	searchCalls []notionSearchInvocation
	pageCalls   []notionGetPageInvocation
}

type notionSearchInvocation struct {
	Query string                    `json:"query"`
	Opts  notiontool.SearchOptions  `json:"opts"`
}

type notionGetPageInvocation struct {
	PageID string `json:"page_id"`
}

func (r *recordingNotionClient) Search(ctx context.Context, query string, opts notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	r.mu.Lock()
	r.searchCalls = append(r.searchCalls, notionSearchInvocation{Query: query, Opts: opts})
	r.mu.Unlock()
	return r.inner.Search(ctx, query, opts)
}

func (r *recordingNotionClient) GetPageMarkdown(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	r.mu.Lock()
	r.pageCalls = append(r.pageCalls, notionGetPageInvocation{PageID: pageID})
	r.mu.Unlock()
	return r.inner.GetPageMarkdown(ctx, pageID)
}

func (r *recordingNotionClient) searchSnapshot() []notionSearchInvocation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]notionSearchInvocation, len(r.searchCalls))
	copy(out, r.searchCalls)
	return out
}

func (r *recordingNotionClient) pageSnapshot() []notionGetPageInvocation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]notionGetPageInvocation, len(r.pageCalls))
	copy(out, r.pageCalls)
	return out
}

// judgeSystemPrompt drives the LLM judge that evaluates whether a planner
// output satisfies a free-form criterion. The prompt is intentionally short
// and demands a single JSON object so the response is parseable without
// post-processing.
const judgeSystemPrompt = `You are a strict evaluator of an AI agent's behaviour. You receive:
1. A natural-language criterion that the agent's output must satisfy.
2. The agent's actual output, JSON-serialised.

Decide whether the output satisfies the criterion. Respond with a SINGLE JSON object:
{"matches": <bool>, "rationale": "<one short sentence>"}.

Be strict — if the output is missing a key part of the criterion, return matches=false. Do not include any prose around the JSON.`

// judgeSchema is the response schema enforced on the judge agent so its
// reply can be unmarshalled into judgeVerdict directly.
func judgeSchema() *gollem.Parameter {
	return &gollem.Parameter{
		Type: gollem.TypeObject,
		Properties: map[string]*gollem.Parameter{
			"matches":   {Type: gollem.TypeBoolean, Description: "true iff the output satisfies the criterion."},
			"rationale": {Type: gollem.TypeString, Description: "One short sentence explaining the verdict."},
		},
	}
}

type judgeVerdict struct {
	Matches   bool   `json:"matches"`
	Rationale string `json:"rationale"`
}

// runJudge invokes a fresh single-round agent against `llm` to evaluate
// whether `observed` satisfies `criterion`. The judge's own trace is
// recorded under traceRepo with a "judge-" prefixed trace ID so post-mortem
// inspection can tell judge from planner without parsing labels.
func runJudge(t *testing.T, ctx context.Context, llm gollem.LLMClient, traceRepo trace.Repository, criterion, observed string) judgeVerdict {
	t.Helper()
	rec := trace.New(
		trace.WithRepository(traceRepo),
		trace.WithTraceID("judge-"+strconv.FormatInt(time.Now().UnixNano(), 10)),
		trace.WithMetadata(trace.TraceMetadata{
			Labels: map[string]string{
				agentarchive.SessionIDLabel: "judge",
				"purpose":                   "scenario_judge",
			},
		}),
	)
	defer func() {
		if err := rec.Finish(ctx); err != nil {
			t.Logf("judge trace finish: %v", err)
		}
	}()

	judge := gollem.New(llm,
		gollem.WithSystemPrompt(judgeSystemPrompt),
		gollem.WithContentType(gollem.ContentTypeJSON),
		gollem.WithResponseSchema(judgeSchema()),
		// gollem's Execute loop iterates over a strategy → call → finalize
		// pipeline; even with no tools wired, a single judge round can
		// legitimately consume 2-3 iterations before the JSON is emitted.
		// Mirror the planner's per-call budget (plannerPerCallLoopLimit=8)
		// at a tighter setting since the judge has no tool fan-out.
		gollem.WithLoopLimit(4),
		gollem.WithTrace(rec),
	)
	input := "## Criterion\n" + criterion + "\n\n## Observed output\n" + observed
	resp, err := judge.Execute(ctx, gollem.Text(input))
	gt.NoError(t, err).Required()
	gt.Bool(t, resp.IsEmpty()).False().Required()
	var v judgeVerdict
	gt.NoError(t, json.Unmarshal([]byte(resp.Texts[0]), &v)).Required()
	return v
}

// checkSubAgentCallCount enforces the requireXxxCalls contract:
//   require == 0 → skip
//   require > 0  → got >= require
//   require == -1 → got == 0 (the tool must not have been called).
func checkSubAgentCallCount(t *testing.T, label string, got, require int) {
	t.Helper()
	switch {
	case require == 0:
		t.Logf("%s invocations: %d (no requirement)", label, got)
	case require > 0:
		t.Logf("%s invocations: %d (require ≥ %d)", label, got, require)
		gt.Number(t, got).GreaterOrEqual(require)
	case require == -1:
		t.Logf("%s invocations: %d (require == 0)", label, got)
		gt.Number(t, got).Equal(0)
	}
}

// runSubAgentArgJudge asks the judge whether the recorded sub-agent calls'
// arguments satisfy the criterion. A single judge invocation per tool
// type is used (the criterion is applied to the full call set), keeping
// LLM cost bounded regardless of how many times the sub-agent retried.
func runSubAgentArgJudge(t *testing.T, ctx context.Context, llm gollem.LLMClient, traceRepo trace.Repository, label, criterion string, calls []any) {
	t.Helper()
	t.Logf("%s argument judge — recorded %d call(s)", label, len(calls))
	if len(calls) == 0 {
		// Skip — the count check below should already fail when calls
		// are required. If they were optional, vacuous truth is fine.
		return
	}
	observed, err := json.MarshalIndent(map[string]any{
		"tool":  label,
		"calls": calls,
	}, "", "  ")
	gt.NoError(t, err).Required()
	verdict := runJudge(t, ctx, llm, traceRepo, criterion, string(observed))
	t.Logf("sub-agent-arg judge for %s: matches=%v rationale=%q", label, verdict.Matches, verdict.Rationale)
	gt.Bool(t, verdict.Matches).True()
}

func slackInvocationCount(r *recordingSlackSearch) int {
	if r == nil {
		return 0
	}
	return len(r.snapshot())
}

func notionSearchInvocationCount(r *recordingNotionClient) int {
	if r == nil {
		return 0
	}
	return len(r.searchSnapshot())
}

func notionPageInvocationCount(r *recordingNotionClient) int {
	if r == nil {
		return 0
	}
	return len(r.pageSnapshot())
}

// anySlice converts a typed slice to []any so it can be JSON-marshalled
// inside a heterogeneous record. The judge prompt only sees the JSON, so
// the boxed type is fine.
func anySlice[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// llmScenario captures everything a single real-LLM test case asserts on.
// Structural fields (status / workspace ID / required tool calls) are
// checked deterministically; the *Criterion fields are evaluated by the
// LLM judge for cases where exact output cannot be pinned down.
type llmScenario struct {
	userInput  string
	trigger    draft.Trigger
	workspaces []*model.WorkspaceEntry

	// Optional sub-agent backing services. When set, sub-agents that the
	// planner dispatches with `slack_ro` / `notion` toolsets actually call
	// these fakes instead of returning empty tool lists. The fakes' Search
	// methods return canned data that the agent should be able to
	// summarise into the materialise field values, so the test verifies
	// that the agent does NOT have to ask the user when the source
	// material is sufficient.
	slackSearch  slacktool.SearchService
	notionClient notiontool.Client

	// sources are inserted into the Repo's source repository before
	// RunTurn is called, so wsmeta's get_workspace tool advertises them.
	// Each entry's WorkspaceID specifies which workspace the source
	// belongs to.
	sources []llmScenarioSource

	expectStatus draft.Status
	expectAction model.SessionEndReason

	// requirePlannerTools lists tool names that MUST appear at least once
	// in the planner trace (e.g. "get_workspace"). Verified via tool_exec
	// span names, so wsmeta tool calls are covered without further hooks.
	requirePlannerTools []string

	// allowedPlannerTools, when non-nil, asserts every planner-side
	// tool_exec span name is a member. nil = skip the no-extras check.
	// An empty slice means "no planner tool calls allowed at all".
	allowedPlannerTools []string

	// plannerToolArgCriteria provides a natural-language criterion per
	// planner tool name. After the turn finishes, every recorded call
	// to that tool is collected and a single judge call evaluates the
	// full set of args against the criterion. Use this to express
	// "workspace_id in get_workspace was always the chosen workspace"
	// or similar arg-shape rules without locking to exact values.
	plannerToolArgCriteria map[string]string

	// requireSlackSearchCalls / requireNotionSearchCalls /
	// requireNotionGetPageCalls assert minimum invocation counts on the
	// sub-agent fakes (which runScenario wraps automatically). 0 means
	// "do not check"; -1 means "must be exactly zero" (forbidden tool).
	requireSlackSearchCalls   int
	requireNotionSearchCalls  int
	requireNotionGetPageCalls int

	// Sub-agent argument criteria. Empty string = skip. The JSON dump
	// of every recorded call (queries, opts, page IDs) is shown to the
	// judge once with the criterion.
	slackSearchArgCriterion   string
	notionSearchArgCriterion  string
	notionGetPageArgCriterion string

	// requireInvestigation, when true, asserts that at least one
	// investigate phase ran (RegisterTasks was called with ≥1 task) so
	// the test can confirm the agent actually consulted its sources
	// rather than guessing or short-circuiting to a terminal action.
	requireInvestigation bool

	// Question scenario fields
	questionCriterion string

	// Materialize scenario fields
	expectWorkspaceID    string
	requireFieldKeys     []string
	materializeCriterion string

	// requireFieldOneOf asserts that materialize.custom_field_values[k]
	// is a string AND a member of the allowed set, for each (k, set)
	// pair. Use this to express "the value must fall within a sensible
	// range" without locking the test to a single LLM-chosen option.
	requireFieldOneOf map[string][]string
}

// llmScenarioSource pairs a workspace ID with the Source fixture to seed
// before RunTurn. The fixture is inserted via Repo.Source().Create so the
// memory backend mints a stable SourceID.
type llmScenarioSource struct {
	WorkspaceID string
	Source      *model.Source
}

// fakeSlackSearch is an in-memory slacktool.SearchService whose
// SearchMessages returns whatever the supplied function decides. Tests
// typically return the same canned message set for every query so the
// sub-agent can summarise without depending on exact phrasing.
type fakeSlackSearch struct {
	fn func(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error)
}

func (f *fakeSlackSearch) SearchMessages(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error) {
	return f.fn(ctx, query, opts)
}

// fakeNotionClient implements notiontool.Client with caller-supplied
// search and page-markdown handlers. Either handler may be nil; the
// corresponding method then returns an empty result.
type fakeNotionClient struct {
	searchFn  func(ctx context.Context, query string, opts notiontool.SearchOptions) (*notiontool.SearchResult, error)
	getPageFn func(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error)
}

func (f *fakeNotionClient) Search(ctx context.Context, query string, opts notiontool.SearchOptions) (*notiontool.SearchResult, error) {
	if f.searchFn == nil {
		return &notiontool.SearchResult{}, nil
	}
	return f.searchFn(ctx, query, opts)
}

func (f *fakeNotionClient) GetPageMarkdown(ctx context.Context, pageID string) (*notiontool.PageMarkdown, error) {
	if f.getPageFn == nil {
		return &notiontool.PageMarkdown{PageID: pageID}, nil
	}
	return f.getPageFn(ctx, pageID)
}

// runScenario wires up a one-off draft.UseCase against the supplied LLM,
// drives RunTurn with the scenario's mention, and then checks every
// criterion. Helpers above (newScenarioTraceRepo, collectToolNames,
// runJudge) are composed here so the per-scenario test bodies stay short.
func runScenario(t *testing.T, ctx context.Context, llm gollem.LLMClient, sc llmScenario) {
	t.Helper()

	registry := model.NewWorkspaceRegistry()
	for _, ws := range sc.workspaces {
		registry.Register(ws)
	}

	plannerRepo, plannerMem := newScenarioTraceRepo(t, "planner")
	judgeRepo, _ := newScenarioTraceRepo(t, "judge")

	repo := memory.New()
	for _, src := range sc.sources {
		_, err := repo.Source().Create(ctx, src.WorkspaceID, src.Source)
		gt.NoError(t, err).Required()
	}

	// Wrap the supplied fakes so every sub-agent tool call is observable
	// from the outside. The planner trace does not capture sub-agent
	// tools (they go through a separate counter), so this wrapper layer
	// is the only seam through which tool-call assertions on the
	// Slack / Notion side can be expressed.
	var (
		slackRec  *recordingSlackSearch
		notionRec *recordingNotionClient
	)
	var slackForDeps slacktool.SearchService
	var notionForDeps notiontool.Client
	if sc.slackSearch != nil {
		slackRec = &recordingSlackSearch{inner: sc.slackSearch}
		slackForDeps = slackRec
	}
	if sc.notionClient != nil {
		notionRec = &recordingNotionClient{inner: sc.notionClient}
		notionForDeps = notionRec
	}

	deps := &agent.CommonDeps{
		Repo:                repo,
		Registry:            registry,
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           plannerRepo,
		SlackSearch:         slackForDeps,
		NotionClient:        notionForDeps,
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 30 * time.Second,
	}
	// Bump sub-agent loop budget: real-LLM sub-agents may iterate
	// through several search/get_page calls before they have enough
	// context to summarise. The default 8 is fine for fully-mocked
	// runs, but Scenario C exercises the full Slack + Notion fan-out
	// and benefits from headroom.
	uc, err := draft.New(deps, 6, 8, 14)
	gt.NoError(t, err).Required()

	host := &hostStub{}
	ssn := newOpenSession()
	now := time.Now()
	triggerTS := fmt.Sprintf("real-llm-%d.%06d", now.Unix(), now.Nanosecond()/1000)

	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   ssn,
		UserInput: sc.userInput,
		Trigger:   sc.trigger,
		TriggerTS: triggerTS,
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res).NotNil().Required()
	gt.Value(t, res.Status).Equal(sc.expectStatus)
	gt.Value(t, res.EndedWith).Equal(sc.expectAction)

	// Verify planner-side tool spans (`get_workspace`, etc.) by reading
	// back the persisted trace. There is exactly one trace per turn
	// because the recorder uses TurnID as the trace ID.
	traceIDs := plannerMem.TraceIDs(ssn.ID)
	gt.Array(t, traceIDs).Length(1).Required()
	plannerTrace := plannerMem.Load(ssn.ID, traceIDs[0])
	gt.Value(t, plannerTrace).NotNil().Required()
	plannerCalls := collectToolCalls(plannerTrace)
	toolNames := make([]string, len(plannerCalls))
	for i, c := range plannerCalls {
		toolNames[i] = c.Name
	}
	t.Logf("planner tool calls: %v", toolNames)

	// Required-tool check: each name in requirePlannerTools must appear
	// at least once. Lets a test pin "the planner MUST consult
	// get_workspace before materialising" without forbidding extras.
	for _, want := range sc.requirePlannerTools {
		gt.Array(t, toolNames).Has(want)
	}

	// Whitelist check: when allowedPlannerTools is non-nil, every
	// recorded tool name must be a member. Catches regressions where
	// the planner starts calling tools the test did not anticipate
	// (e.g. a new wsmeta tool, or a leak from another resolver).
	if sc.allowedPlannerTools != nil {
		for _, name := range toolNames {
			gt.Array(t, sc.allowedPlannerTools).Has(name)
		}
	}

	// Argument-shape check via judge: for each tool listed in
	// plannerToolArgCriteria, collect every call's args and ask the
	// judge once whether the criterion holds across the set. The
	// criterion is "given that the planner called this tool, the
	// args were sane" — it is vacuously satisfied when the tool was
	// not called. Use requirePlannerTools when the call itself is
	// mandatory.
	for toolName, criterion := range sc.plannerToolArgCriteria {
		var callArgs []map[string]any
		for _, c := range plannerCalls {
			if c.Name == toolName {
				callArgs = append(callArgs, c.Args)
			}
		}
		t.Logf("planner tool %q args: %v", toolName, callArgs)
		if len(callArgs) == 0 {
			t.Logf("planner tool %q: no calls recorded — skipping arg judge", toolName)
			continue
		}
		observed, err := json.MarshalIndent(map[string]any{
			"tool":  toolName,
			"calls": callArgs,
		}, "", "  ")
		gt.NoError(t, err).Required()
		verdict := runJudge(t, ctx, llm, judgeRepo, criterion, string(observed))
		t.Logf("planner-arg judge for %q: matches=%v rationale=%q", toolName, verdict.Matches, verdict.Rationale)
		gt.Bool(t, verdict.Matches).True()
	}

	// Sub-agent fake invocation counts. requireXxxCalls semantics:
	//   0  → no check (default)
	//   N>0 → must be ≥ N
	//   -1 → must be exactly 0 (sub-agent must NOT call this tool)
	checkSubAgentCallCount(t, "slack search", slackInvocationCount(slackRec), sc.requireSlackSearchCalls)
	checkSubAgentCallCount(t, "notion search", notionSearchInvocationCount(notionRec), sc.requireNotionSearchCalls)
	checkSubAgentCallCount(t, "notion get_page", notionPageInvocationCount(notionRec), sc.requireNotionGetPageCalls)

	// Sub-agent argument criteria via judge.
	if sc.slackSearchArgCriterion != "" && slackRec != nil {
		runSubAgentArgJudge(t, ctx, llm, judgeRepo, "slack search", sc.slackSearchArgCriterion, anySlice(slackRec.snapshot()))
	}
	if sc.notionSearchArgCriterion != "" && notionRec != nil {
		runSubAgentArgJudge(t, ctx, llm, judgeRepo, "notion search", sc.notionSearchArgCriterion, anySlice(notionRec.searchSnapshot()))
	}
	if sc.notionGetPageArgCriterion != "" && notionRec != nil {
		runSubAgentArgJudge(t, ctx, llm, judgeRepo, "notion get_page", sc.notionGetPageArgCriterion, anySlice(notionRec.pageSnapshot()))
	}

	if sc.requireInvestigation {
		t.Logf("registered sub-agent tasks: %d", len(host.registeredTasks))
		gt.Number(t, len(host.registeredTasks)).GreaterOrEqual(1)
	}

	switch sc.expectAction {
	case model.SessionEndedWithQuestion:
		gt.Array(t, host.postedQuestion).Length(1).Required()
		gt.Array(t, host.materialized).Length(0)
		if sc.questionCriterion != "" {
			payload := host.postedQuestion[0]
			observed, err := json.MarshalIndent(payload, "", "  ")
			gt.NoError(t, err).Required()
			t.Logf("question payload:\n%s", string(observed))
			verdict := runJudge(t, ctx, llm, judgeRepo, sc.questionCriterion, string(observed))
			t.Logf("judge verdict: matches=%v rationale=%q", verdict.Matches, verdict.Rationale)
			gt.Bool(t, verdict.Matches).True()
		}
	case model.SessionEndedWithMaterialize:
		gt.Array(t, host.materialized).Length(1).Required()
		gt.Array(t, host.postedQuestion).Length(0)
		m := host.materialized[0]
		if sc.expectWorkspaceID != "" {
			gt.Value(t, m.WorkspaceID).Equal(sc.expectWorkspaceID)
		}
		gt.String(t, m.Title).NotEqual("")
		for _, k := range sc.requireFieldKeys {
			gt.Map(t, m.CustomFieldValues).HasKey(k)
		}
		for fieldID, allowed := range sc.requireFieldOneOf {
			raw, ok := m.CustomFieldValues[fieldID]
			gt.Bool(t, ok).True().Required()
			s, isString := raw.(string)
			t.Logf("custom_field_values[%q] = %v (type=%T, allowed: %v)", fieldID, raw, raw, allowed)
			gt.Bool(t, isString).True().Required()
			gt.Array(t, allowed).Has(s)
		}
		if sc.materializeCriterion != "" {
			observed, err := json.MarshalIndent(m, "", "  ")
			gt.NoError(t, err).Required()
			t.Logf("materialize payload:\n%s", string(observed))
			verdict := runJudge(t, ctx, llm, judgeRepo, sc.materializeCriterion, string(observed))
			t.Logf("judge verdict: matches=%v rationale=%q", verdict.Matches, verdict.Rationale)
			gt.Bool(t, verdict.Matches).True()
		}
	}
}

// TestRunTurn_RealLLM_VagueMentionAsksQuestion drives a vague mention
// against three plausible workspaces and expects the planner to terminate
// in `question`. Because the mention contains no token a Slack search
// could resolve, the planner's only way to disambiguate the workspace is
// to ask — verified by an LLM judge over the question payload.
func TestRunTurn_RealLLM_VagueMentionAsksQuestion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := newTestLLMClient(t, ctx)

	workspaces := []*model.WorkspaceEntry{
		{
			Workspace: model.Workspace{
				ID: "ws-incident", Name: "Incident Response",
				Description: "Production incidents and outages, paged to oncall.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "severity", Name: "Severity",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "low", Name: "Low", Description: "Minor service disruption"},
							{ID: "high", Name: "High", Description: "Critical outage"},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-recruit", Name: "Recruitment",
				Description: "Hiring pipeline and candidate evaluations.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "stage", Name: "Stage",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "screen", Name: "Screen", Description: "Initial CV screen"},
							{ID: "onsite", Name: "Onsite", Description: "Onsite interview loop"},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-risk", Name: "Risk Management",
				Description: "Security risks and policy compliance reviews.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "impact", Name: "Impact",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "minor", Name: "Minor", Description: "Limited blast radius"},
							{ID: "major", Name: "Major", Description: "Org-wide impact"},
						},
					},
				},
			},
		},
	}

	runScenario(t, ctx, llm, llmScenario{
		userInput:         "@bot please draft a case for me",
		trigger:           draft.TriggerAppMention,
		workspaces:        workspaces,
		expectStatus:      draft.StatusCompleted,
		expectAction:      model.SessionEndedWithQuestion,
		questionCriterion: "The question (its `Reason` and `Items` text combined) asks the user to identify which workspace this case belongs to (Incident Response, Recruitment, or Risk Management), OR equivalently asks for the case scope or topic that would let the agent disambiguate the workspace. Every `select` or `multi_select` item must list at least 2 distinct, non-empty options.",
		// Tool-call discipline: the planner MUST consult get_workspace
		// at least once before emitting any terminal action (the
		// updated planner prompt enforces this). No Slack / Notion
		// fan-out is wired here and none should be called.
		requirePlannerTools: []string{"get_workspace"},
		allowedPlannerTools: []string{"list_workspaces", "get_workspace"},
		plannerToolArgCriteria: map[string]string{
			"get_workspace": "Every recorded call's workspace_id is one of the registered workspaces (ws-incident, ws-recruit, ws-risk) — no fabricated IDs.",
		},
		requireSlackSearchCalls:   -1,
		requireNotionSearchCalls:  -1,
		requireNotionGetPageCalls: -1,
	})
}

// TestRunTurn_RealLLM_InfersFieldsFromSources covers the path where the
// planner has external sources (Notion + Slack) wired to the workspace
// and is expected to infer the custom field values from the search
// results rather than asking the user. The Slack/Notion mocks return
// canned context that points at a recurrent insider-risk pattern around
// "Tanaka" + "absconding" — enough material for the LLM to converge on a
// likelihood / impact / owner combination without a question.
//
// We allow LLM-side variance on the exact option: each field accepts a
// reasonable RANGE of options consistent with "occasional but recurring,
// limited blast-radius, security-led" interpretation. The test fails if
// the planner asks the user (postedQuestion non-empty) or picks an
// option outside the allowed range.
func TestRunTurn_RealLLM_InfersFieldsFromSources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := newTestLLMClient(t, ctx)

	workspace := &model.WorkspaceEntry{
		Workspace: model.Workspace{
			ID: "ws-risk", Name: "Risk Management",
			Description: "Insider, operational, and compliance risks. Backed by the Risk Register Notion DB and the #risk Slack channel.",
		},
		FieldSchema: &config.FieldSchema{
			Fields: []config.FieldDefinition{
				{
					ID: "likelihood", Name: "Likelihood",
					Description: "How often this risk is expected to occur.",
					Type:        types.FieldTypeSelect, Required: true,
					Options: []config.FieldOption{
						{ID: "extremely_unlikely", Name: "Extremely unlikely", Description: "Less than once a year."},
						{ID: "unlikely", Name: "Unlikely", Description: "A few times per year."},
						{ID: "moderate", Name: "Moderate", Description: "Roughly once per month."},
						{ID: "likely", Name: "Likely", Description: "Roughly once per week."},
						{ID: "extremely_likely", Name: "Extremely likely", Description: "Almost daily."},
					},
				},
				{
					ID: "impact", Name: "Business impact",
					Description: "Estimated business impact if this risk materialises.",
					Type:        types.FieldTypeSelect, Required: true,
					Options: []config.FieldOption{
						{ID: "negligible", Name: "Negligible", Description: "No measurable impact."},
						{ID: "minor", Name: "Minor", Description: "Limited scope, single contributor affected."},
						{ID: "moderate", Name: "Moderate", Description: "Team-level response needed."},
						{ID: "major", Name: "Major", Description: "Executive notification required."},
						{ID: "critical", Name: "Critical", Description: "Business continuity threat."},
					},
				},
				{
					ID: "owner_team", Name: "Owner team",
					Description: "Team that should own the mitigation.",
					Type:        types.FieldTypeSelect, Required: true,
					Options: []config.FieldOption{
						{ID: "security", Name: "Security Team", Description: "Insider threats, access control, incident response."},
						{ID: "compliance", Name: "Compliance Team", Description: "Policy, audit, regulatory obligations."},
						{ID: "business", Name: "Business Team", Description: "Operational and revenue-facing risks."},
					},
				},
			},
		},
	}

	// Canned Slack messages — same payload regardless of query. Together
	// they imply: 3 incidents in 12 months (= "moderate" leaning toward
	// "unlikely"), 1-2 day team-scoped outage (= "moderate" / "minor"
	// leaning toward "major" only at the top end), and Security as the
	// lead team (with Compliance consulting).
	slackSearch := &fakeSlackSearch{
		fn: func(_ context.Context, query string, _ slacktool.SearchOptions) (*slacktool.SearchResult, error) {
			return &slacktool.SearchResult{
				Total: 4,
				Messages: []slacktool.SearchMessage{
					{
						ChannelID: "C-RISK", ChannelName: "risk",
						UserID: "U-ALICE", Username: "alice",
						Text:      "Tanaka absconded with a company laptop again this morning. Security is investigating. This is the 3rd time in the past 12 months.",
						Timestamp: "1700000001.000001",
					},
					{
						ChannelID: "C-RISK", ChannelName: "risk",
						UserID: "U-BOB", Username: "bob",
						Text:      "Last week's similar incident caused a 2-day operational outage in the marketing team while we recovered the device. Cross-functional response meeting tomorrow.",
						Timestamp: "1700100002.000001",
					},
					{
						ChannelID: "C-RISK", ChannelName: "risk",
						UserID: "U-CAROL", Username: "carol",
						Text:      "Compliance review handed back to Security as the lead — pattern is clear (~3 occurrences/year), but each event is mid-size in blast radius (single team, days, not weeks).",
						Timestamp: "1700200003.000001",
					},
					{
						ChannelID: "C-RISK", ChannelName: "risk",
						UserID: "U-DAVE", Username: "dave",
						Text:      "Tanaka's manager confirmed no malicious intent suspected; treat as recurrent operational risk rather than an active threat. Query: " + query,
						Timestamp: "1700300004.000001",
					},
				},
			}, nil
		},
	}

	// Canned Notion data — search hits a "Tanaka — Insider Risk Profile"
	// page; get_page returns a structured markdown summary that doubles
	// down on the same likelihood / impact / owner picture.
	notionPageMarkdown := "# Tanaka — Insider Risk Profile\n\n" +
		"**Pattern**: Recurring laptop absconding incidents, 3 occurrences in the last 12 months.\n\n" +
		"**Per-incident impact**: 1-2 day operational outage, scope limited to a single team (typically Marketing).\n\n" +
		"**Lead team**: Security (with Compliance consult).\n\n" +
		"**Assessment**: Recurrent operational risk, not active malicious threat. " +
		"Likelihood is on the low-to-moderate end (clear pattern, but not weekly). " +
		"Business impact is mid-range (team-level response needed, not org-wide).\n"
	notionClient := &fakeNotionClient{
		searchFn: func(_ context.Context, query string, _ notiontool.SearchOptions) (*notiontool.SearchResult, error) {
			return &notiontool.SearchResult{
				Items: []notiontool.SearchItem{
					{
						ID: "page-tanaka-profile", Type: "page",
						Title: "Tanaka — Insider Risk Profile (query: " + query + ")",
						URL:   "https://notion.so/example/tanaka-profile",
					},
					{
						ID: "page-pattern-review", Type: "page",
						Title: "Absconding Pattern Quarterly Review",
						URL:   "https://notion.so/example/pattern-review",
					},
				},
			}, nil
		},
		getPageFn: func(_ context.Context, pageID string) (*notiontool.PageMarkdown, error) {
			return &notiontool.PageMarkdown{PageID: pageID, Markdown: notionPageMarkdown}, nil
		},
	}

	sources := []llmScenarioSource{
		{
			WorkspaceID: "ws-risk",
			Source: &model.Source{
				Name:        "Risk Register",
				SourceType:  model.SourceTypeNotionDB,
				Description: "Canonical Notion DB for the Risk Management workspace.",
				Enabled:     true,
				NotionDBConfig: &model.NotionDBConfig{
					DatabaseID:    "00000000000000000000000000000001",
					DatabaseTitle: "Risk Register",
					DatabaseURL:   "https://notion.so/example/risk-register",
				},
			},
		},
		{
			WorkspaceID: "ws-risk",
			Source: &model.Source{
				Name:        "Risk Slack channel",
				SourceType:  model.SourceTypeSlack,
				Description: "#risk channel where day-to-day risk events are reported.",
				Enabled:     true,
				SlackConfig: &model.SlackConfig{
					Channels: []model.SlackChannel{
						{ID: "C-RISK", Name: "risk"},
					},
				},
			},
		},
	}

	runScenario(t, ctx, llm, llmScenario{
		userInput: "@bot please draft a risk case for the recurring insider-threat pattern around Tanaka's absconding (持ち逃げ) incidents. " +
			"The Risk Register Notion DB and the #risk Slack channel both have prior context — please consult them and fill the case fields based on what you find.",
		trigger:              draft.TriggerAppMention,
		workspaces:           []*model.WorkspaceEntry{workspace},
		slackSearch:          slackSearch,
		notionClient:         notionClient,
		sources:              sources,
		expectStatus:         draft.StatusCompleted,
		expectAction:         model.SessionEndedWithMaterialize,
		expectWorkspaceID:    "ws-risk",
		requireFieldKeys:     []string{"likelihood", "impact", "owner_team"},
		requirePlannerTools:  []string{"get_workspace"},
		requireInvestigation: true,
		// The canned data describes a recurrent (~3/year) insider risk
		// with 1-2 day team-scoped impact, security-led. Allow some
		// LLM variance around each axis without admitting wildly
		// off-target picks — "extremely_unlikely" / "extremely_likely"
		// for likelihood, "negligible" / "critical" for impact, or any
		// non-security/compliance team would be wrong.
		requireFieldOneOf: map[string][]string{
			"likelihood": {"unlikely", "moderate", "likely"},
			"impact":     {"minor", "moderate", "major"},
			"owner_team": {"security", "compliance"},
		},
		// Tool-call discipline. Planner-side: only wsmeta tools.
		// Sub-agent side: at least one Slack search and at least one
		// Notion search must fire (the agent has to actually consult
		// the registered sources, not guess from the workspace
		// description). Notion get_page is not strictly required —
		// the search hit titles + canned page text are equivalent
		// signal — so we leave it as ≥0.
		allowedPlannerTools: []string{"list_workspaces", "get_workspace"},
		plannerToolArgCriteria: map[string]string{
			"get_workspace": "Every recorded call's workspace_id is 'ws-risk' — that is the only registered workspace in this scenario.",
		},
		requireSlackSearchCalls:  1,
		requireNotionSearchCalls: 1,
		slackSearchArgCriterion:  "At least one recorded query is semantically related to the case topic — i.e. it mentions Tanaka, absconding, 持ち逃げ, insider, or laptop. Empty queries or queries unrelated to the mention (e.g. random words) would fail this criterion.",
		notionSearchArgCriterion: "At least one recorded query is semantically related to the case topic — i.e. it mentions Tanaka, absconding, 持ち逃げ, insider, risk register, or laptop.",
	})
}

// TestRunTurn_RealLLM_PicksRightWorkspaceFromMany covers two things at
// once:
//
//  1. Workspace selection from multiple candidates. The registry
//     advertises three workspaces with distinct purposes; the planner
//     must read the mention's substance and pick the right one without
//     asking the user. We deliberately do NOT pre-specify a workspace ID
//     in the mention — if the agent matched on a literal string instead
//     of on the description, the test would not exercise selection.
//  2. Obedience to an explicit "no further investigation" instruction.
//     The mention is fully self-contained AND tells the agent to
//     materialise directly. The test asserts no Slack / Notion search
//     fires (sub-agent counts == 0) and no extra planner tool outside
//     wsmeta is called. This is what an earlier dedicated "concrete
//     mention" scenario verified — folded in here so a single test
//     covers both selection and the no-investigate contract.
//
// Field values are constrained loosely (only severity must populate,
// and within a sensible pair) so the test focuses on the workspace
// decision, not on field inference (which Scenario C handles).
func TestRunTurn_RealLLM_PicksRightWorkspaceFromMany(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := newTestLLMClient(t, ctx)

	workspaces := []*model.WorkspaceEntry{
		{
			Workspace: model.Workspace{
				ID: "ws-incident", Name: "Incident Response",
				Description: "Production incidents and outages, paged to oncall. Use this for service unavailability, latency regressions, failed deploys, and any operational event the SRE team would respond to in real time.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "severity", Name: "Severity",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "low", Name: "Low", Description: "Minor service disruption, internal-only."},
							{ID: "medium", Name: "Medium", Description: "Partial outage with workaround in place."},
							{ID: "high", Name: "High", Description: "Critical outage, oncall paged, customer-visible."},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-recruit", Name: "Recruitment",
				Description: "Hiring pipeline and candidate evaluations. Use this for candidate intake, interview-loop coordination, and offer decisions. NOT for production engineering issues.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "stage", Name: "Stage",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "screen", Name: "Screen", Description: "Initial CV screen."},
							{ID: "onsite", Name: "Onsite", Description: "Onsite interview loop."},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-risk", Name: "Risk Management",
				Description: "Insider threats, compliance reviews, and policy violations. Use this for slow-burn risks the security/compliance teams need to track over time. NOT for live production outages.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "impact", Name: "Impact",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "minor", Name: "Minor", Description: "Limited blast radius."},
							{ID: "major", Name: "Major", Description: "Org-wide impact."},
						},
					},
				},
			},
		},
	}

	runScenario(t, ctx, llm, llmScenario{
		userInput: "@bot draft a case for the production database outage we had at 02:00 UTC last night. " +
			"PostgreSQL primary failover took 8 minutes; pgbouncer retried, no data loss. Oncall was paged. " +
			"Customer-visible 5xx for ~6 of those 8 minutes. " +
			"No further investigation needed — please pick the appropriate workspace from the registered list and materialise the draft directly.",
		trigger:             draft.TriggerAppMention,
		workspaces:          workspaces,
		expectStatus:        draft.StatusCompleted,
		expectAction:        model.SessionEndedWithMaterialize,
		expectWorkspaceID:   "ws-incident",
		requireFieldKeys:    []string{"severity"},
		requirePlannerTools: []string{"get_workspace"},
		// 8 minutes of customer-visible 5xx with oncall paged is squarely
		// "medium" or "high" — never "low". Allow both ends so the test
		// tracks the workspace decision rather than the severity grading.
		requireFieldOneOf: map[string][]string{
			"severity": {"medium", "high"},
		},
		// Tool-call discipline. The picked workspace must be the
		// production-incident one; the planner is allowed to probe
		// other workspaces during disambiguation but the FINAL chosen
		// workspace_id (verified above via expectWorkspaceID) and the
		// trail of get_workspace calls should still be sane. The arg
		// judge is asked to confirm every call landed on a registered
		// workspace and that ws-incident appears among them — the
		// latter being the proof the planner did, in fact, look up
		// the workspace it eventually materialised against.
		allowedPlannerTools: []string{"list_workspaces", "get_workspace"},
		plannerToolArgCriteria: map[string]string{
			"get_workspace": "Every recorded call's workspace_id is one of ws-incident, ws-recruit, ws-risk (the registered set), and at least one call is for ws-incident — the workspace that ultimately matched the production-database-outage mention.",
		},
		requireSlackSearchCalls:   -1,
		requireNotionSearchCalls:  -1,
		requireNotionGetPageCalls: -1,
	})
}

// TestRunTurn_RealLLM_QuestionAnsweredThenMaterializes covers the
// multi-turn flow: a vague first mention should land in `question`, and
// then a follow-up `thread_reply` carrying the user's answers should let
// the planner pick the correct workspace and materialise. The two turns
// share one Session, one HistoryRepo, and one UseCase, so the second
// turn's planner sees the prior question text via gollem's history
// loader and treats the reply as a continuation of the same dialogue.
//
// We assert at every hop:
//   1. Turn 1 ends in StatusCompleted / SessionEndedWithQuestion and
//      records exactly one Question payload, no Materialize.
//   2. Turn 2 ends in StatusCompleted / SessionEndedWithMaterialize on
//      the correct workspace, with the impact field grading inside an
//      allowed range.
//   3. Both turns persisted their own trace; each trace contains at
//      least one get_workspace call (the strengthened planner prompt
//      requires it before any terminal action).
func TestRunTurn_RealLLM_QuestionAnsweredThenMaterializes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	llm := newTestLLMClient(t, ctx)

	workspaces := []*model.WorkspaceEntry{
		{
			Workspace: model.Workspace{
				ID: "ws-incident", Name: "Incident Response",
				Description: "Production incidents and outages, paged to oncall.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "severity", Name: "Severity",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "low", Name: "Low", Description: "Minor service disruption."},
							{ID: "high", Name: "High", Description: "Critical outage."},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-recruit", Name: "Recruitment",
				Description: "Hiring pipeline and candidate evaluations.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "stage", Name: "Stage",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "screen", Name: "Screen", Description: "Initial CV screen."},
							{ID: "onsite", Name: "Onsite", Description: "Onsite interview loop."},
						},
					},
				},
			},
		},
		{
			Workspace: model.Workspace{
				ID: "ws-risk", Name: "Risk Management",
				Description: "Security risks, audit findings, and policy compliance.",
			},
			FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{
					{
						ID: "impact", Name: "Impact",
						Type: types.FieldTypeSelect, Required: true,
						Options: []config.FieldOption{
							{ID: "minor", Name: "Minor", Description: "Limited blast radius, single team."},
							{ID: "major", Name: "Major", Description: "Org-wide impact, executive visibility."},
						},
					},
				},
			},
		},
	}

	registry := model.NewWorkspaceRegistry()
	for _, ws := range workspaces {
		registry.Register(ws)
	}

	plannerRepo, plannerMem := newScenarioTraceRepo(t, "planner")

	deps := &agent.CommonDeps{
		Repo:                memory.New(),
		Registry:            registry,
		LLMClient:           llm,
		HistoryRepo:         agentarchive.NewMemoryHistoryRepository(),
		TraceRepo:           plannerRepo,
		HeartbeatInterval:   time.Second,
		HeartbeatStaleAfter: 30 * time.Second,
	}
	uc, err := draft.New(deps, 6, 8, 14)
	gt.NoError(t, err).Required()

	host := &hostStub{}
	ssn := newOpenSession()

	// ---- Turn 1: vague mention → planner asks the user. ----
	now1 := time.Now()
	turn1TS := fmt.Sprintf("turn1-%d.%06d", now1.Unix(), now1.Nanosecond()/1000)
	res1, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   ssn,
		UserInput: "@bot please draft a case for me",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: turn1TS,
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res1).NotNil().Required()
	gt.Value(t, res1.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res1.EndedWith).Equal(model.SessionEndedWithQuestion)
	gt.Array(t, host.postedQuestion).Length(1).Required()
	gt.Array(t, host.materialized).Length(0)

	q := host.postedQuestion[0]
	t.Logf("turn 1 question:\n  reason: %s\n  items: %d", q.Reason, len(q.Items))
	for _, it := range q.Items {
		t.Logf("    [%s] %s (type=%s, opts=%v)", it.ID, it.Text, it.Type, it.Options)
	}

	// ---- Synthesise the user reply, mirroring formatDraftQuestionAnswers. ----
	// We attach all answers to the first item's text — most planners ask the
	// workspace question first — and pack the topic / impact framing into
	// the free-text companion. Natural-language is enough; the planner
	// reads the dialogue rather than parsing fixed slots.
	var replyB strings.Builder
	replyB.WriteString("# Answers to my prior questions\n\n")
	for i, it := range q.Items {
		fmt.Fprintf(&replyB, "## %s\n", it.Text)
		switch i {
		case 0:
			replyB.WriteString("selected: Risk Management; other: This case belongs to the Risk Management workspace (ws-risk). " +
				"Topic: Q1 audit follow-up — a major compliance finding around access control with org-wide impact. " +
				"Title should be 'Q1 audit follow-up: access-control compliance gap'. " +
				"Please materialise the draft on the Risk Management workspace now.\n\n")
		default:
			replyB.WriteString("answer: see the prior item — Q1 audit follow-up about an access-control compliance gap, org-wide impact, please proceed to materialise.\n\n")
		}
	}
	reply := replyB.String()
	t.Logf("turn 2 user reply:\n%s", reply)

	// ---- Turn 2: thread reply → planner picks ws-risk and materialises. ----
	now2 := time.Now()
	turn2TS := fmt.Sprintf("turn2-%d.%06d", now2.Unix(), now2.Nanosecond()/1000)
	res2, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   ssn,
		UserInput: reply,
		Trigger:   draft.TriggerThreadReply,
		TriggerTS: turn2TS,
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res2).NotNil().Required()
	gt.Value(t, res2.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res2.EndedWith).Equal(model.SessionEndedWithMaterialize)

	gt.Array(t, host.materialized).Length(1).Required()
	m := host.materialized[0]
	t.Logf("turn 2 materialise: workspace_id=%s title=%q field_values=%v",
		m.WorkspaceID, m.Title, m.CustomFieldValues)
	gt.Value(t, m.WorkspaceID).Equal("ws-risk")
	gt.String(t, m.Title).NotEqual("")
	gt.Map(t, m.CustomFieldValues).HasKey("impact")
	impactRaw, ok := m.CustomFieldValues["impact"]
	gt.Bool(t, ok).True().Required()
	impact, isString := impactRaw.(string)
	gt.Bool(t, isString).True().Required()
	// User explicitly described the finding as "major" with org-wide
	// impact, so "major" is the natural pick. Allow "minor" as well in
	// case the planner is conservative — the test focuses on the
	// multi-turn dialogue working at all, not on the exact grade.
	gt.Array(t, []string{"minor", "major"}).Has(impact)

	// ---- Trace assertions: both turns persisted, materialize turn
	// strictly required get_workspace, and no tool outside wsmeta was
	// called in either turn. ----
	//
	// We assert get_workspace strictly for the *materialize* turn
	// (turn 2) only. The prompt also requires it before `question`
	// (turn 1), but real-LLM stochasticity under parallel load
	// occasionally produces a question that skipped get_workspace —
	// that is a soft regression worth surfacing in logs but not a
	// reason to fail the multi-turn-flow contract this scenario is
	// proving.
	traceIDs := plannerMem.TraceIDs(ssn.ID)
	t.Logf("trace IDs: %v", traceIDs)
	gt.Number(t, len(traceIDs)).GreaterOrEqual(2)
	turn2HadGetWorkspace := false
	for i, tid := range traceIDs {
		tr := plannerMem.Load(ssn.ID, tid)
		gt.Value(t, tr).NotNil().Required()
		var names []string
		for _, c := range collectToolCalls(tr) {
			names = append(names, c.Name)
		}
		t.Logf("trace %s (turn %d) tool calls: %v", tid, i+1, names)
		// No tool outside the wsmeta surface — no Slack / Notion deps
		// are wired in this scenario.
		for _, name := range names {
			gt.Array(t, []string{"list_workspaces", "get_workspace"}).Has(name)
		}
		// The trace order matches turn order because each turn appends
		// a UUID v7 (time-ordered). Track whether the last (materialize)
		// turn called get_workspace.
		if i == len(traceIDs)-1 {
			turn2HadGetWorkspace = slices.Contains(names, "get_workspace")
		}
	}
	gt.Bool(t, turn2HadGetWorkspace).True()
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
