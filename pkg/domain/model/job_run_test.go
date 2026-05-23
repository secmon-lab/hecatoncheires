package model_test

import (
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
)

func TestJobRunStatus_IsValid(t *testing.T) {
	gt.Bool(t, model.JobRunStatusSuccess.IsValid()).True()
	gt.Bool(t, model.JobRunStatusFailed.IsValid()).True()
	gt.Bool(t, model.JobRunStatus("RUNNING").IsValid()).False()
	gt.Bool(t, model.JobRunStatus("").IsValid()).False()
}

func TestJobRunKey_Validate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", CaseID: 1, JobID: "job"}
		gt.NoError(t, k.Validate())
	})
	t.Run("empty workspace", func(t *testing.T) {
		k := model.JobRunKey{CaseID: 1, JobID: "job"}
		gt.Error(t, k.Validate())
	})
	t.Run("zero case", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", JobID: "job"}
		gt.Error(t, k.Validate())
	})
	t.Run("empty job", func(t *testing.T) {
		k := model.JobRunKey{WorkspaceID: "ws", CaseID: 1}
		gt.Error(t, k.Validate())
	})
}

func TestJobRun_IsLeased(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	t.Run("active lease", func(t *testing.T) {
		r := &model.JobRun{LeaseUntil: now.Add(5 * time.Minute)}
		gt.Bool(t, r.IsLeased(now)).True()
	})
	t.Run("expired lease", func(t *testing.T) {
		r := &model.JobRun{LeaseUntil: now.Add(-time.Second)}
		gt.Bool(t, r.IsLeased(now)).False()
	})
	t.Run("zero lease (idle)", func(t *testing.T) {
		r := &model.JobRun{}
		gt.Bool(t, r.IsLeased(now)).False()
	})
	t.Run("nil receiver", func(t *testing.T) {
		var r *model.JobRun
		gt.Bool(t, r.IsLeased(now)).False()
	})
}

func TestJobRunStage_IsValid(t *testing.T) {
	gt.Bool(t, model.JobRunStageRunning.IsValid()).True()
	gt.Bool(t, model.JobRunStageSuccess.IsValid()).True()
	gt.Bool(t, model.JobRunStageFailed.IsValid()).True()
	gt.Bool(t, model.JobRunStage("OTHER").IsValid()).False()
	gt.Bool(t, model.JobRunStage("").IsValid()).False()
}

func TestJobRunEventKind_IsValid(t *testing.T) {
	gt.Bool(t, model.JobRunEventKindLLMRequest.IsValid()).True()
	gt.Bool(t, model.JobRunEventKindLLMResponse.IsValid()).True()
	gt.Bool(t, model.JobRunEventKindToolCall.IsValid()).True()
	gt.Bool(t, model.JobRunEventKindRunError.IsValid()).True()
	gt.Bool(t, model.JobRunEventKind("OTHER").IsValid()).False()
	gt.Bool(t, model.JobRunEventKind("").IsValid()).False()
}

func validJobRunLog() *model.JobRunLog {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	return &model.JobRunLog{
		WorkspaceID:    "ws1",
		CaseID:         42,
		JobID:          "job-A",
		RunID:          "run-1",
		TraceID:        "trace-1",
		Stage:          model.JobRunStageRunning,
		StartedAt:      now,
		ExecutorKind:   "single_loop",
		EventType:      "CASE_OPENED",
		EventTriggerAt: now,
		SystemPrompt:   "you are an agent",
	}
}

func TestJobRunLog_Validate(t *testing.T) {
	t.Run("ok running", func(t *testing.T) {
		gt.NoError(t, validJobRunLog().Validate())
	})
	t.Run("ok success", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = model.JobRunStageSuccess
		l.EndedAt = l.StartedAt.Add(time.Second)
		gt.NoError(t, l.Validate())
	})
	t.Run("ok failed", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = model.JobRunStageFailed
		l.EndedAt = l.StartedAt.Add(time.Second)
		l.Error = "boom"
		gt.NoError(t, l.Validate())
	})
	t.Run("nil receiver", func(t *testing.T) {
		var l *model.JobRunLog
		gt.Error(t, l.Validate())
	})
	t.Run("empty workspace id", func(t *testing.T) {
		l := validJobRunLog()
		l.WorkspaceID = ""
		gt.Error(t, l.Validate())
	})
	t.Run("zero case id", func(t *testing.T) {
		l := validJobRunLog()
		l.CaseID = 0
		gt.Error(t, l.Validate())
	})
	t.Run("empty job id", func(t *testing.T) {
		l := validJobRunLog()
		l.JobID = ""
		gt.Error(t, l.Validate())
	})
	t.Run("empty run id", func(t *testing.T) {
		l := validJobRunLog()
		l.RunID = ""
		gt.Error(t, l.Validate())
	})
	t.Run("empty trace id", func(t *testing.T) {
		l := validJobRunLog()
		l.TraceID = ""
		gt.Error(t, l.Validate())
	})
	t.Run("invalid stage", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = "WAT"
		gt.Error(t, l.Validate())
	})
	t.Run("zero started at", func(t *testing.T) {
		l := validJobRunLog()
		l.StartedAt = time.Time{}
		gt.Error(t, l.Validate())
	})
	t.Run("empty executor kind", func(t *testing.T) {
		l := validJobRunLog()
		l.ExecutorKind = ""
		gt.Error(t, l.Validate())
	})
	t.Run("running with ended at", func(t *testing.T) {
		l := validJobRunLog()
		l.EndedAt = l.StartedAt.Add(time.Second)
		gt.Error(t, l.Validate())
	})
	t.Run("success without ended at", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = model.JobRunStageSuccess
		gt.Error(t, l.Validate())
	})
	t.Run("success with error string", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = model.JobRunStageSuccess
		l.EndedAt = l.StartedAt.Add(time.Second)
		l.Error = "should not be set"
		gt.Error(t, l.Validate())
	})
	t.Run("failed without ended at", func(t *testing.T) {
		l := validJobRunLog()
		l.Stage = model.JobRunStageFailed
		gt.Error(t, l.Validate())
	})
	t.Run("system prompt too long", func(t *testing.T) {
		l := validJobRunLog()
		buf := make([]byte, model.MaxInlineBytes+1)
		for i := range buf {
			buf[i] = 'a'
		}
		l.SystemPrompt = string(buf)
		gt.Error(t, l.Validate())
	})
}

