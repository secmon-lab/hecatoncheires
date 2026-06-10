package agent_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent"
)

func TestBudget_CanPlannerCall(t *testing.T) {
	t.Run("under cap allows another call", func(t *testing.T) {
		b := agent.NewBudget(8, 20)
		b.PlannerUsed = 7
		gt.Bool(t, b.CanPlannerCall()).True()
	})
	t.Run("at cap rejects further calls", func(t *testing.T) {
		b := agent.NewBudget(8, 20)
		b.PlannerUsed = 8
		gt.Bool(t, b.CanPlannerCall()).False()
	})
	t.Run("zero used at fresh budget allows call", func(t *testing.T) {
		b := agent.NewBudget(8, 20)
		gt.Bool(t, b.CanPlannerCall()).True()
	})
}

func TestBudget_FormatPrefix(t *testing.T) {
	b := agent.NewBudget(8, 20)
	b.PlannerUsed = 3
	gt.String(t, b.FormatPrefix()).Equal("[budget] planner round 3/8")
}
