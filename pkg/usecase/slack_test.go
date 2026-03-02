package usecase_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/model/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/memory"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase"
	slackevents "github.com/slack-go/slack/slackevents"
)

func TestSlackUseCases_HandleSlackEvent(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	t.Run("handles message event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.Message),
				Data: &slackevents.MessageEvent{
					Type:           "message",
					User:           "U123",
					Text:           "Hello, world!",
					TimeStamp:      "1234567890.123456",
					Channel:        "C123",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)

		if len(messages) > 0 {
			msg := messages[0]
			gt.Value(t, msg.UserID()).Equal("U123")
			gt.Value(t, msg.Text()).Equal("Hello, world!")
		}
	})

	t.Run("handles app_mention event", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: string(slackevents.AppMention),
				Data: &slackevents.AppMentionEvent{
					Type:           "app_mention",
					User:           "U456",
					Text:           "@bot help",
					TimeStamp:      "1234567890.654321",
					Channel:        "C456",
					EventTimeStamp: "1234567890",
				},
			},
			TeamID: "T456",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C456",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1)
	})

	t.Run("ignores unsupported event types", func(t *testing.T) {
		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "some_other_event",
				Data: map[string]interface{}{
					"type": "some_other_event",
				},
			},
			TeamID: "T789",
		}

		// Should not error, just skip
		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})
}

func TestSlackUseCases_HandleSlackMessage(t *testing.T) {
	t.Run("stores message successfully", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"1234567890.123456",
			"C123",
			"",
			"T123",
			"U123",
			"testuser",
			"Test message",
			"1234567890.123456",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was stored
		messages, _, err := repo.Slack().ListMessages(
			ctx,
			"C123",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()

		gt.Array(t, messages).Length(1).Required()

		gt.Value(t, messages[0].ID()).Equal("1234567890.123456")
	})

	t.Run("returns error for nil message", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		err := uc.Slack.HandleSlackMessage(ctx, nil)
		gt.Value(t, err).NotNil()
	})

	t.Run("saves to case sub-collection when channel is mapped", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		// Create a case with SlackChannelID
		created, err := repo.Case().Create(ctx, "ws-test", &model.Case{
			Title:          "Test Case",
			SlackChannelID: "C-MAPPED",
		})
		gt.NoError(t, err).Required()

		// Set up registry with the workspace
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-test", Name: "Test"},
		})

		uc := usecase.New(repo, registry)

		msg := slack.NewMessageFromData(
			"mapped-msg-001",
			"C-MAPPED",
			"",
			"T123",
			"U123",
			"alice",
			"Hello from mapped channel",
			"ev1",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-MAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)

		// Verify message was also saved to case sub-collection
		caseMsgs, _, err := repo.CaseMessage().List(ctx, "ws-test", created.ID, 10, "")
		gt.NoError(t, err).Required()
		gt.Array(t, caseMsgs).Length(1)
		gt.Value(t, caseMsgs[0].Text()).Equal("Hello from mapped channel")
	})

	t.Run("does not save to case sub-collection when channel is not mapped", func(t *testing.T) {
		repo := memory.New()
		uc := usecase.New(repo, nil)
		ctx := context.Background()

		msg := slack.NewMessageFromData(
			"unmapped-msg-001",
			"C-UNMAPPED",
			"",
			"T123",
			"U123",
			"bob",
			"Hello from unmapped channel",
			"ev1",
			time.Now(),
			nil,
		)

		gt.NoError(t, uc.Slack.HandleSlackMessage(ctx, msg)).Required()

		// Verify message was saved to channel-level collection
		channelMsgs, _, err := repo.Slack().ListMessages(
			ctx,
			"C-UNMAPPED",
			time.Now().Add(-1*time.Hour),
			time.Now().Add(1*time.Hour),
			10,
			"",
		)
		gt.NoError(t, err).Required()
		gt.Array(t, channelMsgs).Length(1)
	})
}

