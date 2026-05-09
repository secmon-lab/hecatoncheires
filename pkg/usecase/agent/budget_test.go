package agent_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func TestBudget_CanPlannerCall(t *testing.T) {
	t.Run("under cap allows another call", func(t *testing.T) {
		b := agent.NewBudget(8, 16, 20)
		b.PlannerUsed = 7
		gt.Bool(t, b.CanPlannerCall()).True()
	})
	t.Run("at cap rejects further calls", func(t *testing.T) {
		b := agent.NewBudget(8, 16, 20)
		b.PlannerUsed = 8
		gt.Bool(t, b.CanPlannerCall()).False()
	})
	t.Run("zero used at fresh budget allows call", func(t *testing.T) {
		b := agent.NewBudget(8, 16, 20)
		gt.Bool(t, b.CanPlannerCall()).True()
	})
}

func TestBudget_SubAgentRemaining(t *testing.T) {
	t.Run("returns difference when within cap", func(t *testing.T) {
		b := agent.NewBudget(8, 16, 20)
		b.SubAgentUsed = 5
		gt.Number(t, b.SubAgentRemaining()).Equal(11)
	})
	t.Run("clamps to zero past cap", func(t *testing.T) {
		b := agent.NewBudget(8, 16, 20)
		b.SubAgentUsed = 20
		gt.Number(t, b.SubAgentRemaining()).Equal(0)
	})
}

func TestBudget_FormatPrefix(t *testing.T) {
	b := agent.NewBudget(8, 16, 20)
	b.PlannerUsed = 3
	b.SubAgentUsed = 5
	gt.String(t, b.FormatPrefix()).Equal("[budget] planner 3/8 — investigations 5/16")
}
