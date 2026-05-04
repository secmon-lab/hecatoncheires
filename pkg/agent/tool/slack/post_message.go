package slacktool

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
	slackservice "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/errutil"
)

// postMessageTool posts a message to the case's Slack channel.
type postMessageTool struct {
	slack     slackservice.Service
	channelID string
}

func (t *postMessageTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name:        "slack__post_message",
		Description: "Post a text message to the case's Slack channel. Use thread_ts to reply in a specific thread.",
		Parameters: map[string]*gollem.Parameter{
			"text": {
				Type:        gollem.TypeString,
				Description: "The message text to post",
				Required:    true,
			},
			"thread_ts": {
				Type:        gollem.TypeString,
				Description: "Thread timestamp to reply in a thread (optional). If omitted, posts as a new message.",
				Required:    false,
			},
		},
	}
}

func (t *postMessageTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	text, _ := args["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	threadTS, _ := args["thread_ts"].(string)

	if threadTS != "" {
		tool.Update(ctx, "Posting thread reply...")
		ts, err := t.slack.PostThreadReply(ctx, t.channelID, threadTS, text)
		if err != nil {
			opts := []goerr.Option{
				goerr.V("channel_id", t.channelID),
				goerr.V("thread_ts", threadTS),
			}
			opts = append(opts, slackErrorAttrs(err)...)
			wrapped := goerr.Wrap(err, "failed to post thread reply", opts...)
			errutil.Handle(ctx, wrapped, "slack post thread reply failed")
			return nil, wrapped
		}
		return map[string]any{
			"timestamp":  ts,
			"channel_id": t.channelID,
			"thread_ts":  threadTS,
		}, nil
	}

	tool.Update(ctx, "Posting message...")
	ts, err := t.slack.PostMessage(ctx, t.channelID, nil, text)
	if err != nil {
		opts := []goerr.Option{
			goerr.V("channel_id", t.channelID),
		}
		opts = append(opts, slackErrorAttrs(err)...)
		wrapped := goerr.Wrap(err, "failed to post message", opts...)
		errutil.Handle(ctx, wrapped, "slack post message failed")
		return nil, wrapped
	}
	return map[string]any{
		"timestamp":  ts,
		"channel_id": t.channelID,
	}, nil
}
