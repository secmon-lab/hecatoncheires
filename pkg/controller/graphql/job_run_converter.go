package graphql

import (
	"encoding/json"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// toGraphQLJobRunLog maps the domain JobRunLog to its GraphQL form,
// pre-resolving the human-readable Job name (TOML lookup happens in
// the caller). Nil EndedAt / DurationMs are surfaced when the run is
// still in flight, matching the schema's nullable fields.
func toGraphQLJobRunLog(log *model.JobRunLog, jobName string) *graphql1.JobRunLog {
	if log == nil {
		return nil
	}
	gql := &graphql1.JobRunLog{
		WorkspaceID:    log.WorkspaceID,
		CaseID:         int(log.CaseID),
		JobID:          log.JobID,
		JobName:        jobName,
		RunID:          log.RunID,
		TraceID:        log.TraceID,
		Stage:          jobRunStageToGraphQL(log.Stage),
		StartedAt:      log.StartedAt,
		ErrorMessage:   log.Error,
		SystemPrompt:   log.SystemPrompt,
		EventType:      log.EventType,
		EventTriggerAt: log.EventTriggerAt,
	}
	if !log.EndedAt.IsZero() {
		ended := log.EndedAt
		gql.EndedAt = &ended
		dur := int(log.EndedAt.Sub(log.StartedAt) / 1_000_000) // ms
		gql.DurationMs = &dur
	}
	return gql
}

func jobRunStageToGraphQL(s model.JobRunStage) graphql1.JobRunStage {
	switch s {
	case model.JobRunStageRunning:
		return graphql1.JobRunStageRunning
	case model.JobRunStageSuccess:
		return graphql1.JobRunStageSuccess
	case model.JobRunStageFailed:
		return graphql1.JobRunStageFailed
	default:
		// Defensive default: unknown stages render as FAILED so a stale
		// document never silently appears as "running forever" in the UI.
		return graphql1.JobRunStageFailed
	}
}

// toGraphQLJobRunEvent maps one event to its GraphQL form. The payload
// is JSON-encoded as a string so a single field can carry every event
// kind's distinct shape (LLM request/response, tool call, run error).
// The frontend round-trips it back to an object before rendering.
func toGraphQLJobRunEvent(ev *model.JobRunEvent) (*graphql1.JobRunEvent, error) {
	if ev == nil {
		return nil, nil
	}
	payload, err := encodeJobRunEventPayload(ev)
	if err != nil {
		return nil, goerr.Wrap(err, "encode job run event payload",
			goerr.V("run_id", ev.RunID),
			goerr.V("event_id", ev.EventID))
	}
	return &graphql1.JobRunEvent{
		EventID:        ev.EventID,
		RunID:          ev.RunID,
		Sequence:       int(ev.Sequence),
		OccurredAt:     ev.OccurredAt,
		Kind:           jobRunEventKindToGraphQL(ev.Kind),
		ParentSequence: int(ev.ParentSequence),
		Phase:          ev.Phase,
		AgentLabel:     ev.AgentLabel,
		Payload:        payload,
	}, nil
}

func jobRunEventKindToGraphQL(k model.JobRunEventKind) graphql1.JobRunEventKind {
	switch k {
	case model.JobRunEventKindLLMRequest:
		return graphql1.JobRunEventKindLlmRequest
	case model.JobRunEventKindLLMResponse:
		return graphql1.JobRunEventKindLlmResponse
	case model.JobRunEventKindToolCall:
		return graphql1.JobRunEventKindToolCall
	case model.JobRunEventKindRunError:
		return graphql1.JobRunEventKindRunError
	default:
		return graphql1.JobRunEventKindRunError
	}
}

// encodeJobRunEventPayload picks the populated payload pointer (exactly
// one must be set, enforced by model.JobRunEvent.Validate) and returns
// its JSON marshalled form. Returns "{}" for the impossible all-nil
// case so callers never see empty strings in the GraphQL response.
func encodeJobRunEventPayload(ev *model.JobRunEvent) (string, error) {
	var payload any
	switch {
	case ev.LLMRequest != nil:
		payload = ev.LLMRequest
	case ev.LLMResponse != nil:
		payload = ev.LLMResponse
	case ev.ToolCall != nil:
		payload = ev.ToolCall
	case ev.RunError != nil:
		payload = ev.RunError
	default:
		return "{}", nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", goerr.Wrap(err, "marshal payload to json")
	}
	return string(raw), nil
}
