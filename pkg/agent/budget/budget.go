// Package budget turns an operator-configured ceiling into the
// agentkit.Limiter a Strategy answers Limit with.
//
// There are exactly two ceilings, and either one ends the run:
//
//   - MaxSteps bounds committed transitions. It is the successor of the old
//     planner-round and sub-agent-loop counters: agentkit counts every
//     transition on Process.Metrics.Steps, which Limit can see, whereas a
//     round counter lives in strategy state, which it cannot.
//   - MaxTokens bounds input+output tokens.
//
// Both are cumulative over the whole Process tree: a child folds its metrics
// into its parent when it terminates, so the ceiling on a root run covers every
// sub-agent it spawned.
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
	// MaxTokens is the greatest number of input+output tokens this Process may
	// consume. Must be positive.
	MaxTokens int64
	// NoticeRatio is the fraction of either ceiling at which the strategy is
	// told to wrap up while it still has room to produce an answer. Must be
	// greater than 0 and less than 1.
	NoticeRatio float64
}

// Validate enforces the required-field contract so a wiring mistake fails at
// startup rather than at the first mention.
func (c Config) Validate() error {
	if c.MaxSteps <= 0 {
		return goerr.New("max steps must be positive", goerr.V("max_steps", c.MaxSteps))
	}
	if c.MaxTokens <= 0 {
		return goerr.New("max tokens must be positive", goerr.V("max_tokens", c.MaxTokens))
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
		tokens := m.InputTokens + m.OutputTokens

		// Stop is tested before Notice. The notice threshold is always the lower
		// of the two, so reversing the order would answer "nearly exhausted" at
		// the moment the ceiling is actually reached and let the run continue.
		switch {
		case m.Steps >= c.MaxSteps:
			return agentkit.LimitStop(fmt.Sprintf("step budget exhausted (%d/%d)", m.Steps, c.MaxSteps))
		case tokens >= c.MaxTokens:
			return agentkit.LimitStop(fmt.Sprintf("token budget exhausted (%d/%d)", tokens, c.MaxTokens))
		case float64(m.Steps) >= float64(c.MaxSteps)*c.NoticeRatio:
			return agentkit.LimitNotice(fmt.Sprintf("step budget nearly exhausted (%d/%d)", m.Steps, c.MaxSteps))
		case float64(tokens) >= float64(c.MaxTokens)*c.NoticeRatio:
			return agentkit.LimitNotice(fmt.Sprintf("token budget nearly exhausted (%d/%d)", tokens, c.MaxTokens))
		}
		return agentkit.LimitPass()
	}
}

// Prefix renders the line a strategy prepends to a planner turn so the model
// can plan against what is left. It replaces the old "[budget] planner round
// n/m" line, which counted a quantity that no longer exists.
func (c Config) Prefix(m agentkit.Metrics) string {
	return fmt.Sprintf("[budget] steps %d/%d, tokens %d/%d",
		m.Steps, c.MaxSteps, m.InputTokens+m.OutputTokens, c.MaxTokens)
}
