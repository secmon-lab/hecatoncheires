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

func runSlackRepositoryTest(t *testing.T, newRepo func(t *testing.T) interfaces.Repository) {
	t.Helper()

	t.Run("PutMessage and ListMessages basic test", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Use random IDs to avoid conflicts
		channelID := fmt.Sprintf("C%d", now.UnixNano())
		teamID := fmt.Sprintf("T%d", now.UnixNano())
		userID := fmt.Sprintf("U%d", now.UnixNano())

		// Create test messages
		msg1 := slack.NewMessageFromData(
			fmt.Sprintf("%d.000001", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"First message",
			fmt.Sprintf("%d.000001", now.Unix()),
			now.Add(-2*time.Hour),
			nil,
		)

		msg2 := slack.NewMessageFromData(
			fmt.Sprintf("%d.000002", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"Second message",
			fmt.Sprintf("%d.000002", now.Unix()),
			now.Add(-1*time.Hour),
			nil,
		)

		msg3 := slack.NewMessageFromData(
			fmt.Sprintf("%d.000003", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"Third message",
			fmt.Sprintf("%d.000003", now.Unix()),
			now,
			nil,
		)

		// Put messages
		gt.NoError(t, repo.Slack().PutMessage(ctx, msg1)).Required()
		gt.NoError(t, repo.Slack().PutMessage(ctx, msg2)).Required()
		gt.NoError(t, repo.Slack().PutMessage(ctx, msg3)).Required()

		// List all messages
		messages, nextCursor, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-3*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(3)

		// Messages should be in descending order (newest first)
		gt.Value(t, messages[0].ID()).Equal(msg3.ID())
		gt.Value(t, messages[1].ID()).Equal(msg2.ID())
		gt.Value(t, messages[2].ID()).Equal(msg1.ID())

		// Verify all fields
		gt.Value(t, messages[0].ChannelID()).Equal(channelID)
		gt.Value(t, messages[0].TeamID()).Equal(teamID)
		gt.Value(t, messages[0].UserID()).Equal(userID)
		gt.Value(t, messages[0].Text()).Equal("Third message")

		gt.Value(t, nextCursor).Equal("")
	})

	t.Run("PutMessage performs upsert", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		channelID := fmt.Sprintf("C%d", now.UnixNano())
		messageID := fmt.Sprintf("%d.000001", now.Unix())

		// Create initial message
		msg1 := slack.NewMessageFromData(
			messageID,
			channelID,
			"",
			fmt.Sprintf("T%d", now.UnixNano()),
			fmt.Sprintf("U%d", now.UnixNano()),
			"user1",
			"Original text",
			messageID,
			now,
			nil,
		)

		gt.NoError(t, repo.Slack().PutMessage(ctx, msg1)).Required()

		// Update with same ID
		msg2 := slack.NewMessageFromData(
			messageID,
			channelID,
			"",
			msg1.TeamID(),
			msg1.UserID(),
			"user1",
			"Updated text",
			messageID,
			now.Add(time.Minute),
			nil,
		)

		gt.NoError(t, repo.Slack().PutMessage(ctx, msg2)).Required()

		// List messages - should only have one
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1).Required()

		gt.Value(t, messages[0].Text()).Equal("Updated text")
	})

	t.Run("ListMessages supports pagination", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		channelID := fmt.Sprintf("C%d", now.UnixNano())
		teamID := fmt.Sprintf("T%d", now.UnixNano())
		userID := fmt.Sprintf("U%d", now.UnixNano())

		// Create 5 messages
		for i := 1; i <= 5; i++ {
			msg := slack.NewMessageFromData(
				fmt.Sprintf("%d.%06d", now.Unix(), i),
				channelID,
				"",
				teamID,
				userID,
				"user1",
				fmt.Sprintf("Message %d", i),
				fmt.Sprintf("%d.%06d", now.Unix(), i),
				now.Add(time.Duration(i)*time.Minute),
				nil,
			)
			gt.NoError(t, repo.Slack().PutMessage(ctx, msg)).Required()
		}

		// List with limit=2
		messages, nextCursor, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			2,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(2)

		gt.String(t, nextCursor).NotEqual("")

		// Get next page
		messages2, nextCursor2, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			2,
			nextCursor,
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages2).Length(2)

		// Still one more message remaining (5 total, 2 per page, 1 left after page 2)
		gt.String(t, nextCursor2).NotEqual("")

		// Verify no duplicate messages
		if len(messages) > 0 && len(messages2) > 0 {
			gt.Value(t, messages[len(messages)-1].ID()).NotEqual(messages2[0].ID())
		}
	})

	t.Run("PruneMessages deletes old messages by channel", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		channelID := fmt.Sprintf("C%d", now.UnixNano())
		teamID := fmt.Sprintf("T%d", now.UnixNano())
		userID := fmt.Sprintf("U%d", now.UnixNano())

		// Create old and new messages
		oldMsg := slack.NewMessageFromData(
			fmt.Sprintf("%d.000001", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"Old message",
			fmt.Sprintf("%d.000001", now.Unix()),
			now.Add(-2*time.Hour),
			nil,
		)

		newMsg := slack.NewMessageFromData(
			fmt.Sprintf("%d.000002", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"New message",
			fmt.Sprintf("%d.000002", now.Unix()),
			now,
			nil,
		)

		gt.NoError(t, repo.Slack().PutMessage(ctx, oldMsg)).Required()
		gt.NoError(t, repo.Slack().PutMessage(ctx, newMsg)).Required()

		// Prune messages older than 1 hour ago
		deleted, err := repo.Slack().PruneMessages(ctx, channelID, now.Add(-1*time.Hour))
		gt.NoError(t, err).Required()

		gt.Value(t, deleted).Equal(1)

		// Verify only new message remains
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-3*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)

		if len(messages) == 1 {
			gt.Value(t, messages[0].ID()).Equal(newMsg.ID())
		}
	})

	t.Run("PutMessage and ListMessages with file attachments", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		channelID := fmt.Sprintf("C%d", now.UnixNano())
		teamID := fmt.Sprintf("T%d", now.UnixNano())
		userID := fmt.Sprintf("U%d", now.UnixNano())

		files := []slack.File{
			slack.NewFileFromData(
				"F001", "screenshot.png", "image/png", "png", 102400,
				"https://files.slack.com/files-pri/T123-F001/screenshot.png",
				"https://workspace.slack.com/files/U123/F001/screenshot.png",
				"https://files.slack.com/thumb_480.png",
			),
			slack.NewFileFromData(
				"F002", "document.pdf", "application/pdf", "pdf", 204800,
				"https://files.slack.com/files-pri/T123-F002/document.pdf",
				"https://workspace.slack.com/files/U123/F002/document.pdf",
				"",
			),
		}

		msg := slack.NewMessageFromData(
			fmt.Sprintf("%d.000001", now.Unix()),
			channelID,
			"",
			teamID,
			userID,
			"user1",
			"Message with files",
			fmt.Sprintf("%d.000001", now.Unix()),
			now,
			files,
		)

		gt.NoError(t, repo.Slack().PutMessage(ctx, msg)).Required()

		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1).Required()

		resultFiles := messages[0].Files()
		gt.Array(t, resultFiles).Length(2)

		gt.Value(t, resultFiles[0].ID()).Equal("F001")
		gt.Value(t, resultFiles[0].Name()).Equal("screenshot.png")
		gt.Value(t, resultFiles[0].Mimetype()).Equal("image/png")
		gt.Value(t, resultFiles[0].Filetype()).Equal("png")
		gt.Value(t, resultFiles[0].Size()).Equal(102400)
		gt.Value(t, resultFiles[0].URLPrivate()).Equal("https://files.slack.com/files-pri/T123-F001/screenshot.png")
		gt.Value(t, resultFiles[0].Permalink()).Equal("https://workspace.slack.com/files/U123/F001/screenshot.png")
		gt.Value(t, resultFiles[0].ThumbURL()).Equal("https://files.slack.com/thumb_480.png")

		gt.Value(t, resultFiles[1].ID()).Equal("F002")
		gt.Value(t, resultFiles[1].Name()).Equal("document.pdf")
		gt.Value(t, resultFiles[1].ThumbURL()).Equal("")
	})

	t.Run("PutMessage and ListMessages without files (backward compatibility)", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		channelID := fmt.Sprintf("C%d", now.UnixNano())

		msg := slack.NewMessageFromData(
			fmt.Sprintf("%d.000001", now.Unix()),
			channelID,
			"",
			fmt.Sprintf("T%d", now.UnixNano()),
			fmt.Sprintf("U%d", now.UnixNano()),
			"user1",
			"Message without files",
			fmt.Sprintf("%d.000001", now.Unix()),
			now,
			nil,
		)

		gt.NoError(t, repo.Slack().PutMessage(ctx, msg)).Required()

		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, messages).Length(1).Required()

		gt.Array(t, messages[0].Files()).Length(0)
	})

	t.Run("ListMessages returns empty for non-existent channel", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		now := time.Now()

		// Use non-existent channel
		channelID := fmt.Sprintf("C%d", now.UnixNano())

		messages, nextCursor, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)

		// Should not error, just return empty list
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(0)

		gt.Value(t, nextCursor).Equal("")
	})
}

func TestMemorySlackRepository(t *testing.T) {
	runSlackRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func newFirestoreSlackRepository(t *testing.T) interfaces.Repository {
	t.Helper()

	projectID := os.Getenv("TEST_FIRESTORE_PROJECT_ID")
	if projectID == "" {
		t.Skip("TEST_FIRESTORE_PROJECT_ID not set")
	}

	databaseID := os.Getenv("TEST_FIRESTORE_DATABASE_ID")
	if databaseID == "" {
		t.Skip("TEST_FIRESTORE_DATABASE_ID not set")
	}

	ctx := context.Background()
	repo, err := firestore.New(ctx, projectID, databaseID)
	gt.NoError(t, err).Required()
	t.Cleanup(func() {
		gt.NoError(t, repo.Close())
	})
	return repo
}

func TestFirestoreSlackRepository(t *testing.T) {
	runSlackRepositoryTest(t, newFirestoreSlackRepository)
}
