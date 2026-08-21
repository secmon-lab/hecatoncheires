package kernel

import (
	"github.com/gollem-dev/agentkit"
	"github.com/gollem-dev/gollem"

	"github.com/secmon-lab/hecatoncheires/pkg/utils/pricing"
)

// ModelRoleHandlerForTest applies the model-role middleware to next and returns
// the resulting handler, so a test can drive one Generate and read back the role
// it was pointed at. Reaching it through a Kernel would only show which client
// answered, not which role was selected.
func ModelRoleHandlerForTest(p ModelPolicy, next agentkit.GenerateHandler) agentkit.GenerateHandler {
	return modelRoleMiddleware(p)(next)
}

// ClientForTest is the client a ModelPolicyForTest binds to a non-default model.
// A test compares against it to tell which model a run resolved to.
type ClientForTest struct {
	gollem.LLMClient
	Ref string
}

// ModelPolicyForTest builds a policy over the given reference names, the first of
// which is the default. Every model is priced at $1 / $5 per MTok and the budget
// is $10, which is far above anything a test spends.
func ModelPolicyForTest(refs ...string) ModelPolicy {
	if len(refs) == 0 {
		refs = []string{"test-model"}
	}
	defs := make([]ModelDef, 0, len(refs))
	clients := make(map[string]gollem.LLMClient, len(refs))
	for i, ref := range refs {
		defs = append(defs, ModelDef{
			Ref:      ref,
			Provider: ProviderClaude,
			Model:    ref + "-resolved",
			Rate:     pricing.Rate{Input: 1000, Output: 5000},
		})
		if i > 0 {
			clients[ref] = &ClientForTest{Ref: ref}
		}
	}
	p, err := NewModelPolicy(ModelPolicyInput{
		Defs:          defs,
		DefaultRef:    refs[0],
		Clients:       clients,
		DefaultBudget: pricing.FromUSD(10),
	})
	if err != nil {
		panic(err)
	}
	return p
}

// DescribeArgsForTest exposes the rejected-argument shape renderer. The message
// it builds is read by a model, so its exact wording is part of the contract and
// is pinned directly rather than only through a whole Kernel run.
var DescribeArgsForTest = describeArgs

// ArgShapeMaxLenForTest exposes the length bound, so the truncation test derives
// the cut from the same number the renderer uses instead of restating it.
const ArgShapeMaxLenForTest = argShapeMaxLen

// ToolArgsFeedbackHandlerForTest applies the argument-feedback middleware to next
// and returns the resulting handler, so a test can drive it with an arbitrary
// error. Reaching it through a Kernel can only produce the errors a real tool
// call produces, which is not enough to pin what the middleware must leave
// alone.
func ToolArgsFeedbackHandlerForTest(next agentkit.ToolCallHandler) agentkit.ToolCallHandler {
	return toolArgsFeedbackMiddleware()(next)
}

// DescribeErrorValuesForTest exposes the failed-call value renderer. The message
// it builds is read by a model and sent to the LLM provider, so both its wording
// and what it refuses to render are part of the contract and are pinned directly.
var DescribeErrorValuesForTest = describeErrorValues

// ErrorValueMaxLenForTest and ErrorValuesMaxLenForTest expose the length bounds,
// so the truncation tests derive their cuts from the same numbers the renderer
// uses instead of restating them.
const (
	ErrorValueMaxLenForTest  = errorValueMaxLen
	ErrorValuesMaxLenForTest = errorValuesMaxLen
)

// ToolErrorValuesHandlerForTest applies the failed-call value middleware to next
// and returns the resulting handler, so a test can drive it with an arbitrary
// error. Reaching it through a Kernel can only produce the errors a real tool
// call produces, which is not enough to pin what the middleware must leave
// alone.
func ToolErrorValuesHandlerForTest(next agentkit.ToolCallHandler) agentkit.ToolCallHandler {
	return toolErrorValuesMiddleware()(next)
}
