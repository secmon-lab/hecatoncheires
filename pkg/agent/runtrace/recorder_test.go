package runtrace_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gollem-dev/gollem/trace"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/runtrace"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func openParams(repo *memory.Memory, started time.Time) runtrace.OpenParams {
	return runtrace.OpenParams{
		Repo:         repo,
		WorkspaceID:  "ws-1",
		CaseID:       7,
		JobID:        "job-mention-1",
		RunID:        "run-abc",
		TraceID:      "trace-abc",
		EventType:    model.EventTypeMention,
		ExecutorKind: model.ExecutorKindSingleLoop,
		SystemPrompt: "you are the case agent",
		StartedAt:    started,
		Clock:        func() time.Time { return started },
	}
}

// A successful run must leave a SUCCESS JobRunLog, a JobRun summary doc that
// surfaces via ListByCase (the case agent page read path), and the per-call
// events the handler emitted while running.
func TestRecorder_SuccessLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	started := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	key := model.JobRunKey{WorkspaceID: "ws-1", CaseID: 7, JobID: "job-mention-1"}

	rec, err := runtrace.Open(ctx, openParams(repo, started))
	gt.NoError(t, err).Required()

	// While running, the log exists in RUNNING stage.
	running, err := repo.JobRunLog().Get(ctx, key, "run-abc")
	gt.NoError(t, err).Required()
	gt.Value(t, running.Stage).Equal(model.JobRunStageRunning)
	gt.String(t, running.SystemPrompt).Equal("you are the case agent")
	gt.String(t, running.EventType).Equal(model.EventTypeMention)

	// Drive the handler as a real LLM client would, then close successfully.
	h := rec.Handler()
	llmCtx := h.StartLLMCall(ctx)
	h.EndLLMCall(llmCtx, &trace.LLMCallData{
		Model:    "m",
		Request:  &trace.LLMRequest{},
		Response: &trace.LLMResponse{Texts: []string{"done"}},
	}, nil)
	rec.Finish(ctx, nil)

	// The log is now SUCCESS with an end time and no error.
	done, err := repo.JobRunLog().Get(ctx, key, "run-abc")
	gt.NoError(t, err).Required()
	gt.Value(t, done.Stage).Equal(model.JobRunStageSuccess)
	gt.Bool(t, done.EndedAt.Equal(started)).True()
	gt.String(t, done.Error).Equal("")

	// The JobRun summary doc was materialised so ListByCase (the read path the
	// case agent page uses) surfaces the run.
	runs, err := repo.JobRun().ListByCase(ctx, "ws-1", 7)
	gt.NoError(t, err).Required()
	gt.Array(t, runs).Length(1).Required()
	gt.String(t, runs[0].JobID).Equal("job-mention-1")
	gt.Value(t, runs[0].LastStatus).Equal(model.JobRunStatusSuccess)
	gt.String(t, runs[0].LastRunID).Equal("run-abc")
	gt.String(t, runs[0].LastTraceID).Equal("trace-abc")

	// The per-call events the handler emitted are attributed to this run.
	events, err := repo.JobRunEvent().List(ctx, key, "run-abc")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(2).Required()
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindLLMRequest)
	gt.Value(t, events[1].Kind).Equal(model.JobRunEventKindLLMResponse)
	gt.String(t, events[1].TraceID).Equal("trace-abc")
}

// A failed run must leave a FAILED JobRunLog carrying the error, append a
// RUN_ERROR event, and record FAILED on the JobRun summary.
func TestRecorder_FailureLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	started := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	key := model.JobRunKey{WorkspaceID: "ws-1", CaseID: 7, JobID: "job-mention-1"}

	rec, err := runtrace.Open(ctx, openParams(repo, started))
	gt.NoError(t, err).Required()

	rec.Finish(ctx, errors.New("agent blew up"))

	done, err := repo.JobRunLog().Get(ctx, key, "run-abc")
	gt.NoError(t, err).Required()
	gt.Value(t, done.Stage).Equal(model.JobRunStageFailed)
	gt.String(t, done.Error).Equal("agent blew up")

	runs, err := repo.JobRun().ListByCase(ctx, "ws-1", 7)
	gt.NoError(t, err).Required()
	gt.Array(t, runs).Length(1).Required()
	gt.Value(t, runs[0].LastStatus).Equal(model.JobRunStatusFailed)
	gt.String(t, runs[0].LastError).Equal("agent blew up")

	// The failure is also captured as a RUN_ERROR event in the timeline.
	events, err := repo.JobRunEvent().List(ctx, key, "run-abc")
	gt.NoError(t, err).Required()
	gt.Array(t, events).Length(1).Required()
	gt.Value(t, events[0].Kind).Equal(model.JobRunEventKindRunError)
	gt.String(t, events[0].RunError.Message).Equal("agent blew up")
}

func TestRecorder_OpenValidatesParams(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	started := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	// A missing required identifier fails fast, and nothing is persisted.
	bad := openParams(repo, started)
	bad.RunID = ""
	rec, err := runtrace.Open(ctx, bad)
	gt.Error(t, err)
	gt.Value(t, rec).Nil()

	runs, listErr := repo.JobRun().ListByCase(ctx, "ws-1", 7)
	gt.NoError(t, listErr).Required()
	gt.Array(t, runs).Length(0)
}

// A nil Recorder (Open failed) degrades safely: Handler is nil and Finish is a
// no-op, so a caller can wire it unconditionally.
func TestRecorder_NilSafe(t *testing.T) {
	var rec *runtrace.Recorder
	gt.Value(t, rec.Handler()).Nil()
	rec.Finish(context.Background(), nil) // must not panic
}