func TestSlackUseCases_HandleMembershipEvent(t *testing.T) {
	t.Run("member_joined_channel syncs channel members to case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-%d", time.Now().UnixNano())

		// Create a case with a Slack channel
		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:          "Membership Test Case",
			SlackChannelID: "C-MEMBERSHIP-JOIN",
		})
		gt.NoError(t, err).Required()

		// Seed SlackUser cache so filterHumanUsers can work
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U100", Name: "alice"},
			{ID: "U200", Name: "bob"},
		})).Required()

		// Set up registry
		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		// Mock slack service that returns channel members
		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				if channelID == "C-MEMBERSHIP-JOIN" {
					return []string{"U100", "U200", "UBOT999"}, nil // UBOT999 is not in SlackUser cache
				}
				return nil, nil
			},
		}

		uc := usecase.New(repo, registry, usecase.WithSlackService(slackSvc))

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-MEMBERSHIP-JOIN",
					User:    "U200",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify case was updated with filtered human user IDs
		updated, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.ChannelUserIDs).Length(2) // U100, U200 (UBOT999 filtered out)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U100")).Equal(true)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U200")).Equal(true)
	})

	t.Run("member_left_channel syncs channel members to case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-left-%d", time.Now().UnixNano())

		// Create a case with a Slack channel and initial members
		created, err := repo.Case().Create(ctx, wsID, &model.Case{
			Title:          "Membership Leave Test",
			SlackChannelID: "C-MEMBERSHIP-LEFT",
			ChannelUserIDs: []string{"U100", "U200"},
		})
		gt.NoError(t, err).Required()

		// Seed SlackUser cache
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U100", Name: "alice"},
		})).Required()

		// After U200 leaves, only U100 remains in the channel
		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				return []string{"U100"}, nil
			},
		}

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		uc := usecase.New(repo, registry, usecase.WithSlackService(slackSvc))

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_left_channel",
				Data: &slackevents.MemberLeftChannelEvent{
					Channel: "C-MEMBERSHIP-LEFT",
					User:    "U200",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()

		// Verify case now only has U100
		updated, err := repo.Case().Get(ctx, wsID, created.ID)
		gt.NoError(t, err).Required()
		gt.Array(t, updated.ChannelUserIDs).Length(1)
		gt.Value(t, slices.Contains(updated.ChannelUserIDs, "U100")).Equal(true)
	})

	t.Run("no-op when channel has no associated case", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()
		wsID := fmt.Sprintf("ws-membership-noop-%d", time.Now().UnixNano())

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: wsID, Name: "Test"},
		})

		slackSvc := &mockSlackService{
			getConversationMembersFn: func(ctx context.Context, channelID string) ([]string, error) {
				t.Error("GetConversationMembers should not be called for unrelated channel")
				return nil, nil
			},
		}

		uc := usecase.New(repo, registry, usecase.WithSlackService(slackSvc))

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-UNRELATED",
					User:    "U999",
				},
			},
			TeamID: "T123",
		}

		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})

	t.Run("no-op when slack service is nil", func(t *testing.T) {
		repo := memory.New()
		ctx := context.Background()

		registry := model.NewWorkspaceRegistry()
		registry.Register(&model.WorkspaceEntry{
			Workspace: model.Workspace{ID: "ws-nil-slack", Name: "Test"},
		})

		// No WithSlackService option => slackService is nil
		uc := usecase.New(repo, registry)

		event := &slackevents.EventsAPIEvent{
			Type: slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{
				Type: "member_joined_channel",
				Data: &slackevents.MemberJoinedChannelEvent{
					Channel: "C-ANY",
					User:    "U100",
				},
			},
			TeamID: "T123",
		}

		// Should not error even without slack service
		gt.NoError(t, uc.Slack.HandleSlackEvent(ctx, event)).Required()
	})
}

func TestSlackUseCases_CleanupOldMessages(t *testing.T) {
	repo := memory.New()
	uc := usecase.New(repo, nil)
	ctx := context.Background()

	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	newTime := now.Add(-1 * time.Hour)

	// Create old and new messages
	oldMsg := slack.NewMessageFromData(
		"old.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"Old message",
		"old.123456",
		oldTime,
		nil,
	)

	newMsg := slack.NewMessageFromData(
		"new.123456",
		"C123",
		"",
		"T123",
		"U123",
		"testuser",
		"New message",
		"new.123456",
		newTime,
		nil,
	)

	gt.NoError(t, repo.Slack().PutMessage(ctx, oldMsg)).Required()
	gt.NoError(t, repo.Slack().PutMessage(ctx, newMsg)).Required()

	// Cleanup messages older than 24 hours
	cutoffTime := now.Add(-24 * time.Hour)
	gt.NoError(t, uc.Slack.CleanupOldMessages(ctx, cutoffTime)).Required()

	// Verify only new message remains
	messages, _, err := repo.Slack().ListMessages(
		ctx,
		"C123",
		time.Time{},
		now.Add(1*time.Hour),
		10,
		"",
	)
	gt.NoError(t, err).Required()

	gt.Array(t, messages).Length(1).Required()

	gt.Value(t, messages[0].ID()).Equal("new.123456")
}
