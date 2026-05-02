package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/firestore"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
)

func runActionMessageRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Put and List", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		msg1 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-1", time.Now().UnixNano()),
			"C123", "parent-ts", "T123", "U001", "alice", "first reply", "ev1",
			now.Add(-2*time.Second),
			nil,
		)
		msg2 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-2", time.Now().UnixNano()),
			"C123", "parent-ts", "T123", "U002", "bob", "second reply", "ev2",
			now.Add(-1*time.Second),
			nil,
		)
		msg3 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-3", time.Now().UnixNano()),
			"C123", "parent-ts", "T123", "U001", "alice", "third reply", "ev3",
			now,
			nil,
		)

		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg1)).Required()
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg2)).Required()
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg3)).Required()

		messages, cursor, err := repo.ActionMessage().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(3)
		gt.Value(t, cursor).Equal("")

		// Newest first
		gt.Value(t, messages[0].Text()).Equal("third reply")
		gt.Value(t, messages[1].Text()).Equal("second reply")
		gt.Value(t, messages[2].Text()).Equal("first reply")

		for _, m := range messages {
			gt.Value(t, m.ChannelID()).Equal("C123")
			gt.Value(t, m.ThreadTS()).Equal("parent-ts")
			gt.Value(t, m.TeamID()).Equal("T123")
		}
	})

	t.Run("List with pagination", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		for i := range 5 {
			msg := slack.NewMessageFromData(
				fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), i),
				"C123", "parent-ts", "T123", "U001", "alice",
				fmt.Sprintf("reply %d", i), "ev",
				now.Add(time.Duration(i)*time.Second),
				nil,
			)
			gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg)).Required()
		}

		page1, cursor1, err := repo.ActionMessage().List(ctx, wsID, actionID, 2, "")
		gt.NoError(t, err).Required()
		gt.Array(t, page1).Length(2)
		gt.String(t, cursor1).NotEqual("")

		page2, cursor2, err := repo.ActionMessage().List(ctx, wsID, actionID, 2, cursor1)
		gt.NoError(t, err).Required()
		gt.Array(t, page2).Length(2)
		gt.String(t, cursor2).NotEqual("")

		page3, cursor3, err := repo.ActionMessage().List(ctx, wsID, actionID, 2, cursor2)
		gt.NoError(t, err).Required()
		gt.Array(t, page3).Length(1)
		gt.Value(t, cursor3).Equal("")
	})

	t.Run("List returns empty for non-existent action", func(t *testing.T) {
		repo := newRepo(t)
		messages, cursor, err := repo.ActionMessage().List(ctx, "non-existent-ws", 99999, 10, "")
		gt.NoError(t, err)
		gt.Array(t, messages).Length(0)
		gt.Value(t, cursor).Equal("")
	})

	t.Run("Put upserts existing message", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionID := time.Now().UnixNano()

		msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		msg1 := slack.NewMessageFromData(
			msgID, "C123", "parent-ts", "T123", "U001", "alice", "original", "ev",
			now,
			nil,
		)
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg1)).Required()

		msg2 := slack.NewMessageFromData(
			msgID, "C123", "parent-ts", "T123", "U001", "alice", "edited", "ev",
			now,
			nil,
		)
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionID, msg2)).Required()

		messages, _, err := repo.ActionMessage().List(ctx, wsID, actionID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1)
		gt.Value(t, messages[0].Text()).Equal("edited")
	})

	t.Run("messages are scoped per action", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		actionA := time.Now().UnixNano()
		actionB := actionA + 1

		now := time.Now().UTC().Truncate(time.Millisecond)
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionA, slack.NewMessageFromData(
			fmt.Sprintf("msg-A-%d", time.Now().UnixNano()),
			"C123", "parent-ts", "T123", "U001", "alice", "for A", "ev",
			now, nil,
		))).Required()
		gt.NoError(t, repo.ActionMessage().Put(ctx, wsID, actionB, slack.NewMessageFromData(
			fmt.Sprintf("msg-B-%d", time.Now().UnixNano()),
			"C123", "parent-ts", "T123", "U002", "bob", "for B", "ev",
			now, nil,
		))).Required()

		msgsA, _, err := repo.ActionMessage().List(ctx, wsID, actionA, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, msgsA).Length(1)
		gt.Value(t, msgsA[0].Text()).Equal("for A")

		msgsB, _, err := repo.ActionMessage().List(ctx, wsID, actionB, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, msgsB).Length(1)
		gt.Value(t, msgsB[0].Text()).Equal("for B")
	})
}

func TestActionMessageRepository_Memory(t *testing.T) {
	runActionMessageRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestActionMessageRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runActionMessageRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})
		return repo
	})
}
