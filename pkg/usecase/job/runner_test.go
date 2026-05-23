package job_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	jobagent "github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/job"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/job"
)

func newRunner(t *testing.T, wsID string, jobs []*model.Job, exec jobagent.JobExecutor) (*job.JobRunner, *model.WorkspaceRegistry, *model.Case) {
	t.Helper()
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      jobs,
	})

	r := job.NewJobRunner(job.RunnerDeps{
		Repo:      repo,
		Registry:  registry,
		LLMClient: inertLLM(),
		Executor:  exec,
	})
	_ = repo
	return r, registry, c
}

func TestJobRunner_HappyPath(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "summary for {{.Case.Title}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	runner, _, c := newRunner(t, "ws", []*model.Job{j}, exec)

	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.NoError(t, err).Required()
	gt.Number(t, exec.calls.Load()).Equal(int32(1))
}

func TestJobRunner_SkipsWhenLeaseHeld(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	// Pre-acquire the lease so the runner sees it held.
	key := model.JobRunKey{WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID}
	got, err := repo.JobRun().TryAcquireLease(context.Background(), key, time.Now().UTC(), 5*time.Minute)
	gt.NoError(t, err).Required()
	gt.Bool(t, got).True()

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	err = runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.NoError(t, err).Required()
	gt.Number(t, exec.calls.Load()).Equal(int32(0))
}

// failingExecutor lets the runner record a failure path.
type failingExecutor struct {
	calls atomic.Int32
	err   error
}

func (f *failingExecutor) Execute(_ context.Context, _ jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	f.calls.Add(1)
	return nil, f.err
}

