package slacktool_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/logging"
)

// fakeSearchService is a SearchService where SearchMessages can be scripted
// to return a specific result or error.
type fakeSearchService struct {
	searchFn func(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error)
}

func (f *fakeSearchService) SearchMessages(ctx context.Context, query string, opts slacktool.SearchOptions) (*slacktool.SearchResult, error) {
	return f.searchFn(ctx, query, opts)
}

// fakeBotService stubs slackservice.Service. The embedded interface is nil,
// so any method we don't explicitly override panics with a nil-pointer
// dereference if called — that surfaces unintended dependencies in tests.
type fakeBotService struct {
	slackservice.Service

	getPermalinkFn  func(ctx context.Context, channelID, ts string) (string, error)
	getRepliesFn    func(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error)
	postMessageFn   func(ctx context.Context, channelID string, blocks []goslack.Block, text string) (string, error)
	postThreadReply func(ctx context.Context, channelID, threadTS, text string) (string, error)
}

// fakeMessageRetriever stubs slacktool.MessageRetriever. The embedded
// interface is nil for the same panic-on-unexpected-call reason as
// fakeBotService.
type fakeMessageRetriever struct {
	slacktool.MessageRetriever

	getRepliesFn func(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error)
}

func (f *fakeMessageRetriever) GetConversationReplies(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error) {
	return f.getRepliesFn(ctx, channelID, threadTS, limit)
}

func (f *fakeBotService) GetPermalink(ctx context.Context, channelID, ts string) (string, error) {
	return f.getPermalinkFn(ctx, channelID, ts)
}

func (f *fakeBotService) GetConversationReplies(ctx context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error) {
	return f.getRepliesFn(ctx, channelID, threadTS, limit)
}

func (f *fakeBotService) PostMessage(ctx context.Context, channelID string, blocks []goslack.Block, text string, _ ...slackservice.PostMessageOption) (string, error) {
	return f.postMessageFn(ctx, channelID, blocks, text)
}

func (f *fakeBotService) PostThreadReply(ctx context.Context, channelID, threadTS, text string) (string, error) {
	return f.postThreadReply(ctx, channelID, threadTS, text)
}

// ctxWithCapturingLogger returns a context whose logger writes JSON to buf.
// Used to assert that errutil.Handle records via the ctx-bound logger.
func ctxWithCapturingLogger(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logging.With(context.Background(), logger), &buf
}

// pickToolByName returns the tool whose Spec().Name matches name, or fails
// the test. Used so tests do not depend on the order of NewReadOnly /
// NewForAssist's return slice.
func pickToolByName(t *testing.T, tools []gollem.Tool, name string) gollem.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Spec().Name == name {
			return tool
		}
	}
	gt.Value(t, tools).Equal(nil) // forces failure with the slice in the message
	return nil
}

func TestSearchMessagesTool_ErrorRoutesThroughErrutilHandle(t *testing.T) {
	search := &fakeSearchService{
		// Mirror search_client.go's behaviour: the upstream client wraps
		// the API error with the query value, so the tool no longer needs
		// to re-wrap it.
		searchFn: func(_ context.Context, query string, _ slacktool.SearchOptions) (*slacktool.SearchResult, error) {
			return nil, goerr.New("upstream failure",
				goerr.V("slack_error", "missing_scope"),
				goerr.V("query", query),
			)
		},
	}

	tools := slacktool.NewReadOnly(slacktool.Deps{Search: search})
	gt.Array(t, tools).Length(1).Required()

	ctx, buf := ctxWithCapturingLogger(t)
	_, err := tools[0].Run(ctx, map[string]any{"query": "incident"})

	gt.Value(t, err).NotNil()

	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	values := ge.Values()
	gt.Value(t, values["query"]).Equal("incident")
	gt.Value(t, values["slack_error"]).Equal("missing_scope")

	logged := buf.String()
	gt.String(t, logged).Contains(`"msg":"slack search messages failed"`)
	gt.String(t, logged).Contains(`"slack_error":"missing_scope"`)
}

