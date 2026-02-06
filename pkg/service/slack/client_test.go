package slack_test

import (
	"context"
	"os"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

func TestNew(t *testing.T) {
	t.Run("returns error when token is empty", func(t *testing.T) {
		_, err := slack.New("")
		gt.Value(t, err).NotNil()
	})

	t.Run("creates service when token is provided", func(t *testing.T) {
		svc, err := slack.New("test-token")
		gt.NoError(t, err).Required()
		gt.Value(t, svc).NotNil()
	})
}

func TestIntegration(t *testing.T) {
	token := os.Getenv("TEST_SLACK_BOT_TOKEN")
	if token == "" {
		t.Skip("TEST_SLACK_BOT_TOKEN is not set")
	}

	ctx := context.Background()

	svc, err := slack.New(token)
	gt.NoError(t, err).Required()

	// Fetch channels and users once to avoid repeated API calls and rate limiting
	channels, err := svc.ListJoinedChannels(ctx)
	gt.NoError(t, err).Required()

	users, err := svc.ListUsers(ctx)
	gt.NoError(t, err).Required()

	t.Run("ListJoinedChannels returns channels", func(t *testing.T) {
		if len(channels) == 0 {
			t.Log("Warning: bot is not joined to any channels")
		}

		for _, ch := range channels {
			gt.String(t, ch.ID).NotEqual("")
			gt.String(t, ch.Name).NotEqual("")
			t.Logf("Found channel: %s (%s)", ch.Name, ch.ID)
		}
	})

	t.Run("GetChannelNames resolves channel names", func(t *testing.T) {
		if len(channels) == 0 {
			t.Skip("No channels available to test GetChannelNames")
		}

		channelIDs := []string{channels[0].ID}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		gt.NoError(t, err).Required()

		gt.Map(t, names).HasKey(channels[0].ID)

		name := names[channels[0].ID]
		gt.String(t, name).NotEqual("")
		gt.Value(t, name).Equal(channels[0].Name)

		t.Logf("Resolved channel name: %s -> %s", channels[0].ID, name)
	})

	t.Run("GetChannelNames handles multiple channels", func(t *testing.T) {
		if len(channels) < 2 {
			t.Skip("Need at least 2 channels to test multiple channel resolution")
		}

		channelIDs := []string{channels[0].ID, channels[1].ID}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		gt.NoError(t, err).Required()

		gt.Number(t, len(names)).Equal(2)

		for _, ch := range channels[:2] {
			gt.Map(t, names).HasKey(ch.ID)
			gt.Value(t, names[ch.ID]).Equal(ch.Name)
		}
	})

	t.Run("GetChannelNames handles non-existent channel gracefully", func(t *testing.T) {
		channelIDs := []string{"C00000FAKE"}
		names, err := svc.GetChannelNames(ctx, channelIDs)
		// This may or may not error depending on API behavior
		// The important thing is it doesn't panic
		t.Logf("GetChannelNames with fake ID: names=%v, err=%v", names, err)
	})

	t.Run("GetChannelNames with empty slice returns empty map", func(t *testing.T) {
		names, err := svc.GetChannelNames(ctx, []string{})
		gt.NoError(t, err).Required()
		gt.Number(t, len(names)).Equal(0)
	})

	t.Run("ListUsers returns users", func(t *testing.T) {
		gt.Number(t, len(users)).GreaterOrEqual(1)

		for _, u := range users {
			gt.String(t, u.ID).NotEqual("")
		}

		t.Logf("Total users retrieved: %d", len(users))
	})

	t.Run("GetUserInfo returns user info", func(t *testing.T) {
		if len(users) == 0 {
			t.Skip("No users available to test GetUserInfo")
		}

		user, err := svc.GetUserInfo(ctx, users[0].ID)
		gt.NoError(t, err).Required()

		gt.Value(t, user.ID).Equal(users[0].ID)
		t.Logf("Got user info: %s (%s)", user.RealName, user.ID)
	})
}
