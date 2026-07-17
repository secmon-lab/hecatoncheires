package config_test

import (
	"context"
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/urfave/cli/v3"
)

// runHomeMessageLLM parses the given args into a HomeMessageLLM via its Flags()
// and returns the populated config. Using the real flag wiring keeps the test
// honest about flag names / env bindings.
func runHomeMessageLLM(t *testing.T, args ...string) *config.HomeMessageLLM {
	t.Helper()
	var cfg config.HomeMessageLLM
	cmd := &cli.Command{
		Name:  "test",
		Flags: cfg.Flags(),
		Action: func(context.Context, *cli.Command) error {
			return nil
		},
	}
	err := cmd.Run(context.Background(), append([]string{"test"}, args...))
	gt.NoError(t, err).Required()
	return &cfg
}

func TestHomeMessageLLM_IsEnabled(t *testing.T) {
	t.Parallel()
	gt.Bool(t, runHomeMessageLLM(t).IsEnabled()).False()
	gt.Bool(t, runHomeMessageLLM(t, "--home-message-llm-provider", "openai").IsEnabled()).True()
}

func TestHomeMessageLLM_NewClient_Errors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("openai without api key", func(t *testing.T) {
		cfg := runHomeMessageLLM(t, "--home-message-llm-provider", "openai")
		_, err := cfg.NewClient(ctx)
		gt.Error(t, err)
	})

	t.Run("claude with both api key and gcp project is rejected", func(t *testing.T) {
		cfg := runHomeMessageLLM(t,
			"--home-message-llm-provider", "claude",
			"--home-message-llm-claude-api-key", "sk-test",
			"--home-message-llm-gemini-project-id", "proj",
		)
		_, err := cfg.NewClient(ctx)
		gt.Error(t, err)
	})

	t.Run("claude without any credential", func(t *testing.T) {
		cfg := runHomeMessageLLM(t, "--home-message-llm-provider", "claude")
		_, err := cfg.NewClient(ctx)
		gt.Error(t, err)
	})

	t.Run("gemini without project id", func(t *testing.T) {
		cfg := runHomeMessageLLM(t, "--home-message-llm-provider", "gemini")
		_, err := cfg.NewClient(ctx)
		gt.Error(t, err)
	})

	t.Run("unknown provider", func(t *testing.T) {
		cfg := runHomeMessageLLM(t, "--home-message-llm-provider", "bogus")
		_, err := cfg.NewClient(ctx)
		gt.Error(t, err)
	})
}
