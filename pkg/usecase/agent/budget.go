package agent

import "fmt"

// Budget tracks per-turn LLM call ceilings. It is constructed by each turn
// (NewBudget) and updated by the turn driver after every planner call.
//
// The budget model is two controls — there is deliberately NO running
// "total sub-agent task count" across a turn (see
// .claude/rules/architecture.md → "Agent runtime vocabulary" / "Budget"):
//
//   - PlannerMax bounds the number of rounds in a turn.
//   - SubAgentLoopMax is the per-sub-agent budget, granted fresh every
//     round (so the sub-agent budget recovers per round).
//
// Per-round fan-out is bounded by plan validation, so total sub-agent work
// is naturally bounded without a separate per-turn total cap.
//
// All fields are caller-controlled so the surrounding code can vary the
// limits per environment (CLI flags) without re-importing constants.
type Budget struct {
	PlannerMax      int
	PlannerUsed     int
	SubAgentLoopMax int
}

// NewBudget builds a fresh Budget snapshot for a single turn.
func NewBudget(plannerMax, subAgentLoopMax int) *Budget {
	return &Budget{
		PlannerMax:      plannerMax,
		SubAgentLoopMax: subAgentLoopMax,
	}
}

// CanPlannerCall reports whether one more planner LLM call is within budget.
func (b *Budget) CanPlannerCall() bool { return b.PlannerUsed < b.PlannerMax }

// FormatPrefix renders the prefix line that gets prepended to every planner
// LLM user input so the LLM can plan against the current budget state.
//
// Example: "[budget] planner round 3/8".
func (b *Budget) FormatPrefix() string {
	return fmt.Sprintf("[budget] planner round %d/%d", b.PlannerUsed, b.PlannerMax)
}
