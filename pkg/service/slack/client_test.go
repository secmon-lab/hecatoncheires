package slack_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
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

// inviteFake captures the user IDs the slack-go client tried to invite and
// returns a configurable per-user response. The fake speaks the
// conversations.invite contract: a JSON body with "ok" and optional
// "error" string. Slack's real endpoint is atomic across the supplied
// user list — we mimic that by failing the entire request if its single
// user_id appears in the failingUsers set.
type inviteFake struct {
	mu            sync.Mutex
	failingUsers  map[string]string // userID -> slack error code
	successfulIDs []string          // user IDs that received an "ok" response
	calls         int
}

func (f *inviteFake) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		users := r.FormValue("users")
		if errCode, bad := f.failingUsers[users]; bad {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": errCode})
			return
		}
		f.successfulIDs = append(f.successfulIDs, users)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": map[string]any{"id": "C1"}})
	}
}

func TestInviteUsersToChannel_BadUserDoesNotBlockValidUsers(t *testing.T) {
	fake := &inviteFake{
		failingUsers: map[string]string{
			"U_BAD": "user_not_found",
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.invite", fake.handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, err := slack.NewWithAPIURLForTest("xoxb-test", srv.URL+"/")
	gt.NoError(t, err).Required()

	err = svc.InviteUsersToChannel(context.Background(), "C_TARGET", []string{"U_GOOD_1", "U_BAD", "U_GOOD_2"})

	// The function must return an error so errutil.Handle on the caller
	// records the failure, but the valid users MUST still have been
	// invited individually.
	gt.Value(t, err).NotNil()

	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	values := ge.Values()
	gt.Value(t, values["channel_id"]).Equal("C_TARGET")

	failed, ok := values["failed_user_ids"].([]string)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, failed).Length(1).Required()
	gt.String(t, failed[0]).Equal("U_BAD")

	reasons, ok := values["failure_reasons"].([]string)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, reasons).Length(1).Required()
	gt.String(t, reasons[0]).Contains("user_not_found")

	// Three invite calls were issued (one per user), not one batch.
	fake.mu.Lock()
	defer fake.mu.Unlock()
	gt.Number(t, fake.calls).Equal(3)
	gt.Array(t, fake.successfulIDs).Length(2).Required()
	gt.String(t, fake.successfulIDs[0]).Equal("U_GOOD_1")
	gt.String(t, fake.successfulIDs[1]).Equal("U_GOOD_2")
}

func TestInviteUsersToChannel_AllSuccess_ReturnsNil(t *testing.T) {
	fake := &inviteFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.invite", fake.handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, err := slack.NewWithAPIURLForTest("xoxb-test", srv.URL+"/")
	gt.NoError(t, err).Required()

	err = svc.InviteUsersToChannel(context.Background(), "C_TARGET", []string{"U_A", "U_B"})
	gt.NoError(t, err)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	gt.Number(t, fake.calls).Equal(2)
}

func TestInviteUsersToChannel_EmptyList_NoApiCall(t *testing.T) {
	fake := &inviteFake{}
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.invite", fake.handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, err := slack.NewWithAPIURLForTest("xoxb-test", srv.URL+"/")
	gt.NoError(t, err).Required()

	err = svc.InviteUsersToChannel(context.Background(), "C_TARGET", nil)
	gt.NoError(t, err)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	gt.Number(t, fake.calls).Equal(0)
}

// formCaptor records, per Slack API path, the form values of the last request
// it received and answers "ok". It lets the unfurl test assert what actually
// went on the wire for each chat.* endpoint.
type formCaptor struct {
	mu     sync.Mutex
	values map[string]url.Values
}

func (c *formCaptor) handler(responseBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		c.mu.Lock()
		if c.values == nil {
			c.values = map[string]url.Values{}
		}
		// r.Form is repopulated per request; copy the path's values out.
		c.values[r.URL.Path] = r.Form
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	}
}

func (c *formCaptor) get(path string) url.Values {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[path]
}

// TestBotPostsDisableUnfurl pins the invariant that every bot-originated post /
// update / ephemeral message suppresses Slack link & media unfurling. The bot
// embeds permalinks and URLs everywhere; preview cards would bury the content.
func TestBotPostsDisableUnfurl(t *testing.T) {
	captor := &formCaptor{}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat.postMessage", captor.handler(`{"ok":true,"channel":"C","ts":"1700000000.000100"}`))
	mux.HandleFunc("/chat.update", captor.handler(`{"ok":true,"channel":"C","ts":"1700000000.000100"}`))
	mux.HandleFunc("/chat.postEphemeral", captor.handler(`{"ok":true,"message_ts":"1700000000.000100"}`))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc, err := slack.NewWithAPIURLForTest("xoxb-test", srv.URL+"/")
	gt.NoError(t, err).Required()

	ctx := context.Background()
	blocks := []goslack.Block{goslack.NewSectionBlock(goslack.NewTextBlockObject(goslack.MarkdownType, "hello", false, false), nil, nil)}
	att := goslack.Attachment{Text: "att"}

	assertUnfurlDisabled := func(t *testing.T, path string) {
		t.Helper()
		vals := captor.get(path)
		gt.Value(t, vals).NotNil().Required()
		gt.String(t, vals.Get("unfurl_links")).Equal("false")
		gt.String(t, vals.Get("unfurl_media")).Equal("false")
	}

	t.Run("PostMessage", func(t *testing.T) {
		_, err := svc.PostMessage(ctx, "C", blocks, "fallback")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postMessage")
	})

	t.Run("PostThreadReply", func(t *testing.T) {
		_, err := svc.PostThreadReply(ctx, "C", "1700000000.000001", "reply")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postMessage")
	})

	t.Run("PostThreadMessage", func(t *testing.T) {
		_, err := svc.PostThreadMessage(ctx, "C", "1700000000.000001", blocks, "final")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postMessage")
	})

	t.Run("PostMessageWithAttachments", func(t *testing.T) {
		_, err := svc.PostMessageWithAttachments(ctx, "C", "fallback", []goslack.Attachment{att})
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postMessage")
	})

	t.Run("PostMessageWithAttachment", func(t *testing.T) {
		_, err := svc.PostMessageWithAttachment(ctx, "C", "fallback", att)
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postMessage")
	})

	t.Run("UpdateMessage", func(t *testing.T) {
		err := svc.UpdateMessage(ctx, "C", "1700000000.000100", blocks, "")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.update")
	})

	t.Run("UpdateMessageWithAttachments", func(t *testing.T) {
		err := svc.UpdateMessageWithAttachments(ctx, "C", "1700000000.000100", "", []goslack.Attachment{att})
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.update")
	})

	t.Run("PostEphemeral", func(t *testing.T) {
		err := svc.PostEphemeral(ctx, "C", "U", "ephemeral")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postEphemeral")
	})

	t.Run("PostEphemeralBlocks", func(t *testing.T) {
		_, err := svc.PostEphemeralBlocks(ctx, "C", "U", blocks, "ephemeral")
		gt.NoError(t, err).Required()
		assertUnfurlDisabled(t, "/chat.postEphemeral")
	})
}
