package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runAgentSessionRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Get returns nil for missing session", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		got, err := repo.AgentSession().Get(ctx, wsID, time.Now().UnixNano(), "1700000000.000000")
		gt.NoError(t, err).Required()
		gt.Value(t, got).Nil()
	})

	t.Run("Put then Get round trips all fields", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()
		threadTS := fmt.Sprintf("%d.000001", time.Now().Unix())
		now := time.Now().UTC().Truncate(time.Millisecond)

		s := &model.AgentSession{
			ID:            uuid.Must(uuid.NewV7()).String(),
			WorkspaceID:   wsID,
			CaseID:        caseID,
			ThreadTS:      threadTS,
			ChannelID:     "C123",
			ActionID:      42,
			LastMentionTS: threadTS,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		gt.NoError(t, repo.AgentSession().Put(ctx, s)).Required()

		got, err := repo.AgentSession().Get(ctx, wsID, caseID, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()

		gt.Value(t, got.ID).Equal(s.ID)
		gt.Value(t, got.WorkspaceID).Equal(wsID)
		gt.Value(t, got.CaseID).Equal(caseID)
		gt.Value(t, got.ThreadTS).Equal(threadTS)
		gt.Value(t, got.ChannelID).Equal("C123")
		gt.Value(t, got.ActionID).Equal(int64(42))
		gt.Value(t, got.LastMentionTS).Equal(threadTS)
		gt.Bool(t, got.CreatedAt.Equal(now)).True()
		gt.Bool(t, got.UpdatedAt.Equal(now)).True()
	})

	t.Run("Put overwrites existing session", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()
		threadTS := fmt.Sprintf("%d.000002", time.Now().Unix())
		id := uuid.Must(uuid.NewV7()).String()

		first := &model.AgentSession{
			ID: id, WorkspaceID: wsID, CaseID: caseID, ThreadTS: threadTS,
			ChannelID: "C1", LastMentionTS: "1.0",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		gt.NoError(t, repo.AgentSession().Put(ctx, first)).Required()

		second := *first
		second.LastMentionTS = "2.0"
		second.UpdatedAt = time.Now().UTC().Add(time.Second)
		gt.NoError(t, repo.AgentSession().Put(ctx, &second)).Required()

		got, err := repo.AgentSession().Get(ctx, wsID, caseID, threadTS)
		gt.NoError(t, err).Required()
		gt.Value(t, got).NotNil().Required()
		gt.Value(t, got.LastMentionTS).Equal("2.0")
		gt.Value(t, got.ID).Equal(id)
	})

	t.Run("different threadTS in same case are independent", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()
		threadA := fmt.Sprintf("%d.A", time.Now().UnixNano())
		threadB := fmt.Sprintf("%d.B", time.Now().UnixNano())

		a := &model.AgentSession{
			ID:          uuid.Must(uuid.NewV7()).String(),
			WorkspaceID: wsID, CaseID: caseID, ThreadTS: threadA,
			ChannelID: "C1", LastMentionTS: threadA,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		b := &model.AgentSession{
			ID:          uuid.Must(uuid.NewV7()).String(),
			WorkspaceID: wsID, CaseID: caseID, ThreadTS: threadB,
			ChannelID: "C1", LastMentionTS: threadB,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		gt.NoError(t, repo.AgentSession().Put(ctx, a)).Required()
		gt.NoError(t, repo.AgentSession().Put(ctx, b)).Required()

		gotA, err := repo.AgentSession().Get(ctx, wsID, caseID, threadA)
		gt.NoError(t, err).Required()
		gt.Value(t, gotA).NotNil().Required()
		gt.Value(t, gotA.ID).Equal(a.ID)

		gotB, err := repo.AgentSession().Get(ctx, wsID, caseID, threadB)
		gt.NoError(t, err).Required()
		gt.Value(t, gotB).NotNil().Required()
		gt.Value(t, gotB.ID).Equal(b.ID)
	})

	t.Run("rejects missing required fields", func(t *testing.T) {
		repo := newRepo(t)
		err := repo.AgentSession().Put(ctx, &model.AgentSession{})
		gt.Error(t, err)

		err = repo.AgentSession().Put(ctx, nil)
		gt.Error(t, err)
	})
}

func TestAgentSessionRepository_Memory(t *testing.T) {
	runAgentSessionRepositoryTest(t, func(_ *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestAgentSessionRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}
	runAgentSessionRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})
		return repo
	})
}
