package slacktool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	goslack "github.com/slack-go/slack"

	slacktool "github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slack"
)

func TestPostMessageTool_PostMessageErrorRoutesThroughErrutilHandle(t *testing.T) {
	bot := &fakeBotService{
		postMessageFn: func(_ context.Context, _ string, _ []goslack.Block, _ string) (string, error) {
			return "", goerr.New("post boom", goerr.V("slack_error", "channel_not_found"))
		},
	}
	tools := slacktool.NewForAssist(slacktool.Deps{Bot: bot, ChannelID: "C123"})
	// NewForAssist returns getMessages + postMessage (search nil); pick post.
	postTool := pickToolByName(t, tools, "slack__post_message")

	ctx, buf := ctxWithCapturingLogger(t)
	_, err := postTool.Run(ctx, map[string]any{"text": "hello"})
	gt.Value(t, err).NotNil()

	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	values := ge.Values()
	gt.Value(t, values["channel_id"]).Equal("C123")
	gt.Value(t, values["slack_error"]).Equal("channel_not_found")

	gt.String(t, buf.String()).Contains(`"msg":"slack post message failed"`)
}

func TestPostMessageTool_ThreadReplyErrorRoutesThroughErrutilHandle(t *testing.T) {
	bot := &fakeBotService{
		postThreadReply: func(_ context.Context, _, _, _ string) (string, error) {
			return "", goerr.New("thread reply boom", goerr.V("slack_error", "thread_not_found"))
		},
	}
	tools := slacktool.NewForAssist(slacktool.Deps{Bot: bot, ChannelID: "C123"})
	postTool := pickToolByName(t, tools, "slack__post_message")

	ctx, buf := ctxWithCapturingLogger(t)
	_, err := postTool.Run(ctx, map[string]any{"text": "reply", "thread_ts": "1700.0001"})
	gt.Value(t, err).NotNil()

	var ge *goerr.Error
	gt.Bool(t, errors.As(err, &ge)).True().Required()
	values := ge.Values()
	gt.Value(t, values["thread_ts"]).Equal("1700.0001")
	gt.Value(t, values["slack_error"]).Equal("thread_not_found")

	gt.String(t, buf.String()).Contains(`"msg":"slack post thread reply failed"`)
}

func TestPostMessageTool_Success(t *testing.T) {
	bot := &fakeBotService{
		postMessageFn: func(_ context.Context, channelID string, _ []goslack.Block, text string) (string, error) {
			gt.String(t, channelID).Equal("C123")
			gt.String(t, text).Equal("hello")
			return "1700.9999", nil
		},
	}
	tools := slacktool.NewForAssist(slacktool.Deps{Bot: bot, ChannelID: "C123"})
	postTool := pickToolByName(t, tools, "slack__post_message")

	ctx, buf := ctxWithCapturingLogger(t)
	out, err := postTool.Run(ctx, map[string]any{"text": "hello"})
	gt.NoError(t, err).Required()
	gt.Value(t, out["timestamp"]).Equal("1700.9999")
	gt.Value(t, out["channel_id"]).Equal("C123")
	gt.Number(t, buf.Len()).Equal(0)
}
