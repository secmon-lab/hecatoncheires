package kernel_test

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

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
	// Three array parameters, all optional. Optional keeps the calls that pass no
	// arguments valid, and three of the same type reproduces the situation the
	// argument-feedback middleware exists for: gollem reports the expectation
	// without saying which parameter carried it, so one rejected call has to be
	// attributable to one of the three from the shape alone.
	arrayOfString := func(desc string) *gollem.Parameter {
		return &gollem.Parameter{
			Type:        gollem.TypeArray,
			Description: desc,
			Items:       &gollem.Parameter{Type: gollem.TypeString},
		}
	}
	return gollem.ToolSpec{
		Name:        "probe__ping",
		Description: "ping",
		Parameters: map[string]*gollem.Parameter{
			"items":    arrayOfString("items to ping"),
			"targets":  arrayOfString("targets to ping"),
			"archives": arrayOfString("ids to archive"),
		},
	}
}

func (probeTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return map[string]any{"pong": true}, nil
}

// probeModelName is what the probe client claims to have called, standing in for
// the model a real provider names on its trace data.
const probeModelName = "probe-model-1"

// probeLLM is a client whose single session answers with a fixed text. It records
// the session settings agentkit derived, and reports its model the way a real
// provider client does — through the trace handler in the context, which is the
// only channel that carries the name.
type probeLLM struct {
	mu          sync.Mutex
	promptCache []bool
}

func (p *probeLLM) client() gollem.LLMClient {
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, opts ...gollem.SessionOption) (gollem.Session, error) {
			cfg := gollem.NewSessionConfig(opts...)
			p.mu.Lock()
			p.promptCache = append(p.promptCache, cfg.PromptCache())
			p.mu.Unlock()

			return &mock.SessionMock{
				GenerateFunc: func(ctx context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					if h := trace.HandlerFrom(ctx); h != nil {
						callCtx := h.StartLLMCall(ctx)
						h.EndLLMCall(callCtx, &trace.LLMCallData{Model: probeModelName}, nil)
					}
					return &gollem.Response{
						Texts:                   []string{"probe answer"},
						InputToken:              11,
						OutputToken:             7,
						CacheReadInputToken:     5,
						CacheCreationInputToken: 3,
					}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
}

// promptCacheFlags returns the prompt-cache setting of every session built so far.
func (p *probeLLM) promptCacheFlags() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.promptCache)
}

type probeRuntime struct {
	kernel *agentkit.Kernel
	agent  agentkit.Agent[struct{}]
	traces *agentarchive.MemoryTraceRepository
	llm    *probeLLM
}

