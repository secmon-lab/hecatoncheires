package kernel

import "github.com/gollem-dev/agentkit"

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
