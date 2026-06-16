// Package job is the event-driven Agent Job runtime. It hosts a
// JobExecutor interface and a v1 single-loop implementation (gollem's
// tool-calling agent). Designed so a future plan-and-execute runtime
// can be dropped in behind the same interface.
package job

import (
	"context"
	"strings"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// ExecuteStatus is the terminal outcome of a Job run. The runtime emits
// either SUCCESS or FAILED; a NOOP variant is reserved for future
// idempotent-skip scenarios (e.g. the LLM decided no action was needed).
type ExecuteStatus string

const (
	ExecuteStatusSuccess ExecuteStatus = "SUCCESS"
	ExecuteStatusFailed  ExecuteStatus = "FAILED"
	ExecuteStatusNoOp    ExecuteStatus = "NOOP"
)

// ExecuteRequest is the input to a JobExecutor. The runtime constructs
// the system / user prompts and the resolved tool list outside the
// executor so that future executor implementations (multi-round planner,
// external workflow engine) can swap in without re-doing prompt
// assembly.
type ExecuteRequest struct {
	// JobID identifies the Job being run. Surfaced in trace records and
	// errors.
	JobID string

	// SystemPrompt is the agent's persistent context. Includes role
	// definition, workspace metadata, case snapshot, action list,
	// trigger condition and trigger reason. Held invariant across
	// future multi-round expansions.
	SystemPrompt string

	// Prompt is the initial user message (from the Job's TOML `prompt`
	// after template expansion).
	Prompt string

	// Tools is the resolved tool list (read-only + writer mix).
	Tools []gollem.Tool

	// LLMClient is the underlying LLM client. Wired in so tests can
	// substitute a mock.
	LLMClient gollem.LLMClient

	// HistoryRepository, when non-nil, is used to persist turn history
	// (rare for Jobs but supported for future multi-round runs).
	HistoryRepository gollem.HistoryRepository

	// HistoryKey is the deterministic key into HistoryRepository — typically
	// derived from JobID + CaseID + Event timestamp.
	HistoryKey string

	// ProgressFunc, when non-nil, receives short progress messages emitted
	// by the runtime (tool calls, lifecycle). Used by Handler.Trace.
	ProgressFunc tool.UpdateFunc

	// TraceHandler, when non-nil, is wired into gollem via
	// gollem.WithTrace so the agent loop's LLM/tool boundaries are
	// recorded per-call. JobRunner.Run constructs a jobRunTraceHandler
	// per Run and supplies it here; tests may supply a hand-written
	// trace.Handler or leave this nil for no-op behaviour.
	TraceHandler trace.Handler

	// TraceID is the per-Run identifier the executor surfaces into the
	// underlying runtime's trace.Recorder. Required by the planexec
	// executor; the single-loop executor ignores it (gollem traces are
	// attached via TraceHandler in that path).
	TraceID string

	// Language is the user-facing language label ("Japanese", "English",
	// ...) the executor may inject into prompts. Empty means "no
	// directive" — the planexec planner prompt will skip the language
	// section. Single-loop executor ignores this field.
	Language string
}

// ExecuteResult is the outcome of a Job run.
type ExecuteResult struct {
	Status    ExecuteStatus
	Summary   string       // Final LLM text. May be empty.
	LoopCount int          // Number of internal gollem loops (best-effort).
	Phases    []PhaseTrace // v1 always has exactly one phase ("execute").
}

// PhaseTrace records timing for a single executor phase. v1 produces one
// entry; future multi-round planners can record planner / sub-agent
// rounds without changing the schema.
type PhaseTrace struct {
	Name      string
	StartedAt time.Time
	EndedAt   time.Time
	ToolCalls int
}

// JobExecutor is the abstraction layer for "how a Job actually runs an
// LLM turn". v1 ships SingleLoopJobExecutor; future implementations
// (MultiRoundJobExecutor, etc.) plug in here.
type JobExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error)
}

// SingleLoopJobExecutor runs the gollem agent once with the supplied
// system / user prompt and tools. The LLM itself drives a tool-calling
// loop internally — that's what "single loop" means at the executor
// layer; gollem may issue many round-trips inside.
type SingleLoopJobExecutor struct{}

// NewSingleLoopJobExecutor returns a ready-to-use executor.
func NewSingleLoopJobExecutor() *SingleLoopJobExecutor {
	return &SingleLoopJobExecutor{}
}

// Execute runs the agent. Returns a Result with phase trace and the LLM's
// final text response. On LLM failure the error is wrapped and propagated;
// the caller (JobRunner) is responsible for recording the failure to the
// run repository.
func (e *SingleLoopJobExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResult, error) {
	if req.LLMClient == nil {
		return nil, goerr.New("llm client is required",
			goerr.V("job_id", req.JobID))
	}
	if req.SystemPrompt == "" {
		return nil, goerr.New("system prompt is required",
			goerr.V("job_id", req.JobID))
	}
	if req.Prompt == "" {
		return nil, goerr.New("user prompt is required",
			goerr.V("job_id", req.JobID))
	}

	// Wire progress callback through the tool context so individual tool
	// calls can surface back to the caller.
	if req.ProgressFunc != nil {
		ctx = tool.WithUpdate(ctx, req.ProgressFunc)
	}

	var toolCalls int

	opts := []gollem.Option{
		gollem.WithSystemPrompt(req.SystemPrompt),
		gollem.WithTools(req.Tools...),
		gollem.WithToolMiddleware(
			func(next gollem.ToolHandler) gollem.ToolHandler {
				return func(ctx context.Context, tr *gollem.ToolExecRequest) (*gollem.ToolExecResponse, error) {
					toolCalls++
					return next(ctx, tr)
				}
			},
		),
	}
	if req.HistoryRepository != nil && req.HistoryKey != "" {
		opts = append(opts, gollem.WithHistoryRepository(req.HistoryRepository, req.HistoryKey))
	}
	if req.TraceHandler != nil {
		opts = append(opts, gollem.WithTrace(req.TraceHandler))
	}

	agent := gollem.New(req.LLMClient, opts...)

	startedAt := time.Now().UTC()
	resp, execErr := agent.Execute(ctx, gollem.Text(req.Prompt))
	endedAt := time.Now().UTC()

	phase := PhaseTrace{
		Name:      "execute",
		StartedAt: startedAt,
		EndedAt:   endedAt,
		ToolCalls: toolCalls,
	}

	if execErr != nil {
		return nil, goerr.Wrap(execErr, "execute job agent",
			goerr.V("job_id", req.JobID))
	}

	summary := ""
	if resp != nil {
		summary = strings.Join(resp.Texts, "\n")
	}

	return &ExecuteResult{
		Status:    ExecuteStatusSuccess,
		Summary:   summary,
		LoopCount: 1,
		Phases:    []PhaseTrace{phase},
	}, nil
}
