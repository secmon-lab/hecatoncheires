package config

import (
	"log/slog"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/urfave/cli/v3"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	agentkernel "github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// Agent runtime defaults.
//
// THE ROOT CEILING IS A TOTAL FOR THE WHOLE TURN, not the planner's own count.
// agentkit folds a finished child's entire Metrics into its parent (see
// `.claude/rules/architecture.md` § Budget), so a root run is charged for every
// sub-agent it spawns as well as for its own transitions. Root and Task must
// therefore be read together: Task bounds ONE investigation, Root bounds the
// planner plus all of them.
//
// The Task step ceiling is derived from the loop bounds the pre-agentkit runtime
// used: a sub-agent spends two transitions per tool round (generate, then the
// tool) and ran at most twenty rounds, so 40 covered it and 48 leaves headroom.
// The Root step ceiling is then what a turn needs on top: a plan-execute turn
// spends three transitions per round (plan, collect, replan) plus the terminal
// output and any re-emit of malformed JSON, and it has to afford the sub-agents
// underneath. 128 covers the planner's own work plus roughly two sub-agents at
// their full allowance, or several modest ones. It was 64, which could not even
// cover one — a single busy sub-agent ended the turn with "step budget
// exhausted" and no answer.
//
// A ROOT RUN'S SPEND CEILING IS MONEY, not tokens: which model a run generates
// through is configuration, and the models a deployment may name differ in price
// by more than an order of magnitude, so any token figure is right for one of
// them and wrong for another. The task tier keeps its token ceilings — they bound
// one investigation's share of a turn, and the money ceiling on the root already
// bounds what the tree as a whole may cost.
//
// The task token ceilings are NOT derived from measurement: the pre-agentkit
// runtime counted no tokens at all, so there is no baseline in this repository to
// derive one from. Input and output are bounded separately because output tokens
// cost several times what input tokens do: under one combined ceiling a large
// input allowance would hide an output run-away — the expensive half — until the
// whole allowance was gone.
const (
	defaultAgentMaxSteps = 128
	// fallbackDefaultBudgetUSD is what one run may spend when neither the
	// command line nor the global config's [agent] section says. It is the last
	// resort of the three, and deliberately modest: a deployment that cares
	// about the figure states it, and one that has not thought about it should
	// not discover the answer on an invoice.
	fallbackDefaultBudgetUSD        = 2.0
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
	maxSteps int64
	// defaultBudgetUSD is 0 when the flag was not given. The flag deliberately
	// carries no Value: a built-in default there would make "not specified"
	// indistinguishable from "specified as the default", and the global config's
	// [agent] section would then never get a say. See BudgetOr.
	defaultBudgetUSD    float64
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
		&cli.FloatFlag{
			Name: "agent-default-budget-usd",
			Usage: "Maximum USD one agent run may spend, including its sub-agents. " +
				"Overrides [agent] default_budget_usd in the global config; a Job's budget_usd overrides both",
			Sources:     cli.EnvVars("HECATONCHEIRES_AGENT_DEFAULT_BUDGET_USD"),
			Destination: &a.defaultBudgetUSD,
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
//
// The root tier carries no spend ceiling here: what a run may spend is money,
// and the figure is resolved per run from its Job and the deployment default
// (see BudgetOr).
func (a *Agent) Budgets() (agentkernel.Budgets, error) {
	b := agentkernel.Budgets{
		Root: budget.Root{
			MaxSteps:    a.maxSteps,
			NoticeRatio: a.noticeRatio,
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

// Budget sources, as reported by BudgetOr. They name where the effective figure
// came from, so an operator reading the startup log can tell which of the three
// settings is in force.
const (
	BudgetSourceFlag         = "flag"
	BudgetSourceGlobalConfig = "global_config"
	BudgetSourceBuiltin      = "builtin"
)

// BudgetOr resolves the default budget for one run: the command line first, then
// the global config's [agent] section, then the built-in figure.
//
// The precedence is deliberate. The deployment-wide intent belongs in the
// document alongside the model definitions it is spent on, while a temporary
// change for one environment belongs on the command line — so the narrower
// setting wins. sec may be nil, which is what a deployment with no [agent]
// section has.
//
// It lives here rather than at the composition root so the three-way decision is
// made in ONE place: a second caller resolving it differently is how a run ends
// up bounded by a figure nobody configured.
func (a *Agent) BudgetOr(sec *AgentSection) (pricing.NanoUSD, string, error) {
	if a.defaultBudgetUSD < 0 {
		return 0, "", goerr.Wrap(ErrInvalidBudget,
			"--agent-default-budget-usd must not be negative",
			goerr.V("budget_usd", a.defaultBudgetUSD))
	}
	if a.defaultBudgetUSD > 0 {
		converted, err := budgetFromUSD(a.defaultBudgetUSD)
		if err != nil {
			return 0, "", goerr.Wrap(err, "invalid --agent-default-budget-usd")
		}
		return converted, BudgetSourceFlag, nil
	}
	if fromDoc := sec.DefaultBudget(); fromDoc > 0 {
		return fromDoc, BudgetSourceGlobalConfig, nil
	}
	return pricing.FromUSD(fallbackDefaultBudgetUSD), BudgetSourceBuiltin, nil
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
