package slackpost_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	slackgo "github.com/slack-go/slack"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/slackpost"
)

type postCall struct {
	channelID string
	threadTS  string
	text      string
}

type mockPoster struct {
	posts []postCall
	resp  string
	err   error
}

func (m *mockPoster) PostMessage(ctx context.Context, channelID string, blocks []slackgo.Block, text string) (string, error) {
	m.posts = append(m.posts, postCall{channelID: channelID, text: text})
	if m.err != nil {
		return "", m.err
	}
	return m.resp, nil
}

func (m *mockPoster) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slackgo.Block, text string) (string, error) {
	m.posts = append(m.posts, postCall{channelID: channelID, threadTS: threadTS, text: text})
	if m.err != nil {
		return "", m.err
	}
	return m.resp, nil
}

func TestPostToCaseChannel_TopLevel(t *testing.T) {
	p := &mockPoster{resp: "1234.5678"}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: "C-CASE"})
	gt.Array(t, tools).Length(1).Required()

	out, err := tools[0].Run(context.Background(), map[string]any{
		"text": "hello",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, p.posts).Length(1).Required()
	gt.String(t, p.posts[0].channelID).Equal("C-CASE")
	gt.String(t, p.posts[0].threadTS).Equal("")
	gt.String(t, p.posts[0].text).Equal("hello")
	gt.String(t, out["channel_id"].(string)).Equal("C-CASE")
	gt.String(t, out["message_ts"].(string)).Equal("1234.5678")
	gt.String(t, out["thread_ts"].(string)).Equal("")
}

func TestPostToCaseChannel_ThreadReply(t *testing.T) {
	p := &mockPoster{resp: "9999.0001"}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: "C-CASE"})

	out, err := tools[0].Run(context.Background(), map[string]any{
		"text":      "reply",
		"thread_ts": "111.222",
	})
	gt.NoError(t, err).Required()
	gt.Array(t, p.posts).Length(1).Required()
	gt.String(t, p.posts[0].channelID).Equal("C-CASE")
	gt.String(t, p.posts[0].threadTS).Equal("111.222")
	gt.String(t, p.posts[0].text).Equal("reply")
	gt.String(t, out["thread_ts"].(string)).Equal("111.222")
	gt.String(t, out["message_ts"].(string)).Equal("9999.0001")
}

func TestPostToCaseChannel_RejectsMissingChannel(t *testing.T) {
	p := &mockPoster{}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: ""})
	_, err := tools[0].Run(context.Background(), map[string]any{"text": "x"})
	gt.Error(t, err)
	gt.Array(t, p.posts).Length(0)
}

func TestPostToCaseChannel_RejectsEmptyText(t *testing.T) {
	p := &mockPoster{}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: "C"})
	_, err := tools[0].Run(context.Background(), map[string]any{"text": ""})
	gt.Error(t, err)
	gt.Array(t, p.posts).Length(0)
}

func TestPostToCaseChannel_NoChannelParameter(t *testing.T) {
	// The tool spec must NOT accept a channel parameter — the channel is
	// pinned by the runtime. Lock this so a future contributor cannot
	// silently widen the surface.
	p := &mockPoster{}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: "C"})
	spec := tools[0].Spec()
	_, hasChannel := spec.Parameters["channel_id"]
	gt.Bool(t, hasChannel).False()
	_, hasChannel2 := spec.Parameters["channel"]
	gt.Bool(t, hasChannel2).False()
}

func TestPostToCaseChannel_PropagatesError(t *testing.T) {
	sentinel := goerr.New("slack failed")
	p := &mockPoster{err: sentinel}
	tools := slackpost.New(slackpost.Deps{Poster: p, ChannelID: "C"})
	_, err := tools[0].Run(context.Background(), map[string]any{"text": "x"})
	gt.Error(t, err).Is(sentinel)
}
