package planexec

import (
	"fmt"

	"github.com/m-mizutani/goerr/v2"
)

// BudgetConfig is the per-turn LLM call ceiling configuration handed to the
// Runner. All values are caller-controlled so the surrounding wiring (CLI
// flags) can vary them per environment without re-importing constants.
type BudgetConfig struct {
	// PlannerLoopMax bounds the number of planner (or replan) LLM
	// invocations within one Runner.Run.
	PlannerLoopMax int

	// SubAgentMaxPerTurn bounds the total number of sub-agent tasks
	// (TaskPlan entries) that may run within one Runner.Run, summed
	// across every executePhase round.
	SubAgentMaxPerTurn int

	// SubAgentLoopMax bounds the inner gollem loop limit of each
	// sub-agent (passed through to gollem.WithLoopLimit).
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
	if c.SubAgentMaxPerTurn <= 0 {
		return goerr.New("sub-agent max per turn must be positive",
			goerr.V("sub_agent_max_per_turn", c.SubAgentMaxPerTurn))
	}
	if c.SubAgentLoopMax <= 0 {
		return goerr.New("sub-agent loop max must be positive",
			goerr.V("sub_agent_loop_max", c.SubAgentLoopMax))
	}
	return nil
}

// budget tracks a single Runner.Run's progress against a BudgetConfig.
// Unexported because callers should only construct via newBudget; the
// counters mutate as the planner / phase loop advances.
type budget struct {
	plannerMax      int
	plannerUsed     int
	subAgentMax     int
	subAgentUsed    int
	subAgentLoopMax int
}

func newBudget(cfg BudgetConfig) *budget {
	return &budget{
		plannerMax:      cfg.PlannerLoopMax,
		subAgentMax:     cfg.SubAgentMaxPerTurn,
		subAgentLoopMax: cfg.SubAgentLoopMax,
	}
}

// canPlannerCall reports whether one more planner LLM call fits the budget.
func (b *budget) canPlannerCall() bool { return b.plannerUsed < b.plannerMax }

// subAgentRemaining is the number of sub-agent slots still available.
func (b *budget) subAgentRemaining() int {
	r := b.subAgentMax - b.subAgentUsed
	if r < 0 {
		return 0
	}
	return r
}

// formatPrefix renders the prefix line prepended to every planner LLM user
// input so the LLM can plan against the current budget state.
//
// Example: "[budget] planner 3/8 — investigations 5/16".
func (b *budget) formatPrefix() string {
	return fmt.Sprintf("[budget] planner %d/%d — investigations %d/%d",
		b.plannerUsed, b.plannerMax, b.subAgentUsed, b.subAgentMax)
}
