package planexec

import (
	"fmt"

	"github.com/m-mizutani/goerr/v2"
)

// BudgetConfig is the per-turn LLM call ceiling configuration handed to the
// Runner. All values are caller-controlled so the surrounding wiring (CLI
// flags) can vary them per environment without re-importing constants.
//
// The budget model is two controls — there is deliberately NO running
// "total sub-agent task count" across a turn (see
// .claude/rules/architecture.md → "Agent runtime vocabulary" / "Budget"):
//
//   - PlannerLoopMax bounds the number of rounds in a turn.
//   - SubAgentLoopMax is the per-sub-agent budget, granted fresh every
//     round (so the sub-agent budget recovers per round).
//
// Per-round fan-out is already bounded by plan validation (≤ maxTasksPerPhase
// tasks per phase), so total sub-agent work is naturally bounded by
// PlannerLoopMax × maxTasksPerPhase × SubAgentLoopMax without a separate
// total cap.
type BudgetConfig struct {
	// PlannerLoopMax bounds the number of planner (or replan) LLM
	// invocations (rounds) within one run. Planner / replan output that fails
	// validation is retried within the same pool. (Final-output regeneration in
	// Run[T] is bounded separately by finalOutputMaxRetry and does NOT consume
	// planner rounds.)
	PlannerLoopMax int

	// SubAgentLoopMax bounds the inner gollem loop limit of each
	// sub-agent (passed through to gollem.WithLoopLimit). Granted fresh to
	// every sub-agent, so it recovers per round.
	SubAgentLoopMax int
}

// Validate enforces required-field invariants for BudgetConfig. Every value
// must be positive; zero / negative budgets would silently produce no work.
func (c *BudgetConfig) Validate() error {
	if c == nil {
		return goerr.New("budget config is nil")
	}
	if c.PlannerLoopMax <= 0 {
		return goerr.New("planner loop max must be positive",
			goerr.V("planner_loop_max", c.PlannerLoopMax))
	}
	if c.SubAgentLoopMax <= 0 {
		return goerr.New("sub-agent loop max must be positive",
			goerr.V("sub_agent_loop_max", c.SubAgentLoopMax))
	}
	return nil
}

// budget tracks a single Runner.Run's progress against a BudgetConfig.
// Unexported because callers should only construct via newBudget; the
// counter mutates as the planner loop advances.
type budget struct {
	plannerMax      int
	plannerUsed     int
	subAgentLoopMax int
}

func newBudget(cfg BudgetConfig) *budget {
	return &budget{
		plannerMax:      cfg.PlannerLoopMax,
		subAgentLoopMax: cfg.SubAgentLoopMax,
	}
}

// canPlannerCall reports whether one more planner LLM call fits the budget.
func (b *budget) canPlannerCall() bool { return b.plannerUsed < b.plannerMax }

// formatPrefix renders the prefix line prepended to every planner LLM user
// input so the LLM can plan against the current budget state.
//
// Example: "[budget] planner round 3/8".
func (b *budget) formatPrefix() string {
	return fmt.Sprintf("[budget] planner round %d/%d", b.plannerUsed, b.plannerMax)
}