func TestJobRunner_FailureIsRecorded(t *testing.T) {
	sentinel := goerr.New("llm down")
	exec := &failingExecutor{err: sentinel}
	j := &model.Job{
		ID:     "fail-job",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	err := runner.Run(context.Background(), j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   "ws",
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err).Is(sentinel)

	run, err := repo.JobRun().Get(context.Background(), model.JobRunKey{
		WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, run.LastError).Contains("llm down")
}

func TestJobRunner_SuccessClearsLease(t *testing.T) {
	exec := &recordingExecutor{}
	j := &model.Job{
		ID:     "summarize",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
	})
	gt.NoError(t, runner.Run(context.Background(), j, job.Event{
		Domain: model.JobEventDomainCase, WorkspaceID: "ws", CaseID: c.ID,
		Timestamp: time.Now().UTC(), CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	run, err := repo.JobRun().Get(context.Background(), model.JobRunKey{
		WorkspaceID: "ws", CaseID: c.ID, JobID: j.ID,
	})
	gt.NoError(t, err).Required()
	gt.Value(t, run.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.Bool(t, run.LeaseUntil.IsZero()).True()
}

func TestJobRunner_InvalidJob(t *testing.T) {
	runner := job.NewJobRunner(job.RunnerDeps{
		Repo:     nil, // unreachable: validation fires first
		Registry: model.NewWorkspaceRegistry(),
		Executor: &recordingExecutor{},
	})
	err := runner.Run(context.Background(), &model.Job{}, job.Event{})
	gt.Error(t, err)
}

// toolCapturingExecutor records the resolved tool list it was handed so
// the test can assert the ToolBuilder ran.
type toolCapturingExecutor struct {
	tools []gollem.Tool
}

func (e *toolCapturingExecutor) Execute(_ context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.tools = req.Tools
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

func TestJobRunner_PassesBuilderTools(t *testing.T) {
	exec := &toolCapturingExecutor{}
	j := &model.Job{
		ID:     "with-tools",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, "ws")
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: "ws"}, Jobs: []*model.Job{j}})

	stubTool := &stubTool{name: "stub__t"}
	builder := job.ToolBuilderFunc(func(_ context.Context, _ *model.Case, _ *model.WorkspaceEntry) []gollem.Tool {
		return []gollem.Tool{stubTool}
	})

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
		ToolBuilder: builder,
	})
	gt.NoError(t, runner.Run(context.Background(), j, job.Event{
		Domain: model.JobEventDomainCase, WorkspaceID: "ws", CaseID: c.ID,
		Timestamp: time.Now().UTC(), CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()
	gt.Array(t, exec.tools).Length(1).Required()
	gt.String(t, exec.tools[0].Spec().Name).Equal("stub__t")
}

type stubTool struct {
	name string
}

func (s *stubTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: s.name, Description: "stub"}
}
func (s *stubTool) Run(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, errors.New("stub not invoked in test")
}

// scriptedRunnerExecutor lets a test seed a list of trace events the
// executor will replay through the handler, simulating what a real
// gollem agent loop would produce. It also forwards an optional
// terminal error from the agent loop.
type scriptedRunnerExecutor struct {
	emit       func(ctx context.Context, h *job.JobRunTraceHandlerForTest)
	terminate  error
	gotRequest *jobagent.ExecuteRequest
}

func (e *scriptedRunnerExecutor) Execute(ctx context.Context, req jobagent.ExecuteRequest) (*jobagent.ExecuteResult, error) {
	e.gotRequest = &req
	if e.emit != nil && req.TraceHandler != nil {
		h, ok := req.TraceHandler.(*job.JobRunTraceHandlerForTest)
		if !ok {
			return nil, errors.New("scriptedRunnerExecutor: TraceHandler is not jobRunTraceHandler")
		}
		e.emit(ctx, h)
	}
	if e.terminate != nil {
		return nil, e.terminate
	}
	return &jobagent.ExecuteResult{Status: jobagent.ExecuteStatusSuccess}, nil
}

// TestJobRunner_GoldenPath drives a complete success Run with a
// scripted agent loop and asserts the *entire* contents of JobRunLog,
// JobRunEvent list, and JobRun lock doc field-by-field. This is the
// canonical Layer 5 test for the trace persistence contract.
func TestJobRunner_GoldenPath(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-golden"
	j := &model.Job{
		ID:     "summarize",
		Prompt: "summary for {{.Case.Title}}",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: wsID, Name: "WS"},
		Jobs:      []*model.Job{j},
	})

	fixedT := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	runID := "run-fixed-id"
	traceID := "trace-fixed-id"
	triggeredAt := fixedT.Add(-time.Second)

	// Scripted agent loop: 1 LLM call + 1 tool call.
	exec := &scriptedRunnerExecutor{
		emit: func(ctx context.Context, h *job.JobRunTraceHandlerForTest) {
			llmCtx := h.StartLLMCall(ctx)
			h.EndLLMCall(llmCtx, &traceLLMCallDataForTest, nil)
			toolCtx := h.StartToolExec(ctx, "slack_search", map[string]any{"q": "foo"})
			h.EndToolExec(toolCtx, map[string]any{"hits": 3}, nil)
		},
	}

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
		NewRunID:   func() string { return runID },
		NewTraceID: func() string { return traceID },
		Clock:      func() time.Time { return fixedT },
	})
	gt.NoError(t, runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     triggeredAt,
		ActorUserID:   "U-CALLER",
		CaseLifecycle: model.CaseLifecycleCreated,
	})).Required()

	// Assert JobRunLog: full field check.
	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}
	log, err := repo.JobRunLog().Get(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.String(t, log.WorkspaceID).Equal(wsID)
	gt.Number(t, log.CaseID).Equal(c.ID)
	gt.String(t, log.JobID).Equal(j.ID)
	gt.String(t, log.RunID).Equal(runID)
	gt.String(t, log.TraceID).Equal(traceID)
	gt.Value(t, log.Stage).Equal(model.JobRunStageSuccess)
	gt.Bool(t, log.StartedAt.Equal(fixedT)).True()
	gt.Bool(t, log.EndedAt.Equal(fixedT)).True()
	gt.String(t, log.Error).Equal("")
	gt.String(t, log.ExecutorKind).Equal("single_loop")
	gt.String(t, log.EventType).Equal(string(model.JobEventDomainCase))
	gt.Bool(t, log.EventTriggerAt.Equal(triggeredAt.UTC())).True()
	gt.String(t, log.SystemPrompt).NotEqual("")

	// Assert event list: LLM_REQUEST -> LLM_RESPONSE -> TOOL_CALL.
	events, err := repo.JobRunEvent().List(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()

	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Number(t, events[0].Sequence).Equal(1)
	gt.String(t, events[0].WorkspaceID).Equal(wsID)
	gt.Number(t, events[0].CaseID).Equal(c.ID)
	gt.String(t, events[0].JobID).Equal(j.ID)
	gt.String(t, events[0].RunID).Equal(runID)
	gt.String(t, events[0].TraceID).Equal(traceID)
	gt.String(t, events[0].Phase).Equal("execute")
	gt.String(t, events[0].LLMRequest.Model).Equal("claude-opus-4-7")
	gt.Array(t, events[0].LLMRequest.Tools).Length(1).Required()
	gt.String(t, events[0].LLMRequest.Tools[0].Name).Equal("slack_search")

	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Number(t, events[1].Sequence).Equal(2)
	gt.Array(t, events[1].LLMResponse.Texts).Length(1).Required()
	gt.String(t, events[1].LLMResponse.Texts[0]).Equal("let me search")
	gt.Array(t, events[1].LLMResponse.FunctionCalls).Length(1).Required()
	gt.String(t, events[1].LLMResponse.FunctionCalls[0].Name).Equal("slack_search")
	gt.Number(t, events[1].LLMResponse.InputTokens).Equal(120)
	gt.Number(t, events[1].LLMResponse.OutputTokens).Equal(60)

	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindToolCall)
	gt.Number(t, events[2].Sequence).Equal(3)
	gt.Number(t, events[2].ParentSequence).Equal(2)
	gt.String(t, events[2].ToolCall.ToolName).Equal("slack_search")
	gt.String(t, events[2].ToolCall.ArgumentsJSON).Equal(`{"q":"foo"}`)
	gt.String(t, events[2].ToolCall.ResultJSON).Equal(`{"hits":3}`)
	gt.Bool(t, events[2].ToolCall.IsError).False()
	gt.String(t, events[2].ToolCall.ErrorMessage).Equal("")
	gt.Bool(t, events[2].ToolCall.StartedAt.Equal(fixedT)).True()
	gt.Bool(t, events[2].ToolCall.EndedAt.Equal(fixedT)).True()

	// Assert JobRun lock doc updates.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusSuccess)
	gt.String(t, jr.LastRunID).Equal(runID)
	gt.String(t, jr.LastTraceID).Equal(traceID)
	gt.String(t, jr.LastError).Equal("")
	gt.Bool(t, jr.LastRunAt.Equal(fixedT)).True()
	gt.Bool(t, jr.LeaseUntil.IsZero()).True()
}

