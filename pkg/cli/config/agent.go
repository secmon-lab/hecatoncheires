package config

import (
	"log/slog"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
)

// Agent runtime defaults.
//
// The two step ceilings are derived from the loop bounds the pre-agentkit
// runtime used. A plan-execute turn spends three transitions per round (plan,
// collect, replan) and ran at most eight rounds, so 24 covered the loop; 64
// leaves room for the direct path, the terminal output and a planner that has
// to re-emit malformed JSON. A sub-agent spends two transitions per tool round
// (generate, then the tool) and ran at most twenty rounds, so 40 covered it and
// 48 leaves headroom.
//
// The token ceilings are NOT derived from measurement: the previous runtime
// counted no tokens at all, so there is no baseline in this repository to
// derive one from. Replace them with measured values once Process.Metrics has
// recorded real usage.
//
// Input and output are bounded separately because output tokens cost several
// times what input tokens do: under one combined ceiling a large input
// allowance would hide an output run-away — the expensive half — until the
// whole budget was gone.
//
// A sub-agent gets a fifth of the root allowance, the ratio the previous
// runtime's loop bounds already implied (one investigation is a fraction of a
// turn, and a turn may run several).
const (
	defaultAgentMaxSteps            = 64
	defaultAgentMaxInputTokens      = 500_000
	defaultAgentMaxOutputTokens     = 100_000
	defaultAgentTaskMaxSteps        = 48
	defaultAgentTaskMaxInputTokens  = 100_000
	defaultAgentTaskMaxOutputTokens = 20_000
	// defaultAgentNoticeRatio leaves a fifth of the budget for wrapping up,
	// which is more than the one or two transitions a terminal answer needs.
	defaultAgentNoticeRatio = 0.8

	// defaultAgentWorkerConcurrency bounds how many transitions one instance
	// drives at once, counting both polled claims and the eager dispatch a
	// Spawn on this instance triggers.
	defaultAgentWorkerConcurrency = 8
	// defaultAgentWorkerPollConcurrency is how many poll loops look for work.
	// Eager dispatch covers the latency of a Spawn on this instance, so polling
	// only has to pick up other instances' work and timers.
	defaultAgentWorkerPollConcurrency = 2
	// defaultAgentWorkerLease must exceed the slowest single transition, which
	// is one LLM call. A lease that expires mid-call gets the Process reclaimed
	// and the call re-charged.
	defaultAgentWorkerLease = 120 * time.Second
	// defaultAgentWorkerPollInterval trades idle Firestore reads against the
	// delay before another instance's work is picked up.
	defaultAgentWorkerPollInterval = 2 * time.Second
)

// Agent holds the CLI flags for the agentkit-based agent runtime: the budget
// ceilings every Process runs under, and the worker settings of the in-process
// Serve loop.
type Agent struct {
	maxSteps            int64
	maxInputTokens      int64
	maxOutputTokens     int64
	taskMaxSteps        int64
	taskMaxInputTokens  int64
	taskMaxOutputTokens int64
	noticeRatio         float64

	workerConcurrency     int
	workerPollConcurrency int
	workerLease           time.Duration
	workerPollInterval    time.Duration
}

