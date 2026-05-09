package agent

import "fmt"

// Budget tracks per-turn LLM call ceilings. It is constructed by each turn
// (NewBudget) and updated by the turn driver after every planner / sub-agent
// call. The struct is consulted in two places:
//
//   - planner driver (`canPlannerCall` / FormatPrefix → user input)
//   - investigation scheduler (`SubAgentRemaining` to reject over-allocation)
//
// All fields are caller-controlled so the surrounding code can vary the
// limits per environment (CLI flags) without re-importing constants.
type Budget struct {
	PlannerMax      int
	PlannerUsed     int
	SubAgentMax     int
	SubAgentUsed    int
	SubAgentLoopMax int
}

// NewBudget builds a fresh Budget snapshot for a single turn.
func NewBudget(plannerMax, subAgentMax, subAgentLoopMax int) *Budget {
	return &Budget{
		PlannerMax:      plannerMax,
		SubAgentMax:     subAgentMax,
		SubAgentLoopMax: subAgentLoopMax,
	}
}

// CanPlannerCall reports whether one more planner LLM call is within budget.
func (b *Budget) CanPlannerCall() bool { return b.PlannerUsed < b.PlannerMax }

// SubAgentRemaining is the number of sub-agent slots still available this turn.
func (b *Budget) SubAgentRemaining() int {
	r := b.SubAgentMax - b.SubAgentUsed
	if r < 0 {
		return 0
	}
	return r
}

// FormatPrefix renders the prefix line that gets prepended to every planner
// LLM user input so the LLM can plan against the current budget state.
//
// Example: "[budget] planner 3/8 — investigations 5/16".
func (b *Budget) FormatPrefix() string {
	return fmt.Sprintf("[budget] planner %d/%d — investigations %d/%d",
		b.PlannerUsed, b.PlannerMax, b.SubAgentUsed, b.SubAgentMax)
}
