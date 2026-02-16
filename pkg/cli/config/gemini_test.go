package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

func TestGemini_Configure(t *testing.T) {
	t.Run("fails when project ID is empty", func(t *testing.T) {
		cfg := config.NewGeminiForTest("", "us-central1")
		_, err := cfg.Configure(t.Context())
		gt.Value(t, err).NotNil()
	})

	t.Run("returns flags", func(t *testing.T) {
		cfg := config.NewGeminiForTest("", "")
		flags := cfg.Flags()
		gt.Value(t, len(flags)).Equal(2)
	})
}
