package kernel_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/auth"
	"github.com/secmon-lab/hecatoncheires/pkg/i18n"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

// probeState is the state of the probe strategy below.
type probeState struct {
	Done bool `json:"done"`
}

// probeOutput records what the strategy observed from inside a transition.
type probeOutput struct {
	Lang    string `json:"lang"`
	ActorID string `json:"actor_id"`
	Text    string `json:"text"`
}

// probeStrategy is a minimal strategy that makes exactly one LLM call and one
// tool call, then reports what the surrounding context carried. It exists so the
// middleware can be exercised through a real Kernel and a real Serve loop rather
// than by calling the handlers directly — the wiring is what these tests are
// about.
type probeStrategy struct {
	limiter agentkit.Limiter
}

func (probeStrategy) Version() int { return 1 }

func (s probeStrategy) Limit(ctx context.Context, p *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
	return s.limiter(ctx, p, m)
}

func (probeStrategy) Init(_ struct{}) (probeState, error) { return probeState{}, nil }

func (probeStrategy) Step(ctx context.Context, sys agentkit.Syscalls, st probeState) (probeState, agentkit.Decision[probeOutput], error) {
	res, err := sys.Generate(ctx, []gollem.Input{gollem.Text("probe")})
	if err != nil {
		return st, agentkit.Decision[probeOutput]{}, err
	}
	if _, err := sys.CallTool(ctx, gollem.FunctionCall{ID: "c1", Name: "probe__ping", Arguments: map[string]any{}}); err != nil {
		return st, agentkit.Decision[probeOutput]{}, err
	}

	out := probeOutput{
		Lang: string(i18n.LangFromContext(ctx)),
		Text: firstText(res.Texts),
	}
	if tok, terr := auth.TokenFromContext(ctx); terr == nil && tok != nil {
		out.ActorID = tok.Sub
	}
	st.Done = true
	return st, agentkit.Done(out), nil
}

func firstText(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	return texts[0]
}

func (probeStrategy) EncodeState(st probeState) ([]byte, error) { return json.Marshal(st) }

func (probeStrategy) DecodeState(_ int, raw []byte) (probeState, error) {
	var st probeState
	if err := json.Unmarshal(raw, &st); err != nil {
		return probeState{}, goerr.Wrap(err, "decode probe state")
	}
	return st, nil
}

func (probeStrategy) EncodeOutput(o probeOutput) ([]byte, error) { return json.Marshal(o) }

// probeTool is the one tool the probe strategy calls.
type probeTool struct{}

func (probeTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: "probe__ping", Description: "ping"}
}

func (probeTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return map[string]any{"pong": true}, nil
}

// probeLLM returns a client whose single session answers with a fixed text.
func probeLLM() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return &gollem.Response{
						Texts:       []string{"probe answer"},
						InputToken:  11,
						OutputToken: 7,
					}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

type probeRuntime struct {
	kernel *agentkit.Kernel
	agent  agentkit.Agent[struct{}]
	traces *agentarchive.MemoryTraceRepository
}

func newProbeRuntime(t *testing.T) *probeRuntime {
	t.Helper()

	traces := agentarchive.NewMemoryTraceRepository()
	budgets := kernel.Budgets{
		Root: budget.Config{MaxSteps: 8, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8},
		Task: budget.Config{MaxSteps: 8, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8},
	}

	reg := agentkit.NewRegistry()
	handle, err := agentkit.Register(reg, "probe", 1, probeStrategy{limiter: budgets.Root.Limiter()})
	gt.NoError(t, err).Required()

	k, err := kernel.Build(kernel.Deps{
		Repo:    agentprocmemory.New(),
		History: agentarchive.NewMemoryHistoryStore(),
		LLM:     probeLLM(),
		Trace:   traces,
		Budgets: budgets,
		Agents:  reg,
		Tools: kernel.ToolDeps{
			Repo:      memory.New(),
			Registry:  testRegistry(),
			JiraTools: []gollem.Tool{probeTool{}},
		},
	})
	gt.NoError(t, err).Required()

	return &probeRuntime{kernel: k, agent: handle, traces: traces}
}

