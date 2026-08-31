package graphql_test

import (
	"encoding/json"
	"strconv"
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

// llmRequestEvent builds one LLM_REQUEST event of a conversation, carrying only
// the messages after prefixLen — the diff form the timeline stores.
func llmRequestEvent(seq int64, conversationID string, prefixLen int, texts ...string) *model.JobRunEvent {
	msgs := make([]model.LLMMessage, 0, len(texts))
	for _, txt := range texts {
		msgs = append(msgs, model.LLMMessage{
			Role:     "user",
			Contents: []model.LLMContentBlock{{Type: "text", Text: txt}},
		})
	}
	return &model.JobRunEvent{
		WorkspaceID: "ws1", CaseID: 16, JobID: "triage", RunID: "run-1",
		EventID:  "ev-" + conversationID + "-" + strconv.FormatInt(seq, 10),
		Sequence: seq,
		Kind:     model.JobRunEventKindLLMRequest,
		LLMRequest: &model.LLMRequestPayload{
			Model:             "claude-opus-4-6",
			ConversationID:    conversationID,
			MessagesPrefixLen: prefixLen,
			Messages:          msgs,
		},
	}
}

// payloadMessageTexts reads the message texts out of an encoded payload.
func payloadMessageTexts(t *testing.T, payload string) []string {
	t.Helper()
	var got struct {
		Messages []struct {
			Contents []struct{ Text string }
		}
	}
	gt.NoError(t, json.Unmarshal([]byte(payload), &got)).Required()
	out := make([]string, 0, len(got.Messages))
	for _, m := range got.Messages {
		gt.Array(t, m.Contents).Length(1).Required()
		out = append(out, m.Contents[0].Text)
	}
	return out
}

// TestToGraphQLJobRunEventsRestoresConversations pins the read side of the
// diff-recorded conversation: the timeline stores each LLM_REQUEST as only what
// was new, and the run-detail page must still be handed each call's whole
// request.
func TestToGraphQLJobRunEventsRestoresConversations(t *testing.T) {
	t.Run("a conversation's diffs are concatenated back", func(t *testing.T) {
		events := []*model.JobRunEvent{
			llmRequestEvent(1, "conv-a", 0, "m0"),
			llmRequestEvent(2, "conv-a", 1, "m1"),
			llmRequestEvent(3, "conv-a", 2, "m2", "m3"),
		}
		got, err := graphqlctrl.ToGraphQLJobRunEventsForTest(events)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(3).Required()

		gt.Value(t, payloadMessageTexts(t, got[0].Payload)).Equal([]string{"m0"})
		gt.Value(t, payloadMessageTexts(t, got[1].Payload)).Equal([]string{"m0", "m1"})
		gt.Value(t, payloadMessageTexts(t, got[2].Payload)).
			Equal([]string{"m0", "m1", "m2", "m3"})

		// The stored events are not rewritten: a caller's slice may be shared.
		gt.Array(t, events[2].LLMRequest.Messages).Length(2)
		gt.Number(t, events[2].LLMRequest.MessagesPrefixLen).Equal(2)
	})

	t.Run("two conversations interleaved on one run stay apart", func(t *testing.T) {
		events := []*model.JobRunEvent{
			llmRequestEvent(1, "conv-a", 0, "a0"),
			llmRequestEvent(2, "conv-b", 0, "b0"),
			llmRequestEvent(3, "conv-b", 1, "b1"),
			llmRequestEvent(4, "conv-a", 1, "a1"),
		}
		got, err := graphqlctrl.ToGraphQLJobRunEventsForTest(events)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(4).Required()

		gt.Value(t, payloadMessageTexts(t, got[2].Payload)).Equal([]string{"b0", "b1"})
		gt.Value(t, payloadMessageTexts(t, got[3].Payload)).Equal([]string{"a0", "a1"})
	})

	t.Run("a record written before the diff existed reads as the whole request", func(t *testing.T) {
		// No ConversationID, no prefix: every event carries its own full list,
		// which is what the timeline held before this field was introduced.
		events := []*model.JobRunEvent{
			llmRequestEvent(1, "", 0, "m0"),
			llmRequestEvent(2, "", 0, "m0", "m1"),
		}
		got, err := graphqlctrl.ToGraphQLJobRunEventsForTest(events)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(2).Required()

		gt.Value(t, payloadMessageTexts(t, got[0].Payload)).Equal([]string{"m0"})
		gt.Value(t, payloadMessageTexts(t, got[1].Payload)).Equal([]string{"m0", "m1"})
	})

	t.Run("the tool set recorded once is carried to the later calls", func(t *testing.T) {
		first := llmRequestEvent(1, "conv-a", 0, "m0")
		first.LLMRequest.Tools = []model.LLMToolSpec{{Name: "slack_search", Description: "search"}}
		events := []*model.JobRunEvent{first, llmRequestEvent(2, "conv-a", 1, "m1")}

		got, err := graphqlctrl.ToGraphQLJobRunEventsForTest(events)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(2).Required()

		var second struct{ Tools []model.LLMToolSpec }
		gt.NoError(t, json.Unmarshal([]byte(got[1].Payload), &second)).Required()
		gt.Array(t, second.Tools).Length(1).Required()
		gt.String(t, second.Tools[0].Name).Equal("slack_search")
	})
}

// TestToGraphQLJobRunLogCost pins the unit change at the wire boundary: the run
// record stores nano-USD integers, and the field a page reads is dollars.
func TestToGraphQLJobRunLogCost(t *testing.T) {
	started := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)

	testCases := map[string]struct {
		costNanoUSD int64
		model       string
		wantUSD     float64
	}{
		"cents":                              {costNanoUSD: 12_340_000, model: "gemini-3.7-flash", wantUSD: 0.01234},
		"dollars":                            {costNanoUSD: 2_500_000_000, model: "claude-opus-5", wantUSD: 2.5},
		"a run recorded before cost existed": {costNanoUSD: 0, wantUSD: 0},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			gql := graphqlctrl.ToGraphQLJobRunLogForTest(&model.JobRunLog{
				WorkspaceID: "ws1",
				CaseID:      16,
				JobID:       "triage",
				RunID:       "run-1",
				TraceID:     "trace-1",
				Stage:       model.JobRunStageSuccess,
				StartedAt:   started,
				CostNanoUSD: tc.costNanoUSD,
				Model:       tc.model,
			}, "Triage")
			gt.Value(t, gql).NotNil().Required()
			gt.Value(t, gql.CostUsd).Equal(tc.wantUSD)
			gt.String(t, gql.Model).Equal(tc.model)
		})
	}
}
