package budget_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
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

func validRoot() budget.Root {
	return budget.Root{MaxSteps: 100, NoticeRatio: 0.8}
}

// opusRate is $5 / $25 / $0.50 / $6.25 per MTok, in NanoUSD per token.
var opusRate = pricing.Rate{Input: 5000, Output: 25000, CacheRead: 500, CacheWrite: 6250}

func TestRootValidate(t *testing.T) {
	testCases := map[string]struct {
		mutate  func(budget.Root) budget.Root
		wantErr bool
	}{
		"valid": {
			mutate: func(r budget.Root) budget.Root { return r },
		},
		"zero steps": {
			mutate:  func(r budget.Root) budget.Root { r.MaxSteps = 0; return r },
			wantErr: true,
		},
		"negative steps": {
			mutate:  func(r budget.Root) budget.Root { r.MaxSteps = -1; return r },
			wantErr: true,
		},
		"ratio at zero": {
			mutate:  func(r budget.Root) budget.Root { r.NoticeRatio = 0; return r },
			wantErr: true,
		},
		"ratio at one": {
			mutate:  func(r budget.Root) budget.Root { r.NoticeRatio = 1; return r },
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.mutate(validRoot()).Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestRunLimitValidate(t *testing.T) {
	testCases := map[string]struct {
		limit   budget.RunLimit
		wantErr bool
	}{
		"valid":       {limit: budget.RunLimit{Budget: pricing.FromUSD(1), Rate: opusRate}},
		"zero budget": {limit: budget.RunLimit{Rate: opusRate}, wantErr: true},
		"negative budget": {
			limit:   budget.RunLimit{Budget: -1, Rate: opusRate},
			wantErr: true,
		},
		"unpriced rate": {
			limit:   budget.RunLimit{Budget: pricing.FromUSD(1)},
			wantErr: true,
		},
		"input priced at nothing": {
			limit:   budget.RunLimit{Budget: pricing.FromUSD(1), Rate: pricing.Rate{Output: 25000}},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.limit.Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestRootLimiter(t *testing.T) {
	// $1.00 budget on the opus rate: 200,000 input tokens or 40,000 output
	// tokens would spend it exactly.
	limit := validRoot().Limiter(func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{Budget: pricing.FromUSD(1), Rate: opusRate}
	})
	ctx := context.Background()

	testCases := map[string]struct {
		metrics     agentkit.Metrics
		wantKind    agentkit.LimitKind
		wantMessage string
	}{
		"well within every ceiling": {
			metrics:  agentkit.Metrics{Steps: 1, InputTokens: 1000, OutputTokens: 100},
			wantKind: agentkit.LimitKindPass,
		},
		"just below the notice thresholds": {
			metrics:  agentkit.Metrics{Steps: 79, InputTokens: 100_000, OutputTokens: 11_000},
			wantKind: agentkit.LimitKindPass,
		},
		"step notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 80},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "step budget nearly exhausted (80/100)",
		},
		"cost notice threshold reached": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 160_000},
			wantKind:    agentkit.LimitKindNotice,
			wantMessage: "cost budget nearly exhausted ($0.80/$1.00)",
		},
		"step ceiling reached": {
			metrics:     agentkit.Metrics{Steps: 100},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (100/100)",
		},
		"cost ceiling reached by input": {
			metrics:     agentkit.Metrics{Steps: 1, InputTokens: 200_000},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "cost budget exhausted ($1.00/$1.00)",
		},
		"cost ceiling reached by output": {
			metrics:     agentkit.Metrics{Steps: 1, OutputTokens: 40_000},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "cost budget exhausted ($1.00/$1.00)",
		},
		"cost stop outranks the step notice": {
			metrics:     agentkit.Metrics{Steps: 80, InputTokens: 200_000},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "cost budget exhausted ($1.00/$1.00)",
		},
		"both ceilings crossed reports the step one first": {
			metrics:     agentkit.Metrics{Steps: 100, InputTokens: 200_000},
			wantKind:    agentkit.LimitKindStop,
			wantMessage: "step budget exhausted (100/100)",
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

// TestRootLimiterPricesCacheSeparately is the reason the ceiling is money rather
// than tokens: the same token counts cost different amounts depending on how
// much of the input was served from cache.
func TestRootLimiterPricesCacheSeparately(t *testing.T) {
	limit := validRoot().Limiter(func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{Budget: pricing.FromUSD(1), Rate: opusRate}
	})
	ctx := context.Background()

	// 200,000 input tokens spend the whole budget at the uncached rate...
	uncached := agentkit.Metrics{Steps: 1, InputTokens: 200_000}
	gt.Value(t, limit(ctx, nil, uncached).Kind()).Equal(agentkit.LimitKindStop)

	// ...but the same count served from cache costs a tenth of that.
	cached := agentkit.Metrics{Steps: 1, InputTokens: 200_000, CacheReadInputTokens: 200_000}
	gt.Value(t, limit(ctx, nil, cached).Kind()).Equal(agentkit.LimitKindPass)
}

// TestRootLimiterStopsAnUnpricedRun pins the fail-closed behaviour: a run whose
// budget or price cannot be resolved is stopped rather than run unbounded.
func TestRootLimiterStopsAnUnpricedRun(t *testing.T) {
	ctx := context.Background()
	m := agentkit.Metrics{Steps: 1}

	testCases := map[string]budget.LimitResolver{
		"no resolver": nil,
		"zero budget": func(*agentkit.Process) budget.RunLimit {
			return budget.RunLimit{Rate: opusRate}
		},
		"unpriced model": func(*agentkit.Process) budget.RunLimit {
			return budget.RunLimit{Budget: pricing.FromUSD(1)}
		},
	}

	for name, resolve := range testCases {
		t.Run(name, func(t *testing.T) {
			got := validRoot().Limiter(resolve)(ctx, nil, m)
			gt.Value(t, got.Kind()).Equal(agentkit.LimitKindStop)
			gt.String(t, got.Message()).Equal("this run has no priced budget")
		})
	}
}

// TestRootLimiterCountsChildSpend pins that the money ceiling covers the whole
// tree. agentkit folds a child's metrics into its parent when the child
// terminates, so the parent's Limit sees them and must act on them.
func TestRootLimiterCountsChildSpend(t *testing.T) {
	limit := validRoot().Limiter(func(*agentkit.Process) budget.RunLimit {
		return budget.RunLimit{Budget: pricing.FromUSD(1), Rate: opusRate}
	})
	ctx := context.Background()

	own := agentkit.Metrics{Steps: 3, InputTokens: 10_000, OutputTokens: 500}
	gt.Value(t, limit(ctx, nil, own).Kind()).Equal(agentkit.LimitKindPass)

	// Same Process after two children folded their usage into it.
	withChildren := agentkit.Metrics{Steps: 3, InputTokens: 150_000, OutputTokens: 12_000}
	gt.Value(t, limit(ctx, nil, withChildren).Kind()).Equal(agentkit.LimitKindStop)
}

// TestRootLimiterReadsTheProcess pins that the resolver is given the Process, so
// one registered limiter can judge each run against its own budget and model.
func TestRootLimiterReadsTheProcess(t *testing.T) {
	limit := validRoot().Limiter(func(proc *agentkit.Process) budget.RunLimit {
		if proc != nil && proc.Metadata["tier"] == "cheap" {
			return budget.RunLimit{Budget: pricing.FromUSD(0.10), Rate: opusRate}
		}
		return budget.RunLimit{Budget: pricing.FromUSD(10), Rate: opusRate}
	})
	ctx := context.Background()
	m := agentkit.Metrics{Steps: 1, InputTokens: 100_000} // $0.50

	cheap := &agentkit.Process{Metadata: map[string]string{"tier": "cheap"}}
	gt.Value(t, limit(ctx, cheap, m).Kind()).Equal(agentkit.LimitKindStop)

	rich := &agentkit.Process{Metadata: map[string]string{"tier": "rich"}}
	gt.Value(t, limit(ctx, rich, m).Kind()).Equal(agentkit.LimitKindPass)
}

func TestConfigPrefix(t *testing.T) {
	cfg := budget.Config{
		MaxSteps: 64, MaxInputTokens: 500_000, MaxOutputTokens: 100_000, NoticeRatio: 0.8,
	}
	got := cfg.Prefix(agentkit.Metrics{Steps: 12, InputTokens: 3000, OutputTokens: 400})
	gt.String(t, got).Equal("[budget] steps 12/64, input tokens 3000/500000, output tokens 400/100000")
}
