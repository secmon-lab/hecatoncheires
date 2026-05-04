package agentarchive_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gollem"
	"github.com/m-mizutani/gollem/trace"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
)

func TestMemoryHistoryRepository(t *testing.T) {
	ctx := context.Background()

	t.Run("Load returns nil for missing session", func(t *testing.T) {
		repo := agentarchive.NewMemoryHistoryRepository()
		got, err := repo.Load(ctx, "nonexistent")
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("Save then Load round trip", func(t *testing.T) {
		repo := agentarchive.NewMemoryHistoryRepository()
		h := &gollem.History{
			LLType:  gollem.LLMTypeClaude,
			Version: gollem.HistoryVersion,
		}
		gt.NoError(t, repo.Save(ctx, "session-A", h)).Required()

		got, err := repo.Load(ctx, "session-A")
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.LLType).Equal(gollem.LLMTypeClaude)
		gt.Value(t, got.Version).Equal(gollem.HistoryVersion)
	})

	t.Run("Save overwrites previous entry", func(t *testing.T) {
		repo := agentarchive.NewMemoryHistoryRepository()
		gt.NoError(t, repo.Save(ctx, "id-1", &gollem.History{
			LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion,
		})).Required()
		gt.NoError(t, repo.Save(ctx, "id-1", &gollem.History{
			LLType: gollem.LLMTypeClaude, Version: gollem.HistoryVersion,
		})).Required()

		got, err := repo.Load(ctx, "id-1")
		gt.NoError(t, err).Required()
		gt.Value(t, got.LLType).Equal(gollem.LLMTypeClaude)
	})

	t.Run("rejects empty sessionID; nil history is a no-op", func(t *testing.T) {
		repo := agentarchive.NewMemoryHistoryRepository()
		gt.Error(t, repo.Save(ctx, "", &gollem.History{Version: gollem.HistoryVersion}))
		// gollem passes whatever its session reports, including a nil
		// history when no turns have been recorded — that must not fail.
		gt.NoError(t, repo.Save(ctx, "x", nil))
		got, err := repo.Load(ctx, "x")
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
		_, err = repo.Load(ctx, "")
		gt.Error(t, err)
	})
}

func TestMemoryTraceRepository(t *testing.T) {
	ctx := context.Background()

	t.Run("Save records trace under session", func(t *testing.T) {
		repo := agentarchive.NewMemoryTraceRepository()
		now := time.Now()
		tr := &trace.Trace{
			TraceID: "trace-1",
			Metadata: trace.TraceMetadata{
				Labels: map[string]string{
					agentarchive.SessionIDLabel: "session-A",
				},
			},
			StartedAt: now,
			EndedAt:   now,
		}
		gt.NoError(t, repo.Save(ctx, tr)).Required()

		got := repo.Load("session-A", "trace-1")
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.TraceID).Equal("trace-1")
		gt.Value(t, got.Metadata.Labels[agentarchive.SessionIDLabel]).Equal("session-A")
	})

	t.Run("multiple traces under one session are accumulated", func(t *testing.T) {
		repo := agentarchive.NewMemoryTraceRepository()
		for _, id := range []string{"t1", "t2", "t3"} {
			gt.NoError(t, repo.Save(ctx, &trace.Trace{
				TraceID: id,
				Metadata: trace.TraceMetadata{
					Labels: map[string]string{agentarchive.SessionIDLabel: "S"},
				},
			})).Required()
		}
		ids := repo.TraceIDs("S")
		gt.Array(t, ids).Length(3)
		// Verify each ID is present (order is not guaranteed for map iteration)
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		gt.Bool(t, seen["t1"]).True()
		gt.Bool(t, seen["t2"]).True()
		gt.Bool(t, seen["t3"]).True()
	})

	t.Run("rejects missing session_id label or empty trace_id", func(t *testing.T) {
		repo := agentarchive.NewMemoryTraceRepository()
		gt.Error(t, repo.Save(ctx, &trace.Trace{TraceID: "id"}))
		gt.Error(t, repo.Save(ctx, &trace.Trace{
			Metadata: trace.TraceMetadata{Labels: map[string]string{
				agentarchive.SessionIDLabel: "S",
			}},
		}))
		gt.Error(t, repo.Save(ctx, nil))
	})
}
