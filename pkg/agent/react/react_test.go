package react_test

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
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
)

// recordingTool records every call so a test can assert what the model actually
// executed, not merely how many times something happened.
type recordingTool struct {
	name string
	err  error

	mu    sync.Mutex
	calls []map[string]any
}

func (t *recordingTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: t.name, Description: "test tool"}
}

func (t *recordingTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	t.calls = append(t.calls, args)
	t.mu.Unlock()
	if t.err != nil {
		return nil, t.err
	}
	return map[string]any{"ok": true}, nil
}

func (t *recordingTool) Calls() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]map[string]any, len(t.calls))
	copy(out, t.calls)
	return out
}

// scriptedLLM answers with responses[i] on the i-th Generate. An extra call
// beyond the script fails the test rather than silently repeating the last
// answer.
func scriptedLLM(t *testing.T, responses ...*gollem.Response) (gollem.LLMClient, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	client := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					i := int(n.Add(1)) - 1
					if i >= len(responses) {
						return nil, goerr.New("unexpected extra generate call", goerr.V("call_index", i))
					}
					return responses[i], nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
	return client, &n
}

type runtime struct {
	kernel *agentkit.Kernel
	agent  agentkit.Agent[react.Input]
}

func newRuntime(t *testing.T, llm gollem.LLMClient, cfg budget.Config, tools ...gollem.Tool) *runtime {
	t.Helper()

	reg := agentkit.NewRegistry()
	handle, err := react.Register(reg, "react-test", 1, cfg.Limiter(),
		agentkit.WithHistoryStore[react.Output](agentarchive.NewMemoryHistoryStore()))
	gt.NoError(t, err).Required()

	k, err := agentkit.New(agentprocmemory.New(), llm, reg,
		agentkit.WithToolFactory(func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return tools, nil
		}))
	gt.NoError(t, err).Required()

	return &runtime{kernel: k, agent: handle}
}

// run drives the process to a terminal state and returns it.
func (rt *runtime) run(t *testing.T, in react.Input) *agentkit.Process {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- rt.kernel.Serve(ctx, agentkit.WithPollInterval(5*time.Millisecond)) }()

	pid, err := rt.agent.Spawn(ctx, rt.kernel, in)
	gt.NoError(t, err).Required()

	for {
		proc, err := rt.kernel.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			cancel()
			<-served
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func textResponse(text string) *gollem.Response {
	return &gollem.Response{Texts: []string{text}, InputToken: 5, OutputToken: 3}
}

func callResponse(calls ...*gollem.FunctionCall) *gollem.Response {
	return &gollem.Response{FunctionCalls: calls, InputToken: 5, OutputToken: 3}
}

func generousBudget() budget.Config {
	return budget.Config{MaxSteps: 64, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8}
}

// TestAnswerWithoutTools pins the shortest run: one LLM call, no tools, done.
func TestAnswerWithoutTools(t *testing.T) {
	llm, calls := scriptedLLM(t, textResponse("the answer"))
	rt := newRuntime(t, llm, generousBudget())

	proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "what is it"})

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	out, err := react.DecodeOutput(proc.Output)
	gt.NoError(t, err).Required()
	gt.Array(t, out.Texts).Equal([]string{"the answer"})
	gt.Value(t, calls.Load()).Equal(int32(1))
	gt.Value(t, proc.Metrics.Steps).Equal(int64(1))
}

// TestOneToolCallPerTransition is the property this strategy exists for: a
// response asking for two tools becomes two separate committed transitions, so
// a crash costs at most one tool call rather than the whole round.
func TestOneToolCallPerTransition(t *testing.T) {
	tool := &recordingTool{name: "probe__ping"}
	llm, _ := scriptedLLM(t,
		callResponse(
			&gollem.FunctionCall{ID: "c1", Name: "probe__ping", Arguments: map[string]any{"n": float64(1)}},
			&gollem.FunctionCall{ID: "c2", Name: "probe__ping", Arguments: map[string]any{"n": float64(2)}},
		),
		textResponse("done"),
	)
	rt := newRuntime(t, llm, generousBudget(), tool)

	proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "run twice"})

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)

	// generate, tool, tool, generate.
	gt.Value(t, proc.Metrics.Steps).Equal(int64(4))
	gt.Value(t, proc.Metrics.LLMCalls).Equal(int64(2))
	gt.Value(t, proc.Metrics.ToolCalls).Equal(int64(2))

	got := tool.Calls()
	gt.Array(t, got).Length(2).Required()
	gt.Value(t, got[0]["n"]).Equal(float64(1))
	gt.Value(t, got[1]["n"]).Equal(float64(2))

	out, err := react.DecodeOutput(proc.Output)
	gt.NoError(t, err).Required()
	gt.Array(t, out.Texts).Equal([]string{"done"})
}

// TestFailingToolIsReportedToTheModel pins that a tool error does not fail the
// run: the model is told and gets to react, which is how the previous runtime
// behaved.
func TestFailingToolIsReportedToTheModel(t *testing.T) {
	tool := &recordingTool{name: "probe__ping", err: goerr.New("backend unavailable")}
	llm, _ := scriptedLLM(t,
		callResponse(&gollem.FunctionCall{ID: "c1", Name: "probe__ping", Arguments: map[string]any{}}),
		textResponse("the backend is down"),
	)
	rt := newRuntime(t, llm, generousBudget(), tool)

	proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "try it"})

	gt.Value(t, proc.Status).Equal(agentkit.ProcessSucceeded)
	gt.Array(t, tool.Calls()).Length(1)
	out, err := react.DecodeOutput(proc.Output)
	gt.NoError(t, err).Required()
	gt.Array(t, out.Texts).Equal([]string{"the backend is down"})
}

