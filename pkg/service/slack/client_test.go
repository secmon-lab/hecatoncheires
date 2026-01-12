package slack_test

import (
	"context"
	"os"
	"testing"

	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

func TestNew(t *testing.T) {
	t.Run("returns error when token is empty", func(t *testing.T) {
		_, err := slack.New("")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("creates service when token is provided", func(t *testing.T) {
		svc, err := slack.New("test-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc == nil {
			t.Fatal("expected service, got nil")
		}
	})
}

func TestIntegration(t *testing.T) {
	token := os.Getenv("TEST_SLACK_BOT_TOKEN")
	if token == "" {
		t.Skip("TEST_SLACK_BOT_TOKEN is not set")
	}

	ctx := context.Background()

	svc, err := slack.New(token)
	if err != nil {
		t.Fatalf("failed to create slack service: %v", err)
	}

	t.Run("ListJoinedChannels returns channels", func(t *testing.T) {
		channels, err := svc.ListJoinedChannels(ctx)
		if err != nil {
			t.Fatalf("ListJoinedChannels failed: %v", err)
		}

		// Bot should be in at least one channel
		if len(channels) == 0 {
			t.Log("Warning: bot is not joined to any channels")
		}

		for _, ch := range channels {
			if ch.ID == "" {
				t.Error("channel ID should not be empty")
			}
			if ch.Name == "" {
				t.Error("channel Name should not be empty")
			}
			t.Logf("Found channel: %s (%s)", ch.Name, ch.ID)
		}
	})

	t.Run("GetChannelNames resolves channel names", func(t *testing.T) {
		// First, get some channels to test with
		channels, err := svc.ListJoinedChannels(ctx)
		if err != nil {
			t.Fatalf("ListJoinedChannels failed: %v", err)
		}

		if len(channels) == 0 {
			t.Skip("No channels available to test GetChannelNames")
		}

		// Test with the first channel
		channelIDs := []string{channels[0].ID}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		if err != nil {
			t.Fatalf("GetChannelNames failed: %v", err)
		}

		if len(names) != 1 {
			t.Errorf("expected 1 name, got %d", len(names))
		}

		name, ok := names[channels[0].ID]
		if !ok {
			t.Errorf("channel ID %s not found in result", channels[0].ID)
		}
		if name == "" {
			t.Error("channel name should not be empty")
		}
		if name != channels[0].Name {
			t.Errorf("expected name %s, got %s", channels[0].Name, name)
		}

		t.Logf("Resolved channel name: %s -> %s", channels[0].ID, name)
	})

	t.Run("GetChannelNames handles multiple channels", func(t *testing.T) {
		channels, err := svc.ListJoinedChannels(ctx)
		if err != nil {
			t.Fatalf("ListJoinedChannels failed: %v", err)
		}

		if len(channels) < 2 {
			t.Skip("Need at least 2 channels to test multiple channel resolution")
		}

		channelIDs := []string{channels[0].ID, channels[1].ID}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		if err != nil {
			t.Fatalf("GetChannelNames failed: %v", err)
		}

		if len(names) != 2 {
			t.Errorf("expected 2 names, got %d", len(names))
		}

		for _, ch := range channels[:2] {
			name, ok := names[ch.ID]
			if !ok {
				t.Errorf("channel ID %s not found in result", ch.ID)
			}
			if name != ch.Name {
				t.Errorf("expected name %s for %s, got %s", ch.Name, ch.ID, name)
			}
		}
	})

	t.Run("GetChannelNames handles non-existent channel gracefully", func(t *testing.T) {
		// Use a fake channel ID that doesn't exist
		channelIDs := []string{"C00000FAKE"}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		// This may or may not error depending on API behavior
		// The important thing is it doesn't panic
		t.Logf("GetChannelNames with fake ID: names=%v, err=%v", names, err)
	})

	t.Run("GetChannelNames with empty slice returns empty map", func(t *testing.T) {
		names, err := svc.GetChannelNames(ctx, []string{})
		if err != nil {
			t.Fatalf("GetChannelNames failed: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("expected empty map, got %d entries", len(names))
		}
	})
}
