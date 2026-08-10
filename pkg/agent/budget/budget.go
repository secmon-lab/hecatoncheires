// Package budget turns an operator-configured ceiling into the
// agentkit.Limiter a Strategy answers Limit with.
//
// There are three ceilings, and any one of them ends the run:
//
//   - MaxSteps bounds committed transitions. It is the successor of the old
//     planner-round and sub-agent-loop counters: agentkit counts every
//     transition on Process.Metrics.Steps, which Limit can see, whereas a
//     round counter lives in strategy state, which it cannot.
//   - MaxInputTokens and MaxOutputTokens bound the two token counts
//     separately. Output tokens cost several times what input tokens do, so a
//     single combined ceiling would let a large input allowance hide an output
//     run-away — the expensive half — until the whole budget was gone.
//
// All three are cumulative over the whole Process tree: a child folds its
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
	// NoticeRatio is the fraction of any ceiling at which the strategy is told
	// to wrap up while it still has room to produce an answer. Must be greater
	// than 0 and less than 1.
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