// TestTextsAccumulateAcrossRounds pins that a model narrating between tool calls
// keeps every block, in order — the reply a host posts is built from all of them.
func TestTextsAccumulateAcrossRounds(t *testing.T) {
	tool := &recordingTool{name: "probe__ping"}
	llm, _ := scriptedLLM(t,
		&gollem.Response{
			Texts:         []string{"let me check"},
			FunctionCalls: []*gollem.FunctionCall{{ID: "c1", Name: "probe__ping", Arguments: map[string]any{}}},
		},
		textResponse("all good"),
	)
	rt := newRuntime(t, llm, generousBudget(), tool)

	proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "check"})

	out, err := react.DecodeOutput(proc.Output)
	gt.NoError(t, err).Required()
	gt.Array(t, out.Texts).Equal([]string{"let me check", "all good"})
	gt.String(t, out.Text()).Equal("let me check\nall good")
}

// TestStepBudgetStopsTheRun pins that the ceiling actually ends a run that will
// not stop on its own, and that it does so as a limit failure rather than a
// generic error.
func TestStepBudgetStopsTheRun(t *testing.T) {
	tool := &recordingTool{name: "probe__ping"}
	responses := make([]*gollem.Response, 0, 20)
	for range 20 {
		responses = append(responses, callResponse(&gollem.FunctionCall{
			ID: "c", Name: "probe__ping", Arguments: map[string]any{},
		}))
	}
	llm, _ := scriptedLLM(t, responses...)
	rt := newRuntime(t, llm, budget.Config{
		MaxSteps: 6, MaxInputTokens: 100_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8,
	}, tool)

	proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "loop forever"})

	gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
	gt.Value(t, proc.Failure).NotNil().Required()
	gt.Value(t, proc.Failure.Code).Equal(agentkit.FailureLimitExceeded)
	gt.Number(t, proc.Metrics.Steps).GreaterOrEqual(6)
}

// TestTokenBudgetsStopTheRun pins the two token ceilings independently of the
// step count and of each other. A run with plenty of steps left still stops once
// it has spent either allowance, and the output ceiling — the expensive one —
// bites even while the input allowance is barely touched.
func TestTokenBudgetsStopTheRun(t *testing.T) {
	testCases := map[string]struct {
		cfg    budget.Config
		input  int
		output int
	}{
		"input ceiling": {
			cfg:    budget.Config{MaxSteps: 1000, MaxInputTokens: 120, MaxOutputTokens: 100_000, NoticeRatio: 0.8},
			input:  40,
			output: 1,
		},
		"output ceiling with input to spare": {
			cfg:    budget.Config{MaxSteps: 1000, MaxInputTokens: 100_000, MaxOutputTokens: 120, NoticeRatio: 0.8},
			input:  1,
			output: 40,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tool := &recordingTool{name: "probe__ping"}
			responses := make([]*gollem.Response, 0, 20)
			for range 20 {
				responses = append(responses, &gollem.Response{
					FunctionCalls: []*gollem.FunctionCall{{ID: "c", Name: "probe__ping", Arguments: map[string]any{}}},
					InputToken:    tc.input,
					OutputToken:   tc.output,
				})
			}
			llm, _ := scriptedLLM(t, responses...)
			rt := newRuntime(t, llm, tc.cfg, tool)

			proc := rt.run(t, react.Input{SystemPrompt: "be helpful", Prompt: "loop forever"})

			gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
			gt.Value(t, proc.Failure).NotNil().Required()
			gt.Value(t, proc.Failure.Code).Equal(agentkit.FailureLimitExceeded)
		})
	}
}

func TestRegisterRequiresALimiter(t *testing.T) {
	_, err := react.Register(agentkit.NewRegistry(), "react-test", 1, nil)
	gt.Error(t, err).Is(agentkit.ErrInvalidAgentDef)
}

// TestInitRejectsEmptyPrompts pins that a host wiring bug fails at Spawn, where
// the caller sees it, rather than as an agent that runs with no instructions.
func TestInitRejectsEmptyPrompts(t *testing.T) {
	llm, _ := scriptedLLM(t)
	rt := newRuntime(t, llm, generousBudget())
	ctx := context.Background()

	testCases := map[string]react.Input{
		"no system prompt": {Prompt: "hello"},
		"no prompt":        {SystemPrompt: "be helpful"},
		"neither":          {},
	}
	for name, in := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := rt.agent.Spawn(ctx, rt.kernel, in)
			gt.Value(t, err).NotNil()
		})
	}
}

func TestDecodeOutput(t *testing.T) {
	t.Run("round trips", func(t *testing.T) {
		out, err := react.DecodeOutput([]byte(`{"texts":["a","b"]}`))
		gt.NoError(t, err).Required()
		gt.Array(t, out.Texts).Equal([]string{"a", "b"})
	})
	t.Run("reports malformed bytes", func(t *testing.T) {
		_, err := react.DecodeOutput([]byte("not json"))
		gt.Value(t, err).NotNil()
	})
}
