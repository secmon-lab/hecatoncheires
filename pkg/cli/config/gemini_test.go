package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestGemini_Configure(t *testing.T) {
	t.Run("returns nil client when project ID is empty", func(t *testing.T) {
		cfg := config.NewGeminiForTest("", "us-central1")
		client, err := cfg.Configure(t.Context())
		gt.NoError(t, err)
		gt.Value(t, client).Nil()
	})

	t.Run("returns flags", func(t *testing.T) {
		cfg := config.NewGeminiForTest("", "")
		flags := cfg.Flags()
		gt.Value(t, len(flags)).Equal(2)
	})
}