// runToCompletion serves until the spawned Process reaches a terminal state.
func runToCompletion(t *testing.T, rt *probeRuntime, sc kernel.Scope) *agentkit.Process {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() {
		served <- rt.kernel.Serve(ctx, agentkit.WithPollInterval(10*time.Millisecond))
	}()

	pid, err := rt.agent.Spawn(ctx, rt.kernel, struct{}{}, agentkit.WithMetadata(sc.Metadata()))
	gt.NoError(t, err).Required()

	var proc *agentkit.Process
	for {
		got, err := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if got.Status.Terminal() {
			proc = got
			break
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-served
	return proc
}

// TestClaimMiddlewareEstablishesTheRequestScope pins that the language and the
// access actor recorded at Spawn are what a transition actually sees. Without
// them the agent renders English copy for a Japanese user and every private-case
// gate reads as "no membership".
func TestClaimMiddlewareEstablishesTheRequestScope(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{
		ActorUserID: "U-actor",
		Lang:        string(i18n.LangJA),
		ToolSets:    []string{agent.ToolSetJira},
	})

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	var out probeOutput
	gt.NoError(t, json.Unmarshal(proc.Output, &out)).Required()
	gt.String(t, out.Lang).Equal(string(i18n.LangJA))
	gt.String(t, out.ActorID).Equal("U-actor")
	gt.String(t, out.Text).Equal("probe answer")
}

// TestEffectMiddlewareRecordsTheTrace pins that both effect boundaries reach the
// trace sink. agentkit builds its own gollem session and never runs
// gollem.WithTrace, so without this middleware the archive would be empty for
// every run.
func TestEffectMiddlewareRecordsTheTrace(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	ids := rt.traces.TraceIDs(string(proc.RootID))
	gt.Array(t, ids).Length(1).Required()

	tr := rt.traces.Load(string(proc.RootID), ids[0])
	gt.Value(t, tr).NotNil().Required()

	kinds := spanKinds(tr)
	gt.Bool(t, kinds["llm_call"]).True()
	gt.Bool(t, kinds["tool_exec"]).True()
}

// TestClaimTraceIDIsPerClaim pins that the archive id identifies one CLAIM, not
// one Process. trace.Repository.Save overwrites by id, and a claim that dies
// before committing is reclaimed at the same transition count — so the id has to
// carry the lease token, which is the one value that differs per claim.
func TestClaimTraceIDIsPerClaim(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})

	ids := rt.traces.TraceIDs(string(proc.RootID))
	gt.Array(t, ids).Length(1).Required()

	prefix := string(proc.ID) + ".0."
	gt.String(t, ids[0]).HasPrefix(prefix)
	gt.String(t, strings.TrimPrefix(ids[0], prefix)).NotEqual("")
}

// TestTokensAreMeteredOntoTheProcess pins that the metrics the budget reads are
// actually accumulated. A Limit that never sees tokens is a budget that never
// triggers.
func TestTokensAreMeteredOntoTheProcess(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})

	gt.Value(t, proc.Metrics.InputTokens).Equal(int64(11))
	gt.Value(t, proc.Metrics.OutputTokens).Equal(int64(7))
	gt.Value(t, proc.Metrics.LLMCalls).Equal(int64(1))
	gt.Value(t, proc.Metrics.ToolCalls).Equal(int64(1))
}