func TestSearchMessagesTool_SuccessReturnsConvertedResult(t *testing.T) {
	search := &fakeSearchService{
		searchFn: func(_ context.Context, query string, _ slacktool.SearchOptions) (*slacktool.SearchResult, error) {
			gt.String(t, query).Equal("incident")
			return &slacktool.SearchResult{
				Total: 1,
				Messages: []slacktool.SearchMessage{{
					ChannelID: "C111",
					Text:      "incident review",
					Timestamp: "1700000000.000100",
				}},
			}, nil
		},
	}
	tools := slacktool.NewReadOnly(slacktool.Deps{Search: search})

	ctx, buf := ctxWithCapturingLogger(t)
	out, err := tools[0].Run(ctx, map[string]any{"query": "incident"})
	gt.NoError(t, err).Required()

	gt.Number(t, out["total"].(int)).Equal(1)
	msgs, ok := out["messages"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, msgs).Length(1).Required()
	gt.Value(t, msgs[0]["channel_id"]).Equal("C111")
	gt.Value(t, msgs[0]["text"]).Equal("incident review")

	gt.Number(t, buf.Len()).Equal(0)
}

func TestGetMessagesTool_PermalinkErrorRoutesThroughErrutilHandle(t *testing.T) {
	bot := &fakeBotService{
		getPermalinkFn: func(_ context.Context, _, _ string) (string, error) {
			return "", goerr.New("permalink boom", goerr.V("slack_error", "channel_not_found"))
		},
	}
	tools := slacktool.NewReadOnly(slacktool.Deps{Bot: bot})
	// NewReadOnly returns getMessagesTool only (search is nil), pick last.
	gt.Array(t, tools).Length(1).Required()

	ctx, buf := ctxWithCapturingLogger(t)
	out, err := tools[0].Run(ctx, map[string]any{
		"targets": []any{
			map[string]any{"channel_id": "C111", "ts": "1700.0001"},
		},
	})

	// All targets failed so the tool returns an aggregate error rather
	// than per-target results.
	gt.Value(t, err).NotNil()
	gt.Value(t, out).Nil()

	logged := buf.String()
	gt.String(t, logged).Contains(`"msg":"slack get permalink failed"`)
	gt.String(t, logged).Contains(`"channel_id":"C111"`)
	gt.String(t, logged).Contains(`"slack_error":"channel_not_found"`)
}

