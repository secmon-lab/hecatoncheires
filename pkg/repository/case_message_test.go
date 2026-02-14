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

func runCaseMessageRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()
	ctx := context.Background()

	t.Run("Put and List", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		msg1 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-1", time.Now().UnixNano()),
			"C123", "", "T123", "U001", "alice", "hello", "ev1",
			now.Add(-2*time.Second),
			nil,
		)
		msg2 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-2", time.Now().UnixNano()),
			"C123", "", "T123", "U002", "bob", "world", "ev2",
			now.Add(-1*time.Second),
			nil,
		)
		msg3 := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d-3", time.Now().UnixNano()),
			"C123", "thread-ts", "T123", "U001", "alice", "reply", "ev3",
			now,
			nil,
		)

		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg1)).Required()
		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg2)).Required()
		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg3)).Required()

		messages, cursor, err := repo.CaseMessage().List(ctx, wsID, caseID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(3)
		gt.Value(t, cursor).Equal("")

		// Should be newest first
		gt.Value(t, messages[0].Text()).Equal("reply")
		gt.Value(t, messages[1].Text()).Equal("world")
		gt.Value(t, messages[2].Text()).Equal("hello")

		// Verify ChannelID and TeamID are correctly round-tripped
		for _, m := range messages {
			gt.Value(t, m.ChannelID()).Equal("C123")
			gt.Value(t, m.TeamID()).Equal("T123")
		}
	})

	t.Run("List with pagination", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		for i := 0; i < 5; i++ {
			msg := slack.NewMessageFromData(
				fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), i),
				"C123", "", "T123", "U001", "alice",
				fmt.Sprintf("message %d", i), "ev",
				now.Add(time.Duration(i)*time.Second),
				nil,
			)
			gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg)).Required()
		}

		// First page
		page1, cursor1, err := repo.CaseMessage().List(ctx, wsID, caseID, 2, "")
		gt.NoError(t, err).Required()
		gt.Array(t, page1).Length(2)
		gt.String(t, cursor1).NotEqual("")

		// Second page
		page2, cursor2, err := repo.CaseMessage().List(ctx, wsID, caseID, 2, cursor1)
		gt.NoError(t, err).Required()
		gt.Array(t, page2).Length(2)
		gt.String(t, cursor2).NotEqual("")

		// Third page
		page3, cursor3, err := repo.CaseMessage().List(ctx, wsID, caseID, 2, cursor2)
		gt.NoError(t, err).Required()
		gt.Array(t, page3).Length(1)
		gt.Value(t, cursor3).Equal("")
	})

	t.Run("List returns empty for non-existent case", func(t *testing.T) {
		repo := newRepo(t)
		messages, cursor, err := repo.CaseMessage().List(ctx, "non-existent-ws", 99999, 10, "")
		gt.NoError(t, err)
		gt.Array(t, messages).Length(0)
		gt.Value(t, cursor).Equal("")
	})

	t.Run("Prune deletes old messages", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		oldMsg := slack.NewMessageFromData(
			fmt.Sprintf("msg-old-%d", time.Now().UnixNano()),
			"C123", "", "T123", "U001", "alice", "old message", "ev",
			now.Add(-10*time.Minute),
			nil,
		)
		newMsg := slack.NewMessageFromData(
			fmt.Sprintf("msg-new-%d", time.Now().UnixNano()),
			"C123", "", "T123", "U002", "bob", "new message", "ev",
			now,
			nil,
		)

		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, oldMsg)).Required()
		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, newMsg)).Required()

		deleted, err := repo.CaseMessage().Prune(ctx, wsID, caseID, now.Add(-5*time.Minute))
		gt.NoError(t, err).Required()
		gt.Number(t, deleted).Equal(1)

		messages, _, err := repo.CaseMessage().List(ctx, wsID, caseID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1)
		gt.Value(t, messages[0].Text()).Equal("new message")
	})

	t.Run("Put and List with file attachments", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		files := []slack.File{
			slack.NewFileFromData(
				"F001", "image.png", "image/png", "png", 51200,
				"https://files.slack.com/private/image.png",
				"https://workspace.slack.com/files/U123/F001/image.png",
				"https://files.slack.com/thumb_360.png",
			),
		}

		msg := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			"C123", "", "T123", "U001", "alice", "file attached", "ev1",
			now,
			files,
		)

		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg)).Required()

		messages, _, err := repo.CaseMessage().List(ctx, wsID, caseID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1).Required()

		resultFiles := messages[0].Files()
		gt.Array(t, resultFiles).Length(1)
		gt.Value(t, resultFiles[0].ID()).Equal("F001")
		gt.Value(t, resultFiles[0].Name()).Equal("image.png")
		gt.Value(t, resultFiles[0].Mimetype()).Equal("image/png")
		gt.Value(t, resultFiles[0].Size()).Equal(51200)
		gt.Value(t, resultFiles[0].Permalink()).Equal("https://workspace.slack.com/files/U123/F001/image.png")
		gt.Value(t, resultFiles[0].ThumbURL()).Equal("https://files.slack.com/thumb_360.png")
	})

	t.Run("Put and List without files (backward compatibility)", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		now := time.Now().UTC().Truncate(time.Millisecond)
		msg := slack.NewMessageFromData(
			fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			"C123", "", "T123", "U001", "alice", "no files", "ev1",
			now,
			nil,
		)

		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg)).Required()

		messages, _, err := repo.CaseMessage().List(ctx, wsID, caseID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1).Required()
		gt.Array(t, messages[0].Files()).Length(0)
	})

	t.Run("Put upserts existing message", func(t *testing.T) {
		repo := newRepo(t)
		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
		caseID := time.Now().UnixNano()

		msgID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
		now := time.Now().UTC().Truncate(time.Millisecond)

		msg1 := slack.NewMessageFromData(
			msgID, "C123", "", "T123", "U001", "alice", "original", "ev",
			now,
			nil,
		)
		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg1)).Required()

		msg2 := slack.NewMessageFromData(
			msgID, "C123", "", "T123", "U001", "alice", "updated", "ev",
			now,
			nil,
		)
		gt.NoError(t, repo.CaseMessage().Put(ctx, wsID, caseID, msg2)).Required()

		messages, _, err := repo.CaseMessage().List(ctx, wsID, caseID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1)
		gt.Value(t, messages[0].Text()).Equal("updated")
	})
}

func TestCaseMessageRepository_Memory(t *testing.T) {
	runCaseMessageRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestCaseMessageRepository_Firestore(t *testing.T) {
	projectID := os.Getenv("FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("FIRESTORE_PROJECT_ID not set")
	}

	runCaseMessageRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		repo, err := firestore.New(context.Background(), projectID, "")
		gt.NoError(t, err).Required()
		t.Cleanup(func() {
			gt.NoError(t, repo.Close())
		})
		return repo
	})
}