// TestRunTimelineLinksAToolCallAcrossAClaimBoundary drives a real ReAct run with
// ONE transition per claim, which is the arrangement the timeline has to survive:
// the LLM call and the tool call it asked for land in different claims, each with
// its own Handler. A Handler that only remembered the response in memory would
// emit ParentSequence 0 here, and JobRunEvent.Validate would drop the tool row
// from the timeline altogether.
func TestRunTimelineLinksAToolCallAcrossAClaimBoundary(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	seedCase(t, ctx, repo, &model.Case{Title: "timeline target"})

	cfg := budget.Config{MaxSteps: 16, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	reg := agentkit.NewRegistry()
	handle, err := react.Register(reg, kernel.AgentCaseChannel, 1, cfg.Limiter(),
		agentkit.WithHistoryStore[react.Output](agentarchive.NewMemoryHistoryStore()))
	gt.NoError(t, err).Required()

	// One tool call, then an answer.
	var calls atomic.Int32
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					if calls.Add(1) == 1 {
						return &gollem.Response{
							FunctionCalls: []*gollem.FunctionCall{
								{ID: "c1", Name: "probe__ping", Arguments: map[string]any{}},
							},
							InputToken: 10, OutputToken: 5,
						}, nil
					}
					return &gollem.Response{Texts: []string{"done"}, InputToken: 3, OutputToken: 2}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}

	k, err := kernel.Build(kernel.Deps{
		Repo:    agentprocmemory.New(),
		History: agentarchive.NewMemoryHistoryStore(),
		LLM:     llm,
		Trace:   agentarchive.NewMemoryTraceRepository(),
		Budgets: kernel.Budgets{Root: cfg, Task: cfg},
		Agents:  reg,
		Tools: kernel.ToolDeps{
			Repo:      repo,
			Registry:  testRegistry(channelWorkspace()),
			JiraTools: []gollem.Tool{probeTool{}},
		},
	})
	gt.NoError(t, err).Required()

	sc := kernel.Scope{
		WorkspaceID: "ws-1", CaseID: 1, ActorUserID: "U1",
		ToolSets: []string{agent.ToolSetJira},
		JobID:    "job-mention", JobRunID: "run-mention",
	}

	serveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- k.Serve(serveCtx,
			agentkit.WithPollInterval(5*time.Millisecond),
			// One transition per claim, so every LLM call and every tool call gets
			// its own claim — and its own Handler.
			agentkit.WithMaxStepsPerClaim(1),
		)
	}()

	pid, err := handle.Spawn(ctx, k, react.Input{SystemPrompt: "be helpful", Prompt: "ping it"},
		agentkit.WithMetadata(sc.Metadata()))
	gt.NoError(t, err).Required()

	for {
		proc, gerr := k.GetProcess(serveCtx, pid)
		gt.NoError(t, gerr).Required()
		if proc.Status.Terminal() {
			gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
			break
		}
		select {
		case <-serveCtx.Done():
			gt.NoError(t, serveCtx.Err()).Required()
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-served

	events, err := repo.JobRunEvent().List(ctx,
		model.JobRunKey{WorkspaceID: "ws-1", CaseID: 1, JobID: "job-mention"}, "run-mention")
	gt.NoError(t, err).Required()

	// Two LLM calls (request + response each) and one tool call.
	var toolEvents []*model.JobRunEvent
	responses := map[int64]bool{}
	for _, ev := range events {
		switch ev.Kind {
		case model.JobRunEventKindLLMResponse:
			responses[ev.Sequence] = true
		case model.JobRunEventKindToolCall:
			toolEvents = append(toolEvents, ev)
		}
	}
	gt.Array(t, toolEvents).Length(1).Required()
	gt.String(t, toolEvents[0].ToolCall.ToolName).Equal("probe__ping")
	// The link survived the claim boundary and points at a real LLM_RESPONSE.
	gt.Number(t, toolEvents[0].ParentSequence).GreaterOrEqual(1)
	gt.Bool(t, responses[toolEvents[0].ParentSequence]).True()
}

// spanKinds collects the span kinds recorded in a trace, at any depth.
func spanKinds(t *trace.Trace) map[string]bool {
	out := map[string]bool{}
	var walk func(spans []*trace.Span)
	walk = func(spans []*trace.Span) {
		for _, s := range spans {
			if s == nil {
				continue
			}
			out[string(s.Kind)] = true
			walk(s.Children)
		}
	}
	if t != nil && t.RootSpan != nil {
		walk([]*trace.Span{t.RootSpan})
	}
	return out
}
