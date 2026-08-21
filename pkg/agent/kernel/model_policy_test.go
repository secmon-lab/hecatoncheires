package kernel_test

import (
	"context"
	"testing"

	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// opusRate and flashRate stand in for an expensive and a cheap model. The whole
// point of pricing a run rather than counting its tokens is that these two spend
// a budget at very different speeds.
var (
	opusRate  = pricing.Rate{Input: 5000, Output: 25000, CacheRead: 500, CacheWrite: 6250}
	flashRate = pricing.Rate{Input: 750, Output: 3750, CacheRead: 75}
)

func modelDef(ref, model string, rate pricing.Rate) kernel.ModelDef {
	return kernel.ModelDef{
		Ref: ref, Provider: kernel.ProviderClaude, Model: model, Rate: rate,
	}
}

func TestModelDefValidate(t *testing.T) {
	testCases := map[string]struct {
		def     kernel.ModelDef
		wantErr bool
	}{
		"valid": {def: modelDef("main", "claude-opus-5", opusRate)},
		"no reference name": {
			def:     kernel.ModelDef{Provider: kernel.ProviderClaude, Model: "m", Rate: opusRate},
			wantErr: true,
		},
		"no model name": {
			def:     kernel.ModelDef{Ref: "main", Provider: kernel.ProviderClaude, Rate: opusRate},
			wantErr: true,
		},
		"unknown provider": {
			def:     kernel.ModelDef{Ref: "main", Provider: "bedrock", Model: "m", Rate: opusRate},
			wantErr: true,
		},
		"gemini provider is accepted": {
			def: kernel.ModelDef{
				Ref: "cheap", Provider: kernel.ProviderGemini, Model: "gemini-3.7-flash", Rate: flashRate,
			},
		},
		"openai provider is accepted": {
			def: kernel.ModelDef{
				Ref: "gpt", Provider: kernel.ProviderOpenAI, Model: "gpt-4o", Rate: flashRate,
			},
		},
		"priced at nothing": {
			def:     kernel.ModelDef{Ref: "main", Provider: kernel.ProviderClaude, Model: "m"},
			wantErr: true,
		},
		"negative cache price": {
			def: kernel.ModelDef{
				Ref: "main", Provider: kernel.ProviderClaude, Model: "m",
				Rate: pricing.Rate{Input: 1, Output: 1, CacheRead: -1},
			},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			err := tc.def.Validate()
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func validPolicyInput() kernel.ModelPolicyInput {
	return kernel.ModelPolicyInput{
		Defs: []kernel.ModelDef{
			modelDef("main", "claude-opus-5", opusRate),
			modelDef("cheap", "gemini-3.7-flash", flashRate),
		},
		DefaultRef:    "main",
		Clients:       map[string]gollem.LLMClient{"cheap": &mock.LLMClientMock{}},
		DefaultBudget: pricing.FromUSD(2),
	}
}

func TestModelPolicyInputValidate(t *testing.T) {
	testCases := map[string]struct {
		mutate  func(kernel.ModelPolicyInput) kernel.ModelPolicyInput
		wantErr bool
	}{
		"valid": {mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput { return in }},
		"no definitions": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.Defs = nil
				return in
			},
			wantErr: true,
		},
		"duplicate reference name": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.Defs = append(in.Defs, modelDef("main", "claude-sonnet-5", opusRate))
				return in
			},
			wantErr: true,
		},
		"unpriced definition": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.Defs = append(in.Defs, kernel.ModelDef{
					Ref: "free", Provider: kernel.ProviderClaude, Model: "m",
				})
				return in
			},
			wantErr: true,
		},
		"no default reference": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.DefaultRef = ""
				return in
			},
			wantErr: true,
		},
		"default reference is not defined": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.DefaultRef = "missing"
				return in
			},
			wantErr: true,
		},
		"client for an undefined model": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.Clients["ghost"] = &mock.LLMClientMock{}
				return in
			},
			wantErr: true,
		},
		"nil client": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.Clients["cheap"] = nil
				return in
			},
			wantErr: true,
		},
		"zero budget": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.DefaultBudget = 0
				return in
			},
			wantErr: true,
		},
		"negative budget": {
			mutate: func(in kernel.ModelPolicyInput) kernel.ModelPolicyInput {
				in.DefaultBudget = -1
				return in
			},
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := kernel.NewModelPolicy(tc.mutate(validPolicyInput()))
			if tc.wantErr {
				gt.Value(t, err).NotNil()
				return
			}
			gt.NoError(t, err)
		})
	}
}

func TestModelPolicyIsZero(t *testing.T) {
	gt.Bool(t, kernel.ModelPolicy{}.IsZero()).True()

	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()
	gt.Bool(t, p.IsZero()).False()
	gt.Array(t, p.Refs()).Equal([]string{"cheap", "main"})
}

