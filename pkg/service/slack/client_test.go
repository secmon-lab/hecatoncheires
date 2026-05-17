package slack_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	goslack "github.com/slack-go/slack"
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
	channels, err := svc.ListJoinedChannels(ctx, "")
	gt.NoError(t, err).Required()

	users, err := svc.ListUsers(ctx, "")
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

func TestWrapSlackViewError(t *testing.T) {
	t.Run("plain error only carries the trigger_id", func(t *testing.T) {
		base := errors.New("network blew up")
		wrapped := slack.WrapSlackViewErrorForTest(base, "failed to open Slack modal view", "trig-1")

		var ge *goerr.Error
		gt.Bool(t, errors.As(wrapped, &ge)).True()
		values := ge.Values()
		gt.Value(t, values["trigger_id"]).Equal("trig-1")
		// Plain errors must NOT pretend to surface Slack metadata.
		_, hasSlackError := values["slack_error"]
		gt.Bool(t, hasSlackError).False()
	})

	t.Run("SlackErrorResponse surfaces response metadata", func(t *testing.T) {
		errMsg := "validation error on element initial_value"
		se := goslack.SlackErrorResponse{
			Err: "invalid_arguments",
			Errors: []goslack.SlackResponseErrors{
				{Message: &errMsg},
			},
			ResponseMetadata: goslack.ResponseMetadata{
				Messages: []string{"[ERROR] failed to match schema [json-pointer:/view/blocks/1/element/initial_value]"},
				Warnings: []string{"deprecated_block_kit"},
			},
		}
		wrapped := slack.WrapSlackViewErrorForTest(fmt.Errorf("openView: %w", se), "failed to open Slack modal view", "trig-2")

		var ge *goerr.Error
		gt.Bool(t, errors.As(wrapped, &ge)).True()
		values := ge.Values()
		gt.Value(t, values["trigger_id"]).Equal("trig-2")
		gt.Value(t, values["slack_error"]).Equal("invalid_arguments")

		msgs, ok := values["slack_response_messages"].([]string)
		gt.Bool(t, ok).True()
		gt.Array(t, msgs).Length(1).Required()
		gt.String(t, msgs[0]).Contains("json-pointer:/view/blocks/1/element/initial_value")

		warns, ok := values["slack_response_warnings"].([]string)
		gt.Bool(t, ok).True()
		gt.Array(t, warns).Length(1).Required()
		gt.String(t, warns[0]).Equal("deprecated_block_kit")

		// The Errors slice itself is surfaced so the per-failure detail
		// (e.g. apps.manifest field paths) reaches Sentry without a replay.
		errs, ok := values["slack_response_errors"].([]goslack.SlackResponseErrors)
		gt.Bool(t, ok).True()
		gt.Array(t, errs).Length(1).Required()
		gt.Value(t, errs[0].Message).NotNil()
		gt.String(t, *errs[0].Message).Equal(errMsg)
	})
}

func TestResolveDisplayName(t *testing.T) {
	t.Run("prefers Profile.DisplayName when present", func(t *testing.T) {
		u := goslack.User{
			Name:     "alice",
			RealName: "Alice The Real",
			Profile: goslack.UserProfile{
				DisplayName: "Alice In Wonderland",
				RealName:    "Alice Profile Real",
			},
		}
		gt.String(t, slack.ResolveDisplayNameForTest(u)).Equal("Alice In Wonderland")
	})

	t.Run("falls back to Profile.RealName when DisplayName is empty", func(t *testing.T) {
		u := goslack.User{
			Name:     "alice",
			RealName: "Alice The Real",
			Profile: goslack.UserProfile{
				DisplayName: "",
				RealName:    "Alice Profile Real",
			},
		}
		gt.String(t, slack.ResolveDisplayNameForTest(u)).Equal("Alice Profile Real")
	})

	t.Run("falls back to top-level RealName when both profile fields are empty", func(t *testing.T) {
		u := goslack.User{
			Name:     "alice",
			RealName: "Alice The Real",
			Profile: goslack.UserProfile{
				DisplayName: "",
				RealName:    "",
			},
		}
		gt.String(t, slack.ResolveDisplayNameForTest(u)).Equal("Alice The Real")
	})

	t.Run("returns empty string when every name field is empty", func(t *testing.T) {
		u := goslack.User{
			Name:     "alice",
			RealName: "",
			Profile: goslack.UserProfile{
				DisplayName: "",
				RealName:    "",
			},
		}
		gt.String(t, slack.ResolveDisplayNameForTest(u)).Equal("")
	})
}