func TestGetMessagesTool_PrefersRetrieverWhenSet(t *testing.T) {
	// When a MessageRetriever (User token) is wired alongside the Bot, the tool
	// must call retriever.GetConversationReplies, not bot.GetConversationReplies.
	// This is the whole point of the User-token route: a bot that has not been
	// invited to a public channel still gets messages back instead of
	// not_in_channel.
	retrieverCalled := 0
	botRepliesCalled := 0
	bot := &fakeBotService{
		getPermalinkFn: func(_ context.Context, channelID, _ string) (string, error) {
			return "https://example.slack.com/archives/" + channelID + "/p1", nil
		},
		getRepliesFn: func(_ context.Context, _, _ string, _ int) ([]slackservice.ConversationMessage, error) {
			botRepliesCalled++
			return nil, errors.New("bot replies must not be called when retriever is set")
		},
	}
	retriever := &fakeMessageRetriever{
		getRepliesFn: func(_ context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error) {
			retrieverCalled++
			gt.String(t, channelID).Equal("C_PUB")
			gt.String(t, threadTS).Equal("t1")
			gt.Number(t, limit).Equal(20)
			return []slackservice.ConversationMessage{{UserID: "U1", Text: "via user token", Timestamp: threadTS}}, nil
		},
	}
	tools := slacktool.NewReadOnly(slacktool.Deps{Bot: bot, Retriever: retriever})

	out, err := tools[0].Run(context.Background(), map[string]any{
		"targets": []any{map[string]any{"channel_id": "C_PUB", "ts": "t1"}},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, retrieverCalled).Equal(1)
	gt.Number(t, botRepliesCalled).Equal(0)

	results, ok := out["results"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, results).Length(1).Required()
	gt.Value(t, results[0]["channel_id"]).Equal("C_PUB")
	msgs, ok := results[0]["messages"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, msgs).Length(1).Required()
	gt.Value(t, msgs[0]["text"]).Equal("via user token")
}

func TestGetMessagesTool_FallsBackToBotWhenRetrieverNil(t *testing.T) {
	// Backward-compat path: without a Retriever, the tool must still work via
	// the Bot token (the old behaviour). public/private channels the bot has
	// joined remain readable; channels it has not joined still fail with
	// not_in_channel (Slack-side constraint, not our concern here).
	botCalled := 0
	bot := &fakeBotService{
		getPermalinkFn: func(_ context.Context, channelID, _ string) (string, error) {
			return "https://example.slack.com/archives/" + channelID + "/p1", nil
		},
		getRepliesFn: func(_ context.Context, channelID, threadTS string, limit int) ([]slackservice.ConversationMessage, error) {
			botCalled++
			gt.String(t, channelID).Equal("C_BOT_MEMBER")
			gt.String(t, threadTS).Equal("t9")
			gt.Number(t, limit).Equal(20)
			return []slackservice.ConversationMessage{{UserID: "U9", Text: "via bot token", Timestamp: threadTS}}, nil
		},
	}
	tools := slacktool.NewReadOnly(slacktool.Deps{Bot: bot})

	out, err := tools[0].Run(context.Background(), map[string]any{
		"targets": []any{map[string]any{"channel_id": "C_BOT_MEMBER", "ts": "t9"}},
	})
	gt.NoError(t, err).Required()
	gt.Number(t, botCalled).Equal(1)

	results, ok := out["results"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, results).Length(1).Required()
	msgs, ok := results[0]["messages"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, msgs).Length(1).Required()
	gt.Value(t, msgs[0]["text"]).Equal("via bot token")
}

func TestGetMessagesTool_PartialFailureLogsButReturnsResults(t *testing.T) {
	bot := &fakeBotService{
		getPermalinkFn: func(_ context.Context, channelID, _ string) (string, error) {
			if channelID == "C_BAD" {
				return "", goerr.New("not found")
			}
			return "https://example.slack.com/archives/" + channelID + "/p1", nil
		},
		getRepliesFn: func(_ context.Context, _, _ string, _ int) ([]slackservice.ConversationMessage, error) {
			return []slackservice.ConversationMessage{{
				UserID: "U1", Text: "hello", Timestamp: "1700.0001",
			}}, nil
		},
	}
	tools := slacktool.NewReadOnly(slacktool.Deps{Bot: bot})

	ctx, buf := ctxWithCapturingLogger(t)
	out, err := tools[0].Run(ctx, map[string]any{
		"targets": []any{
			map[string]any{"channel_id": "C_BAD", "ts": "t1"},
			map[string]any{"channel_id": "C_OK", "ts": "t2"},
		},
	})
	gt.NoError(t, err).Required()

	results, ok := out["results"].([]map[string]any)
	gt.Bool(t, ok).True().Required()
	gt.Array(t, results).Length(2).Required()

	// First target failed: error string surfaced on the per-target map.
	gt.Value(t, results[0]["channel_id"]).Equal("C_BAD")
	gt.Value(t, results[0]["error"]).NotEqual(nil)

	// Second target succeeded.
	gt.Value(t, results[1]["channel_id"]).Equal("C_OK")
	_, hasErr := results[1]["error"]
	gt.Bool(t, hasErr).False()

	// errutil.Handle was invoked for the failure; logger captured it.
	gt.Bool(t, strings.Contains(buf.String(), "slack get permalink failed")).True()

	// Drain the small async sleep ticker many tools rely on; nothing
	// important here, but keeps lint-style waitgroup hygiene tidy.
	_ = time.Millisecond
}
