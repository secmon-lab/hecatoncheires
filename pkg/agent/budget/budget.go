// Package budget turns an operator-configured ceiling into the
// agentkit.Limiter a Strategy answers Limit with.
//
// There are two tiers, and they are bounded by different quantities:
//
//   - Root is what an application entry point spawns. Its spend ceiling is
//     MONEY (see Root and RunLimit), because a token is not a unit of cost: one
//     run may execute on a model twenty times dearer than another's, so a token
//     figure that is right for one is wrong for the other. It keeps a step
//     ceiling, but only to stop a run that never terminates.
//   - Config is the sub-agent tier (one planned task). It stays bounded by
//     steps and tokens: a task's ceiling exists to keep one investigation from
//     spending the whole turn, and the money ceiling on the root already bounds
//     what the tree as a whole may cost.
//
// MaxInputTokens and MaxOutputTokens bound the two token counts separately.
// Output tokens cost several times what input tokens do, so a single combined
// ceiling would let a large input allowance hide an output run-away — the
// expensive half — until the whole allowance was gone.
//
// Every ceiling is cumulative over the whole Process tree: a child folds its
// metrics into its parent when it terminates, so the ceiling on a root run
// covers every sub-agent it spawned.
//
// It lives under pkg/agent rather than pkg/usecase because it is a policy value
// object the runtime evaluates on the transition hot path, not a business
// operation: it reaches nothing, decides nothing about a Case, and takes no
// request context.
package budget

