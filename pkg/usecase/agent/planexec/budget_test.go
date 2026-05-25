package planexec_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/usecase/agent/planexec"
)

func TestBudgetConfig_Validate_OK(t *testing.T) {
	cfg := planexec.BudgetConfig{
		PlannerLoopMax:     8,
		SubAgentMaxPerTurn: 16,
		SubAgentLoopMax:    20,
	}
	gt.NoError(t, cfg.Validate())
}

func TestBudgetConfig_Validate_RejectsNonPositive(t *testing.T) {
	bases := planexec.BudgetConfig{
		PlannerLoopMax:     8,
		SubAgentMaxPerTurn: 16,
		SubAgentLoopMax:    20,
	}

	cases := []struct {
		name   string
		mutate func(*planexec.BudgetConfig)
	}{
		{"planner zero", func(c *planexec.BudgetConfig) { c.PlannerLoopMax = 0 }},
		{"planner negative", func(c *planexec.BudgetConfig) { c.PlannerLoopMax = -1 }},
		{"subagent max zero", func(c *planexec.BudgetConfig) { c.SubAgentMaxPerTurn = 0 }},
		{"subagent loop zero", func(c *planexec.BudgetConfig) { c.SubAgentLoopMax = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := bases
			tc.mutate(&cfg)
			gt.Error(t, cfg.Validate())
		})
	}
}

func TestBudgetConfig_Validate_NilReceiver(t *testing.T) {
	var cfg *planexec.BudgetConfig
	gt.Error(t, cfg.Validate())
}
