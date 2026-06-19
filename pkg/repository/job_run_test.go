package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runJobRunRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Get returns ErrJobRunNotFound for missing record", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "missing",
		}
		_, err := repo.JobRun().Get(ctx, key)
		gt.Error(t, err).Is(interfaces.ErrJobRunNotFound)
	})

	t.Run("RecordRun round-trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "job-1",
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		err := repo.JobRun().RecordRun(ctx, key, model.JobRunStatusSuccess, now, "run-abc", "trace-abc", "")
		gt.NoError(t, err).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.String(t, got.WorkspaceID).Equal(key.WorkspaceID)
		gt.Number(t, got.CaseID).Equal(key.CaseID)
		gt.String(t, got.JobID).Equal(key.JobID)
		gt.Bool(t, got.LastRunAt.Equal(now)).True()
		gt.Value(t, got.LastStatus).Equal(model.JobRunStatusSuccess)
		gt.String(t, got.LastError).Equal("")
		gt.String(t, got.LastRunID).Equal("run-abc")
		gt.String(t, got.LastTraceID).Equal("trace-abc")
		gt.Bool(t, got.LeaseUntil.IsZero()).True()
	})

	t.Run("RecordRun FAILED stores error message", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "job-fail",
		}
		now := time.Now().UTC().Truncate(time.Millisecond)
		err := repo.JobRun().RecordRun(ctx, key, model.JobRunStatusFailed, now, "run-x", "trace-x", "llm timeout")
		gt.NoError(t, err).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Value(t, got.LastStatus).Equal(model.JobRunStatusFailed)
		gt.String(t, got.LastError).Equal("llm timeout")
	})

	t.Run("TryAcquireLease succeeds when idle", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-1",
		}
		now := time.Now().UTC()
		acquired, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, acquired).True()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Bool(t, got.LeaseUntil.After(now)).True()
	})

	t.Run("TryAcquireLease blocks while lease is active", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-block",
		}
		now := time.Now().UTC()
		first, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, first).True()

		second, err := repo.JobRun().TryAcquireLease(ctx, key, now.Add(time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, second).False()
	})

	t.Run("TryAcquireLease reclaims after lease expiry", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-reclaim",
		}
		t0 := time.Now().UTC()
		first, err := repo.JobRun().TryAcquireLease(ctx, key, t0, time.Second)
		gt.NoError(t, err).Required()
		gt.Bool(t, first).True()

		// Lease elapsed.
		second, err := repo.JobRun().TryAcquireLease(ctx, key, t0.Add(2*time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, second).True()
	})

	t.Run("ReleaseLease lets the next acquirer in", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "lease-release",
		}
		now := time.Now().UTC()
		_, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.NoError(t, repo.JobRun().ReleaseLease(ctx, key)).Required()

		again, err := repo.JobRun().TryAcquireLease(ctx, key, now.Add(time.Second), 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.Bool(t, again).True()
	})

	t.Run("ReleaseLease is idempotent without prior acquire", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "no-prior",
		}
		gt.NoError(t, repo.JobRun().ReleaseLease(ctx, key))
	})

	t.Run("RecordRun also clears lease", func(t *testing.T) {
		repo := newRepo(t)
		key := model.JobRunKey{
			WorkspaceID: fmt.Sprintf("ws-%d", time.Now().UnixNano()),
			CaseID:      time.Now().UnixNano(),
			JobID:       "rec-clear",
		}
		now := time.Now().UTC()
		_, err := repo.JobRun().TryAcquireLease(ctx, key, now, 5*time.Minute)
		gt.NoError(t, err).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx, key, model.JobRunStatusSuccess, now.Add(time.Second), "run-lease-clear", "tr", "")).Required()

		got, err := repo.JobRun().Get(ctx, key)
		gt.NoError(t, err).Required()
		gt.Bool(t, got.LeaseUntil.IsZero()).True()
	})

	t.Run("ListByCase returns runs scoped to the (workspace, case) pair", func(t *testing.T) {
		repo := newRepo(t)
		ws := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseA := time.Now().UnixNano()
		caseB := time.Now().UnixNano() + 1
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseA, JobID: "a1"},
			model.JobRunStatusSuccess, now, "r1", "t1", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseA, JobID: "a2"},
			model.JobRunStatusSuccess, now, "r2", "t2", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws, CaseID: caseB, JobID: "b1"},
			model.JobRunStatusSuccess, now, "r3", "t3", "")).Required()

		runsA, err := repo.JobRun().ListByCase(ctx, ws, caseA)
		gt.NoError(t, err).Required()
		gt.Array(t, runsA).Length(2).Required()
		for _, r := range runsA {
			gt.Number(t, r.CaseID).Equal(caseA)
			gt.String(t, r.WorkspaceID).Equal(ws)
		}

		runsB, err := repo.JobRun().ListByCase(ctx, ws, caseB)
		gt.NoError(t, err).Required()
		gt.Array(t, runsB).Length(1).Required()
		gt.String(t, runsB[0].JobID).Equal("b1")
	})

	t.Run("ListByCase returns empty for a case with no runs", func(t *testing.T) {
		repo := newRepo(t)
		ws := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		runs, err := repo.JobRun().ListByCase(ctx, ws, time.Now().UnixNano())
		gt.NoError(t, err).Required()
		gt.Array(t, runs).Length(0)
	})

	t.Run("ListByCase scopes by workspace (same case id in different workspaces)", func(t *testing.T) {
		repo := newRepo(t)
		ws1 := fmt.Sprintf("ws1-%d", time.Now().UnixNano())
		ws2 := fmt.Sprintf("ws2-%d", time.Now().UnixNano())
		caseShared := time.Now().UnixNano()
		now := time.Now().UTC().Truncate(time.Millisecond)

		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws1, CaseID: caseShared, JobID: "j"},
			model.JobRunStatusSuccess, now, "r1", "t1", "")).Required()
		gt.NoError(t, repo.JobRun().RecordRun(ctx,
			model.JobRunKey{WorkspaceID: ws2, CaseID: caseShared, JobID: "j"},
			model.JobRunStatusSuccess, now, "r2", "t2", "")).Required()

		runs1, err := repo.JobRun().ListByCase(ctx, ws1, caseShared)
		gt.NoError(t, err).Required()
		gt.Array(t, runs1).Length(1).Required()
		gt.Value(t, runs1[0].LastTraceID).Equal("t1")
	})

	t.Run("invalid key surfaces error", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.JobRun().Get(ctx, model.JobRunKey{})
		gt.Error(t, err)
		gt.Bool(t, errors.Is(err, interfaces.ErrJobRunNotFound)).False()
	})
}

