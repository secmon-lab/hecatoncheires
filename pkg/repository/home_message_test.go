package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runHomeMessageRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	repo := newRepo(t)

	t.Run("Add and ListRecent round-trips fields, newest first, limited", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())
		base := time.Now().UTC().Truncate(time.Millisecond)

		// Add three messages with strictly increasing CreatedAt.
		mk := func(msg string, offset time.Duration) model.HomeMessageID {
			id := model.NewHomeMessageID()
			gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
				ID:        id,
				UserID:    userID,
				Message:   msg,
				Lang:      "en",
				CreatedAt: base.Add(offset),
			})).Required()
			return id
		}
		mk("oldest", 0)
		mk("middle", time.Second)
		newestID := mk("newest", 2*time.Second)

		// Limit 2 returns the two newest, newest first.
		got, err := repo.HomeMessage().ListRecent(ctx, userID, 2)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(2).Required()
		gt.Value(t, got[0].ID).Equal(newestID)
		gt.String(t, got[0].Message).Equal("newest")
		gt.String(t, got[0].Lang).Equal("en")
		gt.Bool(t, got[0].CreatedAt.Equal(base.Add(2*time.Second))).True()
		gt.String(t, got[1].Message).Equal("middle")

		// Limit larger than count returns all three.
		all, err := repo.HomeMessage().ListRecent(ctx, userID, 10)
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(3).Required()
		gt.String(t, all[0].Message).Equal("newest")
		gt.String(t, all[1].Message).Equal("middle")
		gt.String(t, all[2].Message).Equal("oldest")
	})

	t.Run("ListRecent for unknown user is empty", func(t *testing.T) {
		ctx := context.Background()
		userID := fmt.Sprintf("U-%d", time.Now().UnixNano())

		got, err := repo.HomeMessage().ListRecent(ctx, userID, 5)
		gt.NoError(t, err).Required()
		gt.Array(t, got).Length(0)
	})

	t.Run("ListRecent isolates users", func(t *testing.T) {
		ctx := context.Background()
		userA := fmt.Sprintf("U-A-%d", time.Now().UnixNano())
		userB := fmt.Sprintf("U-B-%d", time.Now().UnixNano())
		now := time.Now().UTC()

		gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
			ID: model.NewHomeMessageID(), UserID: userA, Message: "for A", Lang: "ja", CreatedAt: now,
		})).Required()
		gt.NoError(t, repo.HomeMessage().Add(ctx, &model.HomeMessage{
			ID: model.NewHomeMessageID(), UserID: userB, Message: "for B", Lang: "ja", CreatedAt: now,
		})).Required()

		gotA, err := repo.HomeMessage().ListRecent(ctx, userA, 5)
		gt.NoError(t, err).Required()
		gt.Array(t, gotA).Length(1).Required()
		gt.String(t, gotA[0].Message).Equal("for A")
	})

	t.Run("Add rejects invalid message", func(t *testing.T) {
		ctx := context.Background()
		err := repo.HomeMessage().Add(ctx, &model.HomeMessage{
			ID: model.NewHomeMessageID(), UserID: "U-x", Message: "", Lang: "en", CreatedAt: time.Now(),
		})
		gt.Error(t, err).Is(model.ErrHomeMessageValidation)
	})
}

func TestHomeMessageRepository_Memory(t *testing.T) {
	t.Parallel()
	runHomeMessageRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestHomeMessageRepository_Firestore(t *testing.T) {
	t.Parallel()
	runHomeMessageRepositoryTest(t, newFirestoreRepository)
}
