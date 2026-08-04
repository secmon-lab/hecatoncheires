package export_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/export"
)

// TestEncodeEventJSON_OversizedPayload pins what a job_run_events JSON cell
// larger than the export limit turns into. The cap exists because the Storage
// Write API rejects a request over 10 MB and no batching can split a single
// row, so an unbounded cell would fail the whole table.
func TestEncodeEventJSON_OversizedPayload(t *testing.T) {
	ctx := context.Background()
	repo, entry, wsID, normalID, _, _ := seededWorkspace(t)
	now := time.Now().UTC().Truncate(time.Second)

	// One message per ~64 KiB, enough of them to pass model.MaxInlineBytes once
	// marshalled. Every message on its own is within the per-string cap.
	const chunk = 64 * 1024
	msgCount := model.MaxInlineBytes/chunk + 4
	messages := make([]model.LLMMessage, 0, msgCount)
	for range msgCount {
		messages = append(messages, model.LLMMessage{
			Role:     "user",
			Contents: []model.LLMContentBlock{{Type: "text", Text: strings.Repeat("x", chunk)}},
		})
	}

	oversized := &model.JobRunEvent{
		WorkspaceID: wsID, CaseID: normalID, JobID: "triage", RunID: "run-triage",
		TraceID: "trace-triage", EventID: "triage-oversized", Sequence: 5,
		OccurredAt: now, Kind: model.JobRunEventKindLLMRequest,
		Phase: "execute", AgentLabel: "investigator",
		LLMRequest: &model.LLMRequestPayload{Model: "claude-opus-4-7", Messages: messages},
	}
	gt.NoError(t, repo.JobRunEvent().Append(ctx, oversized)).Required()

	sink := newFakeSink()
	exporter := export.New(repo, sink)
	gt.NoError(t, exporter.Run(ctx, []export.Target{{Entry: entry, Namespace: "ns"}})).Required()

	events := sink.table("ns", "job_run_events")
	gt.Value(t, events).NotNil().Required()

	t.Run("the oversized cell is replaced by a valid JSON placeholder", func(t *testing.T) {
		row := findEventRow(events, normalID, 5)
		gt.Value(t, row).NotNil().Required()

		cell, ok := row["messages_json"].(string)
		gt.Bool(t, ok).True().Required()
		gt.Number(t, len(cell)).LessOrEqual(model.MaxInlineBytes)

		// Truncating the JSON would leave a value no consumer can read, so the
		// replacement has to parse.
		var decoded struct {
			Oversized     bool `json:"oversized"`
			OriginalBytes int  `json:"original_bytes"`
		}
		gt.NoError(t, json.Unmarshal([]byte(cell), &decoded)).Required()
		gt.Bool(t, decoded.Oversized).True()
		gt.Number(t, decoded.OriginalBytes).Greater(model.MaxInlineBytes)
	})

	t.Run("a payload within the limit is untouched", func(t *testing.T) {
		// Sequence 1 is the ordinary LLM_REQUEST seeded by seedJobRun.
		row := findEventRow(events, normalID, 1)
		gt.Value(t, row).NotNil().Required()

		cell, ok := row["messages_json"].(string)
		gt.Bool(t, ok).True().Required()
		gt.String(t, cell).Contains("investigate the case")

		var decoded []model.LLMMessage
		gt.NoError(t, json.Unmarshal([]byte(cell), &decoded)).Required()
		gt.Array(t, decoded).Length(1).Required()
		gt.Value(t, decoded[0].Role).Equal("user")
	})

	t.Run("every row stays small enough for one append request", func(t *testing.T) {
		// The sink refuses a row it cannot fit in an AppendRows call, so the
		// cap above is what keeps the events table writable at all.
		for _, row := range events.Rows {
			total := 0
			for _, v := range row {
				if s, ok := v.(string); ok {
					total += len(s)
				}
			}
			gt.Number(t, total).LessOrEqual(9 * 1024 * 1024)
		}
	})
}
