package slack_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/service/slack"
)

func TestApplyPostThreadOptions(t *testing.T) {
	t.Run("no options leaves broadcast disabled", func(t *testing.T) {
		cfg := slack.ApplyPostThreadOptions()
		gt.Bool(t, cfg.Broadcast).False()
	})

	t.Run("WithBroadcastToChannel enables broadcast", func(t *testing.T) {
		cfg := slack.ApplyPostThreadOptions(slack.WithBroadcastToChannel())
		gt.Bool(t, cfg.Broadcast).True()
	})

	t.Run("applying broadcast twice is idempotent", func(t *testing.T) {
		cfg := slack.ApplyPostThreadOptions(
			slack.WithBroadcastToChannel(),
			slack.WithBroadcastToChannel(),
		)
		gt.Bool(t, cfg.Broadcast).True()
	})
}
