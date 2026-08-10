package budget_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
)

func TestConfigValidate(t *testing.T) {
	testCases := map[string]struct {
		cfg     budget.Config
		wantErr bool
	}{
		"valid": {
			cfg: budget.Config{MaxSteps: 64, MaxTokens: 1000, NoticeRatio: 0.8},
		},
		"zero steps": {
			cfg:     budget.Config{MaxSteps: 0, MaxTokens: 1000, NoticeRatio: 0.8},
			wantErr: true,
		},
		"negative steps": {
			cfg:     budget.Config{MaxSteps: -1, MaxTokens: 1000, NoticeRatio: 0.8},
			wantErr: true,
		},
		"zero tokens": {
			cfg:     budget.Config{MaxSteps: 64, MaxTokens: 0, NoticeRatio: 0.8},
			wantErr: true,
		},
		"ratio at zero": {
			cfg:     budget.Config{MaxSteps: 64, MaxTokens: 1000, NoticeRatio: 0},
			wantErr: true,
		},
		"ratio at one": {
			cfg:     budget.Config{MaxSteps: 64, MaxTokens: 1000, NoticeRatio: 1},
			wantErr: true,
		},
		"ratio above one": {
			cfg:     budget.Config{MaxSteps: 64, MaxTokens: 1000, NoticeRatio: 1.5},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestConfigLimiter(t *testing.T) {
	cfg := budget.Config{MaxSteps: 10, MaxTokens: 1000, NoticeRatio: 0.8}
	limit := cfg.Limiter()
	ctx := context.Background()

	testCases := map[string]struct {
		metrics     agentkit.Metrics
		wantKind    agentkit.LimitKind
		wantMessage string
	}{
		"well within both ceilings": {
			metrics:  agentkit.Metrics{Steps: 1, InputTokens: 10, OutputTokens: 10},
			wantKind: agentkit.LimitKindPass,
		},
		"just below both notice thresholds": {
			metrics:  agentkit.Metrics{Steps: 7, InputTokens: 400, OutputTokens: 399},
			wantKind: agentkit.LimitKindPass,
		},
		"step notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 8, InputTokens: 0, OutputTokens: 0},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "step budget nearly exhausted (8/10)",
		},
		"token notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 500, OutputTokens: 300},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "token budget nearly exhausted (800/1000)",
		},
		"step ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 10},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (10/10)",
		},
		"step ceiling exceeded": {
			metrics:     agentkit.Metrics{Steps: 11},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (11/10)",
		},
		"token ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 600, OutputTokens: 400},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "token budget exhausted (1000/1000)",
		},
		// Both thresholds are crossed at once: Stop must win, otherwise a run
		// that hit its ceiling would be told merely to wrap up and keep going.
		"both ceilings crossed reports stop": {
			metrics:     agentkit.Metrics{Steps: 10, InputTokens: 900, OutputTokens: 200},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (10/10)",
		},
		// The token ceiling alone must stop even while the step count is only at
		// its notice threshold.
		"token stop outranks step notice": {
			metrics:     agentkit.Metrics{Steps: 8, InputTokens: 900, OutputTokens: 200},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "token budget exhausted (1100/1000)",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := limit(ctx, nil, tc.metrics)
			gt.Value(t, got.Kind()).Equal(tc.wantKind)
			gt.String(t, got.Message()).Equal(tc.wantMessage)
		})
	}
}

// TestConfigLimiterCountsChildMetrics pins that the ceiling covers the whole
// tree. agentkit folds a child's metrics into its parent when the child
// terminates, so the parent's Limit sees them and must act on them.
func TestConfigLimiterCountsChildMetrics(t *testing.T) {
	cfg := budget.Config{MaxSteps: 100, MaxTokens: 1000, NoticeRatio: 0.8}
	limit := cfg.Limiter()

	own := agentkit.Metrics{Steps: 3, InputTokens: 100, OutputTokens: 50}
	gt.Value(t, limit(context.Background(), nil, own).Kind()).Equal(agentkit.LimitKindPass)

	// Same Process after two children folded 500 tokens each into it.
	withChildren := agentkit.Metrics{Steps: 3, InputTokens: 900, OutputTokens: 150}
	gt.Value(t, limit(context.Background(), nil, withChildren).Kind()).Equal(agentkit.LimitKindStop)
}

func TestConfigPrefix(t *testing.T) {
	cfg := budget.Config{MaxSteps: 64, MaxTokens: 1500000, NoticeRatio: 0.8}
	got := cfg.Prefix(agentkit.Metrics{Steps: 12, InputTokens: 3000, OutputTokens: 400})
	gt.String(t, got).Equal("[budget] steps 12/64, tokens 3400/1500000")
}