func validJobRunEvent(kind model.JobRunEventKind) *model.JobRunEvent {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	ev := &model.JobRunEvent{
		WorkspaceID: "ws1",
		CaseID:      42,
		JobID:       "job-A",
		RunID:       "run-1",
		TraceID:     "trace-1",
		Sequence:    1,
		OccurredAt:  now,
		Kind:        kind,
		Phase:       "execute",
	}
	switch kind {
	case model.JobRunEventKindLLMRequest:
		ev.LLMRequest = &model.LLMRequestPayload{Model: "claude-opus-4-7"}
	case model.JobRunEventKindLLMResponse:
		ev.LLMResponse = &model.LLMResponsePayload{Model: "claude-opus-4-7", Texts: []string{"hi"}}
	case model.JobRunEventKindToolCall:
		ev.Sequence = 3
		ev.ParentSequence = 2
		ev.ToolCall = &model.ToolCallPayload{ToolName: "slack_search", StartedAt: now, EndedAt: now}
	case model.JobRunEventKindRunError:
		ev.RunError = &model.RunErrorPayload{Stage: "execute", Message: "boom"}
	}
	return ev
}

func TestJobRunEvent_Validate(t *testing.T) {
	t.Run("ok llm_request", func(t *testing.T) {
		gt.NoError(t, validJobRunEvent(model.JobRunEventKindLLMRequest).Validate())
	})
	t.Run("ok llm_response", func(t *testing.T) {
		gt.NoError(t, validJobRunEvent(model.JobRunEventKindLLMResponse).Validate())
	})
	t.Run("ok tool_call", func(t *testing.T) {
		gt.NoError(t, validJobRunEvent(model.JobRunEventKindToolCall).Validate())
	})
	t.Run("ok run_error", func(t *testing.T) {
		gt.NoError(t, validJobRunEvent(model.JobRunEventKindRunError).Validate())
	})
	t.Run("nil receiver", func(t *testing.T) {
		var e *model.JobRunEvent
		gt.Error(t, e.Validate())
	})
	t.Run("empty workspace id", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.WorkspaceID = ""
		gt.Error(t, e.Validate())
	})
	t.Run("zero sequence", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.Sequence = 0
		gt.Error(t, e.Validate())
	})
	t.Run("zero occurred at", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.OccurredAt = time.Time{}
		gt.Error(t, e.Validate())
	})
	t.Run("invalid kind", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.Kind = "OTHER"
		gt.Error(t, e.Validate())
	})
	t.Run("kind/payload mismatch", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.LLMRequest = nil
		e.LLMResponse = &model.LLMResponsePayload{}
		gt.Error(t, e.Validate())
	})
	t.Run("no payload populated", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.LLMRequest = nil
		gt.Error(t, e.Validate())
	})
	t.Run("two payloads populated", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		e.LLMResponse = &model.LLMResponsePayload{}
		gt.Error(t, e.Validate())
	})
	t.Run("tool_call without parent", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindToolCall)
		e.ParentSequence = 0
		gt.Error(t, e.Validate())
	})
	t.Run("tool_call parent not earlier", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindToolCall)
		e.ParentSequence = e.Sequence
		gt.Error(t, e.Validate())
	})
	t.Run("phase too long", func(t *testing.T) {
		e := validJobRunEvent(model.JobRunEventKindLLMRequest)
		buf := make([]byte, 65)
		for i := range buf {
			buf[i] = 'a'
		}
		e.Phase = string(buf)
		gt.Error(t, e.Validate())
	})
}
