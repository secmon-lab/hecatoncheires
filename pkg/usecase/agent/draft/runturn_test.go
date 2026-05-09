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
	mu             sync.Mutex
	postedMessages []string
	postedQuestion []draft.QuestionPayload
	materialized   []draft.MaterializePayload
	traceLines     []string
	busyCalls      int
}

func (h *hostStub) PostMessage(_ context.Context, _ *model.Session, text string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.postedMessages = append(h.postedMessages, text)
	return nil
}
func (h *hostStub) PostQuestion(_ context.Context, _ *model.Session, q draft.QuestionPayload) error {
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

func TestRunTurn_PostMessageHappyPath(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"already enough","action":"post_message","post_message":{"text":"Got it, thanks."}}`,
	})
	uc := mustDraft(t, llm, 8, 16)

	host := &hostStub{}
	res, err := uc.RunTurn(ctx, draft.TurnRequest{
		Session:   newOpenSession(),
		UserInput: "@bot just say hi",
		Trigger:   draft.TriggerAppMention,
		TriggerTS: "1700000001.000001",
		Handler:   host,
	})
	async.Wait()
	gt.NoError(t, err).Required()
	gt.Value(t, res).NotNil().Required()
	gt.Value(t, res.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res.EndedWith).Equal(model.SessionEndedWithMessage)
	gt.Array(t, host.postedMessages).Length(1)
	gt.String(t, host.postedMessages[0]).Equal("Got it, thanks.")
	gt.Array(t, host.materialized).Length(0)
	gt.Array(t, host.postedQuestion).Length(0)
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

	// Trace should mention both planning rounds and the investigation.
	traceJoined := strings.Join(host.traceLines, "\n")
	gt.String(t, traceJoined).Contains("Planning [1/8]")
	gt.String(t, traceJoined).Contains("Planning [2/8]")
	gt.String(t, traceJoined).Contains("Recent thread")
	gt.String(t, traceJoined).Contains("inner loops")
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
	gt.Value(t, res.Status).Equal(draft.StatusCompleted)
	gt.Value(t, res.EndedWith).Equal(model.SessionEndedWithMessage)
	// Fallback message went to the host.
	gt.Array(t, host.postedMessages).Length(1).Required()
	gt.String(t, host.postedMessages[0]).Contains("couldn't reach a conclusion")
}

func TestRunTurn_BusyShortCircuits(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"x","action":"post_message","post_message":{"text":"first"}}`,
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
	gt.Array(t, host.postedMessages).Length(0)
}

func TestRunTurn_IdempotentRetryDropsSilently(t *testing.T) {
	ctx := context.Background()
	llm := newScriptedLLM([]string{
		`{"reasoning":"x","action":"post_message","post_message":{"text":"first"}}`,
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
	gt.Array(t, host.postedMessages).Length(0)
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
