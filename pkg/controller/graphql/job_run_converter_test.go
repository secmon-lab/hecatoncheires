package graphql_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	graphqlctrl "github.com/secmon-lab/hecatoncheires/pkg/controller/graphql"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	graphql1 "github.com/secmon-lab/hecatoncheires/pkg/domain/model/graphql"
)

// TestToGraphQLJobRunEvent pins the event → GraphQL mapping, including the
// payload JSON. The payload is the only wire form the per-call token figures
// have — the JobRunEvent type declares no token fields of its own — so this is
// what both the run-detail UI and a downloaded run file read.
func TestToGraphQLJobRunEvent(t *testing.T) {
	occurred := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)

	t.Run("LLM_RESPONSE payload carries the prompt-cache token split", func(t *testing.T) {
		ev, err := graphqlctrl.ToGraphQLJobRunEventForTest(&model.JobRunEvent{
			WorkspaceID: "ws1",
			CaseID:      16,
			JobID:       "triage",
			RunID:       "run-1",
			EventID:     "ev-2",
			Sequence:    2,
			OccurredAt:  occurred,
			Kind:        model.JobRunEventKindLLMResponse,
			Phase:       "execute",
			AgentLabel:  "investigator",
			LLMResponse: &model.LLMResponsePayload{
				Model:                    "claude-opus-4-6",
				Texts:                    []string{"done"},
				InputTokens:              1200,
				OutputTokens:             180,
				CacheCreationInputTokens: 400,
				CacheReadInputTokens:     700,
				DurationMs:               1450,
			},
		})
		gt.NoError(t, err).Required()
		gt.Value(t, ev).NotNil().Required()
		gt.String(t, ev.EventID).Equal("ev-2")
		gt.Number(t, ev.Sequence).Equal(2)
		gt.Value(t, ev.Kind).Equal(graphql1.JobRunEventKindLlmResponse)
		gt.String(t, ev.Phase).Equal("execute")
		gt.String(t, ev.AgentLabel).Equal("investigator")

		// Decoded as a generic map, not back into LLMResponsePayload: the wire
		// contract is the JSON key spelling, and a round-trip through the same Go
		// type would keep passing if the keys were renamed. The payload carries no
		// struct tags, so the keys are the Go field names, which is what
		// frontend/src/pages/JobRunLogDetail.tsx reads (payload.InputTokens).
		var got map[string]any
		gt.NoError(t, json.Unmarshal([]byte(ev.Payload), &got)).Required()
		gt.Value(t, got["Model"]).Equal("claude-opus-4-6")
		gt.Value(t, got["InputTokens"]).Equal(float64(1200))
		gt.Value(t, got["OutputTokens"]).Equal(float64(180))
		gt.Value(t, got["CacheCreationInputTokens"]).Equal(float64(400))
		gt.Value(t, got["CacheReadInputTokens"]).Equal(float64(700))
		gt.Value(t, got["DurationMs"]).Equal(float64(1450))

		// snake_case spellings must not appear: adding a json tag to the payload
		// would silently break every consumer reading the Go field names.
		for _, absent := range []string{
			"cache_creation_input_tokens", "cache_read_input_tokens",
			"input_tokens", "output_tokens",
		} {
			_, ok := got[absent]
			gt.Bool(t, ok).False()
		}
	})

	t.Run("an event with no payload encodes an empty object", func(t *testing.T) {
		ev, err := graphqlctrl.ToGraphQLJobRunEventForTest(&model.JobRunEvent{
			WorkspaceID: "ws1",
			CaseID:      16,
			JobID:       "triage",
			RunID:       "run-1",
			EventID:     "ev-1",
			Sequence:    1,
			OccurredAt:  occurred,
			Kind:        model.JobRunEventKindLLMRequest,
		})
		gt.NoError(t, err).Required()
		gt.Value(t, ev).NotNil().Required()
		gt.String(t, ev.Payload).Equal("{}")
	})
}