// newProbeRuntime builds the Kernel the probe strategy runs on. tool overrides
// what probe__ping resolves to, so a test can substitute a tool that does
// something other than answer.
func newProbeRuntime(t *testing.T, tool ...gollem.Tool) *probeRuntime {
	t.Helper()

	tools := []gollem.Tool{probeTool{}}
	if len(tool) > 0 {
		tools = tool
	}

	traces := agentarchive.NewMemoryTraceRepository()
	budgets := kernel.Budgets{
		Root: budget.Config{MaxSteps: 8, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8},
		Task: budget.Config{MaxSteps: 8, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8},
	}

	reg := agentkit.NewRegistry()
	handle, err := agentkit.Register(reg, "probe", 1, probeStrategy{limiter: budgets.Root.Limiter()})
	gt.NoError(t, err).Required()

	llm := &probeLLM{}
	k, err := kernel.Build(kernel.Deps{
		Repo:    agentprocmemory.New(),
		History: agentarchive.NewMemoryHistoryStore(),
		LLM:     llm.client(),
		Trace:   traces,
		Budgets: budgets,
		Agents:  reg,
		Tools: kernel.ToolDeps{
			Repo:      memory.New(),
			Registry:  testRegistry(),
			JiraTools: tools,
		},
	})
	gt.NoError(t, err).Required()

	return &probeRuntime{kernel: k, agent: handle, traces: traces, llm: llm}
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

// TestPromptCacheIsOnForEveryGenerate pins that every session agentkit builds
// asks the provider for prompt caching.
//
// The pre-agentkit hosts each passed gollem.WithPromptCache to the client they
// built; agentkit builds the session itself, so the setting has only one place
// left to come from. Losing it took the cache hit rate on scheduled Job runs from
// ~80% of input tokens to zero, and a cache read bills at roughly a tenth of the
// base input rate.
func TestPromptCacheIsOnForEveryGenerate(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	gt.Array(t, rt.llm.promptCacheFlags()).Equal([]bool{true})
}

// TestTheRecordedLLMCallNamesTheModel pins that the model a provider reports
// reaches the recorded call.
//
// agentkit.GenerateResult carries the tokens but not the model, so the name has
// to be taken from the trace callback the provider client drives — and taken
// without letting that client record the call a second time. An empty model on
// every event is what the run-detail page and the cost aggregations showed after
// the migration.
func TestTheRecordedLLMCallNamesTheModel(t *testing.T) {
	rt := newProbeRuntime(t)
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	ids := rt.traces.TraceIDs(string(proc.RootID))
	gt.Array(t, ids).Length(1).Required()

	calls := llmCallSpans(rt.traces.Load(string(proc.RootID), ids[0]))
	// Exactly one: the provider's own Start/End pair must not add a second span.
	gt.Array(t, calls).Length(1).Required()
	gt.String(t, calls[0].Model).Equal(probeModelName)
	gt.Value(t, calls[0].CacheReadInputTokens).Equal(5)
	gt.Value(t, calls[0].CacheCreationInputTokens).Equal(3)
}

// probeEmbeddingModelName is what the tool below claims to have called, standing
// in for the embedding model a knowledge tool reaches.
const probeEmbeddingModelName = "probe-embedding-1"

// embeddingProbeTool stands in for a tool that talks to an LLM itself — the
// knowledge tools' embedding calls, webfetch's page analysis. gollem's clients
// find the handler in the context and nowhere else, so a tool whose context
// carries none is a tool whose LLM calls are invisible.
type embeddingProbeTool struct{}

func (embeddingProbeTool) Spec() gollem.ToolSpec { return probeTool{}.Spec() }

func (embeddingProbeTool) Run(ctx context.Context, _ map[string]any) (map[string]any, error) {
	h := trace.HandlerFrom(ctx)
	if h == nil {
		return nil, goerr.New("no trace handler reached the tool")
	}
	callCtx := h.StartLLMCall(ctx)
	h.EndLLMCall(callCtx, &trace.LLMCallData{Model: probeEmbeddingModelName, InputTokens: 4}, nil)
	return map[string]any{"pong": true}, nil
}

// TestAToolsOwnLLMCallIsRecorded pins that a tool reaching an LLM directly still
// appears on the trace.
//
// The pre-agentkit hosts got this from gollem.WithTrace, which published the
// handler for the whole Execute; after the migration nothing published it, so the
// knowledge tools' embedding calls stopped being recorded at all.
func TestAToolsOwnLLMCallIsRecorded(t *testing.T) {
	rt := newProbeRuntime(t, embeddingProbeTool{})
	proc := runToCompletion(t, rt, kernel.Scope{ToolSets: []string{agent.ToolSetJira}})
	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	ids := rt.traces.TraceIDs(string(proc.RootID))
	gt.Array(t, ids).Length(1).Required()

	var models []string
	for _, call := range llmCallSpans(rt.traces.Load(string(proc.RootID), ids[0])) {
		models = append(models, call.Model)
	}
	slices.Sort(models)
	gt.Array(t, models).Equal([]string{probeEmbeddingModelName, probeModelName})
}

// llmCallSpans collects the call data of every llm_call span in a trace.
func llmCallSpans(tr *trace.Trace) []*trace.LLMCallData {
	var out []*trace.LLMCallData
	var walk func(spans []*trace.Span)
	walk = func(spans []*trace.Span) {
		for _, s := range spans {
			if s == nil {
				continue
			}
			if s.Kind == trace.SpanKindLLMCall && s.LLMCall != nil {
				out = append(out, s.LLMCall)
			}
			walk(s.Children)
		}
	}
	if tr != nil && tr.RootSpan != nil {
		walk([]*trace.Span{tr.RootSpan})
	}
	return out
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
	gt.Value(t, proc.Metrics.CacheReadInputTokens).Equal(int64(5))
	gt.Value(t, proc.Metrics.CacheCreationInputTokens).Equal(int64(3))
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

// TestARejectedToolCallTellsTheModelWhatItSent drives a real Kernel through a
// tool call whose argument has the wrong type, and pins that what comes back to
// the model identifies the offending argument.
//
// gollem's own message names the tool and the expectation but not the parameter,
// so a tool with several same-typed parameters leaves the model guessing and
// re-emitting the same call. The received shape is what closes that gap, and it
// has to survive the whole path: the middleware chain, the strategy's tool
// response, and the next LLM call's inputs.
func TestARejectedToolCallTellsTheModelWhatItSent(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	seedCase(t, ctx, repo, &model.Case{Title: "argument feedback target"})

	cfg := budget.Config{MaxSteps: 16, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
	reg := agentkit.NewRegistry()
	handle, err := react.Register(reg, kernel.AgentCaseChannel, 1, cfg.Limiter(),
		agentkit.WithHistoryStore[react.Output](agentarchive.NewMemoryHistoryStore()))
	gt.NoError(t, err).Required()

	var calls atomic.Int32
	var reported atomic.Value
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, inputs []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					if calls.Add(1) == 1 {
						return &gollem.Response{
							FunctionCalls: []*gollem.FunctionCall{{
								ID:   "c1",
								Name: "probe__ping",
								// Two of the three arrays are well formed; targets is a
								// stringified array. gollem reports one "expected array
								// type" for the call and names no parameter, so only the
								// shape can attribute it to targets.
								Arguments: map[string]any{
									"items":    []any{"a", "b"},
									"targets":  `["c","d"]`,
									"archives": []any{"e"},
								},
							}},
							InputToken: 10, OutputToken: 5,
						}, nil
					}
					for _, in := range inputs {
						if res, ok := in.(gollem.FunctionResponse); ok && res.Error != nil {
							reported.Store(res.Error.Error())
						}
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
		JobID:    "job-feedback", JobRunID: "run-feedback",
	}

	serveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- k.Serve(serveCtx, agentkit.WithPollInterval(5*time.Millisecond))
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

	// The failure was answered rather than swallowed: the run continued to a
	// second LLM call, and that call carried the tool's error.
	told, ok := reported.Load().(string)
	gt.Bool(t, ok).True().Required()

	// gollem's half: which tool, and what it expected.
	gt.String(t, told).Contains(`"probe__ping"`)
	gt.String(t, told).Contains("expected array type")
	// The half this middleware adds. All three arrays are named, and only
	// targets contradicts the expectation — which is the attribution gollem's
	// own message cannot make.
	gt.String(t, told).Contains(
		"The arguments received were: archives=array[1] of string, items=array[2] of string, targets=string")

	// The run timeline records the SAME message the model was given. This is what
	// pins the middleware order: registered outside the trace bracket instead of
	// inside it, the timeline would keep gollem's unattributed message and an
	// operator reading the run would not see which argument was refused.
	events, err := repo.JobRunEvent().List(ctx,
		model.JobRunKey{WorkspaceID: "ws-1", CaseID: 1, JobID: "job-feedback"}, "run-feedback")
	gt.NoError(t, err).Required()

	var toolEvents []*model.JobRunEvent
	for _, ev := range events {
		if ev.Kind == model.JobRunEventKindToolCall {
			toolEvents = append(toolEvents, ev)
		}
	}
	gt.Array(t, toolEvents).Length(1).Required()
	gt.Bool(t, toolEvents[0].ToolCall.IsError).True()
	gt.String(t, toolEvents[0].ToolCall.ErrorMessage).Equal(told)
}

// TestToolArgsFeedbackLeavesEveryOtherErrorAlone pins the two properties the
// argument-feedback middleware must hold for its callers: a rejected call still
// answers errors.Is against gollem's sentinel, and anything that is not an
// argument rejection is passed through untouched.
//
// Neither is reachable through a Kernel — a real tool call can only produce the
// errors a real tool produces — so the middleware is driven directly here.
func TestToolArgsFeedbackLeavesEveryOtherErrorAlone(t *testing.T) {
	ctx := context.Background()
	req := &agentkit.ToolCallRequest{
		Call: gollem.FunctionCall{ID: "c1", Name: "probe__ping", Arguments: map[string]any{"items": "x"}},
	}

	t.Run("an argument rejection keeps gollem's sentinel and gains the shape", func(t *testing.T) {
		spec := probeTool{}.Spec()
		cause := goerr.Wrap(spec.ValidateArgs(req.Call.Arguments), "validate tool args")
		h := kernel.ToolArgsFeedbackHandlerForTest(
			func(context.Context, *agentkit.ToolCallRequest) (map[string]any, error) {
				return nil, cause
			})

		out, err := h(ctx, req)
		gt.Value(t, out).Nil()
		gt.Error(t, err).Is(gollem.ErrToolArgsValidation)
		gt.String(t, err.Error()).Contains("The arguments received were: items=string")
	})

	t.Run("a tool's own failure is passed through unchanged", func(t *testing.T) {
		cause := goerr.New("the backend refused the write")
		h := kernel.ToolArgsFeedbackHandlerForTest(
			func(context.Context, *agentkit.ToolCallRequest) (map[string]any, error) {
				return nil, cause
			})

		_, err := h(ctx, req)
		gt.Value(t, err).Equal(cause)
	})

	t.Run("a successful call is passed through unchanged", func(t *testing.T) {
		want := map[string]any{"pong": true}
		h := kernel.ToolArgsFeedbackHandlerForTest(
			func(context.Context, *agentkit.ToolCallRequest) (map[string]any, error) {
				return want, nil
			})

		out, err := h(ctx, req)
		gt.NoError(t, err)
		gt.Value(t, out).Equal(want)
	})
}

// TestRejectedArgumentsAreDescribedByShapeNotByValue pins the wording of the
// shape line and, more importantly, that no argument VALUE appears in it. The
// line reaches the operator's Sentry and the run timeline as well as the model,
// and tool arguments carry case content.
func TestRejectedArgumentsAreDescribedByShapeNotByValue(t *testing.T) {
	testCases := map[string]struct {
		args map[string]any
		want string
	}{
		"no arguments at all": {
			args: map[string]any{},
			want: "no arguments",
		},
		"names are sorted so one call always reads the same way": {
			args: map[string]any{"updates": nil, "archives": []any{}, "creates": "x"},
			want: "archives=array[0], creates=string, updates=null",
		},
		"scalars are named by type, never by value": {
			args: map[string]any{"count": 3.0, "flag": true, "title": "a secret memo title"},
			want: "count=number, flag=boolean, title=string",
		},
		"an array reports its length and its element shape": {
			args: map[string]any{"archives": []any{"id-1", "id-2"}},
			want: "archives=array[2] of string",
		},
		// The shape of a real memo__apply_memo_changes entry: creates[] holds an
		// object whose fields[] holds objects carrying values[]. gollem reports a
		// violation anywhere in there without saying where, so every level has to
		// be reachable.
		"the deepest position a tool spec declares is still described": {
			args: map[string]any{"creates": []any{
				map[string]any{
					"title": "confidential",
					"fields": []any{
						map[string]any{"field_id": "severity", "values": []any{"high"}},
					},
				},
			}},
			want: "creates=array[1] of object{fields: array[1] of object{field_id: string, values: array[1] of string}, title: string}",
		},
		// The entry gollem refused is the one that differs, and gollem drops its
		// index — so a mixed array must not be collapsed onto its first entry.
		"a mixed array is listed per index rather than collapsed": {
			args: map[string]any{"creates": []any{
				map[string]any{"title": "ok"},
				"a stringified entry",
			}},
			want: "creates=array[2]{0: object{title: string}, 1: string}",
		},
		"a long shape is cut on a rune boundary, not a byte offset": {
			args: longMultibyteArgs(),
			want: longMultibyteArgsWant(),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := kernel.DescribeArgsForTest(tc.args)
			gt.String(t, got).Equal(tc.want)
			gt.Bool(t, utf8.ValidString(got)).True()
		})
	}
}

// longMultibyteArgs builds arguments whose rendered shape overruns the length
// bound with a multi-byte object key straddling the cut. A workspace names its
// own memo field ids, so a non-ASCII key is ordinary input, and cutting at a
// byte offset would put a broken rune in front of the model and in Sentry.
func longMultibyteArgs() map[string]any {
	fields := map[string]any{}
	for i := range 120 {
		fields["フィールド"+strconv.Itoa(i)] = "v"
	}
	return map[string]any{"creates": []any{map[string]any{"fields": fields}}}
}

// longMultibyteArgsWant renders what describeArgs must produce for
// longMultibyteArgs: the untruncated shape, cut back to the last rune boundary
// at or before the bound, with the marker appended.
func longMultibyteArgsWant() string {
	fields := longMultibyteArgs()["creates"].([]any)[0].(map[string]any)["fields"].(map[string]any)

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": string")
	}
	full := "creates=array[1] of object{fields: object{" + strings.Join(parts, ", ") + "}}"

	cut := kernel.ArgShapeMaxLenForTest
	for cut > 0 && !utf8.RuneStart(full[cut]) {
		cut--
	}
	return full[:cut] + "..."
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
