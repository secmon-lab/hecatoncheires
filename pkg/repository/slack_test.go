package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/secmon-lab/hecatoncheires/pkg/domain/interfaces"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
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
		)

		// Put messages
		if err := repo.Slack().PutMessage(ctx, msg1); err != nil {
			t.Fatalf("failed to put message1: %v", err)
		}

		if err := repo.Slack().PutMessage(ctx, msg2); err != nil {
			t.Fatalf("failed to put message2: %v", err)
		}

		if err := repo.Slack().PutMessage(ctx, msg3); err != nil {
			t.Fatalf("failed to put message3: %v", err)
		}

		// List all messages
		messages, nextCursor, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-3*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 3 {
			t.Errorf("expected 3 messages, got %d", len(messages))
		}

		// Messages should be in descending order (newest first)
		if len(messages) >= 3 {
			if messages[0].ID() != msg3.ID() {
				t.Errorf("expected first message ID to be %q, got %q", msg3.ID(), messages[0].ID())
			}
			if messages[1].ID() != msg2.ID() {
				t.Errorf("expected second message ID to be %q, got %q", msg2.ID(), messages[1].ID())
			}
			if messages[2].ID() != msg1.ID() {
				t.Errorf("expected third message ID to be %q, got %q", msg1.ID(), messages[2].ID())
			}

			// Verify all fields
			if messages[0].ChannelID() != channelID {
				t.Errorf("expected ChannelID to be %q, got %q", channelID, messages[0].ChannelID())
			}
			if messages[0].TeamID() != teamID {
				t.Errorf("expected TeamID to be %q, got %q", teamID, messages[0].TeamID())
			}
			if messages[0].UserID() != userID {
				t.Errorf("expected UserID to be %q, got %q", userID, messages[0].UserID())
			}
			if messages[0].Text() != "Third message" {
				t.Errorf("expected Text to be %q, got %q", "Third message", messages[0].Text())
			}
		}

		if nextCursor != "" {
			t.Errorf("expected no next cursor, got %q", nextCursor)
		}
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
		)

		if err := repo.Slack().PutMessage(ctx, msg1); err != nil {
			t.Fatalf("failed to put initial message: %v", err)
		}

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
		)

		if err := repo.Slack().PutMessage(ctx, msg2); err != nil {
			t.Fatalf("failed to update message: %v", err)
		}

		// List messages - should only have one
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 1 {
			t.Errorf("expected 1 message after upsert, got %d", len(messages))
		}

		if len(messages) == 1 {
			if messages[0].Text() != "Updated text" {
				t.Errorf("expected text to be updated to %q, got %q", "Updated text", messages[0].Text())
			}
		}
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
			)
			if err := repo.Slack().PutMessage(ctx, msg); err != nil {
				t.Fatalf("failed to put message %d: %v", i, err)
			}
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
		if err != nil {
			t.Fatalf("failed to list first page: %v", err)
		}

		if len(messages) != 2 {
			t.Errorf("expected 2 messages in first page, got %d", len(messages))
		}

		if nextCursor == "" {
			t.Error("expected next cursor, got empty string")
		}

		// Get next page
		messages2, nextCursor2, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-1*time.Hour),
			now.Add(1*time.Hour),
			2,
			nextCursor,
		)
		if err != nil {
			t.Fatalf("failed to list second page: %v", err)
		}

		if len(messages2) != 2 {
			t.Errorf("expected 2 messages in second page, got %d", len(messages2))
		}

		// Still one more message remaining (5 total, 2 per page, 1 left after page 2)
		if nextCursor2 == "" {
			t.Error("expected next cursor for second page, got empty string")
		}

		// Verify no duplicate messages
		if len(messages) > 0 && len(messages2) > 0 {
			if messages[len(messages)-1].ID() == messages2[0].ID() {
				t.Error("found duplicate message between pages")
			}
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
		)

		if err := repo.Slack().PutMessage(ctx, oldMsg); err != nil {
			t.Fatalf("failed to put old message: %v", err)
		}

		if err := repo.Slack().PutMessage(ctx, newMsg); err != nil {
			t.Fatalf("failed to put new message: %v", err)
		}

		// Prune messages older than 1 hour ago
		deleted, err := repo.Slack().PruneMessages(ctx, channelID, now.Add(-1*time.Hour))
		if err != nil {
			t.Fatalf("failed to prune messages: %v", err)
		}

		if deleted != 1 {
			t.Errorf("expected 1 message to be deleted, got %d", deleted)
		}

		// Verify only new message remains
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			channelID,
			now.Add(-3*time.Hour),
			now.Add(1*time.Hour),
			10,
			"",
		)
		if err != nil {
			t.Fatalf("failed to list messages: %v", err)
		}

		if len(messages) != 1 {
			t.Errorf("expected 1 message to remain, got %d", len(messages))
		}

		if len(messages) == 1 && messages[0].ID() != newMsg.ID() {
			t.Errorf("expected remaining message ID to be %q, got %q", newMsg.ID(), messages[0].ID())
		}
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
		if err != nil {
			t.Fatalf("expected no error for empty channel, got: %v", err)
		}

		if len(messages) != 0 {
			t.Errorf("expected 0 messages for empty channel, got %d", len(messages))
		}

		if nextCursor != "" {
			t.Errorf("expected no cursor for empty channel, got %q", nextCursor)
		}
	})
}

func TestMemorySlackRepository(t *testing.T) {
	runSlackRepositoryTest(t, func(t *testing.T) interfaces.Repository {
		return memory.New()
	})
}

func TestFirestoreSlackRepository(t *testing.T) {
	runSlackRepositoryTest(t, newFirestoreRepository)
}
