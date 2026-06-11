package cli

import (
	"context"

	slackgo "github.com/slack-go/slack"

	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// slackPosterAdapter bridges slackpost.Poster onto the existing
// slacksvc.Service. The Poster interface intentionally exposes only
// PostMessage / PostThreadMessage, so an LLM with the slack_post tool
// cannot reach the broader Slack API surface.
type slackPosterAdapter struct {
	svc slacksvc.Service
}

func (a slackPosterAdapter) PostMessage(ctx context.Context, channelID string, blocks []slackgo.Block, text string) (string, error) {
	if a.svc == nil {
		return "", nil
	}
	return a.svc.PostMessage(ctx, channelID, blocks, text)
}

func (a slackPosterAdapter) PostThreadMessage(ctx context.Context, channelID string, threadTS string, blocks []slackgo.Block, text string) (string, error) {
	if a.svc == nil {
		return "", nil
	}
	return a.svc.PostThreadMessage(ctx, channelID, threadTS, blocks, text)
}
