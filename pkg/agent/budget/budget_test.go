package budget_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
)

func validConfig() budget.Config {
	return budget.Config{MaxSteps: 10, MaxInputTokens: 1000, MaxOutputTokens: 200, NoticeRatio: 0.8}
}

func TestConfigValidate(t *testing.T) {
	testCases := map[string]struct {
		mutate  func(budget.Config) budget.Config
		wantErr bool
	}{
		"valid": {
			mutate: func(c budget.Config) budget.Config { return c },
		},
		"zero steps": {
			mutate:  func(c budget.Config) budget.Config { c.MaxSteps = 0; return c },
			wantErr: true,
		},
		"negative steps": {
			mutate:  func(c budget.Config) budget.Config { c.MaxSteps = -1; return c },
			wantErr: true,
		},
		"zero input tokens": {
			mutate:  func(c budget.Config) budget.Config { c.MaxInputTokens = 0; return c },
			wantErr: true,
		},
		"zero output tokens": {
			mutate:  func(c budget.Config) budget.Config { c.MaxOutputTokens = 0; return c },
			wantErr: true,
		},
		"ratio at zero": {
			mutate:  func(c budget.Config) budget.Config { c.NoticeRatio = 0; return c },
			wantErr: true,
		},
		"ratio at one": {
			mutate:  func(c budget.Config) budget.Config { c.NoticeRatio = 1; return c },
			wantErr: true,
		},
		"ratio above one": {
			mutate:  func(c budget.Config) budget.Config { c.NoticeRatio = 1.5; return c },
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.mutate(validConfig()).Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestConfigLimiter(t *testing.T) {
	limit := validConfig().Limiter()
	ctx := context.Background()

	testCases := map[string]struct {
		metrics     agentkit.Metrics
		wantKind    agentkit.LimitKind
		wantMessage string
	}{
		"well within every ceiling": {
			metrics:  agentkit.Metrics{Steps: 1, InputTokens: 10, OutputTokens: 10},
			wantKind: agentkit.LimitKindPass,
		},
		"just below every notice threshold": {
			metrics:  agentkit.Metrics{Steps: 7, InputTokens: 799, OutputTokens: 159},
			wantKind: agentkit.LimitKindPass,
		},
		"step notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 8},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "step budget nearly exhausted (8/10)",
		},
		"input notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 800},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "input token budget nearly exhausted (800/1000)",
		},
		"output notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 1, OutputTokens: 160},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "output token budget nearly exhausted (160/200)",
		},
		"step ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 10},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (10/10)",
		},
		"input ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 1000},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "input token budget exhausted (1000/1000)",
		},
		"output ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 1, OutputTokens: 200},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "output token budget exhausted (200/200)",
		},
		// The output ceiling is the expensive one. A run that is only near its
		// step and input thresholds but has spent its output allowance must be
		// stopped, not merely told to wrap up.
		"output stop outranks step and input notices": {
			metrics:     agentkit.Metrics{Steps: 8, InputTokens: 900, OutputTokens: 200},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "output token budget exhausted (200/200)",
		},
		"every ceiling crossed reports the step one first": {
			metrics:     agentkit.Metrics{Steps: 10, InputTokens: 1000, OutputTokens: 200},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (10/10)",
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

// TestOutputCeilingIsNotHiddenByInputHeadroom is the reason the two token counts
// are bounded separately. Under one combined ceiling this run would still look
// comfortable, and the expensive half would keep growing.
func TestOutputCeilingIsNotHiddenByInputHeadroom(t *testing.T) {
	limit := budget.Config{
		MaxSteps: 1000, MaxInputTokens: 500_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8,
	}.Limiter()

	m := agentkit.Metrics{Steps: 5, InputTokens: 20_000, OutputTokens: 100_000}
	got := limit(context.Background(), nil, m)

	gt.Value(t, got.Kind()).Equal(agentkit.LimitKindStop)
	gt.String(t, got.Message()).Contains("output token budget exhausted")
}

// TestConfigLimiterCountsChildMetrics pins that the ceiling covers the whole
// tree. agentkit folds a child's metrics into its parent when the child
// terminates, so the parent's Limit sees them and must act on them.
func TestConfigLimiterCountsChildMetrics(t *testing.T) {
	limit := budget.Config{
		MaxSteps: 100, MaxInputTokens: 1000, MaxOutputTokens: 1000, NoticeRatio: 0.8,
	}.Limiter()

	own := agentkit.Metrics{Steps: 3, InputTokens: 100, OutputTokens: 50}
	gt.Value(t, limit(context.Background(), nil, own).Kind()).Equal(agentkit.LimitKindPass)

	// Same Process after two children folded their usage into it.
	withChildren := agentkit.Metrics{Steps: 3, InputTokens: 1000, OutputTokens: 150}
	gt.Value(t, limit(context.Background(), nil, withChildren).Kind()).Equal(agentkit.LimitKindStop)
}

func TestConfigPrefix(t *testing.T) {
	cfg := budget.Config{
		MaxSteps: 64, MaxInputTokens: 500_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8,
	}
	got := cfg.Prefix(agentkit.Metrics{Steps: 12, InputTokens: 3000, OutputTokens: 400})
	gt.String(t, got).Equal("[budget] steps 12/64, input tokens 3000/500000, output tokens 400/100000")
}