func TestJobRunRepository_Memory(t *testing.T) {
	t.Parallel()
	runJobRunRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestJobRunRepository_Firestore(t *testing.T) {
	t.Parallel()
	runJobRunRepositoryTest(t, newFirestoreRepository)
}

func newJobRunKey(prefix string) model.JobRunKey {
	return model.JobRunKey{
		WorkspaceID: fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()),
		CaseID:      time.Now().UnixNano(),
		JobID:       "job-A",
	}
}

func runJobRunLogRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Get returns ErrJobRunLogNotFound for missing record", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.JobRunLog().Get(ctx, newJobRunKey("ws"), "missing-run")
		gt.Error(t, err).Is(interfaces.ErrJobRunLogNotFound)
	})

	t.Run("Create then Get round-trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		started := time.Now().UTC().Truncate(time.Millisecond)
		triggered := started.Add(-time.Second)
		log := &model.JobRunLog{
			WorkspaceID:     key.WorkspaceID,
			CaseID:          key.CaseID,
			JobID:           key.JobID,
			RunID:           "run-1",
			TraceID:         "trace-1",
			Stage:           model.JobRunStageRunning,
			StartedAt:       started,
			ExecutorKind:    "single_loop",
			ExecutorVersion: "v1",
			EventType:       "CASE_OPENED",
			EventTriggerAt:  triggered,
			SystemPrompt:    "you are a careful agent",
		}
		gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()

		got, err := repo.JobRunLog().Get(ctx, key, "run-1")
		gt.NoError(t, err).Required()
		gt.String(t, got.WorkspaceID).Equal(key.WorkspaceID)
		gt.Number(t, got.CaseID).Equal(key.CaseID)
		gt.String(t, got.JobID).Equal(key.JobID)
		gt.String(t, got.RunID).Equal("run-1")
		gt.String(t, got.TraceID).Equal("trace-1")
		gt.Value(t, got.Stage).Equal(model.JobRunStageRunning)
		gt.Bool(t, got.StartedAt.Equal(started)).True()
		gt.Bool(t, got.EndedAt.IsZero()).True()
		gt.String(t, got.Error).Equal("")
		gt.String(t, got.ExecutorKind).Equal("single_loop")
		gt.String(t, got.ExecutorVersion).Equal("v1")
		gt.String(t, got.EventType).Equal("CASE_OPENED")
		gt.Bool(t, got.EventTriggerAt.Equal(triggered)).True()
		gt.String(t, got.SystemPrompt).Equal("you are a careful agent")
	})

	t.Run("Create rejects duplicate (key, runID)", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		now := time.Now().UTC().Truncate(time.Millisecond)
		log := &model.JobRunLog{
			WorkspaceID:  key.WorkspaceID,
			CaseID:       key.CaseID,
			JobID:        key.JobID,
			RunID:        "run-dup",
			TraceID:      "trace-dup",
			Stage:        model.JobRunStageRunning,
			StartedAt:    now,
			ExecutorKind: "single_loop",
		}
		gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()
		err := repo.JobRunLog().Create(ctx, log)
		gt.Error(t, err).Is(interfaces.ErrJobRunLogExists)
	})

	t.Run("Finish transitions to SUCCESS with EndedAt", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		started := time.Now().UTC().Truncate(time.Millisecond)
		log := &model.JobRunLog{
			WorkspaceID:  key.WorkspaceID,
			CaseID:       key.CaseID,
			JobID:        key.JobID,
			RunID:        "run-finish",
			TraceID:      "trace-finish",
			Stage:        model.JobRunStageRunning,
			StartedAt:    started,
			ExecutorKind: "single_loop",
		}
		gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()

		log.Stage = model.JobRunStageSuccess
		log.EndedAt = started.Add(time.Second)
		gt.NoError(t, repo.JobRunLog().Finish(ctx, log)).Required()

		got, err := repo.JobRunLog().Get(ctx, key, "run-finish")
		gt.NoError(t, err).Required()
		gt.Value(t, got.Stage).Equal(model.JobRunStageSuccess)
		gt.Bool(t, got.EndedAt.Equal(log.EndedAt)).True()
		gt.String(t, got.Error).Equal("")
	})

	t.Run("Finish rejects RUNNING terminal stage", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		started := time.Now().UTC().Truncate(time.Millisecond)
		log := &model.JobRunLog{
			WorkspaceID:  key.WorkspaceID,
			CaseID:       key.CaseID,
			JobID:        key.JobID,
			RunID:        "run-noterm",
			TraceID:      "trace-noterm",
			Stage:        model.JobRunStageRunning,
			StartedAt:    started,
			ExecutorKind: "single_loop",
		}
		gt.NoError(t, repo.JobRunLog().Create(ctx, log)).Required()
		gt.Error(t, repo.JobRunLog().Finish(ctx, log))
	})

	t.Run("List returns descending by StartedAt with limit", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		base := time.Now().UTC().Truncate(time.Millisecond)
		for i := range 3 {
			gt.NoError(t, repo.JobRunLog().Create(ctx, &model.JobRunLog{
				WorkspaceID:  key.WorkspaceID,
				CaseID:       key.CaseID,
				JobID:        key.JobID,
				RunID:        fmt.Sprintf("run-%d", i),
				TraceID:      fmt.Sprintf("trace-%d", i),
				Stage:        model.JobRunStageRunning,
				StartedAt:    base.Add(time.Duration(i) * time.Second),
				ExecutorKind: "single_loop",
			})).Required()
		}

		all, err := repo.JobRunLog().List(ctx, key, 0)
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(3).Required()
		gt.String(t, all[0].RunID).Equal("run-2")
		gt.String(t, all[1].RunID).Equal("run-1")
		gt.String(t, all[2].RunID).Equal("run-0")

		limited, err := repo.JobRunLog().List(ctx, key, 2)
		gt.NoError(t, err).Required()
		gt.Array(t, limited).Length(2).Required()
		gt.String(t, limited[0].RunID).Equal("run-2")
		gt.String(t, limited[1].RunID).Equal("run-1")
	})

	t.Run("List returns empty for absent key", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("absent")
		got, err := repo.JobRunLog().List(ctx, key, 0)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
	})
}

func runJobRunEventRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Append + List round-trips all fields and orders by Sequence", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		runID := "run-events"
		now := time.Now().UTC().Truncate(time.Millisecond)

		ev1 := &model.JobRunEvent{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     "trace-1",
			EventID:     "ev-1",
			Sequence:    1,
			OccurredAt:  now,
			Kind:        model.JobRunEventKindLLMRequest,
			Phase:       "execute",
			LLMRequest: &model.LLMRequestPayload{
				Model: "claude-opus-4-7",
				Messages: []model.LLMMessage{
					{
						Role: "user",
						Contents: []model.LLMContentBlock{
							{Type: "text", Text: "investigate case"},
						},
					},
				},
				Tools: []model.LLMToolSpec{
					{Name: "slack_search", Description: "search slack"},
				},
			},
		}
		ev2 := &model.JobRunEvent{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     "trace-1",
			EventID:     "ev-2",
			Sequence:    2,
			OccurredAt:  now.Add(time.Second),
			Kind:        model.JobRunEventKindLLMResponse,
			Phase:       "execute",
			LLMResponse: &model.LLMResponsePayload{
				Model: "claude-opus-4-7",
				Texts: []string{"let me search"},
				FunctionCalls: []model.LLMFunctionCall{
					{ID: "abc", Name: "slack_search", ArgumentsJSON: `{"q":"foo"}`},
				},
				InputTokens:  100,
				OutputTokens: 50,
				DurationMs:   1234,
			},
		}
		ev3 := &model.JobRunEvent{
			WorkspaceID:    key.WorkspaceID,
			CaseID:         key.CaseID,
			JobID:          key.JobID,
			RunID:          runID,
			TraceID:        "trace-1",
			EventID:        "ev-3",
			Sequence:       3,
			OccurredAt:     now.Add(2 * time.Second),
			Kind:           model.JobRunEventKindToolCall,
			Phase:          "execute",
			ParentSequence: 2,
			ToolCall: &model.ToolCallPayload{
				ToolName:      "slack_search",
				ArgumentsJSON: `{"q":"foo"}`,
				ResultJSON:    `{"hits":3}`,
				IsError:       false,
				StartedAt:     now.Add(2 * time.Second),
				EndedAt:       now.Add(3 * time.Second),
			},
		}
		ev4 := &model.JobRunEvent{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       runID,
			TraceID:     "trace-1",
			EventID:     "ev-4",
			Sequence:    4,
			OccurredAt:  now.Add(4 * time.Second),
			Kind:        model.JobRunEventKindRunError,
			Phase:       "execute",
			RunError:    &model.RunErrorPayload{Stage: "execute", Message: "boom"},
		}

		// Append out of order to verify List sorts by Sequence asc.
		gt.NoError(t, repo.JobRunEvent().Append(ctx, ev3)).Required()
		gt.NoError(t, repo.JobRunEvent().Append(ctx, ev1)).Required()
		gt.NoError(t, repo.JobRunEvent().Append(ctx, ev4)).Required()
		gt.NoError(t, repo.JobRunEvent().Append(ctx, ev2)).Required()

		got, err := repo.JobRunEvent().List(ctx, key, runID)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(4).Required()

		// LLM_REQUEST
		gt.Number(t, got[0].Sequence).Equal(1)
		gt.Value(t, got[0].Kind).Equal(model.JobRunEventKindLLMRequest)
		gt.Value(t, got[0].LLMRequest).NotNil()
		gt.String(t, got[0].LLMRequest.Model).Equal("claude-opus-4-7")
		gt.Array(t, got[0].LLMRequest.Messages).Length(1).Required()
		gt.String(t, got[0].LLMRequest.Messages[0].Role).Equal("user")
		gt.Array(t, got[0].LLMRequest.Messages[0].Contents).Length(1).Required()
		gt.String(t, got[0].LLMRequest.Messages[0].Contents[0].Type).Equal("text")
		gt.String(t, got[0].LLMRequest.Messages[0].Contents[0].Text).Equal("investigate case")
		gt.Array(t, got[0].LLMRequest.Tools).Length(1).Required()
		gt.String(t, got[0].LLMRequest.Tools[0].Name).Equal("slack_search")
		gt.String(t, got[0].LLMRequest.Tools[0].Description).Equal("search slack")

		// LLM_RESPONSE
		gt.Number(t, got[1].Sequence).Equal(2)
		gt.Value(t, got[1].Kind).Equal(model.JobRunEventKindLLMResponse)
		gt.Array(t, got[1].LLMResponse.Texts).Length(1).Required()
		gt.String(t, got[1].LLMResponse.Texts[0]).Equal("let me search")
		gt.Array(t, got[1].LLMResponse.FunctionCalls).Length(1).Required()
		gt.String(t, got[1].LLMResponse.FunctionCalls[0].ID).Equal("abc")
		gt.String(t, got[1].LLMResponse.FunctionCalls[0].Name).Equal("slack_search")
		gt.String(t, got[1].LLMResponse.FunctionCalls[0].ArgumentsJSON).Equal(`{"q":"foo"}`)
		gt.Number(t, got[1].LLMResponse.InputTokens).Equal(100)
		gt.Number(t, got[1].LLMResponse.OutputTokens).Equal(50)
		gt.Number(t, got[1].LLMResponse.DurationMs).Equal(1234)

		// TOOL_CALL
		gt.Number(t, got[2].Sequence).Equal(3)
		gt.Value(t, got[2].Kind).Equal(model.JobRunEventKindToolCall)
		gt.Number(t, got[2].ParentSequence).Equal(2)
		gt.String(t, got[2].ToolCall.ToolName).Equal("slack_search")
		gt.String(t, got[2].ToolCall.ArgumentsJSON).Equal(`{"q":"foo"}`)
		gt.String(t, got[2].ToolCall.ResultJSON).Equal(`{"hits":3}`)
		gt.Bool(t, got[2].ToolCall.IsError).False()
		gt.Bool(t, got[2].ToolCall.StartedAt.Equal(ev3.ToolCall.StartedAt)).True()
		gt.Bool(t, got[2].ToolCall.EndedAt.Equal(ev3.ToolCall.EndedAt)).True()

		// RUN_ERROR
		gt.Number(t, got[3].Sequence).Equal(4)
		gt.Value(t, got[3].Kind).Equal(model.JobRunEventKindRunError)
		gt.String(t, got[3].RunError.Stage).Equal("execute")
		gt.String(t, got[3].RunError.Message).Equal("boom")
	})

	t.Run("Append rejects duplicate Sequence", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("ws")
		now := time.Now().UTC().Truncate(time.Millisecond)
		ev := &model.JobRunEvent{
			WorkspaceID: key.WorkspaceID,
			CaseID:      key.CaseID,
			JobID:       key.JobID,
			RunID:       "run-dup",
			TraceID:     "trace-dup",
			EventID:     "ev-dup",
			Sequence:    1,
			OccurredAt:  now,
			Kind:        model.JobRunEventKindLLMRequest,
			Phase:       "execute",
			LLMRequest:  &model.LLMRequestPayload{Model: "m"},
		}
		gt.NoError(t, repo.JobRunEvent().Append(ctx, ev)).Required()
		err := repo.JobRunEvent().Append(ctx, ev)
		gt.Error(t, err).Is(interfaces.ErrJobRunEventExists)
	})

	t.Run("List returns empty for absent run", func(t *testing.T) {
		repo := newRepo(t)
		key := newJobRunKey("absent")
		got, err := repo.JobRunEvent().List(ctx, key, "missing-run")
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
	})
}

func TestJobRunLogRepository_Memory(t *testing.T) {
	t.Parallel()
	runJobRunLogRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestJobRunLogRepository_Firestore(t *testing.T) {
	t.Parallel()
	runJobRunLogRepositoryTest(t, newFirestoreRepository)
}

func TestJobRunEventRepository_Memory(t *testing.T) {
	t.Parallel()
	runJobRunEventRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestJobRunEventRepository_Firestore(t *testing.T) {
	t.Parallel()
	runJobRunEventRepositoryTest(t, newFirestoreRepository)
}
