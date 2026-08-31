package graphql

import (
	"encoding/json"

	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
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
		Strategy:       jobStrategyFromExecutorKind(log.ExecutorKind),
		RunID:          log.RunID,
		TraceID:        log.TraceID,
		Stage:          jobRunStageToGraphQL(log.Stage),
		StartedAt:      log.StartedAt,
		ErrorMessage:   log.Error,
		SystemPrompt:   log.SystemPrompt,
		EventType:      log.EventType,
		EventTriggerAt: log.EventTriggerAt,
		// The wire carries dollars: GraphQL's Int is 32-bit, which a nano-USD
		// figure overflows, and this value is read to be displayed.
		CostUsd: pricing.NanoUSD(log.CostNanoUSD).USDValue(),
		Model:   log.Model,
	}
	if !log.EndedAt.IsZero() {
		ended := log.EndedAt
		gql.EndedAt = &ended
		dur := int(log.EndedAt.Sub(log.StartedAt) / 1_000_000) // ms
		gql.DurationMs = &dur
	}
	return gql
}

// jobStrategyFromExecutorKind maps the persisted ExecutorKind string
// (single_loop / plan_execute / future values) onto the GraphQL
// JobStrategy enum. Unknown values fall back to SIMPLE so a Run row
// produced by an older binary, or a typo, never breaks the page.
func jobStrategyFromExecutorKind(executorKind string) graphql1.JobStrategy {
	switch executorKind {
	case "plan_execute":
		return graphql1.JobStrategyPlanexec
	default:
		return graphql1.JobStrategySimple
	}
}

func jobRunStageToGraphQL(s model.JobRunStage) graphql1.JobRunStage {
	switch s {
	case model.JobRunStageRunning:
		return graphql1.JobRunStageRunning
	case model.JobRunStageSuccess:
		return graphql1.JobRunStageSuccess
	case model.JobRunStageFailed:
		return graphql1.JobRunStageFailed
	case model.JobRunStageAwaitingInput:
		return graphql1.JobRunStageAwaitingInput
	default:
		// Defensive default: unknown stages render as FAILED so a stale
		// document never silently appears as "running forever" in the UI.
		return graphql1.JobRunStageFailed
	}
}

// toGraphQLJobRunEvents maps a run's whole timeline, in the order it was read,
// and reassembles the conversations along the way.
//
// An LLM_REQUEST is stored as a diff: it carries only the messages that were
// not already recorded by earlier events of the same conversation (see
// model.LLMRequestPayload.MessagesPrefixLen). The run detail page shows one
// call's request on its own, so the whole message list is put back here rather
// than in every consumer. That makes this the list-level converter: the
// reconstruction needs the earlier events, which a per-event call cannot see.
func toGraphQLJobRunEvents(events []*model.JobRunEvent) ([]*graphql1.JobRunEvent, error) {
	conversations := map[string][]model.LLMMessage{}
	tools := map[string][]model.LLMToolSpec{}
	out := make([]*graphql1.JobRunEvent, 0, len(events))
	for _, ev := range events {
		gq, err := toGraphQLJobRunEvent(restoreConversation(ev, conversations, tools))
		if err != nil {
			return nil, err
		}
		out = append(out, gq)
	}
	return out, nil
}

// restoreConversation returns ev with its LLM_REQUEST payload expanded back to
// the full request, and records the result so the next event of the same
// conversation can build on it. Any other event is returned unchanged.
//
// The event is never mutated: the payload is replaced on a copy, because the
// caller's slice may be shared (a repository is free to hand out the documents
// it cached) and a consumer must not find the timeline rewritten under it.
func restoreConversation(
	ev *model.JobRunEvent,
	conversations map[string][]model.LLMMessage,
	tools map[string][]model.LLMToolSpec,
) *model.JobRunEvent {
	if ev == nil || ev.LLMRequest == nil {
		return ev
	}
	p := *ev.LLMRequest
	id := p.ConversationID

	// A prefix longer than what was seen cannot be honoured — an event read out
	// of order, or a timeline whose earlier part was trimmed. Take what there
	// is rather than dropping the messages this event does carry.
	prefix := conversations[id]
	if p.MessagesPrefixLen < len(prefix) {
		prefix = prefix[:p.MessagesPrefixLen]
	}
	full := make([]model.LLMMessage, 0, len(prefix)+len(p.Messages))
	full = append(full, prefix...)
	full = append(full, p.Messages...)
	conversations[id] = full

	if len(p.Tools) > 0 {
		tools[id] = p.Tools
	} else {
		p.Tools = tools[id]
	}

	p.Messages = full
	// The payload now holds the whole request, so the diff bookkeeping would
	// only mislead a reader of the GraphQL response.
	p.MessagesPrefixLen = 0

	restored := *ev
	restored.LLMRequest = &p
	return &restored
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
