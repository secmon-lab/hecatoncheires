// Package slackpost exposes the Slack-posting gollem tool available to
// event-driven Agent Jobs. The tool is hard-pinned to the Case's bound
// Slack channel; the LLM cannot pick an arbitrary channel ID. This is
// the only writer-side Slack surface a Job has access to.
package slackpost

import (
	"context"
	"fmt"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gollem"
	slackgo "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool"
)

// Poster is the narrow surface of slack.Service the slackpost tool depends
// on. Defined here so the package does not import the full service layer
// and we can mock it cleanly in tests.
type Poster interface {
	PostMessage(ctx context.Context, channelID string, blocks []slackgo.Block, text string) (string, error)
	PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slackgo.Block, text string) (string, error)
}

// Deps groups the dependencies the slackpost tool needs.
type Deps struct {
	Poster Poster
	// ChannelID is the Slack channel ID bound to the current Case. The
	// tool always posts to this channel — the agent cannot select another.
	// Empty ChannelID makes the tool fail loudly at invocation time
	// (typically a draft-mode Case that never got a channel).
	ChannelID string
	// DefaultThreadTS is the thread the message defaults into when the agent
	// does not pass an explicit thread_ts. For thread-mode cases this is the
	// case's SlackThreadTS so Job output lands in the case thread rather than
	// at the monitored channel's root. Empty for channel-mode cases (posts go
	// to the channel root by default).
	DefaultThreadTS string
}

// New builds the writer-side Slack tools available to Jobs.
func New(deps Deps) []gollem.Tool {
	return []gollem.Tool{
		&postToCaseChannelTool{deps: deps},
	}
}

type postToCaseChannelTool struct {
	deps Deps
}

func (t *postToCaseChannelTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{
		Name: "slack__post_to_case_channel",
		Description: "Post a plain-text message to the Slack channel bound to the current case. " +
			"The channel ID is fixed by the runtime and cannot be changed by the agent. " +
			"To reply in an existing thread, pass thread_ts; omit it to post a top-level message.",
		Parameters: map[string]*gollem.Parameter{
			"text": {
				Type:        gollem.TypeString,
				Description: "Message text. Slack mrkdwn formatting is supported.",
				Required:    true,
			},
			"thread_ts": {
				Type: gollem.TypeString,
				Description: "Optional Slack thread parent timestamp. When set, the message is " +
					"posted as a thread reply. When omitted, the message is posted at the " +
					"channel root.",
			},
		},
	}
}

func (t *postToCaseChannelTool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	if t.deps.Poster == nil {
		return nil, goerr.New("slack poster is not configured")
	}
	if t.deps.ChannelID == "" {
		return nil, goerr.New("case has no Slack channel; cannot post")
	}

	text, _ := args["text"].(string)
	if text == "" {
		return nil, goerr.New("text is required")
	}

	threadTS := t.deps.DefaultThreadTS
	if v, ok := args["thread_ts"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return nil, goerr.New("thread_ts must be a string", goerr.V("type", fmt.Sprintf("%T", v)))
		}
		threadTS = s
	}

	tool.Update(ctx, fmt.Sprintf("Posting to Slack channel %s...", t.deps.ChannelID))

	var (
		ts  string
		err error
	)
	if threadTS != "" {
		ts, err = t.deps.Poster.PostThreadMessage(ctx, t.deps.ChannelID, threadTS, nil, text)
	} else {
		ts, err = t.deps.Poster.PostMessage(ctx, t.deps.ChannelID, nil, text)
	}
	if err != nil {
		return nil, goerr.Wrap(err, "failed to post to Slack",
			goerr.V("channel_id", t.deps.ChannelID),
			goerr.V("thread_ts", threadTS))
	}

	return map[string]any{
		"channel_id": t.deps.ChannelID,
		"thread_ts":  threadTS,
		"message_ts": ts,
	}, nil
}