// Flags returns the CLI flags for the agent runtime.
func (a *Agent) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.Int64Flag{
			Name:        "agent-max-steps",
			Usage:       "Maximum committed transitions one agent run may execute, including its sub-agents",
			Value:       defaultAgentMaxSteps,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_MAX_STEPS"),
			Destination: &a.maxSteps,
		},
		&cli.Int64Flag{
			Name:        "agent-max-input-tokens",
			Usage:       "Maximum input tokens one agent run may consume, including its sub-agents",
			Value:       defaultAgentMaxInputTokens,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_MAX_INPUT_TOKENS"),
			Destination: &a.maxInputTokens,
		},
		&cli.Int64Flag{
			Name:        "agent-max-output-tokens",
			Usage:       "Maximum output tokens one agent run may produce, including its sub-agents",
			Value:       defaultAgentMaxOutputTokens,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_MAX_OUTPUT_TOKENS"),
			Destination: &a.maxOutputTokens,
		},
		&cli.Int64Flag{
			Name:        "agent-task-max-steps",
			Usage:       "Maximum committed transitions one sub-agent may execute",
			Value:       defaultAgentTaskMaxSteps,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_TASK_MAX_STEPS"),
			Destination: &a.taskMaxSteps,
		},
		&cli.Int64Flag{
			Name:        "agent-task-max-input-tokens",
			Usage:       "Maximum input tokens one sub-agent may consume",
			Value:       defaultAgentTaskMaxInputTokens,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_TASK_MAX_INPUT_TOKENS"),
			Destination: &a.taskMaxInputTokens,
		},
		&cli.Int64Flag{
			Name:        "agent-task-max-output-tokens",
			Usage:       "Maximum output tokens one sub-agent may produce",
			Value:       defaultAgentTaskMaxOutputTokens,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_TASK_MAX_OUTPUT_TOKENS"),
			Destination: &a.taskMaxOutputTokens,
		},
		&cli.FloatFlag{
			Name:        "agent-budget-notice-ratio",
			Usage:       "Fraction of a budget at which the agent is told to finish with what it has (0 < r < 1)",
			Value:       defaultAgentNoticeRatio,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_BUDGET_NOTICE_RATIO"),
			Destination: &a.noticeRatio,
		},
		&cli.IntFlag{
			Name:        "agent-worker-concurrency",
			Usage:       "Maximum agent transitions this instance drives at once",
			Value:       defaultAgentWorkerConcurrency,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_WORKER_CONCURRENCY"),
			Destination: &a.workerConcurrency,
		},
		&cli.IntFlag{
			Name:        "agent-worker-poll-concurrency",
			Usage:       "Number of parallel poll loops looking for runnable agent processes",
			Value:       defaultAgentWorkerPollConcurrency,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_WORKER_POLL_CONCURRENCY"),
			Destination: &a.workerPollConcurrency,
		},
		&cli.DurationFlag{
			Name:        "agent-worker-lease",
			Usage:       "How long a worker holds a claimed agent process before another may reclaim it",
			Value:       defaultAgentWorkerLease,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_WORKER_LEASE"),
			Destination: &a.workerLease,
		},
		&cli.DurationFlag{
			Name:        "agent-worker-poll-interval",
			Usage:       "How often a worker polls for runnable agent processes",
			Value:       defaultAgentWorkerPollInterval,
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_WORKER_POLL_INTERVAL"),
			Destination: &a.workerPollInterval,
		},
	}
}

// Budgets returns the validated ceilings for the kernel.
func (a *Agent) Budgets() (agentkernel.Budgets, error) {
	b := agentkernel.Budgets{
		Root: budget.Config{
			MaxSteps:        a.maxSteps,
			MaxInputTokens:  a.maxInputTokens,
			MaxOutputTokens: a.maxOutputTokens,
			NoticeRatio:     a.noticeRatio,
		},
		Task: budget.Config{
			MaxSteps:        a.taskMaxSteps,
			MaxInputTokens:  a.taskMaxInputTokens,
			MaxOutputTokens: a.taskMaxOutputTokens,
			NoticeRatio:     a.noticeRatio,
		},
	}
	if err := b.Validate(); err != nil {
		return agentkernel.Budgets{}, goerr.Wrap(err, "invalid agent budget configuration")
	}
	return b, nil
}

// WorkerLease returns the configured claim lease duration.
func (a *Agent) WorkerLease() time.Duration { return a.workerLease }

// WorkerPollInterval returns the configured poll interval.
func (a *Agent) WorkerPollInterval() time.Duration { return a.workerPollInterval }

// WorkerConcurrency returns the hard limit on concurrently driven claims.
func (a *Agent) WorkerConcurrency() int { return a.workerConcurrency }

// WorkerPollConcurrency returns the number of poll loops.
func (a *Agent) WorkerPollConcurrency() int { return a.workerPollConcurrency }

// ValidateWorker enforces the worker settings so a misconfiguration fails at
// startup rather than as a Serve loop that never claims anything.
func (a *Agent) ValidateWorker() error {
	if a.workerConcurrency <= 0 {
		return goerr.New("agent worker concurrency must be positive",
			goerr.V("concurrency", a.workerConcurrency))
	}
	if a.workerPollConcurrency <= 0 {
		return goerr.New("agent worker poll concurrency must be positive",
			goerr.V("poll_concurrency", a.workerPollConcurrency))
	}
	if a.workerLease <= 0 {
		return goerr.New("agent worker lease must be positive", goerr.V("lease", a.workerLease))
	}
	if a.workerPollInterval <= 0 {
		return goerr.New("agent worker poll interval must be positive",
			goerr.V("poll_interval", a.workerPollInterval))
	}
	return nil
}

// LogAttrs returns log attributes describing the configuration.
func (a *Agent) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Int64("max_steps", a.maxSteps),
		slog.Int64("max_input_tokens", a.maxInputTokens),
		slog.Int64("max_output_tokens", a.maxOutputTokens),
		slog.Int64("task_max_steps", a.taskMaxSteps),
		slog.Int64("task_max_input_tokens", a.taskMaxInputTokens),
		slog.Int64("task_max_output_tokens", a.taskMaxOutputTokens),
		slog.Float64("notice_ratio", a.noticeRatio),
		slog.Int("worker_concurrency", a.workerConcurrency),
		slog.Int("worker_poll_concurrency", a.workerPollConcurrency),
		slog.Duration("worker_lease", a.workerLease),
		slog.Duration("worker_poll_interval", a.workerPollInterval),
	}
}