// TestModelPolicyResolve pins that each run is judged against ITS model's price
// and ITS budget, and that a run naming neither gets the deployment defaults.
func TestModelPolicyResolve(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	testCases := map[string]struct {
		scope      kernel.Scope
		wantBudget pricing.NanoUSD
		wantRate   pricing.Rate
		wantModel  string
	}{
		"no model and no budget takes both defaults": {
			scope:      kernel.Scope{},
			wantBudget: pricing.FromUSD(2),
			wantRate:   opusRate,
			wantModel:  "claude-opus-5",
		},
		"a named model is priced at its own rate": {
			scope:      kernel.Scope{LLMModel: "cheap"},
			wantBudget: pricing.FromUSD(2),
			wantRate:   flashRate,
			wantModel:  "gemini-3.7-flash",
		},
		"the run's own budget wins over the default": {
			scope:      kernel.Scope{LLMModel: "cheap", Budget: pricing.FromUSD(0.5)},
			wantBudget: pricing.FromUSD(0.5),
			wantRate:   flashRate,
			wantModel:  "gemini-3.7-flash",
		},
		"an unknown model falls back to the default one, price included": {
			scope:      kernel.Scope{LLMModel: "was-removed"},
			wantBudget: pricing.FromUSD(2),
			wantRate:   opusRate,
			wantModel:  "claude-opus-5",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			proc := &agentkit.Process{Metadata: tc.scope.Metadata()}
			limit := p.Resolve(proc)
			gt.Value(t, limit.Budget).Equal(tc.wantBudget)
			gt.Value(t, limit.Rate).Equal(tc.wantRate)
			// The scope the run carries and the Process it is read from must
			// resolve to the same model; the record and the ceiling are priced
			// through different entry points.
			gt.String(t, p.ModelName(tc.scope)).Equal(tc.wantModel)
			gt.String(t, p.ModelName(kernel.ScopeFrom(proc.Metadata))).Equal(tc.wantModel)
		})
	}
}

// TestModelPolicyResolveHandlesANilProcess pins the defensive path: the Limiter
// contract hands the Process in, and a caller that has none must still get a
// bounded answer rather than a zero one that would stop the run.
func TestModelPolicyResolveHandlesANilProcess(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	limit := p.Resolve(nil)
	gt.Value(t, limit.Budget).Equal(pricing.FromUSD(2))
	gt.Value(t, limit.Rate).Equal(opusRate)
}

// TestModelPolicyCostPricesTheRunsOwnModel is what the run record stores: the
// same token counts cost different amounts depending on which model ran.
func TestModelPolicyCostPricesTheRunsOwnModel(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	metrics := agentkit.Metrics{InputTokens: 100_000, OutputTokens: 10_000}

	// 100,000 * 5000 + 10,000 * 25000 = $0.75
	gt.Value(t, p.Cost(kernel.Scope{}, metrics)).Equal(pricing.FromUSD(0.75))

	// 100,000 * 750 + 10,000 * 3750 = $0.1125
	gt.Value(t, p.Cost(kernel.Scope{LLMModel: "cheap"}, metrics)).Equal(pricing.FromUSD(0.1125))

	// A model the configuration no longer defines is priced at the default rate,
	// which is also the model such a run actually generates through.
	gt.Value(t, p.Cost(kernel.Scope{LLMModel: "was-removed"}, metrics)).
		Equal(pricing.FromUSD(0.75))
}

// TestModelPolicyCostChargesCacheAtItsOwnRate pins that the four token kinds are
// priced separately, which is the reason Rate has four components.
func TestModelPolicyCostChargesCacheAtItsOwnRate(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	metrics := agentkit.Metrics{
		InputTokens:              100_000,
		CacheReadInputTokens:     80_000,
		CacheCreationInputTokens: 10_000,
		OutputTokens:             1_000,
	}

	// 10,000 uncached * 5000 + 80,000 read * 500 + 10,000 write * 6250 + 1,000 out * 25000
	want := pricing.NanoUSD(10_000*5000 + 80_000*500 + 10_000*6250 + 1_000*25000)
	gt.Value(t, p.Cost(kernel.Scope{}, metrics)).Equal(want)
}

// TestModelPolicyCostOfAnEmptyPolicy pins that the zero value — what a
// deployment with no LLM configured holds — prices nothing rather than panicking.
func TestModelPolicyCostOfAnEmptyPolicy(t *testing.T) {
	var p kernel.ModelPolicy
	metrics := agentkit.Metrics{InputTokens: 1000, OutputTokens: 1000}

	gt.Value(t, p.Cost(kernel.Scope{}, metrics)).Equal(pricing.NanoUSD(0))
	gt.String(t, p.ModelName(kernel.Scope{})).Equal("")
}

// TestModelRoleMiddlewarePointsARunAtItsModel pins the one mechanism that makes a
// per-Job model possible: the role a strategy asked for is replaced by the role
// the run's own metadata selects.
func TestModelRoleMiddlewarePointsARunAtItsModel(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	strategyRole := agentkit.DefineModelRole("test.planner")

	var seen agentkit.ModelRole
	handler := kernel.ModelRoleHandlerForTest(p,
		func(_ context.Context, req *agentkit.GenerateRequest) (*agentkit.GenerateResult, error) {
			seen = req.Role
			return &agentkit.GenerateResult{}, nil
		})

	call := func(sc kernel.Scope) agentkit.ModelRole {
		seen = nil
		req := &agentkit.GenerateRequest{
			Effect: agentkit.EffectContext{Metadata: sc.Metadata()},
			Role:   strategyRole,
		}
		_, err := handler(context.Background(), req)
		gt.NoError(t, err).Required()
		return seen
	}

	t.Run("a run naming a model is pointed at that model's role", func(t *testing.T) {
		got := call(kernel.Scope{LLMModel: "cheap"})
		gt.Value(t, got).NotNil()
		gt.Value(t, got == strategyRole).Equal(false)
	})

	// The default model has no role of its own: an unbound role already resolves
	// to the Kernel's positional client, so the strategy's own role must survive.
	t.Run("a run on the default model keeps the strategy's role", func(t *testing.T) {
		gt.Value(t, call(kernel.Scope{}) == strategyRole).Equal(true)
		gt.Value(t, call(kernel.Scope{LLMModel: "main"}) == strategyRole).Equal(true)
	})

	t.Run("an unknown model keeps the strategy's role", func(t *testing.T) {
		gt.Value(t, call(kernel.Scope{LLMModel: "was-removed"}) == strategyRole).Equal(true)
	})
}
