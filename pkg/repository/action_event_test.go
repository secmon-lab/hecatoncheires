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
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runActionEventRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Put and List", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		ev1 := &model.ActionEvent{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Kind:      types.ActionEventCreated,
			ActorID:   "U001",
			NewValue:  "Initial Title",
			CreatedAt: now.Add(-2 * time.Second),
		}
		ev2 := &model.ActionEvent{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Kind:      types.ActionEventTitleChanged,
			ActorID:   "U001",
			OldValue:  "Initial Title",
			NewValue:  "Updated Title",
			CreatedAt: now.Add(-1 * time.Second),
		}
		ev3 := &model.ActionEvent{
			ID:        uuid.NewString(),
			ActionID:  actionID,
			Kind:      types.ActionEventStatusChanged,
			ActorID:   "U002",
			OldValue:  "TODO",
			NewValue:  "IN_PROGRESS",
			CreatedAt: now,
		}

		gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionID, ev1)).Required()
		gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionID, ev2)).Required()
		gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionID, ev3)).Required()

		events, cursor, err := repo.ActionEvent().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, events).Length(3).Required()
		gt.Value(t, cursor).Equal("")

		// Newest first.
		gt.Value(t, events[0].ID).Equal(ev3.ID)
		gt.Value(t, events[0].Kind).Equal(types.ActionEventStatusChanged)
		gt.Value(t, events[0].ActorID).Equal("U002")
		gt.Value(t, events[0].OldValue).Equal("TODO")
		gt.Value(t, events[0].NewValue).Equal("IN_PROGRESS")

		gt.Value(t, events[1].ID).Equal(ev2.ID)
		gt.Value(t, events[1].Kind).Equal(types.ActionEventTitleChanged)
		gt.Value(t, events[1].OldValue).Equal("Initial Title")
		gt.Value(t, events[1].NewValue).Equal("Updated Title")

		gt.Value(t, events[2].ID).Equal(ev1.ID)
		gt.Value(t, events[2].Kind).Equal(types.ActionEventCreated)
		gt.Value(t, events[2].NewValue).Equal("Initial Title")
	})

	t.Run("List with pagination", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		for i := range 5 {
			ev := &model.ActionEvent{
				ID:        uuid.NewString(),
				ActionID:  actionID,
				Kind:      types.ActionEventTitleChanged,
				ActorID:   "U001",
				NewValue:  fmt.Sprintf("title-%d", i),
				CreatedAt: now.Add(time.Duration(i) * time.Second),
			}
			gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionID, ev)).Required()
		}

		page1, cursor1, err := repo.ActionEvent().List(ctx, wsID, actionID, 2, "")
		gt.NoError(t, err).Required()
		gt.Array(t, page1).Length(2)
		gt.String(t, cursor1).NotEqual("")

		page2, cursor2, err := repo.ActionEvent().List(ctx, wsID, actionID, 2, cursor1)
		gt.NoError(t, err).Required()
		gt.Array(t, page2).Length(2)
		gt.String(t, cursor2).NotEqual("")

		page3, cursor3, err := repo.ActionEvent().List(ctx, wsID, actionID, 2, cursor2)
		gt.NoError(t, err).Required()
		gt.Array(t, page3).Length(1)
		gt.Value(t, cursor3).Equal("")
	})

	t.Run("List returns empty for non-existent action", func(t *testing.T) {
		repo := newRepo(t)
		events, cursor, err := repo.ActionEvent().List(ctx, "non-existent-ws", 99999, 10, "")
		gt.NoError(t, err)
		gt.Array(t, events).Length(0)
		gt.Value(t, cursor).Equal("")
	})

	t.Run("events are scoped per action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionA := time.Now().UnixNano()
		actionB := actionA + 1

		now := time.Now().UTC().Truncate(time.Millisecond)
		gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionA, &model.ActionEvent{
			ID: uuid.NewString(), ActionID: actionA, Kind: types.ActionEventCreated,
			NewValue: "for A", CreatedAt: now,
		})).Required()
		gt.NoError(t, repo.ActionEvent().Put(ctx, wsID, actionB, &model.ActionEvent{
			ID: uuid.NewString(), ActionID: actionB, Kind: types.ActionEventCreated,
			NewValue: "for B", CreatedAt: now,
		})).Required()

		eventsA, _, err := repo.ActionEvent().List(ctx, wsID, actionA, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, eventsA).Length(1)
		gt.Value(t, eventsA[0].NewValue).Equal("for A")

		eventsB, _, err := repo.ActionEvent().List(ctx, wsID, actionB, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, eventsB).Length(1)
		gt.Value(t, eventsB[0].NewValue).Equal("for B")
	})

	t.Run("rejects nil event and empty ID", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		gt.Error(t, repo.ActionEvent().Put(ctx, wsID, actionID, nil))
		gt.Error(t, repo.ActionEvent().Put(ctx, wsID, actionID, &model.ActionEvent{
			ID: "", ActionID: actionID, Kind: types.ActionEventCreated,
		}))
	})
}

func TestActionEventRepository_Memory(t *testing.T) {
	runActionEventRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestActionEventRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runActionEventRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})
		return repo
	})
}
