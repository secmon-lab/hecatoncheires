package job_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/llm/claude"
	"github.com/m-mizutani/gollem/llm/gemini"
	"github.com/m-mizutani/gollem/llm/openai"
	"github.com/m-mizutani/gollem/mock"
	"github.com/m-mizutani/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
)

// scriptedLLM returns the supplied response texts one-by-one across
// successive Generate calls. Used to drive the executor without a real
// LLM.
func scriptedLLM(scripts []string) *mock.LLMClientMock {
	idx := atomic.Int32{}
	return &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					n := idx.Add(1) - 1
					if int(n) >= len(scripts) {
						return nil, goerr.New("no more scripted responses")
					}
					return &gollem.Response{Texts: []string{scripts[int(n)]}}, nil
				},
			}, nil
		},
	}
}

func TestSingleLoopJobExecutor_HappyPath(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	llm := scriptedLLM([]string{"all done"})

	out, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "you are an agent",
		Prompt:       "do the thing",
		LLMClient:    llm,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, out.Status).Equal(job.ExecuteStatusSuccess)
	gt.String(t, out.Summary).Equal("all done")
	gt.Array(t, out.Phases).Length(1).Required()
	gt.String(t, out.Phases[0].Name).Equal("execute")
	gt.Number(t, out.Phases[0].ToolCalls).Equal(0)
	gt.Bool(t, out.Phases[0].StartedAt.IsZero()).False()
	gt.Bool(t, out.Phases[0].EndedAt.IsZero()).False()
}

func TestSingleLoopJobExecutor_RequiresLLM(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	_, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "x",
		Prompt:       "y",
	})
	gt.Error(t, err)
}

func TestSingleLoopJobExecutor_RequiresSystemPrompt(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	_, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:     "test-job",
		Prompt:    "y",
		LLMClient: scriptedLLM([]string{"x"}),
	})
	gt.Error(t, err)
}

func TestSingleLoopJobExecutor_RequiresUserPrompt(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	_, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "x",
		LLMClient:    scriptedLLM([]string{"x"}),
	})
	gt.Error(t, err)
}

// recordingTraceHandler is a trace.Handler that just counts which hooks
// were called. Used to verify Executor wires req.TraceHandler into gollem.
type recordingTraceHandler struct {
	mu              sync.Mutex
	startLLMCalls   int
	endLLMCalls     int
	startToolCalls  int
	endToolCalls    int
	finishCalls     int
	startAgentCalls int
	endAgentCalls   int
	addEventCalls   int
	startSubCalls   int
	endSubCalls     int
	startChildCalls int
	endChildCalls   int
}

func (h *recordingTraceHandler) inc(field *int) {
	h.mu.Lock()
	*field++
	h.mu.Unlock()
}

func (h *recordingTraceHandler) StartAgentExecute(ctx context.Context) context.Context {
	h.inc(&h.startAgentCalls)
	return ctx
}
func (h *recordingTraceHandler) EndAgentExecute(ctx context.Context, err error) {
	h.inc(&h.endAgentCalls)
}
func (h *recordingTraceHandler) StartLLMCall(ctx context.Context) context.Context {
	h.inc(&h.startLLMCalls)
	return ctx
}
func (h *recordingTraceHandler) EndLLMCall(ctx context.Context, data *trace.LLMCallData, err error) {
	h.inc(&h.endLLMCalls)
}
func (h *recordingTraceHandler) StartToolExec(ctx context.Context, toolName string, args map[string]any) context.Context {
	h.inc(&h.startToolCalls)
	return ctx
}
func (h *recordingTraceHandler) EndToolExec(ctx context.Context, result map[string]any, err error) {
	h.inc(&h.endToolCalls)
}
func (h *recordingTraceHandler) StartSubAgent(ctx context.Context, name string) context.Context {
	h.inc(&h.startSubCalls)
	return ctx
}
func (h *recordingTraceHandler) EndSubAgent(ctx context.Context, err error) {
	h.inc(&h.endSubCalls)
}
func (h *recordingTraceHandler) StartChildAgent(ctx context.Context, name string) context.Context {
	h.inc(&h.startChildCalls)
	return ctx
}
func (h *recordingTraceHandler) EndChildAgent(ctx context.Context, err error) {
	h.inc(&h.endChildCalls)
}
func (h *recordingTraceHandler) AddEvent(ctx context.Context, kind string, data any) {
	h.inc(&h.addEventCalls)
}
func (h *recordingTraceHandler) Finish(ctx context.Context) error {
	h.inc(&h.finishCalls)
	return nil
}

// TestSingleLoopJobExecutor_TraceHandlerIsWired pins the contract that
// non-nil ExecuteRequest.TraceHandler reaches gollem.WithTrace. The
// proxy signal is that StartLLMCall + EndLLMCall both fire during the
// agent loop.
func TestSingleLoopJobExecutor_TraceHandlerIsWired(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	llm := scriptedLLM([]string{"done"})
	handler := &recordingTraceHandler{}

	_, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "x",
		Prompt:       "y",
		LLMClient:    llm,
		TraceHandler: handler,
	})
	gt.NoError(t, err).Required()
	// gollem.go invokes StartAgentExecute and EndAgentExecute on the
	// configured trace handler from inside Execute() regardless of which
	// LLM client is in use. (StartLLMCall / EndLLMCall are emitted by
	// the provider-specific client packages, NOT by mock.LLMClientMock,
	// so they can't be used as a wire-signal in unit tests.) If our
	// handler did not reach gollem.WithTrace, these counters would stay 0.
	gt.Number(t, handler.startAgentCalls).Equal(1)
	gt.Number(t, handler.endAgentCalls).Equal(1)
	gt.Number(t, handler.finishCalls).Equal(1)
}