import (
	"context"
	"fmt"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/goerr/v2"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// Config is one agent kind's ceiling.
type Config struct {
	// MaxSteps is the greatest number of committed transitions this Process may
	// run. Must be positive.
	MaxSteps int64
	// MaxInputTokens is the greatest number of input tokens this Process may
	// consume. Must be positive.
	MaxInputTokens int64
	// MaxOutputTokens is the greatest number of output tokens this Process may
	// produce. Must be positive.
	MaxOutputTokens int64
	// NoticeRatio is the fraction of any ceiling a Process may spend before the
	// strategy is told to conclude. What is left is the reserve, which has to
	// cover the final tool call the strategy asks for AND the call that writes
	// the result from it — not one wrap-up call. Must be greater than 0 and less
	// than 1.
	NoticeRatio float64
}

// Validate enforces the required-field contract so a wiring mistake fails at
// startup rather than at the first mention.
func (c Config) Validate() error {
	if c.MaxSteps <= 0 {
		return goerr.New("max steps must be positive", goerr.V("max_steps", c.MaxSteps))
	}
	if c.MaxInputTokens <= 0 {
		return goerr.New("max input tokens must be positive",
			goerr.V("max_input_tokens", c.MaxInputTokens))
	}
	if c.MaxOutputTokens <= 0 {
		return goerr.New("max output tokens must be positive",
			goerr.V("max_output_tokens", c.MaxOutputTokens))
	}
	if c.NoticeRatio <= 0 || c.NoticeRatio >= 1 {
		return goerr.New("notice ratio must be between 0 and 1 exclusive",
			goerr.V("notice_ratio", c.NoticeRatio))
	}
	return nil
}

// Limiter returns the agentkit.Limiter that answers Strategy.Limit.
//
// The returned function is read-only, non-blocking and does no I/O, as the
// Limiter contract requires: agentkit calls it at every transition boundary and
// both before and after every LLM call, tool call and child spawn, so anything
// that acquires or waits here would be charged several times per effect and
// would turn a throttle into a lease expiry.
func (c Config) Limiter() agentkit.Limiter {
	return func(_ context.Context, _ *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
		// Every Stop is tested before any Notice. A notice threshold is always
		// the lower of the pair, so interleaving them would answer "nearly
		// exhausted" about one ceiling at the moment another was actually
		// reached, and let the run continue past it.
		switch {
		case m.Steps >= c.MaxSteps:
			return agentkit.LimitStop(fmt.Sprintf("step budget exhausted (%d/%d)", m.Steps, c.MaxSteps))
		case m.InputTokens >= c.MaxInputTokens:
			return agentkit.LimitStop(fmt.Sprintf("input token budget exhausted (%d/%d)",
				m.InputTokens, c.MaxInputTokens))
		case m.OutputTokens >= c.MaxOutputTokens:
			return agentkit.LimitStop(fmt.Sprintf("output token budget exhausted (%d/%d)",
				m.OutputTokens, c.MaxOutputTokens))
		case atNotice(m.Steps, c.MaxSteps, c.NoticeRatio):
			return agentkit.LimitNotice(fmt.Sprintf("step budget nearly exhausted (%d/%d)",
				m.Steps, c.MaxSteps))
		case atNotice(m.InputTokens, c.MaxInputTokens, c.NoticeRatio):
			return agentkit.LimitNotice(fmt.Sprintf("input token budget nearly exhausted (%d/%d)",
				m.InputTokens, c.MaxInputTokens))
		case atNotice(m.OutputTokens, c.MaxOutputTokens, c.NoticeRatio):
			return agentkit.LimitNotice(fmt.Sprintf("output token budget nearly exhausted (%d/%d)",
				m.OutputTokens, c.MaxOutputTokens))
		}
		return agentkit.LimitPass()
	}
}

func atNotice(used, ceiling int64, ratio float64) bool {
	return float64(used) >= float64(ceiling)*ratio
}

// Prefix renders the line a strategy prepends to a planner turn so the model
// can plan against what is left. It replaces the old "[budget] planner round
// n/m" line, which counted a quantity that no longer exists.
//
// Its consumer is the plan-execute strategy, which needs the full picture to
// decide how many tasks a round can afford. A ReAct run does not: it reads the
// Limit verdict instead, which already names the ceiling it is close to.
func (c Config) Prefix(m agentkit.Metrics) string {
	return fmt.Sprintf("[budget] steps %d/%d, input tokens %d/%d, output tokens %d/%d",
		m.Steps, c.MaxSteps,
		m.InputTokens, c.MaxInputTokens,
		m.OutputTokens, c.MaxOutputTokens)
}

// Root is the ceiling every Process an application entry point spawns runs
// under. Unlike Config it names no token allowance: what a root run may spend is
// an amount of money, resolved per run (see RunLimit), because the model a run
// generates through — and therefore what a token costs it — is configuration.
type Root struct {
	// MaxSteps is the greatest number of committed transitions this Process may
	// run. Must be positive. It is not a spend ceiling: it exists so a run that
	// never terminates is stopped even while it costs almost nothing.
	MaxSteps int64
	// NoticeRatio is the fraction of any ceiling a Process may spend before the
	// strategy is told to conclude. What is left is the reserve, which has to
	// cover the final tool call the strategy asks for AND the call that writes
	// the result from it — not one wrap-up call. Must be greater than 0 and less
	// than 1.
	NoticeRatio float64
}

// Validate enforces the required-field contract so a wiring mistake fails at
// startup rather than at the first run.
func (r Root) Validate() error {
	if r.MaxSteps <= 0 {
		return goerr.New("max steps must be positive", goerr.V("max_steps", r.MaxSteps))
	}
	if r.NoticeRatio <= 0 || r.NoticeRatio >= 1 {
		return goerr.New("notice ratio must be between 0 and 1 exclusive",
			goerr.V("notice_ratio", r.NoticeRatio))
	}
	return nil
}

// RunLimit is what ONE run is judged against: the money it may spend and the
// price of the model it spends it on.
//
// It is resolved per run rather than configured once because both halves come
// from the run itself — the Job that started it names its budget, and the model
// it named fixes the rate. The two travel together so a run can never be metered
// at one model's price while generating with another's.
type RunLimit struct {
	// Budget is the greatest amount this run may spend. Must be positive.
	Budget pricing.NanoUSD
	// Rate prices the run's model. Must price input and output above zero.
	Rate pricing.Rate
}

// Validate reports whether this run can be bounded at all. A run that cannot is
// stopped rather than run unbounded (see Root.Limiter).
func (l RunLimit) Validate() error {
	if l.Budget <= 0 {
		return goerr.New("budget must be positive", goerr.V("budget", int64(l.Budget)))
	}
	if !l.Rate.IsPriced() {
		return goerr.New("model rate must price input and output above zero",
			goerr.V("input", int64(l.Rate.Input)), goerr.V("output", int64(l.Rate.Output)))
	}
	return nil
}

// LimitResolver answers "what is this run judged against". The caller supplies
// it because resolving a run's model and budget means reading its metadata,
// which is the kernel's vocabulary, not this package's.
//
// It must be read-only, non-blocking and free of I/O for the same reason the
// Limiter is: it runs inside the Limiter.
type LimitResolver func(proc *agentkit.Process) RunLimit

// Limiter returns the agentkit.Limiter that answers Strategy.Limit for a root
// run.
//
// The returned function is read-only, non-blocking and does no I/O, as the
// Limiter contract requires: agentkit calls it at every transition boundary and
// both before and after every LLM call, tool call and child spawn, so anything
// that acquires or waits here would be charged several times per effect and
// would turn a throttle into a lease expiry.
func (r Root) Limiter(resolve LimitResolver) agentkit.Limiter {
	return func(_ context.Context, proc *agentkit.Process, m agentkit.Metrics) agentkit.LimitDecision {
		// A run whose budget or price cannot be resolved is STOPPED, not run
		// unbounded. Startup validation makes this unreachable for a configured
		// deployment, so reaching it means the run's metadata says something this
		// build cannot price — and spending money on it is the one outcome that
		// cannot be undone.
		if resolve == nil {
			return agentkit.LimitStop("this run has no priced budget")
		}
		limit := resolve(proc)
		if err := limit.Validate(); err != nil {
			return agentkit.LimitStop("this run has no priced budget")
		}

		spent := limit.Rate.Cost(m.InputTokens, m.OutputTokens,
			m.CacheReadInputTokens, m.CacheCreationInputTokens)

		// Every Stop is tested before any Notice. A notice threshold is always
		// the lower of the pair, so interleaving them would answer "nearly
		// exhausted" about one ceiling at the moment another was actually
		// reached, and let the run continue past it.
		switch {
		case m.Steps >= r.MaxSteps:
			return agentkit.LimitStop(fmt.Sprintf("step budget exhausted (%d/%d)", m.Steps, r.MaxSteps))
		case spent >= limit.Budget:
			return agentkit.LimitStop(fmt.Sprintf("cost budget exhausted (%s/%s)",
				spent.USD(), limit.Budget.USD()))
		case atNotice(m.Steps, r.MaxSteps, r.NoticeRatio):
			return agentkit.LimitNotice(fmt.Sprintf("step budget nearly exhausted (%d/%d)",
				m.Steps, r.MaxSteps))
		case atNotice(int64(spent), int64(limit.Budget), r.NoticeRatio):
			return agentkit.LimitNotice(fmt.Sprintf("cost budget nearly exhausted (%s/%s)",
				spent.USD(), limit.Budget.USD()))
		}
		return agentkit.LimitPass()
	}
}