// traceLLMCallDataForTest is reused across runner tests to drive the
// handler's EndLLMCall hook with a known LLMCallData shape.
var traceLLMCallDataForTest = trace.LLMCallData{
	Model:        "claude-opus-4-7",
	InputTokens:  120,
	OutputTokens: 60,
	Request: &trace.LLMRequest{
		Messages: []trace.Message{
			{
				Role: "user",
				Contents: []trace.MessageContent{
					{Type: "text", Text: "investigate case"},
				},
			},
		},
		Tools: []trace.ToolSpec{
			{Name: "slack_search", Description: "search slack"},
		},
	},
	Response: &trace.LLMResponse{
		Texts: []string{"let me search"},
		FunctionCalls: []*trace.FunctionCall{
			{ID: "fc-1", Name: "slack_search", Arguments: map[string]any{"q": "foo"}},
		},
	},
}

// TestJobRunner_LLMFailure_AppendsRunErrorAndFails verifies that when
// the executor returns an error, the runner emits a RUN_ERROR event
// (with Stage="execute") AND transitions the JobRunLog to FAILED with
// the error message preserved.
func TestJobRunner_LLMFailure_AppendsRunError(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-fail"
	j := &model.Job{
		ID:     "fail-job",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{Workspace: model.Workspace{ID: wsID}, Jobs: []*model.Job{j}})

	fixedT := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	runID := "run-fail-id"
	traceID := "trace-fail-id"
	sentinel := errors.New("llm timeout")

	exec := &scriptedRunnerExecutor{
		emit: func(ctx context.Context, h *job.JobRunTraceHandlerForTest) {
			llmCtx := h.StartLLMCall(ctx)
			h.EndLLMCall(llmCtx, &traceLLMCallDataForTest, nil)
		},
		terminate: sentinel,
	}

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: exec,
		NewRunID:   func() string { return runID },
		NewTraceID: func() string { return traceID },
		Clock:      func() time.Time { return fixedT },
	})
	err := runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     fixedT,
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err).Is(sentinel)

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}

	// JobRunLog: FAILED with error captured.
	log, err := repo.JobRunLog().Get(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Value(t, log.Stage).Equal(model.JobRunStageFailed)
	gt.String(t, log.Error).Equal("llm timeout")
	gt.Bool(t, log.EndedAt.Equal(fixedT)).True()

	// Events: LLM_REQUEST + LLM_RESPONSE + RUN_ERROR.
	events, err := repo.JobRunEvent().List(ctx, key, runID)
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(3).Required()
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.Value(t, events[2].Kind).Equal(model.JobRunEventKindRunError)
	gt.Number(t, events[2].Sequence).Equal(3)
	gt.String(t, events[2].RunError.Stage).Equal("execute")
	gt.String(t, events[2].RunError.Message).Equal("llm timeout")

	// JobRun lock doc: FAILED with LastRunID/LastTraceID populated.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, jr.LastRunID).Equal(runID)
	gt.String(t, jr.LastTraceID).Equal(traceID)
	gt.String(t, jr.LastError).Equal("llm timeout")
}

