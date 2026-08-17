package cli

import (
	"context"

	"github.com/m-mizutani/goerr/v2"

	slacksvc "github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

// slackNotifierAdapter bridges the runner's job.SlackNotifier onto the
// broader slacksvc.Service. It exposes only the plain-text root / thread
// posts the operational session log needs.
type slackNotifierAdapter struct {
	svc slacksvc.Service
}

func (a slackNotifierAdapter) PostMessage(ctx context.Context, channelID, text string) (string, error) {
	if a.svc == nil {
		return "", goerr.New("slack service is nil")
	}
	return a.svc.PostMessage(ctx, channelID, nil, text)
}

func (a slackNotifierAdapter) PostThreadReply(ctx context.Context, channelID, threadTS, text string) (string, error) {
	if a.svc == nil {
		return "", goerr.New("slack service is nil")
	}
	return a.svc.PostThreadReply(ctx, channelID, threadTS, text)
}
