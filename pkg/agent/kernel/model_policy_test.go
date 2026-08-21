package kernel_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gollem-dev/agentkit"
	agentprocmemory "github.com/gollem-dev/agentkit/repository/memory"
	"github.com/gollem-dev/gollem"
	"github.com/gollem-dev/gollem/mock"
	"github.com/m-mizutani/gt"

	"github.com/secmon-lab/hecatoncheires/pkg/agent/budget"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/kernel"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/react"
	"github.com/secmon-lab/hecatoncheires/pkg/repository/agentarchive"
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
			gt.String(t, p.ModelOf(proc)).Equal(tc.wantModel)
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
	gt.Value(t, p.CostOf(nil)).Equal(pricing.NanoUSD(0))
}

// TestModelPolicyCostOfPricesTheRunsOwnModel is what the run record stores: the
// same token counts cost different amounts depending on which model ran.
func TestModelPolicyCostOfPricesTheRunsOwnModel(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	metrics := agentkit.Metrics{InputTokens: 100_000, OutputTokens: 10_000}

	onDefault := &agentkit.Process{Metadata: kernel.Scope{}.Metadata(), Metrics: metrics}
	// 100,000 * 5000 + 10,000 * 25000 = $0.75
	gt.Value(t, p.CostOf(onDefault)).Equal(pricing.FromUSD(0.75))

	onCheap := &agentkit.Process{
		Metadata: kernel.Scope{LLMModel: "cheap"}.Metadata(),
		Metrics:  metrics,
	}
	// 100,000 * 750 + 10,000 * 3750 = $0.1125
	gt.Value(t, p.CostOf(onCheap)).Equal(pricing.FromUSD(0.1125))
}