// TestSingleLoopJobExecutor_TraceHandlerNilIsBackwardCompat ensures the
// executor still runs when no handler is supplied (existing callers).
func TestSingleLoopJobExecutor_TraceHandlerNilIsBackwardCompat(t *testing.T) {
	exec := job.NewSingleLoopJobExecutor()
	llm := scriptedLLM([]string{"done"})

	out, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "x",
		Prompt:       "y",
		LLMClient:    llm,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, out.Status).Equal(job.ExecuteStatusSuccess)
}

func TestSingleLoopJobExecutor_LLMErrorPropagates(t *testing.T) {
	sentinel := goerr.New("llm crashed")
	llm := &mock.LLMClientMock{
		NewSessionFunc: func(_ context.Context, _ ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(_ context.Context, _ []gollem.Input, _ ...gollem.GenerateOption) (*gollem.Response, error) {
					return nil, sentinel
				},
			}, nil
		},
	}
	exec := job.NewSingleLoopJobExecutor()
	_, err := exec.Execute(context.Background(), job.ExecuteRequest{
		JobID:        "test-job",
		SystemPrompt: "x",
		Prompt:       "y",
		LLMClient:    llm,
	})
	gt.Error(t, err).Is(sentinel)
}

// TestSingleLoopJobExecutor_LiveLLMSmoke verifies that the executor can
// drive a real LLM through a single tool call. Gated by
// HECATONCHEIRES_LIVE_LLM_PROVIDER and the provider's credential env
// vars; skips when unset (the only acceptable t.Skip pattern).
//
// The assertion is intentionally minimal: a real LLM is non-deterministic
// and this test exists purely to catch "prompt assembly broke" /
// "tool schema doesn't reach the model" regressions, not to grade the
// model's reasoning quality.
func TestSingleLoopJobExecutor_LiveLLMSmoke(t *testing.T) {
	provider := os.Getenv("HECATONCHEIRES_LIVE_LLM_PROVIDER")
	if provider == "" {
		t.Skip("HECATONCHEIRES_LIVE_LLM_PROVIDER not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	llmClient, skipMsg, err := newLiveLLMClient(ctx, provider)
	if skipMsg != "" {
		t.Skip(skipMsg)
	}
	gt.NoError(t, err).Required()

	called := false
	echoTool := &liveEchoTool{onRun: func() { called = true }}

	exec := job.NewSingleLoopJobExecutor()
	_, err = exec.Execute(ctx, job.ExecuteRequest{
		JobID:        "live-smoke",
		SystemPrompt: "You are a tool-calling agent. Always call the exact tool the user asks for. After the tool returns, respond briefly.",
		Prompt:       `Call the tool named echo with argument message="hello".`,
		Tools:        []gollem.Tool{echoTool},
		LLMClient:    llmClient,
	})
	gt.NoError(t, err).Required()
	gt.Bool(t, called).True()
}

func newLiveLLMClient(ctx context.Context, provider string) (gollem.LLMClient, string, error) {
	switch provider {
	case "openai":
		key := os.Getenv("HECATONCHEIRES_LIVE_LLM_OPENAI_API_KEY")
		if key == "" {
			return nil, "HECATONCHEIRES_LIVE_LLM_OPENAI_API_KEY not set", nil
		}
		c, err := openai.New(ctx, key)
		return c, "", err
	case "claude":
		key := os.Getenv("HECATONCHEIRES_LIVE_LLM_CLAUDE_API_KEY")
		if key == "" {
			return nil, "HECATONCHEIRES_LIVE_LLM_CLAUDE_API_KEY not set", nil
		}
		c, err := claude.New(ctx, key)
		return c, "", err
	case "gemini":
		project := os.Getenv("HECATONCHEIRES_LIVE_LLM_GEMINI_PROJECT_ID")
		location := os.Getenv("HECATONCHEIRES_LIVE_LLM_GEMINI_LOCATION")
		if project == "" || location == "" {
			return nil, "HECATONCHEIRES_LIVE_LLM_GEMINI_PROJECT_ID / _LOCATION not set", nil
		}
		c, err := gemini.New(ctx, project, location)
		return c, "", err
	default:
		return nil, "unknown live provider: " + provider, nil
	}
}

type liveEchoTool struct {
	onRun func()
}

func (t *liveEchoTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "echo",
		Description: "Echo back the provided message verbatim.",
		Parameters: map[string]*gollem.Parameter{
			"message": {
				Type:        gollem.TypeString,
				Description: "The message to echo.",
				Required:    true,
			},
		},
	}
}

func (t *liveEchoTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	if t.onRun != nil {
		t.onRun()
	}
	msg, _ := args["message"].(string)
	return map[string]any{"message": msg}, nil
}
