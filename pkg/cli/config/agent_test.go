package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
)

// runAgentFlags parses argv through the flag set so the test exercises the same
// path production does, defaults included.
func runAgentFlags(t *testing.T, args ...string) *config.Agent {
	t.Helper()
	var cfg config.Agent
	cmd := &cli.Command{
		Name:   "test",
		Flags:  cfg.Flags(),
		Action: func(context.Context, *cli.Command) error { return nil },
	}
	gt.NoError(t, cmd.Run(context.Background(), append([]string{"test"}, args...))).Required()
	return &cfg
}

func TestAgentDefaults(t *testing.T) {
	cfg := runAgentFlags(t)

	budgets, err := cfg.Budgets()
	gt.NoError(t, err).Required()
	gt.Value(t, budgets.Root.MaxSteps).Equal(int64(64))
	gt.Value(t, budgets.Root.MaxTokens).Equal(int64(1_500_000))
	gt.Value(t, budgets.Task.MaxSteps).Equal(int64(48))
	gt.Value(t, budgets.Task.MaxTokens).Equal(int64(300_000))
	gt.Value(t, budgets.Root.NoticeRatio).Equal(0.8)
	gt.Value(t, budgets.Task.NoticeRatio).Equal(0.8)

	gt.NoError(t, cfg.ValidateWorker())
	gt.Value(t, cfg.WorkerConcurrency()).Equal(8)
	gt.Value(t, cfg.WorkerPollConcurrency()).Equal(2)
	gt.Value(t, cfg.WorkerLease()).Equal(120 * time.Second)
	gt.Value(t, cfg.WorkerPollInterval()).Equal(2 * time.Second)
}

func TestAgentFlagsOverrideDefaults(t *testing.T) {
	cfg := runAgentFlags(t,
		"--agent-max-steps", "10",
		"--agent-max-tokens", "2000",
		"--agent-task-max-steps", "6",
		"--agent-task-max-tokens", "500",
		"--agent-budget-notice-ratio", "0.5",
		"--agent-worker-concurrency", "3",
		"--agent-worker-poll-concurrency", "1",
		"--agent-worker-lease", "30s",
		"--agent-worker-poll-interval", "500ms",
	)

	budgets, err := cfg.Budgets()
	gt.NoError(t, err).Required()
	gt.Value(t, budgets.Root.MaxSteps).Equal(int64(10))
	gt.Value(t, budgets.Root.MaxTokens).Equal(int64(2000))
	gt.Value(t, budgets.Task.MaxSteps).Equal(int64(6))
	gt.Value(t, budgets.Task.MaxTokens).Equal(int64(500))
	gt.Value(t, budgets.Root.NoticeRatio).Equal(0.5)

	gt.Value(t, cfg.WorkerConcurrency()).Equal(3)
	gt.Value(t, cfg.WorkerPollConcurrency()).Equal(1)
	gt.Value(t, cfg.WorkerLease()).Equal(30 * time.Second)
	gt.Value(t, cfg.WorkerPollInterval()).Equal(500 * time.Millisecond)
}

// TestAgentBudgetsRejectInvalidValues pins that a nonsensical ceiling fails at
// startup. A zero step budget would stop every run at its first transition, and
// the only symptom would be agents that answer nothing.
func TestAgentBudgetsRejectInvalidValues(t *testing.T) {
	testCases := map[string][]string{
		"zero steps":     {"--agent-max-steps", "0"},
		"zero tokens":    {"--agent-max-tokens", "0"},
		"ratio at one":   {"--agent-budget-notice-ratio", "1"},
		"negative ratio": {"--agent-budget-notice-ratio", "-0.1"},
		"zero task steps": {
			"--agent-task-max-steps", "0",
		},
	}

	for name, args := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := runAgentFlags(t, args...).Budgets()
			gt.Value(t, err).NotNil()
		})
	}
}

func TestAgentValidateWorkerRejectsInvalidValues(t *testing.T) {
	testCases := map[string][]string{
		"zero concurrency":      {"--agent-worker-concurrency", "0"},
		"zero poll concurrency": {"--agent-worker-poll-concurrency", "0"},
		"zero lease":            {"--agent-worker-lease", "0s"},
		"zero poll interval":    {"--agent-worker-poll-interval", "0s"},
	}

	for name, args := range testCases {
		t.Run(name, func(t *testing.T) {
			gt.Value(t, runAgentFlags(t, args...).ValidateWorker()).NotNil()
		})
	}
}

func TestAgentLogAttrs(t *testing.T) {
	attrs := runAgentFlags(t).LogAttrs()
	keys := map[string]bool{}
	for _, a := range attrs {
		keys[a.Key] = true
	}
	for _, want := range []string{
		"max_steps", "max_tokens", "task_max_steps", "task_max_tokens",
		"notice_ratio", "worker_concurrency", "worker_poll_concurrency",
		"worker_lease", "worker_poll_interval",
	} {
		gt.Bool(t, keys[want]).True()
	}
}