// TestJobRunner_WorkspaceLoadFailure_NoLog asserts that prepare-stage
// failures (here: missing workspace) do NOT leave a JobRunLog behind.
// The JobRun lock doc still records FAILED for the lifecycle, but no
// RunID was ever allocated so events are not attributable.
func TestJobRunner_WorkspaceLoadFailure_NoLog(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-missing"
	j := &model.Job{
		ID:     "j",
		Prompt: "x",
		Events: model.JobEvents{
			Case: &model.CaseEventConfig{On: []model.CaseLifecycle{model.CaseLifecycleCreated}},
		},
	}
	repo, c := setupCase(t, wsID)
	// Note: NewWorkspaceRegistry is empty — Registry.Get returns an error.
	registry := model.NewWorkspaceRegistry()

	runner := job.NewJobRunner(job.RunnerDeps{
		Repo: repo, Registry: registry, LLMClient: inertLLM(), Executor: &recordingExecutor{},
	})
	err := runner.Run(ctx, j, job.Event{
		Domain:        model.JobEventDomainCase,
		WorkspaceID:   wsID,
		CaseID:        c.ID,
		Timestamp:     time.Now().UTC(),
		CaseLifecycle: model.CaseLifecycleCreated,
	})
	gt.Error(t, err)

	key := model.JobRunKey{WorkspaceID: wsID, CaseID: c.ID, JobID: j.ID}

	// No JobRunLog written.
	logs, err := repo.JobRunLog().List(ctx, key, 0)
	gt.NoError(t, err).Required()
	gt.Array(t, logs).Length(0)

	// JobRun lock doc transitioned to FAILED.
	jr, err := repo.JobRun().Get(ctx, key)
	gt.NoError(t, err).Required()
	gt.Value(t, jr.LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, jr.LastRunID).Equal("")
	gt.String(t, jr.LastTraceID).Equal("")
}