// TestModelPolicyCostOfChargesCacheAtItsOwnRate pins that the four token kinds
// are priced separately, which is the reason Rate has four components.
func TestModelPolicyCostOfChargesCacheAtItsOwnRate(t *testing.T) {
	p, err := kernel.NewModelPolicy(validPolicyInput())
	gt.NoError(t, err).Required()

	proc := &agentkit.Process{
		Metadata: kernel.Scope{}.Metadata(),
		Metrics: agentkit.Metrics{
			InputTokens:              100_000,
			CacheReadInputTokens:     80_000,
			CacheCreationInputTokens: 10_000,
			OutputTokens:             1_000,
		},
	}

	// 10,000 uncached * 5000 + 80,000 read * 500 + 10,000 write * 6250 + 1,000 out * 25000
	want := pricing.NanoUSD(10_000*5000 + 80_000*500 + 10_000*6250 + 1_000*25000)
	gt.Value(t, p.CostOf(proc)).Equal(want)
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

// budgetPingTool is the tool the never-terminating model below keeps calling.
type budgetPingTool struct{}

func (budgetPingTool) Spec() gollem.ToolSpec {
	return gollem.ToolSpec{Name: "budget__ping", Description: "ping"}
}

func (budgetPingTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return map[string]any{"pong": true}, nil
}

// loopingLLM answers every Generate with the same tool call and the same token
// counts, so it never produces an answer: a run it drives ends only when
// something stops it. The counter is therefore how far the run got before the
// ceiling closed, which is what the budget is supposed to decide.
func loopingLLM(inputTokens, outputTokens int) (gollem.LLMClient, *atomic.Int32) {
	var calls atomic.Int32
	client := &mock.LLMClientMock{
		NewSessionFunc: func(context.Context, ...gollem.SessionOption) (gollem.Session, error) {
			return &mock.SessionMock{
				GenerateFunc: func(context.Context, []gollem.Input, ...gollem.GenerateOption) (*gollem.Response, error) {
					calls.Add(1)
					return &gollem.Response{
						FunctionCalls: []*gollem.FunctionCall{
							{ID: "c", Name: "budget__ping", Arguments: map[string]any{}},
						},
						InputToken:  inputTokens,
						OutputToken: outputTokens,
					}, nil
				},
				HistoryFunc: func() (*gollem.History, error) {
					return &gollem.History{LLType: gollem.LLMTypeOpenAI, Version: gollem.HistoryVersion}, nil
				},
			}, nil
		},
	}
	return client, &calls
}

// runUntilStopped spawns ONE root run carrying sc, judged by the money limiter
// this policy resolves, and drives it to a terminal state.
//
// The strategy is the production ReAct one rather than a stub, because what is
// being pinned is that a Stop from this limiter actually ends a run rather than
// being reported and ignored. The Kernel is built with agentkit.New rather than
// kernel.Build: the limiter reads the Process agentkit hands it, so no middleware
// is involved, and which client a run generates through is pinned separately by
// TestModelRoleMiddlewarePointsARunAtItsModel.
func runUntilStopped(t *testing.T, llm gollem.LLMClient, root budget.Root,
	policy kernel.ModelPolicy, sc kernel.Scope) *agentkit.Process {
	t.Helper()
	return runWithLimiter(t, llm, root.Limiter(policy.Resolve), sc)
}

// runWithLimiter is runUntilStopped for a limiter that no policy can produce —
// the fail-closed shapes below, where the point is that nothing was spent.
func runWithLimiter(t *testing.T, llm gollem.LLMClient, limiter agentkit.Limiter,
	sc kernel.Scope) *agentkit.Process {
	t.Helper()

	reg := agentkit.NewRegistry()
	handle, err := react.Register(reg, "budget-test", 1, limiter,
		agentkit.WithHistoryStore[react.Output](agentarchive.NewMemoryHistoryStore()))
	gt.NoError(t, err).Required()

	k, err := agentkit.New(agentprocmemory.New(), llm, reg,
		agentkit.WithToolFactory(func(context.Context, *agentkit.Process) ([]gollem.Tool, error) {
			return []gollem.Tool{budgetPingTool{}}, nil
		}))
	gt.NoError(t, err).Required()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- k.Serve(ctx, agentkit.WithPollInterval(5*time.Millisecond)) }()

	pid, err := handle.Spawn(ctx, k,
		react.Input{SystemPrompt: "be helpful", Prompt: "loop forever"},
		agentkit.WithMetadata(sc.Metadata()))
	gt.NoError(t, err).Required()

	for {
		proc, err := k.GetProcess(ctx, pid)
		gt.NoError(t, err).Required()
		if proc.Status.Terminal() {
			cancel()
			<-served
			return proc
		}
		select {
		case <-ctx.Done():
			gt.NoError(t, ctx.Err()).Required()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestAConfiguredBudgetStopsTheRun is the end of the money path: the amount an
// operator configured is what a real run is actually stopped at.
//
// Every piece leading up to it is pinned above — a Scope carries the run's model
// and budget, Resolve turns them into a RunLimit, and budget.Root.Limiter turns a
// RunLimit plus metrics into a decision — and none of that says a run stops. The
// token-counted sub-agent ceiling has had such a test since it existed
// (react.TestTokenBudgetsStopTheRun); the money ceiling every root run is bounded
// by did not.
func TestAConfiguredBudgetStopsTheRun(t *testing.T) {
	// The prices are round so the arithmetic is readable off the test: "main"
	// costs $1 per MTok of input and $5 per MTok of output, "cheap" a quarter of
	// each. Every Generate below reports 5,000 input and 3,000 output tokens, so
	// one call costs $0.02 on "main" (5000*1000 + 3000*5000 nanoUSD) and $0.005
	// on "cheap".
	mainRate := pricing.Rate{Input: 1000, Output: 5000}
	cheapRate := pricing.Rate{Input: 250, Output: 1250}

	newPolicy := func(t *testing.T, defaultBudget pricing.NanoUSD) kernel.ModelPolicy {
		t.Helper()
		p, err := kernel.NewModelPolicy(kernel.ModelPolicyInput{
			Defs: []kernel.ModelDef{
				modelDef("main", "main-model", mainRate),
				modelDef("cheap", "cheap-model", cheapRate),
			},
			DefaultRef:    "main",
			Clients:       map[string]gollem.LLMClient{"cheap": &mock.LLMClientMock{}},
			DefaultBudget: defaultBudget,
		})
		gt.NoError(t, err).Required()
		return p
	}

	// MaxSteps is far above the transitions the longest case below runs, so the
	// step ceiling cannot be what ends these runs. The asserted message says
	// which ceiling did.
	root := budget.Root{MaxSteps: 1000, NoticeRatio: 0.8}

	testCases := map[string]struct {
		defaultBudget pricing.NanoUSD
		scope         kernel.Scope
		wantCalls     int32
		wantMessage   string
	}{
		"a run naming no budget is bounded by the deployment default": {
			defaultBudget: pricing.FromUSD(0.10), // $0.10 at $0.02 per call
			scope:         kernel.Scope{},
			wantCalls:     5,
			wantMessage:   "cost budget exhausted ($0.10/$0.10)",
		},
		"the run's own budget bounds it below the deployment default": {
			defaultBudget: pricing.FromUSD(0.40),
			scope:         kernel.Scope{Budget: pricing.FromUSD(0.10)},
			wantCalls:     5,
			wantMessage:   "cost budget exhausted ($0.10/$0.10)",
		},
		"a larger budget buys proportionally more of the same work": {
			defaultBudget: pricing.FromUSD(0.40), // $0.40 at $0.02 per call
			scope:         kernel.Scope{},
			wantCalls:     20,
			wantMessage:   "cost budget exhausted ($0.40/$0.40)",
		},
		// The reason the ceiling is money rather than tokens: the same budget and
		// the same token counts reach four times as far on a model priced at a
		// quarter of the other.
		"the same budget goes further on a cheaper model": {
			defaultBudget: pricing.FromUSD(0.10), // $0.10 at $0.005 per call
			scope:         kernel.Scope{LLMModel: "cheap"},
			wantCalls:     20,
			wantMessage:   "cost budget exhausted ($0.10/$0.10)",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			llm, calls := loopingLLM(5000, 3000)
			proc := runUntilStopped(t, llm, root, newPolicy(t, tc.defaultBudget), tc.scope)

			gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
			gt.Value(t, proc.Failure).NotNil().Required()
			gt.Value(t, proc.Failure.Code).Equal(agentkit.FailureLimitExceeded)
			gt.String(t, proc.Failure.Message).Contains(tc.wantMessage)
			gt.Value(t, calls.Load()).Equal(tc.wantCalls)
		})
	}
}

// TestAnUnpricedRunIsStoppedBeforeItSpends pins the fail-closed half through a
// real run: a Process whose ceiling cannot be resolved is stopped BEFORE its
// first LLM call, not run unbounded. The decision function on its own cannot show
// that no money was spent.
//
// The two shapes below are the ones a wiring mistake actually produces. A per-run
// budget cannot produce them: Scope.Metadata omits a non-positive budget and
// Scope.Validate rejects a negative one, so a run's own budget either names an
// amount or leaves the deployment default in force.
func TestAnUnpricedRunIsStoppedBeforeItSpends(t *testing.T) {
	testCases := map[string]budget.LimitResolver{
		// A deployment with no model definitions at all: nothing to price a run
		// against, so nothing may be spent on it.
		"a policy with no models": kernel.ModelPolicy{}.Resolve,
		// The Limiter built without a resolver, which is what forgetting to pass
		// the policy at a Register call site looks like.
		"no resolver at all": nil,
	}

	for name, resolve := range testCases {
		t.Run(name, func(t *testing.T) {
			llm, calls := loopingLLM(5000, 3000)
			root := budget.Root{MaxSteps: 1000, NoticeRatio: 0.8}
			proc := runWithLimiter(t, llm, root.Limiter(resolve), kernel.Scope{})

			gt.Value(t, proc.Status).Equal(agentkit.ProcessFailed)
			gt.Value(t, proc.Failure).NotNil().Required()
			gt.Value(t, proc.Failure.Code).Equal(agentkit.FailureLimitExceeded)
			gt.String(t, proc.Failure.Message).Contains("this run has no priced budget")
			gt.Value(t, calls.Load()).Equal(int32(0))
		})
	}
}
