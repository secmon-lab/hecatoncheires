package config_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/cli/config"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
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
	// The root ceiling covers the planner AND every sub-agent it spawns, because a
	// finished child's usage is folded into its parent. It therefore has to stay
	// well clear of the task ceiling: at 64 a single busy sub-agent ended the turn.
	gt.Value(t, budgets.Root.MaxSteps).Equal(int64(128))
	gt.Value(t, budgets.Task.MaxSteps).Equal(int64(48))
	gt.Bool(t, budgets.Root.MaxSteps > budgets.Task.MaxSteps*2).True()
	gt.Value(t, budgets.Task.MaxInputTokens).Equal(int64(100_000))
	gt.Value(t, budgets.Task.MaxOutputTokens).Equal(int64(20_000))
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
		"--agent-default-budget-usd", "7.5",
		"--agent-task-max-steps", "6",
		"--agent-task-max-input-tokens", "500",
		"--agent-task-max-output-tokens", "100",
		"--agent-budget-notice-ratio", "0.5",
		"--agent-worker-concurrency", "3",
		"--agent-worker-poll-concurrency", "1",
		"--agent-worker-lease", "30s",
		"--agent-worker-poll-interval", "500ms",
	)

	budgets, err := cfg.Budgets()
	gt.NoError(t, err).Required()
	gt.Value(t, budgets.Root.MaxSteps).Equal(int64(10))
	gt.Value(t, budgets.Task.MaxSteps).Equal(int64(6))
	gt.Value(t, budgets.Task.MaxInputTokens).Equal(int64(500))
	gt.Value(t, budgets.Task.MaxOutputTokens).Equal(int64(100))
	gt.Value(t, budgets.Root.NoticeRatio).Equal(0.5)

	got, source, err := cfg.BudgetOr(nil)
	gt.NoError(t, err).Required()
	gt.Value(t, got).Equal(pricing.FromUSD(7.5))
	gt.String(t, source).Equal(config.BudgetSourceFlag)

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
		"zero steps":              {"--agent-max-steps", "0"},
		"ratio at one":            {"--agent-budget-notice-ratio", "1"},
		"negative ratio":          {"--agent-budget-notice-ratio", "-0.1"},
		"zero task steps":         {"--agent-task-max-steps", "0"},
		"zero task input tokens":  {"--agent-task-max-input-tokens", "0"},
		"zero task output tokens": {"--agent-task-max-output-tokens", "0"},
	}

	for name, args := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := runAgentFlags(t, args...).Budgets()
			gt.Value(t, err).NotNil()
		})
	}
}

// TestAgentBudgetPrecedence pins the three-way decision: the flag wins over the
// document, the document wins over the built-in figure, and each answer says
// where it came from so the startup log can report it.
func TestAgentBudgetPrecedence(t *testing.T) {
	fromDoc := &config.AgentSection{DefaultBudgetUSD: 3}

	t.Run("flag beats the document", func(t *testing.T) {
		got, source, err := runAgentFlags(t, "--agent-default-budget-usd", "9").BudgetOr(fromDoc)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(pricing.FromUSD(9))
		gt.String(t, source).Equal(config.BudgetSourceFlag)
	})

	t.Run("document is used when the flag is absent", func(t *testing.T) {
		got, source, err := runAgentFlags(t).BudgetOr(fromDoc)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(pricing.FromUSD(3))
		gt.String(t, source).Equal(config.BudgetSourceGlobalConfig)
	})

	t.Run("built-in figure when neither is set", func(t *testing.T) {
		got, source, err := runAgentFlags(t).BudgetOr(nil)
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(pricing.FromUSD(2))
		gt.String(t, source).Equal(config.BudgetSourceBuiltin)
	})

	t.Run("a section setting no budget falls through", func(t *testing.T) {
		got, source, err := runAgentFlags(t).BudgetOr(&config.AgentSection{})
		gt.NoError(t, err).Required()
		gt.Value(t, got).Equal(pricing.FromUSD(2))
		gt.String(t, source).Equal(config.BudgetSourceBuiltin)
	})

	t.Run("negative flag value is refused", func(t *testing.T) {
		_, _, err := runAgentFlags(t, "--agent-default-budget-usd", "-1").BudgetOr(nil)
		gt.Error(t, err).Is(config.ErrInvalidBudget)
	})
}

func TestAgentLogAttrs(t *testing.T) {
	attrs := runAgentFlags(t).LogAttrs()
	keys := map[string]bool{}
	for _, a := range attrs {
		keys[a.Key] = true
	}
	for _, want := range []string{
		"max_steps",
		"task_max_steps", "task_max_input_tokens", "task_max_output_tokens",
		"notice_ratio", "worker_concurrency", "worker_poll_concurrency",
		"worker_lease", "worker_poll_interval",
	} {
		gt.Bool(t, keys[want]).True()
	}
}
